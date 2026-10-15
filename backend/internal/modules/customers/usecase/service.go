// Package usecase holds the customers business rules (TEC-705). Every
// method takes the organization id of the caller explicitly (from
// orgctx.Scope in handlers, from the job in workers) and passes it to the
// repository.
//
// Branch scope: customers are organization-wide. A branch-restricted member
// sees every customer of the organization (the counter must find a
// returning customer whatever branch they rented from before); what a
// member sees is limited by permissions instead: read shows masked values,
// tenant.customers.reveal is needed for plaintext.
//
// Sensitive values: identity / licence numbers are encrypted with the
// organization in the associated data and indexed with a keyed blind index
// (see sensitive.go); phone and e-mail are stored plain and masked on every
// read. Plaintext leaves only through Reveal (and an export with
// include_sensitive), both recorded as activity events by the caller.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Cmd+K spec id of customers.
const SearchSpec = "customers"

// Caps of exports, bulk "all matching" scopes and sub-lists.
const (
	MaxListTargets  = 10000
	MaxCustomerRows = 500
)

var (
	ErrNotFound           = errors.New("customer not found")
	ErrContactNotFound    = errors.New("contact not found")
	ErrDriverNotFound     = errors.New("driver not found")
	ErrDocumentNotFound   = errors.New("customer document not found")
	ErrIdentityTaken      = errors.New("a customer with this identity number already exists")
	ErrLicenceTaken       = errors.New("this customer already has a driver with this licence number")
	ErrAnonymized         = errors.New("the customer is anonymized")
	ErrNoFile             = errors.New("the document has no file")
	ErrStorageUnavailable = errors.New("object storage is not configured")
	ErrCipherUnavailable  = errors.New("field encryption is not configured")
)

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

// Service implements the customers use cases.
type Service struct {
	repo    *repository.Postgres
	cipher  *crypto.FieldCipher
	storage storage.Driver
	indexer SearchIndexer
	log     *slog.Logger
	now     func() time.Time
}

// New builds the service. store and indexer may be nil; cipher is required
// for any write or reveal of sensitive values.
func New(repo *repository.Postgres, cipher *crypto.FieldCipher, store storage.Driver, indexer SearchIndexer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, cipher: cipher, storage: store, indexer: indexer, log: log, now: time.Now}
}

// SetSearchIndexer sets the Cmd+K indexer after construction (worker wiring).
func (s *Service) SetSearchIndexer(indexer SearchIndexer) {
	s.indexer = indexer
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"display_name": "display_name", "customer_type": "customer_type", "nationality": "nationality",
	"created_at": "created_at", "updated_at": "updated_at",
}

// List returns one page of the organization's customers (masked).
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Customer, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, total, err := s.repo.ListCustomers(ctx, orgID, s.listParams(orgID, f))
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Customer, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapListRow(row))
	}
	return out, total, nil
}

// listParams resolves the free-text query: names / e-mail by ILIKE, phone
// digits, and an exact identity number through its blind index (the
// plaintext query is never compared with stored values).
func (s *Service) listParams(orgID int64, f model.ListFilter) repository.ListParams {
	p := repository.ListParams{ListFilter: f}
	if f.Q == "" {
		return p
	}
	p.QDigits = digitsQuery(f.Q)
	if s.cipher != nil {
		p.IdentityBidxs = s.identityCandidates(orgID, f.Q)
	}
	return p
}

// Facets returns the dynamic filter options.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.repo.Facets(ctx, orgID)
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{CustomerType: []model.Facet{}, Nationality: []model.Facet{}, Tag: []model.Facet{}, Consent: []model.Facet{}}
	for _, row := range rows {
		f := model.Facet{Value: row.Value, Count: row.Count}
		switch row.Facet {
		case "customer_type":
			out.CustomerType = append(out.CustomerType, f)
		case "nationality":
			out.Nationality = append(out.Nationality, f)
		case "tag":
			out.Tag = append(out.Tag, f)
		case "consent":
			out.Consent = append(out.Consent, f)
		}
	}
	return out, nil
}

// Get returns one customer of the organization (masked).
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Customer, error) {
	c, err := s.getCustomer(ctx, orgID, id)
	if err != nil {
		return model.Customer{}, err
	}
	return s.hydrate(ctx, orgID, c)
}

// Create validates and stores a new customer.
func (s *Service) Create(ctx context.Context, orgID int64, in model.CustomerInput) (model.Customer, error) {
	f := repository.CustomerFields{CustomerType: model.TypeIndividual, Tags: []string{}}
	if err := s.applyCustomer(orgID, &f, in, true); err != nil {
		return model.Customer{}, err
	}
	if taken, err := s.repo.IdentityTaken(ctx, orgID, f.IdentityBidx, nil); err != nil {
		return model.Customer{}, err
	} else if taken {
		return model.Customer{}, ErrIdentityTaken
	}
	row, err := s.repo.CreateCustomer(ctx, orgID, f, actorArg(ctx))
	if err != nil {
		return model.Customer{}, mapErr(err)
	}
	s.index(ctx, orgID, row.Uuid)
	return s.hydrate(ctx, orgID, row)
}

