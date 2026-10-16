// Package usecase is the finance core: the customer / supplier ledger
// ("cari", TEC-717) and the cash / bank book with expenses (TEC-720).
// docs/design/ledger.md and docs/design/cash-accounts.md are the design.
//
// Ledger rules:
//   - entries are append-only (database trigger); a correction is a
//     reversal (the exact opposite of one entry) or an adjustment, both
//     with a reason and tenant.finance.ledger.adjust;
//   - a firm debt is accrued exactly once under its accrual key (rent of a
//     rental, an approved charge); SyncRental reconciles the live accruals
//     of a rental with its current state (reversal + new accrual when an
//     amount changes) and runs inside the caller's transaction;
//   - confirmed payments and refunds post one collection / refund entry
//     each (unique per payment / refund), so a repeated webhook, poll or
//     sweep posts nothing twice;
//   - invoices (TEC-722) link to entries and never post.
//
// Every method takes the organization id explicitly; the branch scope comes
// from orgctx (entries and cash accounts of other branches are invisible).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrEntryNotFound       = errors.New("ledger entry not found")
	ErrRentalNotFound      = errors.New("rental not found")
	ErrCustomerNotFound    = errors.New("customer not found")
	ErrCashAccountNotFound = errors.New("cash account not found")
	ErrMovementNotFound    = errors.New("cash movement not found")
	ErrCategoryNotFound    = errors.New("expense category not found")
	ErrExpenseNotFound     = errors.New("expense not found")
	ErrNoAttachment        = errors.New("the expense has no attachment")
	// ErrForbidden: the action needs a permission the caller lacks (403).
	ErrForbidden = errors.New("missing permission")
	// ErrAllBranchesRequired: the action needs every branch (403).
	ErrAllBranchesRequired = errors.New("this action needs access to every branch")
	// ErrStorageUnavailable: no object storage is configured.
	ErrStorageUnavailable = errors.New("file storage is not configured")
	// ErrCodeTaken: the code is used by another record (409).
	ErrCodeTaken = errors.New("the code is already in use")
)

// StateError answers 409 with Code.
type StateError struct{ Code, Message string }

func (e *StateError) Error() string { return e.Message }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// Detail is a field validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
type ValidationError struct{ Details []Detail }

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

// PayLinks is the payments port (statement + pay link).
type PayLinks interface {
	CreateLink(ctx context.Context, orgID int64, reservationUUID uuid.UUID, in paymodel.CreateLinkInput) (paymodel.Link, error)
}

// SMS is the SMS port (statement send).
type SMS interface {
	Send(ctx context.Context, orgID int64, in smsusecase.SendInput) (smsusecase.Sent, error)
}

// Service implements the finance use cases.
type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	storage storage.Driver
	events  resourceevents.Publisher
	links   PayLinks
	sms     SMS
	// payments refunds online collections (TEC-718).
	payments PaymentRefunder
	preauth  *preAuthDeps
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service. store may be nil (no expense attachments).
func New(pool *pgxpool.Pool, q *db.Queries, store storage.Driver) *Service {
	return &Service{
		pool: pool, q: q, storage: store, events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		s.events = p
	}
}

// SetMessaging wires the pay-link and SMS ports of the statement send.
func (s *Service) SetMessaging(links PayLinks, sms SMS) {
	s.links, s.sms = links, sms
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

// today is the Istanbul calendar day of now.
func (s *Service) today() time.Time { return day(s.clock()) }

// day truncates t to its Istanbul calendar day (as a UTC midnight date).
func day(t time.Time) time.Time {
	l := t.In(rentaltime.Location())
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("finance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil {
			out = append(out, id.String())
		}
	}
	s.events.Publish(ctx, resource, action, out...)
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func has(ctx context.Context, slug string) bool {
	sc, ok := orgctx.From(ctx)
	return ok && sc.Has(slug)
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func pgCode(err error, code string) (string, bool) {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == code {
		return pe.ConstraintName, true
	}
	return "", false
}

func uniqueViolation(err error) bool {
	_, ok := pgCode(err, "23505")
	return ok
}

func m(n pgtype.Numeric) money.Money { return money.FromNumericOrZero(n) }

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func uuidOf(v uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: v, Valid: v != uuid.Nil} }

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func textOf(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}

func dateOf(t time.Time) pgtype.Date { return pgtype.Date{Time: day0(t), Valid: true} }

// day0 keeps the calendar date of a date value (no zone shift).
func day0(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func dateNarg(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return dateOf(*t)
}

func dateString(d pgtype.Date) model.Date {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func numericOf(v *money.Money) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	return v.Numeric()
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilIDs(v []int64) []int64 {
	if len(v) == 0 {
		return nil
	}
	return v
}

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

func refOf(id pgtype.UUID, code, name pgtype.Text) *model.Ref {
	if !id.Valid {
		return nil
	}
	return &model.Ref{UUID: id.Bytes, Code: code.String, Name: name.String}
}

// ParseDate reads YYYY-MM-DD.
func ParseDate(field, raw string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, validation(field, "invalid", field+" must be a date (YYYY-MM-DD)")
	}
	return t, nil
}

// optionalDate parses an optional date with a default.
func optionalDate(ve *ValidationError, field string, raw *string, def time.Time) time.Time {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return def
	}
	t, err := ParseDate(field, *raw)
	if err != nil {
		ve.add(field, "invalid", field+" must be a date (YYYY-MM-DD)")
		return def
	}
	return t
}

func requireReason(field, reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if len([]rune(r)) < 3 || len([]rune(r)) > 500 {
		return "", validation(field, "required", "a reason is required (3-500 characters)")
	}
	return r, nil
}

func contains(set []string, v string) bool {
	for _, x := range set {
		if x == v {
			return true
		}
	}
	return false
}

// branchIDs resolves branch uuids of a filter inside the organization;
// unknown uuids answer a validation error (never another organization's).
func (s *Service) branchIDs(ctx context.Context, orgID int64, field string, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		b, err := s.q.GetBranchRefByUUID(ctx, db.GetBranchRefByUUIDParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, validation(field, "not_found", "unknown branch")
		}
		if err != nil {
			return nil, err
		}
		out = append(out, b.ID)
	}
	return out, nil
}

// branchFor resolves the branch a new entry / movement / record belongs
// to. An explicit branch must be in the caller's scope; without one the
// default applies, and a branch-scoped caller needs a branch.
func (s *Service) branchFor(ctx context.Context, orgID int64, field string, id *uuid.UUID, def int64) (int64, error) {
	if id != nil {
		b, err := s.q.GetBranchRefByUUID(ctx, db.GetBranchRefByUUIDParams{OrganizationID: orgID, Uuid: *id})
		if notFound(err) || (err == nil && orgctx.CheckBranch(ctx, b.ID) != nil) {
			return 0, validation(field, "not_found", "unknown branch")
		}
		if err != nil {
			return 0, err
		}
		return b.ID, nil
	}
	if def != 0 {
		if orgctx.CheckBranch(ctx, def) != nil {
			return 0, validation(field, "not_found", "the record belongs to a branch outside your scope")
		}
		return def, nil
	}
	if orgctx.RequireAllBranches(ctx) != nil {
		return 0, validation(field, "required", "choose a branch")
	}
	return 0, nil
}

// customerName is the display name of a customer row.
func customerName(kind, first, last, company string, anonymized bool) string {
	if anonymized {
		return ""
	}
	if kind == "corporate" {
		return strings.TrimSpace(company)
	}
	return strings.TrimSpace(first + " " + last)
}
