// Package usecase holds toll passes (TEC-725, RT-A20, docs/design/tolls.md):
// ingesting passes from a provider fetch or a file import (one row per
// provider transaction, revisions for changed facts), matching them to the
// rental whose ACTUAL handover → return interval of the vehicle contains
// the pass time, the toll and processing fee charges on that rental
// (approved facts; an amount change or a re-assignment reverses the charge
// and creates a new one, never edits it), the review queue for everything
// ambiguous, manual corrections with a mandatory reason, vehicle tags,
// settings and the periodic provider fetch through the integration job
// runner.
//
// Every method takes the organization id explicitly. Branch scope comes
// from orgctx: a pass is visible when its vehicle's home / current branch
// or its rental's branches are in the caller's scope.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound: unknown pass (or another organization's / outside the
	// branch scope).
	ErrNotFound = errors.New("toll pass not found")
	// ErrVehicleNotFound: unknown vehicle (or outside the scope).
	ErrVehicleNotFound = errors.New("vehicle not found")
	// ErrTagNotFound: unknown tag of the vehicle.
	ErrTagNotFound = errors.New("toll tag not found")
	// ErrNoAccount: no active toll provider account is connected.
	ErrNoAccount = errors.New("no active toll provider account is connected")
)

// Detail is a field-level validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError carries field details (400).
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

// Accounts is the part of the integration accounts service tolls need.
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
}

// Service implements toll passes.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	accounts  Accounts
	runner    *jobsusecase.Runner
	providers map[string]tolls.Provider
	ledger    model.LedgerPoster
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service and registers the fetch operation of every
// provider on ops (the runner's operation registry). accounts / runner /
// ops may be nil (no provider fetch: file import only).
func New(pool *pgxpool.Pool, q *db.Queries, accounts Accounts, runner *jobsusecase.Runner,
	ops *integrations.OperationRegistry, providers ...tolls.Provider) *Service {
	s := &Service{
		pool: pool, q: q, accounts: accounts, runner: runner, providers: map[string]tolls.Provider{},
		ledger: model.NoLedger{}, events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		s.providers[p.Provider()] = p
		ops.Register(p.Provider(), &fetchOperation{svc: s, provider: p})
	}
	return s
}

// SetLedger plugs the finance ledger (TEC-717).
func (s *Service) SetLedger(l model.LedgerPoster) {
	if l != nil {
		s.ledger = l
	}
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		s.events = p
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

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tolls: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// publish tells open lists what changed (passes, and the rentals whose
// toll charges changed).
func (s *Service) publish(ctx context.Context, orgID int64, action string, passes []uuid.UUID, rentals []uuid.UUID) {
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	if len(passes) > 0 {
		ids := make([]string, 0, len(passes))
		for _, p := range passes {
			ids = append(ids, p.String())
		}
		s.events.Publish(ctx, model.Resource, action, ids...)
	}
	if len(rentals) > 0 {
		ids := make([]string, 0, len(rentals))
		seen := map[uuid.UUID]bool{}
		for _, r := range rentals {
			if !seen[r] {
				seen[r] = true
				ids = append(ids, r.String())
			}
		}
		s.events.Publish(ctx, model.ResourceRentals, "updated", ids...)
	}
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v > 0} }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

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

func exclusionViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23P01"
}

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

// requireReason validates a mandatory reason (3-500 characters).
func requireReason(field, v string) (string, error) {
	v = strings.TrimSpace(v)
	if len([]rune(v)) < 3 || len([]rune(v)) > 500 {
		return "", validation(field, "required", "a reason is required (3-500 characters)")
	}
	return v, nil
}

// visible reports whether one of the branches is in the caller's scope
// (or the caller sees every branch).
func visible(ctx context.Context, ids ...pgtype.Int8) bool {
	sc, ok := orgctx.From(ctx)
	if !ok || sc.BranchIDs == nil {
		return true
	}
	for _, id := range ids {
		if id.Valid && orgctx.CheckBranch(ctx, id.Int64) == nil {
			return true
		}
	}
	return false
}

func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