// Patch updates the given fields of a customer.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.CustomerInput) (model.Customer, error) {
	current, err := s.liveCustomer(ctx, orgID, id)
	if err != nil {
		return model.Customer{}, err
	}
	f := repository.FieldsFromCustomer(current)
	if err := s.applyCustomer(orgID, &f, in, false); err != nil {
		return model.Customer{}, err
	}
	if f.IdentityBidx != current.IdentityBidx {
		if taken, err := s.repo.IdentityTaken(ctx, orgID, f.IdentityBidx, &id); err != nil {
			return model.Customer{}, err
		} else if taken {
			return model.Customer{}, ErrIdentityTaken
		}
	}
	row, err := s.repo.UpdateCustomer(ctx, orgID, id, f)
	if err != nil {
		return model.Customer{}, mapErr(err)
	}
	s.index(ctx, orgID, id)
	return s.hydrate(ctx, orgID, row)
}

// SetTags replaces the tags of a customer and returns the previous ones
// (bulk engine).
func (s *Service) SetTags(ctx context.Context, orgID int64, id uuid.UUID, tags []string) ([]string, error) {
	current, err := s.liveCustomer(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	clean, err := normalizeTags(tags)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.SetTags(ctx, orgID, id, clean); err != nil {
		return nil, mapErr(err)
	}
	prev := current.Tags
	if prev == nil {
		prev = []string{}
	}
	return prev, nil
}

// Delete soft-deletes a customer.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	if _, err := s.getCustomer(ctx, orgID, id); err != nil {
		return err
	}
	if err := s.repo.DeleteCustomer(ctx, orgID, id); err != nil {
		return mapErr(err)
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return nil
}

// TargetUUIDs resolves the customers matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	return s.repo.ListCustomerUUIDs(ctx, orgID, s.listParams(orgID, f), limit)
}

// Activity lists the audit events of a customer.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getCustomer(ctx, orgID, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		entry := model.ActivityEntry{
			UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if len(entry.Payload) == 0 {
			entry.Payload = json.RawMessage("{}")
		}
		if row.ActorUuid.Valid {
			entry.Actor = &model.Actor{
				UUID: row.ActorUuid.Bytes, Name: row.ActorName.String, Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, entry)
	}
	return out, total, nil
}

// Lookup loads a live customer of the organization (reservations resolve a
// customer_uuid with it; another organization's uuid is ErrNotFound).
func (s *Service) Lookup(ctx context.Context, orgID int64, id uuid.UUID) (db.Customer, error) {
	return s.getCustomer(ctx, orgID, id)
}

// getCustomer loads a customer of the organization (anonymized included).
func (s *Service) getCustomer(ctx context.Context, orgID int64, id uuid.UUID) (db.Customer, error) {
	row, err := s.repo.GetCustomer(ctx, orgID, id)
	if err != nil {
		return db.Customer{}, mapErr(err)
	}
	return row, nil
}

// liveCustomer loads a customer that may still be changed (not anonymized).
func (s *Service) liveCustomer(ctx context.Context, orgID int64, id uuid.UUID) (db.Customer, error) {
	row, err := s.getCustomer(ctx, orgID, id)
	if err != nil {
		return db.Customer{}, err
	}
	if row.AnonymizedAt.Valid {
		return db.Customer{}, ErrAnonymized
	}
	return row, nil
}

// hydrate maps a customer row with its open consents and driver count.
func (s *Service) hydrate(ctx context.Context, orgID int64, c db.Customer) (model.Customer, error) {
	out := mapCustomer(c)
	consents, err := s.repo.ListConsents(ctx, orgID, c.ID)
	if err != nil {
		return model.Customer{}, err
	}
	for _, row := range consents {
		if !row.WithdrawnAt.Valid {
			out.ConsentChannels = append(out.ConsentChannels, row.Channel)
		}
	}
	sortStrings(out.ConsentChannels)
	drivers, err := s.repo.ListDrivers(ctx, orgID, c.ID)
	if err != nil {
		return model.Customer{}, err
	}
	out.DriversCount = int64(len(drivers))
	return out, nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// actorArg is the signed-in user of a request context (null in workers).
func actorArg(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, repository.ErrIdentityTaken):
		return ErrIdentityTaken
	case errors.Is(err, repository.ErrLicenceTaken):
		return ErrLicenceTaken
	}
	return err
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return mapErr(err)
}
