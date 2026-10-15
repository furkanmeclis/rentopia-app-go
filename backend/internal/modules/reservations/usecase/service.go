// Package usecase holds the reservation core: quotes (TEC-704),
// reservations with their state machine and capacity commitment (TEC-706),
// the provisional plate plan, re-planning and resolution tasks (TEC-698).
// docs/design/reservations.md is the design.
//
// Capacity is the availability engine's: every reservation write that
// touches capacity runs inside the engine's transaction (model.TxHook), so
// the allocation and the reservation row commit or roll back together and a
// release happens exactly once (the allocation leaves "active" once, the
// reservation leaves its status once: both updates are optimistic).
//
// Every method takes the organization id explicitly; branch scope comes
// from orgctx (a reservation is visible when its pick-up OR return branch
// is in the caller's scope).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxListTargets caps exports and bulk "all matching" scopes.
const MaxListTargets = 10000

var (
	ErrNotFound      = errors.New("reservation not found")
	ErrQuoteNotFound = errors.New("quote not found")
	ErrTaskNotFound  = errors.New("resolution task not found")
	// ErrForbidden: the action needs a permission the caller lacks (403).
	ErrForbidden = errors.New("missing permission")
	// ErrConcurrent: the reservation changed in between (409).
	ErrConcurrent = errors.New("the reservation was changed by someone else; reload and retry")
	// ErrExternalReferenceTaken: the channel's reservation number exists.
	ErrExternalReferenceTaken = errors.New("a reservation with this external reference exists for the channel")
)

// StateError is answered with 409 INVALID_STATE (the action does not fit
// the reservation's state).
type StateError struct {
	Code    string
	Message string
}

func (e *StateError) Error() string { return e.Message }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// QuoteError is answered with 409: the quote can no longer be used.
// Code: QUOTE_EXPIRED, QUOTE_USED, QUOTE_PRICE_CHANGED (Replacement is the
// fresh quote to show), QUOTE_MISMATCH.
type QuoteError struct {
	Code        string
	Message     string
	Replacement *model.Quote
}

func (e *QuoteError) Error() string { return e.Message }

// PricingError is answered with 422 PRICING_UNAVAILABLE (no rate plan …).
type PricingError struct {
	Code    string
	Message string
}

func (e *PricingError) Error() string { return e.Message }

// EligibilityError is answered with 422 DRIVER_NOT_ELIGIBLE.
type EligibilityError struct {
	Drivers []model.Eligibility
}

func (e *EligibilityError) Error() string { return "a driver does not meet the rental conditions" }

// ExtensionConflictError is answered with 409 EXTENSION_CONFLICT: the
// extension collides (RT-A31); alternatives come with difference quotes.
type ExtensionConflictError struct {
	Alternatives model.ExtensionAlternatives
}

func (e *ExtensionConflictError) Error() string {
	return "the extension collides with another commitment"
}

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

// Settings reads the organization settings (approval mode, windows,
// quote validity, prepayment, policies, reference prefix).
type Settings interface {
	Current(ctx context.Context, orgID int64) (osmodel.Version, error)
}

// Service implements quotes, reservations and resolution tasks.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	avail    *availusecase.Service
	pricing  *pricingusecase.Service
	settings Settings

	equipment model.EquipmentReserver
	coupons   model.CouponRedeemer
	events    resourceevents.Publisher

	now func() time.Time
}

// New builds the service with the default (not yet wired) extras,
// equipment and coupon ports.
func New(pool *pgxpool.Pool, q *db.Queries, avail *availusecase.Service, pricing *pricingusecase.Service, settings Settings) *Service {
	return &Service{
		pool: pool, q: q, avail: avail, pricing: pricing, settings: settings,
		equipment: model.NoEquipment{}, coupons: model.NoCoupons{},
		events: resourceevents.Nop{}, now: time.Now,
	}
}

// SetEquipment wires the equipment stock (TEC-702).
func (s *Service) SetEquipment(e model.EquipmentReserver) {
	if e != nil {
		s.equipment = e
	}
}

// SetCoupons wires campaigns / coupons (TEC-703).
func (s *Service) SetCoupons(c model.CouponRedeemer) {
	if c != nil {
		s.coupons = c
	}
}

// SetResourceEvents publishes live list changes made outside a request
// (block hook, expiry sweep).
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	if pub != nil {
		s.events = pub
	}
}

