// Package usecase holds vehicle inspections (TEC-712 / TEC-713 / TEC-714):
// the mobile handover and return forms, their evidence (photos with a
// server-side SHA-256 and server time, EXIF kept apart, signatures) and
// the return charges. Finalizing a form runs the existing rentals use case
// (Handover / Return) and finalizes the form inside the same transaction;
// a finalized form is immutable (database trigger) and a correction is a
// new version with a reason. docs/design/inspections.md is the design.
//
// Every method takes the organization id explicitly; the branch scope is
// the rental's (rentals.Get answers not found outside the scope).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound         = errors.New("inspection not found")
	ErrRentalNotFound   = errors.New("rental not found")
	ErrTemplateNotFound = errors.New("inspection template not found")
	ErrDamageNotFound   = errors.New("damage not found")
	ErrPhotoNotFound    = errors.New("photo not found")
	// ErrForbidden: the action needs a permission the caller lacks (403).
	ErrForbidden = errors.New("missing permission")
	// ErrConcurrent: the form changed in between (409).
	ErrConcurrent = errors.New("the form was changed by someone else; reload and retry")
	// ErrStorageUnavailable: no object store configured (503).
	ErrStorageUnavailable = errors.New("storage is not configured")
	// ErrMediaLink: an invalid or expired media link (404).
	ErrMediaLink = errors.New("media link is invalid or expired")
)

// StateError is answered with 409 and Code.
type StateError struct {
	Code    string
	Message string
	// Data carries details (the readiness of an incomplete form).
	Data any
}

func (e *StateError) Error() string { return e.Message }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// Detail is one field-level validation problem.
type Detail struct {
	Field, Code, Message string
}

// ValidationError carries field details (400).
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
	if e == nil || len(e.Details) == 0 {
		return nil
	}
	return e
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// Rentals is the rentals use case as inspections drive it.
type Rentals interface {
	Get(ctx context.Context, orgID int64, id uuid.UUID) (rentalmodel.Rental, error)
	Handover(ctx context.Context, orgID int64, id uuid.UUID, in rentalmodel.HandoverInput) (rentalmodel.Rental, error)
	Return(ctx context.Context, orgID int64, id uuid.UUID, in rentalmodel.ReturnInput) (rentalmodel.Rental, error)
	CompleteReturn(ctx context.Context, orgID int64, id uuid.UUID, in rentalmodel.CompleteReturnInput) (rentalmodel.Rental, error)
}

// Contracts records a contract approval given with a form's signature.
type Contracts interface {
	ApproveWithSignature(ctx context.Context, orgID int64, in contractsusecase.SignatureApproval) (contractsmodel.Document, error)
}

// Store is the object storage of the evidence.
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Exists(ctx context.Context, path string) (bool, error)
	Delete(ctx context.Context, path string) error
}

// Scanner is the optional malware scan of uploaded evidence (a ClamAV
// clamd client fits here). Clean false marks the photo infected and the
// completion is refused.
type Scanner interface {
	Scan(ctx context.Context, r io.Reader) (clean bool, err error)
}

// HandoverListener is told about a finalized handover (after commit).
//
// KABİS (TEC-726) does not use it: the rental start notification is
// recorded by the rentals Handover use case inside the handover
// transaction (rentals usecase.KabisRecorder), so a form-based handover
// and a direct one are reported the same way, exactly once.
type HandoverListener interface {
	HandedOver(ctx context.Context, orgID int64, rental, inspection uuid.UUID, at time.Time)
}

type nopListener struct{}

func (nopListener) HandedOver(context.Context, int64, uuid.UUID, uuid.UUID, time.Time) {}

// Service implements inspections.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	rentals   Rentals
	contracts Contracts
	store     Store
	scanner   Scanner
	listener  HandoverListener
	media     *MediaSigner
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service. store may be nil (no evidence uploads).
func New(pool *pgxpool.Pool, q *db.Queries, rentals Rentals, store Store, media *MediaSigner) *Service {
	s := &Service{
		pool: pool, q: q, rentals: rentals, media: media, listener: nopListener{},
		events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
	if store != nil && !isNilStore(store) {
		s.store = store
	}
	return s
}

func isNilStore(st Store) bool {
	if v, ok := st.(storage.Driver); ok {
		return v == nil
	}
	return false
}

// SetContracts wires the contract approval with a signature.
func (s *Service) SetContracts(c Contracts) { s.contracts = c }

// SetScanner wires the optional malware scanner.
func (s *Service) SetScanner(sc Scanner) { s.scanner = sc }

// SetHandoverListener wires the handover listener.
func (s *Service) SetHandoverListener(l HandoverListener) {
	if l != nil {
		s.listener = l
	}
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	if pub != nil {
		s.events = pub
	}
}

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inspections: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	if s.events == nil || len(ids) == 0 {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	s.events.Publish(ctx, resource, action, out...)
}

func actorID(ctx context.Context) *int64 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		id := p.UserInternal
		return &id
	}
	return nil
}

func has(ctx context.Context, slug string) bool {
	sc, ok := orgctx.From(ctx)
	return ok && sc.Has(slug)
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func int4Ptr(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func int2Ptr(v *int16) pgtype.Int2 {
	if v == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: *v, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func textPtr(v *string) pgtype.Text {
	if v == nil || *v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func uniqueViolation(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && (constraint == "" || pe.ConstraintName == constraint)
}

// immutable reports the database refusing a change of a finalized form.
func immutable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23514" &&
		(pe.ConstraintName == "inspections_immutable" || pe.ConstraintName == "inspection_children_immutable")
}

func requireReason(field, reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if len([]rune(r)) < 3 || len(r) > 500 {
		return "", validation(field, "required", "a reason is required (3-500 characters)")
	}
	return r, nil
}

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

// rentalEvent writes a rental timeline entry.
func rentalEvent(ctx context.Context, q *db.Queries, orgID, rentalID int64, action, reason string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	return q.InsertRentalEvent(ctx, db.InsertRentalEventParams{
		OrganizationID: orgID, RentalID: rentalID, Action: action, Reason: reason, Data: mustJSON(data),
		ActorID: int8Ptr(actorID(ctx)),
	})
}
