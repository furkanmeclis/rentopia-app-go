// Package usecase implements field tasks (TEC-724, RT-A29): the
// assignment board, "my tasks" of the field staff, the state machine with
// its timeline, transfer tasks on the fleet transfer, the delivery tasks a
// reservation opens and the delivery eligibility / fee of service points.
// docs/design/field-tasks.md is the design.
//
// Every method takes the organization id explicitly; branch scope comes
// from orgctx (a task is visible when its branch, or a transfer's
// destination, is in the caller's scope). Members with only
// tenant.field_tasks.execute see the tasks assigned to them.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	fleetopsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxTargets caps bulk "all matching".
const MaxTargets = 2000

const permAssign = rbac.PermTenantFieldTasksAssign

var (
	// ErrNotFound: unknown, another organization's, out of scope or not
	// visible to the caller (404).
	ErrNotFound = errors.New("field task not found")
	// ErrForbidden: visible but the caller may not do this (403).
	ErrForbidden = errors.New("missing permission")
)

// StateError is answered with 409 (Code says why).
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

// Transfers is the fleet transfer flow a transfer task runs on (fleetops).
type Transfers interface {
	CreateTransfer(ctx context.Context, orgID int64, in fleetopsmodel.TransferInput) (fleetopsmodel.Transfer, error)
	PatchTransfer(ctx context.Context, orgID int64, id uuid.UUID, in fleetopsmodel.TransferInput) (fleetopsmodel.Transfer, error)
	StartTransfer(ctx context.Context, orgID int64, id uuid.UUID, in fleetopsmodel.TransferActionInput) (fleetopsmodel.Transfer, error)
	CompleteTransfer(ctx context.Context, orgID int64, id uuid.UUID, in fleetopsmodel.TransferActionInput) (fleetopsmodel.Transfer, error)
	CancelTransfer(ctx context.Context, orgID int64, id uuid.UUID, in fleetopsmodel.TransferActionInput) (fleetopsmodel.Transfer, error)
}

// Finisher completes or cancels a task with the rentals' rules (handover /
// return wait for the recorded fact; cleaning, maintenance and preparation
// make the vehicle available).
type Finisher interface {
	FinishTask(ctx context.Context, orgID int64, id uuid.UUID, done bool, note string) error
}

// Notifier enqueues the assignee's in-app notification (mirrored to the
// member's realtime channel and to web push when subscribed).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Service implements field tasks.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	transfers Transfers
	finisher  Finisher
	notifier  Notifier
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q, events: resourceevents.Nop{}, log: slog.Default(), now: time.Now}
}

// SetTransfers wires fleet transfers.
func (s *Service) SetTransfers(t Transfers) { s.transfers = t }

// SetFinisher wires the rentals' task completion.
func (s *Service) SetFinisher(f Finisher) { s.finisher = f }

// SetNotifier wires assignment notifications.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

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
		return fmt.Errorf("fieldtasks: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// publish tells every list of these rows (board, my tasks, the rentals'
// operation tasks).
func (s *Service) publish(ctx context.Context, action string, ids ...uuid.UUID) {
	if len(ids) == 0 {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	for _, r := range []string{model.Resource, model.ResourceMine, model.ResourceOperationTasks} {
		s.events.Publish(ctx, r, action, out...)
	}
}

// MirrorPublisher republishes the rentals' operation task events on the
// field task lists, so the board and "my tasks" follow handovers, returns
// and inspections without those modules knowing about them.
type MirrorPublisher struct {
	resourceevents.Publisher
}

// Publish implements resourceevents.Publisher.
func (m MirrorPublisher) Publish(ctx context.Context, resource, action string, uuids ...string) {
	m.Publisher.Publish(ctx, resource, action, uuids...)
	if resource == model.ResourceOperationTasks {
		m.Publisher.Publish(ctx, model.Resource, action, uuids...)
		m.Publisher.Publish(ctx, model.ResourceMine, action, uuids...)
	}
}

func actorID(ctx context.Context) int64 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return p.UserInternal
	}
	return 0
}

func actorArg(ctx context.Context) pgtype.Int8 {
	id := actorID(ctx)
	return pgtype.Int8{Int64: id, Valid: id > 0}
}

func has(ctx context.Context, slug string) bool {
	sc, ok := orgctx.From(ctx)
	return ok && sc.Has(slug)
}

// unscoped drops the branch restriction (system effects of an authorized
// action: a reservation's delivery tasks).
func unscoped(ctx context.Context) context.Context {
	sc, ok := orgctx.From(ctx)
	if !ok || sc.BranchIDs == nil {
		return ctx
	}
	sc.BranchIDs = nil
	return orgctx.WithScope(ctx, sc)
}

func inScope(ctx context.Context, ids ...int64) bool {
	for _, id := range ids {
		if id != 0 && orgctx.CheckBranch(ctx, id) == nil {
			return true
		}
	}
	return false
}

// access is what the caller may do with field tasks.
type access struct {
	read, assign, execute bool
	me                    int64
}

func accessOf(ctx context.Context) access {
	return access{
		read: has(ctx, rbac.PermTenantFieldTasksRead), assign: has(ctx, rbac.PermTenantFieldTasksAssign),
		execute: has(ctx, rbac.PermTenantFieldTasksExecute), me: actorID(ctx),
	}
}

func (a access) mine(row db.GetFieldTaskRow) bool {
	return a.me > 0 && row.AssigneeID.Valid && row.AssigneeID.Int64 == a.me
}

// sees: the board readers and planners see every task in scope, executors
// their own.
func (a access) sees(row db.GetFieldTaskRow) bool {
	return a.read || a.assign || (a.execute && a.mine(row))
}

// works: planners work any task, executors their own.
func (a access) works(row db.GetFieldTaskRow) bool {
	return a.assign || (a.execute && a.mine(row))
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func floatPtr(f pgtype.Float8) *float64 {
	if !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func float8(v *float64) pgtype.Float8 {
	if v == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *v, Valid: true}
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilUUIDs(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

func qArg(q string) pgtype.Text {
	q = strings.TrimSpace(q)
	return pgtype.Text{String: q, Valid: q != ""}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func requireNote(field, note string) (string, error) {
	n := strings.TrimSpace(note)
	if len([]rune(n)) < 3 || len([]rune(n)) > 1000 {
		return "", validation(field, "required", "a note is required (3-1000 characters)")
	}
	return n, nil
}
