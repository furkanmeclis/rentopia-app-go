// Package usecase holds the fleet classification rules (TEC-691): vehicle
// segments, rental groups and versioned rental products. Every method takes
// the organization id of the caller explicitly (from orgctx.Scope in
// handlers, from the job in workers) and passes it to the repository.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpecProducts is the Cmd+K spec id of rental products.
const SearchSpecProducts = "rental_products"

var (
	ErrSegmentNotFound = errors.New("segment not found")
	ErrGroupNotFound   = errors.New("rental group not found")
	ErrProductNotFound = errors.New("rental product not found")
	ErrVersionNotFound = errors.New("product version not found")
	ErrImageNotFound   = errors.New("product image not found")
	ErrCodeTaken       = errors.New("code is already used in this organization")
	// ErrSegmentInUse: live groups or products still reference the segment.
	ErrSegmentInUse = errors.New("segment is used by rental groups or products")
	// ErrGroupInUse: the current version of a live product guarantees the group.
	ErrGroupInUse = errors.New("rental group is guaranteed by a product")
	// ErrStorageUnavailable: no object store is configured.
	ErrStorageUnavailable = errors.New("object storage is not configured")
	// ErrVersionConflict: a concurrent edit created the same version.
	ErrVersionConflict = errors.New("product was changed concurrently, retry")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the fleet use cases.
type Service struct {
	repo    *repository.Postgres
	storage storage.Driver
	indexer SearchIndexer
}

// New builds the service. store and indexer may be nil.
func New(repo *repository.Postgres, store storage.Driver, indexer SearchIndexer) *Service {
	return &Service{repo: repo, storage: store, indexer: indexer}
}

// MaxListTargets caps exports and bulk "all matching" scopes.
const MaxListTargets = 10000

// Sortable list fields per resource.
var (
	SegmentSortColumns = map[string]string{
		"code": "code", "name": "name", "sort_order": "sort_order", "is_active": "is_active",
		"created_at": "created_at", "updated_at": "updated_at",
	}
	GroupSortColumns = map[string]string{
		"code": "code", "name": "name", "acriss_code": "acriss_code", "min_seats": "min_seats",
		"is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
	}
	ProductSortColumns = map[string]string{
		"code": "code", "title": "title", "status": "status", "current_version": "current_version",
		"is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
	}
)

// --- validation helpers ---------------------------------------------------------

var (
	codeRE  = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)
	moneyRE = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)
)

func normalizeCode(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

// applyCode validates a code (create: required).
func applyCode(ve *ValidationError, in *string, create bool, dst *string) {
	if in == nil && !create {
		return
	}
	code := ""
	if in != nil {
		code = normalizeCode(*in)
	}
	switch {
	case code == "":
		ve.add("code", "required", "code is required")
	case !codeRE.MatchString(code):
		ve.add("code", "invalid", "code must be 1-32 characters: letters, digits, - or _")
	default:
		*dst = code
	}
}

// applyName validates a required single-line name.
func applyName(ve *ValidationError, field string, in *string, create bool, dst *string) {
	if in == nil && !create {
		return
	}
	v := ""
	if in != nil {
		v = strings.TrimSpace(*in)
	}
	switch {
	case v == "":
		ve.add(field, "required", field+" is required")
	case utf8.RuneCountInString(v) > 255:
		ve.add(field, "too_long", field+" must be at most 255 characters")
	default:
		*dst = v
	}
}

// applyText validates an optional free text.
func applyText(ve *ValidationError, field string, in *string, max int, dst *string) {
	if in == nil {
		return
	}
	v := strings.TrimSpace(*in)
	if utf8.RuneCountInString(v) > max {
		ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
		return
	}
	*dst = v
}

// applyEnum validates an optional (nullable) enum.
func applyEnum(ve *ValidationError, field string, in model.OptionalString, allowed []string, dst *pgtype.Text) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Text{}
		return
	}
	v := strings.TrimSpace(*in.Value)
	if !model.Contains(allowed, v) {
		ve.add(field, "invalid", field+" must be one of "+strings.Join(allowed, ", "))
		return
	}
	*dst = pgtype.Text{String: v, Valid: true}
}

