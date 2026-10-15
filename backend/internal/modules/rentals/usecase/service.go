// Package usecase holds rentals and operation tasks (TEC-709): a rental is
// born from a confirmed reservation once its plate is fixed, the handover
// turns the reservation's allocation into a rental allocation, the return
// moves the engine's end to the actual return (time and branch), a swap
// moves the rest of the range to another vehicle, and the overdue sweep
// keeps a late vehicle busy and opens late_return resolution tasks for the
// reservations it runs into. docs/design/rentals.md is the design.
//
// Capacity is the availability engine's: every rental write that touches
// capacity runs inside the engine transaction (availability TxHook), so the
// allocation, the rental row, its legs, events, tasks and the vehicle
// status commit or roll back together.
//
// Every method takes the organization id explicitly; branch scope comes from
// orgctx (a rental is visible when its planned pick-up, planned return or
// actual return branch is in the caller's scope).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxListTargets caps exports.
const MaxListTargets = 10000

var (
	ErrNotFound       = errors.New("rental not found")
	ErrTaskNotFound   = errors.New("operation task not found")
	ErrChargeNotFound = errors.New("charge not found")
	// ErrForbidden: the action needs a permission the caller lacks (403).
	ErrForbidden = errors.New("missing permission")
	// ErrConcurrent: the rental changed in between (409).
	ErrConcurrent = errors.New("the rental was changed by someone else; reload and retry")
)

// StateError is answered with 409 INVALID_STATE.
type StateError struct {
	Code    string
	Message string
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

// Reservations is the reservation core as rentals use it (plate fix,
// extension, plan effects, late return tasks).
type Reservations interface {
	FixPlate(ctx context.Context, orgID int64, reservation, vehicle uuid.UUID) error
	Extend(ctx context.Context, orgID int64, reservation uuid.UUID, endsAt time.Time, reason string) (ExtendResult, error)
	RecordPlanEffects(ctx context.Context, tx pgx.Tx, orgID int64, r availmodel.HookResult, cause string, causeRef uuid.UUID) ([]uuid.UUID, error)
	LateReturnTasks(ctx context.Context, orgID int64, rental uuid.UUID, allocations []uuid.UUID, data map[string]any) []uuid.UUID
}

// ExtendResult is what an extension changed on the reservation.
type ExtendResult struct {
	Difference string
	Total      string
}

// Service implements rentals and operation tasks.
type Service struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	avail        *availusecase.Service
	reservations Reservations
	vehicles     *vehicleusecase.Service
	documents    model.Documents
	events       resourceevents.Publisher
	now          func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries, avail *availusecase.Service, reservations Reservations, vehicles *vehicleusecase.Service) *Service {
	return &Service{
		pool: pool, q: q, avail: avail, reservations: reservations, vehicles: vehicles,
		documents: model.NoDocuments{}, events: resourceevents.Nop{}, now: time.Now,
	}
}

// SetDocuments wires contracts (TEC-710).
func (s *Service) SetDocuments(d model.Documents) {
	if d != nil {
		s.documents = d
	}
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	if pub != nil {
		s.events = pub
	}
}

// SetClock replaces the clock (tests). The engine has its own.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rentals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// hookOf adapts a rental write to the engine's transaction hook.
func (s *Service) hookOf(fn func(ctx context.Context, q *db.Queries, tx pgx.Tx, r availmodel.HookResult) error) availmodel.TxHook {
	return func(ctx context.Context, tx pgx.Tx, r availmodel.HookResult) error {
		return fn(ctx, s.q.WithTx(tx), tx, r)
	}
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

func inScope(ctx context.Context, ids ...int64) bool {
	for _, id := range ids {
		if id != 0 && orgctx.CheckBranch(ctx, id) == nil {
			return true
		}
	}
	return false
}

// unscoped drops the branch restriction (system effects of an authorized
// rental action: the vehicle, the engine, other reservations).
func unscoped(ctx context.Context) context.Context {
	sc, ok := orgctx.From(ctx)
	if !ok || sc.BranchIDs == nil {
		return ctx
	}
	sc.BranchIDs = nil
	return orgctx.WithScope(ctx, sc)
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

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func int2Ptr(v *int16) pgtype.Int2 {
	if v == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: *v, Valid: true}
}

func int4Ptr(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func textOf(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

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

func requireReason(field, reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if len(r) < 3 || len(r) > 500 {
		return "", validation(field, "required", "a reason is required (3-500 characters)")
	}
	return r, nil
}

// event writes one timeline entry.
func event(ctx context.Context, q *db.Queries, orgID, rentalID int64, action, from, to, reason string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	return q.InsertRentalEvent(ctx, db.InsertRentalEventParams{
		OrganizationID: orgID, RentalID: rentalID, Action: action, FromStatus: textOf(from), ToStatus: textOf(to),
		Reason: reason, Data: mustJSON(data), ActorID: int8Ptr(actorID(ctx)),
	})
}

// mapEngineErr turns engine errors into ours.
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
	case errors.Is(err, availusecase.ErrNotActive):
		return stateErr("allocation_not_active", "the reservation no longer holds the vehicle")
	case errors.Is(err, availusecase.ErrNotReservation):
		return stateErr("not_confirmed", "only a confirmed reservation can be handed over")
	case errors.Is(err, availusecase.ErrNotRental):
		return stateErr("not_handed_over", "the vehicle is not handed over")
	case errors.Is(err, availusecase.ErrRentalUnderway):
		return stateErr("rental_underway", "the vehicle is already handed over")
	case errors.Is(err, availusecase.ErrAllocationNotFound):
		return ErrNotFound
	}
	return err
}

// ConflictError is answered with 409 and Code (data = alternatives /
// details), e.g. EXTENSION_CONFLICT from the reservation's extension rule.
type ConflictError struct {
	Code    string
	Detail  string
	Message string
	Data    any
}

func (e *ConflictError) Error() string { return e.Message }