// SetClock replaces the clock (tests). The availability engine has its own.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// tx runs fn in one transaction.
func (s *Service) tx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reservations: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// actorID is the signed-in user of a request context (nil in workers).
func actorID(ctx context.Context) *int64 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		id := p.UserInternal
		return &id
	}
	return nil
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

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

func textOf(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
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

// has reports whether the caller's tenant role grants slug. Contexts
// without a scope (workers, system hooks) have no request permissions.
func has(ctx context.Context, slug string) bool {
	sc, ok := orgctx.From(ctx)
	return ok && sc.Has(slug)
}

// inScope: a reservation is visible when its pick-up OR return branch is
// in the caller's branch scope.
func inScope(ctx context.Context, pickup, ret int64) bool {
	return orgctx.CheckBranch(ctx, pickup) == nil || orgctx.CheckBranch(ctx, ret) == nil
}

// unscoped drops the branch restriction (system follow-ups such as
// re-planning a reservation another member's block affected).
func unscoped(ctx context.Context) context.Context {
	sc, ok := orgctx.From(ctx)
	if !ok || sc.BranchIDs == nil {
		return ctx
	}
	sc.BranchIDs = nil
	return orgctx.WithScope(ctx, sc)
}

// publish tells open lists what changed (best effort).
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

// settingsOf returns the current settings version.
func (s *Service) settingsOf(ctx context.Context, orgID int64) (osmodel.Version, error) {
	if s.settings == nil {
		return osmodel.Version{Settings: osmodel.Defaults()}, nil
	}
	return s.settings.Current(ctx, orgID)
}

func minutesOr(v *int, def int) int {
	if v != nil && *v > 0 {
		return *v
	}
	return def
}

var slugLetters = regexp.MustCompile(`[^A-Z0-9]`)

// referencePrefix is the settings prefix, else the first letters of the
// organization slug ("ides-oto" → "IDE").
func referencePrefix(settings osmodel.Settings, slug string) string {
	if p := strings.TrimSpace(settings.Reservation.ReferencePrefix); p != "" {
		return p
	}
	letters := slugLetters.ReplaceAllString(strings.ToUpper(slug), "")
	for len(letters) > 0 && letters[0] >= '0' && letters[0] <= '9' {
		letters = letters[1:]
	}
	if len(letters) > 3 {
		letters = letters[:3]
	}
	if len(letters) < 2 {
		return "RZV"
	}
	return letters
}

// nextReference allocates PREFIX-YYYY-NNNNNN inside the create transaction.
func (s *Service) nextReference(ctx context.Context, q *db.Queries, orgID int64, settings osmodel.Settings) (string, error) {
	slug, err := q.GetReservationOrganizationSlug(ctx, orgID)
	if err != nil {
		return "", err
	}
	year := rentaltime.Local(s.clock()).Year()
	n, err := q.NextReservationNumber(ctx, db.NextReservationNumberParams{OrganizationID: orgID, Year: int32(year)})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d-%06d", referencePrefix(settings, slug), year, n), nil
}

// capacityErr unwraps the engine's capacity refusal.
func capacityErr(err error) (*availusecase.CapacityError, bool) {
	return availusecase.IsCapacity(err)
}

// mapEngineErr turns engine validation errors into ours (same field codes).
func mapEngineErr(err error) error {
	var ve *availusecase.ValidationError
	if errors.As(err, &ve) {
		out := &ValidationError{}
		for _, d := range ve.Details {
			out.add(d.Field, d.Code, d.Message)
		}
		return out
	}
	switch {
	case errors.Is(err, availusecase.ErrHoldExpired):
		return stateErr("hold_expired", "the option (hold) of the reservation has expired")
	case errors.Is(err, availusecase.ErrNotActive):
		return stateErr("allocation_not_active", "the reservation no longer holds capacity")
	case errors.Is(err, availusecase.ErrAllocationNotFound):
		return ErrNotFound
	}
	return err
}

var _ model.PaymentRecorder = (*Service)(nil)

type pgxTx = pgx.Tx

func itoa(i int) string { return strconv.Itoa(i) }

// hookOf adapts a reservation write to the engine's transaction hook.
func hookOf(fn func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error, s *Service) availmodel.TxHook {
	return func(ctx context.Context, tx pgx.Tx, r availmodel.HookResult) error {
		return fn(ctx, s.q.WithTx(tx), r)
	}
}