func parseMoney(ve *ValidationError, field string, in *json.Number, maxIntDigits int, dst *pgtype.Numeric) {
	if in == nil {
		return
	}
	v := strings.ReplaceAll(strings.TrimSpace(in.String()), ",", ".")
	var n pgtype.Numeric
	intPart := v
	if i := strings.IndexByte(v, '.'); i >= 0 {
		intPart = v[:i]
	}
	if !moneyRE.MatchString(v) || len(intPart) > maxIntDigits || n.Scan(v) != nil {
		ve.add(field, "invalid", field+" must be a non-negative amount with at most 2 decimals")
		return
	}
	*dst = n
}

// FormatMoney renders a numeric as a decimal string with two fraction digits.
func FormatMoney(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0.00"
	}
	cents := new(big.Int).Set(n.Int)
	shift := int(n.Exp) + 2
	ten := big.NewInt(10)
	if shift >= 0 {
		cents.Mul(cents, new(big.Int).Exp(ten, big.NewInt(int64(shift)), nil))
	} else {
		cents.Quo(cents, new(big.Int).Exp(ten, big.NewInt(int64(-shift)), nil))
	}
	neg := cents.Sign() < 0
	cents.Abs(cents)
	s := cents.String()
	for len(s) < 3 {
		s = "0" + s
	}
	out := s[:len(s)-2] + "." + s[len(s)-2:]
	if neg {
		out = "-" + out
	}
	return out
}

func money(v string) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(v)
	return n
}

// --- mapping helpers --------------------------------------------------------------

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func int2Ptr(v pgtype.Int2) *int16 {
	if !v.Valid {
		return nil
	}
	n := v.Int16
	return &n
}

func segmentRef(row db.VehicleSegment) model.SegmentRef {
	return model.SegmentRef{UUID: row.Uuid, Code: row.Code, Name: row.Name}
}

func modelRef(row db.FleetCatalogModelRefsByIDsRow) model.CatalogModelRef {
	return model.CatalogModelRef{
		UUID: row.Uuid, Name: row.Name,
		Brand: model.BrandRef{UUID: row.BrandUuid, DisplayName: row.BrandDisplayName},
		Label: model.CatalogModelLabel(row.BrandDisplayName, row.Name),
	}
}

func typeRef(row db.FleetCatalogTypeRefsByIDsRow) model.CatalogTypeRef {
	ref := model.CatalogTypeRef{
		UUID: row.Uuid, Name: row.RawName,
		Brand: model.BrandRef{UUID: row.BrandUuid, DisplayName: row.BrandDisplayName},
		Label: model.CatalogTypeLabel(row.BrandDisplayName, row.RawName),
	}
	if row.ModelUuid.Valid {
		ref.Model = &model.NamedRef{UUID: row.ModelUuid.Bytes, Name: row.ModelName.String}
	}
	return ref
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapCodeTaken(err error) error {
	if errors.Is(err, repository.ErrCodeTaken) {
		return ErrCodeTaken
	}
	return err
}

func uniqueIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// activityEntries maps activity rows.
func activityEntries(rows []db.ListOrganizationResourceActivityRow) []model.ActivityEntry {
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		entry := model.ActivityEntry{
			UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload),
			CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if len(entry.Payload) == 0 {
			entry.Payload = json.RawMessage("{}")
		}
		if row.ActorUuid.Valid {
			entry.Actor = &model.Actor{
				UUID: row.ActorUuid.Bytes, Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, entry)
	}
	return out
}

// Activity lists the audit events of a fleet record (resource = engine slug).
func (s *Service) Activity(ctx context.Context, orgID int64, resource string, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	var err error
	switch resource {
	case model.ResourceSegments:
		_, err = s.repo.GetSegment(ctx, orgID, id)
		err = mapNotFound(err, ErrSegmentNotFound)
	case model.ResourceRentalGroups:
		_, err = s.repo.GetGroup(ctx, orgID, id)
		err = mapNotFound(err, ErrGroupNotFound)
	case model.ResourceRentalProducts:
		_, err = s.repo.GetProduct(ctx, orgID, id)
		err = mapNotFound(err, ErrProductNotFound)
	default:
		return nil, 0, fmt.Errorf("fleet: unknown activity resource %q", resource)
	}
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, resource, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return activityEntries(rows), total, nil
}
