// Package usecase is the availability engine service (TEC-697): capacity
// search, holds and the commitment API reservations (TEC-706) build on.
// docs/design/availability-engine.md is the design.
//
// Every write runs in one transaction under the advisory lock of the
// eligibility component it touches: load the component (vehicles, blocks,
// transfers, allocations), expire due holds, plan with the pure engine,
// write the plan (moves + the allocation), commit. The EXCLUDE constraint
// on vehicle_allocations is the backstop: a stale plan fails at the
// database and is retried with fresh data.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAllocationNotFound = errors.New("allocation not found")
	ErrRuleNotFound       = errors.New("capacity rule not found")
	// ErrNotActive: the allocation was already released / expired.
	ErrNotActive = errors.New("the allocation is not active")
	// ErrHoldExpired: the hold lapsed before the call.
	ErrHoldExpired = errors.New("the hold has expired")
	// ErrNotHold: the call needs a hold.
	ErrNotHold = errors.New("the allocation is not a hold")
	// ErrRuleTaken: a live rule already covers this type, channel, product
	// and branch.
	ErrRuleTaken = errors.New("a rule with the same type, channel, product and branch exists")
	// errStale: the plan no longer matches the database (retry).
	errStale = errors.New("availability: stale plan")
)

// CapacityError is answered with 409 CAPACITY_UNAVAILABLE.
type CapacityError struct {
	Reason       string
	Alternatives *model.Alternatives
}

func (e *CapacityError) Error() string { return "capacity unavailable: " + e.Reason }

// IsCapacity unwraps a CapacityError.
func IsCapacity(err error) (*CapacityError, bool) {
	var ce *CapacityError
	ok := errors.As(err, &ce)
	return ce, ok
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

// Settings reads the organization settings (sales channels, hold minutes).
type Settings interface {
	Current(ctx context.Context, orgID int64) (orgsettingsmodel.Version, error)
}

// Service is the availability engine.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	vehicles *vehicleusecase.Service
	settings Settings
	now      func() time.Time
	opts     engine.Options
	cache    *fleetCache
	// noLock disables the component lock (tests prove the EXCLUDE
	// constraint alone keeps concurrent writers correct).
	noLock bool
	// maxAttempts bounds the retries of a stale plan.
	maxAttempts int
	// planMoved records reassignments made outside a reservation flow
	// (rental group membership removals) on the reservations they moved.
	planMoved PlanMovedHandler
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries, vehicles *vehicleusecase.Service, settings Settings) *Service {
	return &Service{
		pool: pool, q: q, vehicles: vehicles, settings: settings, now: time.Now,
		opts: engine.DefaultOptions, cache: newFleetCache(), maxAttempts: 5,
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// DisableComponentLock turns the advisory lock off (tests only).
func (s *Service) DisableComponentLock() { s.noLock = true }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// lockKey is the advisory lock key of a component.
func lockKey(orgID, component int64) int64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "availability:%d:%d", orgID, component)
	return int64(h.Sum64())
}

// write runs fn in a transaction with retries on stale plans (exclusion
// violations, deadlocks, serialization failures, optimistic misses).
func (s *Service) write(ctx context.Context, orgID int64, fn func(q *db.Queries, tx pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < s.maxAttempts; attempt++ {
		err = s.tx(ctx, fn)
		if !retryable(err) {
			return err
		}
		s.cache.invalidate(orgID)
	}
	return err
}

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("availability: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// read runs fn in a read-only repeatable-read transaction (one consistent
// snapshot for a whole search).
func (s *Service) read(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("availability: begin read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errStale) {
		return true
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23P01", "40P01", "40001":
			return true
		}
	}
	return false
}

// lock takes the component lock and defers nothing else.
func (s *Service) lock(ctx context.Context, q *db.Queries, orgID, component int64) error {
	if s.noLock {
		return nil
	}
	return q.LockAvailabilityComponent(ctx, lockKey(orgID, component))
}

// lockFresh takes the component lock and checks, inside it, that the
// eligibility graph the plan uses is still current. A fleet change (a
// rental group membership removed, a vehicle edited) committed between
// reading the snapshot and taking the lock makes the plan stale: the
// writer retries with a fresh graph, so a new plan never lands on a
// vehicle that has just left a guarantee.
func (s *Service) lockFresh(ctx context.Context, q *db.Queries, orgID int64, snap *fleetSnapshot, component int64) error {
	if err := s.lock(ctx, q, orgID, component); err != nil {
		return err
	}
	fp, err := q.GetAvailabilityFingerprint(ctx, orgID)
	if err != nil {
		return err
	}
	if fp != snap.fingerprint {
		return errStale
	}
	return nil
}

// deferExclusion makes the overlap constraint checked at commit, so a
// reassignment plan can be written in any order.
func deferExclusion(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SET CONSTRAINTS ex_vehicle_allocations_overlap DEFERRED")
	return err
}
