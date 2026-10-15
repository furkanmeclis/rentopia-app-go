// Package usecase holds the fleet operations business rules: vehicle
// blocks (TEC-694), transfers and one-way rules (TEC-695). Every method
// takes the organization id of the caller explicitly (orgctx.Scope in
// handlers, the job in workers).
//
// Branch scope:
//   - blocks belong to a vehicle and follow its scope (home OR current
//     branch in the caller's scope, otherwise 404);
//   - transfers and one-way rules are visible when their from OR to branch
//     is in the scope; creating a transfer needs one of them in the scope,
//     creating a rule needs the from branch.
//
// Vehicle status and branch changes go through the vehicles use case
// (vehicle_events), inside the same transaction as the block / transfer
// row.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	pkgmoney "github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxListTargets caps exports and bulk "all matching" scopes.
const MaxListTargets = 10000

var (
	ErrVehicleNotFound  = errors.New("vehicle not found")
	ErrBlockNotFound    = errors.New("block not found")
	ErrTransferNotFound = errors.New("transfer not found")
	ErrRuleNotFound     = errors.New("one-way rule not found")
	ErrRouteTaken       = errors.New("a live rule already covers this route and product")
)

// Conflict codes (409 details).
const (
	ConflictBlockOverlap    = "block_overlap"
	ConflictTransferOverlap = "transfer_overlap"
	ConflictState           = "invalid_state"
	ConflictNotAtOrigin     = "vehicle_not_at_origin"
	ConflictVehicleBusy     = "vehicle_busy"
)

// Conflict is one record a change collides with.
type Conflict struct {
	Kind     string
	UUID     uuid.UUID
	StartsAt time.Time
	EndsAt   *time.Time
}

// ConflictError is answered with 409 CONFLICT; Items are the colliding
// records (details: field = kind, code = Code, message = uuid + range).
type ConflictError struct {
	Code    string
	Message string
	Items   []Conflict
}

func (e *ConflictError) Error() string { return e.Message }

func conflict(code, msg string, items ...Conflict) error {
	return &ConflictError{Code: code, Message: msg, Items: items}
}

// IsConflict unwraps a ConflictError.
func IsConflict(err error) (*ConflictError, bool) {
	var ce *ConflictError
	if errors.As(err, &ce) {
		return ce, true
	}
	return nil, false
}

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
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

func (e *ValidationError) has(field string) bool {
	for _, d := range e.Details {
		if d.Field == field {
			return true
		}
	}
	return false
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

// Service implements the fleet operations use cases.
type Service struct {
	repo     *repository.Postgres
	vehicles *vehicleusecase.Service
	now      func() time.Time
}

// New builds the service. vehicles applies vehicle status / branch changes.
func New(repo *repository.Postgres, vehicles *vehicleusecase.Service) *Service {
	return &Service{repo: repo, vehicles: vehicles, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

func (s *Service) clock() time.Time {
	return s.now().UTC().Truncate(time.Microsecond)
}

// vehicleTx is the vehicles use case bound to a fleetops transaction.
func (s *Service) vehicleTx(tx *repository.Postgres) *vehicleusecase.Service {
	return s.vehicles.WithQueries(tx.Queries())
}

// getVehicle loads a live vehicle of the organization in the caller's
// branch scope (home OR current branch), like the vehicles resource.
func (s *Service) getVehicle(ctx context.Context, orgID int64, id uuid.UUID) (repository.VehicleRow, error) {
	v, err := s.repo.GetVehicle(ctx, orgID, id)
	if err != nil {
		return repository.VehicleRow{}, mapNotFound(err, ErrVehicleNotFound)
	}
	if orgctx.CheckBranch(ctx, v.HomeBranchID) != nil && orgctx.CheckBranch(ctx, v.CurrentBranchID) != nil {
		return repository.VehicleRow{}, ErrVehicleNotFound
	}
	return v, nil
}

// eitherInScope reports whether the caller's scope covers a or b.
func eitherInScope(ctx context.Context, a, b int64) bool {
	return orgctx.CheckBranch(ctx, a) == nil || orgctx.CheckBranch(ctx, b) == nil
}

// unscoped lifts the branch restriction for a vehicle change the caller is
// already authorised for (a transfer the caller may see moves a vehicle
// whose branches may both be outside the caller's scope, e.g. the
// receiving branch completes the arrival).
func unscoped(ctx context.Context) context.Context {
	sc, ok := orgctx.From(ctx)
	if !ok || sc.BranchIDs == nil {
		return ctx
	}
	sc.BranchIDs = nil
	return orgctx.WithScope(ctx, sc)
}

// actorArg is the signed-in user of a request context (null in workers).
func actorArg(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func actorFrom(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func checkNote(ve *ValidationError, field, v string) string {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > 2000 {
		ve.add(field, "too_long", field+" must be at most 2000 characters")
	}
	return v
}

var moneyRE = regexp.MustCompile(`^\d+(\.\d{1,2})?$`)

// parseMoney parses a non-negative TRY amount with at most 2 decimals into
// a NUMERIC(14,2).
func parseMoney(ve *ValidationError, field string, in *json.Number) pgtype.Numeric {
	if in == nil {
		return pgtype.Numeric{}
	}
	v := strings.ReplaceAll(strings.TrimSpace(in.String()), ",", ".")
	intPart := v
	if i := strings.IndexByte(v, '.'); i >= 0 {
		intPart = v[:i]
	}
	var n pgtype.Numeric
	if !moneyRE.MatchString(v) || len(intPart) > 12 || n.Scan(v) != nil {
		ve.add(field, "invalid", field+" must be a non-negative amount with at most 2 decimals")
		return pgtype.Numeric{}
	}
	return n
}

// FormatMoney renders a numeric as a decimal string with two fraction digits
// (pkg/money: one rounding rule for every amount).
func FormatMoney(n pgtype.Numeric) string {
	return pkgmoney.FormatNumeric(n)
}

func moneyPtr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	v := FormatMoney(n)
	return &v
}

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
		entry.Actor = actorFrom(row.ActorUuid, row.ActorName, row.ActorSurname, row.ActorEmail)
		out = append(out, entry)
	}
	return out
}

func endsBefore(end *time.Time, t time.Time) bool {
	return end != nil && !end.After(t)
}
