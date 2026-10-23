package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReconcilableStatuses are the states a "reconciled" resolution applies to
// (TEC-962): work that may or may not have reached the provider. Final
// states (succeeded, reconciled, reported_manually, ignored) are refused,
// and so are the held states other modules move on.
var ReconcilableStatuses = []string{StatusPending, StatusSubmitted, StatusNeedsVerification, StatusFailed}

// ErrNotReconcilable: the job is final, inbound, held or owned by another
// flow (payment refunds, KABİS notifications).
var ErrNotReconcilable = errors.New("the job cannot be marked reconciled in its current state")

// CanReconcile reports whether the staff "reconciled" resolution applies.
// Payment refunds run through /payments/{uuid}/refund and KABİS
// notifications through their own reconcile (reason, reference, evidence).
func CanReconcile(kind, direction, status string) bool {
	if direction != "outbound" || kind == "payment.refund" || strings.HasPrefix(kind, "kabis.") {
		return false
	}
	return slices.Contains(ReconcilableStatuses, status)
}

// Note / reference limits of a reconcile.
const (
	ReconcileNoteMin   = 3
	ReconcileNoteMax   = 500
	ReconcileRefMax    = 200
	ReconcileKindsMax  = 50
	reconcileListLimit = 10000
)

// ReconcileInput is a staff "reconciled" resolution: why, and optionally
// the provider's reference of the work (e.g. the Netgsm jobid seen in the
// provider panel).
type ReconcileInput struct {
	Note        string
	ExternalRef string
	// ActorID is the member who reconciled (0 = an operator tool).
	ActorID int64
}

// Reconciled is one job a reconcile moved.
type Reconciled struct {
	OrganizationID int64
	ID             int64
	UUID           uuid.UUID
	Kind           string
	PreviousStatus string
}

func actorParam(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }

// Reconcile marks one job reconciled: the work was confirmed outside the
// runner, so nothing is ever sent for it again. It sends nothing itself
// (allowed whatever the plan, like disabling an account). A job held by a
// worker answers ErrBusy.
func (r *Runner) Reconcile(ctx context.Context, orgID int64, id uuid.UUID, in ReconcileInput) (Reconciled, error) {
	row, err := r.q.GetRunnerJobByUUID(ctx, db.GetRunnerJobByUUIDParams{OrganizationID: orgParam(orgID), Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Reconciled{}, ErrNotFound
	}
	if err != nil {
		return Reconciled{}, err
	}
	if !CanReconcile(row.Kind, row.Direction, row.Status) {
		return Reconciled{}, ErrNotReconcilable
	}
	n, err := r.q.ReconcileRunnerJob(ctx, db.ReconcileRunnerJobParams{
		ReconciledBy: actorParam(in.ActorID), Note: strings.TrimSpace(in.Note), ExternalRef: strings.TrimSpace(in.ExternalRef),
		OrganizationID: orgParam(orgID), ID: row.ID, FromStatuses: []string{row.Status},
	})
	if err != nil {
		return Reconciled{}, err
	}
	if n != 1 {
		// Moved by a worker in between, or held by one right now.
		fresh, err := r.load(ctx, orgID, row.ID)
		if err == nil && fresh.Status != row.Status {
			return Reconciled{}, ErrNotReconcilable
		}
		return Reconciled{}, ErrBusy
	}
	r.Changed(ctx, orgID, row.ID)
	return Reconciled{OrganizationID: orgID, ID: row.ID, UUID: row.Uuid, Kind: row.Kind, PreviousStatus: row.Status}, nil
}

// BulkReconcileFilter selects the jobs of a bulk reconcile: created before
// Before, in one of Statuses (a subset of ReconcilableStatuses), of one of
// Kinds when given.
type BulkReconcileFilter struct {
	Before   time.Time
	Statuses []string
	Kinds    []string
	// OrganizationUUIDs / JobUUIDs narrow the operator listing (nil =
	// every organization / job).
	OrganizationUUIDs []uuid.UUID
	JobUUIDs          []uuid.UUID
}

// Validate checks a bulk filter (field, message).
func (f BulkReconcileFilter) Validate() (string, string) {
	if f.Before.IsZero() {
		return "before", "a cut-off time is required"
	}
	if len(f.Statuses) == 0 {
		return "statuses", "at least one status is required"
	}
	for _, s := range f.Statuses {
		if !slices.Contains(ReconcilableStatuses, s) {
			return "statuses", "statuses must be among pending, submitted, needs_verification, failed"
		}
	}
	if len(f.Kinds) > ReconcileKindsMax {
		return "kinds", "too many kinds"
	}
	for _, k := range f.Kinds {
		if strings.TrimSpace(k) == "" || len(k) > 48 {
			return "kinds", "kinds must be job kinds such as sms.send"
		}
	}
	return "", ""
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// ReconcileMatching marks every matching job of the organization
// reconciled (the restore runbook's bulk route). Jobs held by a worker are
// skipped. Listeners and the live list hear each change.
func (r *Runner) ReconcileMatching(ctx context.Context, orgID int64, f BulkReconcileFilter, in ReconcileInput) ([]Reconciled, error) {
	if orgID <= 0 {
		return nil, ErrNotFound
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	rows, err := r.q.ReconcileRunnerJobs(ctx, db.ReconcileRunnerJobsParams{
		ReconciledBy: actorParam(in.ActorID), Note: strings.TrimSpace(in.Note), OrganizationID: orgParam(orgID),
		Statuses: f.Statuses, Before: pgtype.Timestamptz{Time: f.Before.UTC(), Valid: true}, Kinds: nilIfEmpty(f.Kinds),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Reconciled, 0, len(rows))
	for _, row := range rows {
		r.Changed(ctx, orgID, row.ID)
		out = append(out, Reconciled{OrganizationID: orgID, ID: row.ID, UUID: row.Uuid, Kind: row.Kind, PreviousStatus: row.PreviousStatus})
	}
	return out, nil
}

// Candidate is a job a bulk reconcile would move (operator dry run).
type Candidate struct {
	OrganizationID   int64
	OrganizationUUID uuid.UUID
	OrganizationSlug string
	UUID             uuid.UUID
	Provider         string
	Kind             string
	Status           string
	Attempts         int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ReconcileCandidates lists the jobs a bulk reconcile would move, across
// organizations (cmd/integration-jobs-reconcile).
func (r *Runner) ReconcileCandidates(ctx context.Context, f BulkReconcileFilter) ([]Candidate, error) {
	var orgs, jobs []uuid.UUID
	if len(f.OrganizationUUIDs) > 0 {
		orgs = f.OrganizationUUIDs
	}
	if len(f.JobUUIDs) > 0 {
		jobs = f.JobUUIDs
	}
	rows, err := r.q.ListReconcilableRunnerJobs(ctx, db.ListReconcilableRunnerJobsParams{
		OrganizationUuids: orgs, JobUuids: jobs, Statuses: f.Statuses, Before: pgtype.Timestamptz{Time: f.Before.UTC(), Valid: true},
		Kinds: nilIfEmpty(f.Kinds),
	})
	if err != nil {
		return nil, err
	}
	if len(rows) > reconcileListLimit {
		rows = rows[:reconcileListLimit]
	}
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, Candidate{
			OrganizationID: row.OrganizationID.Int64, OrganizationUUID: row.OrganizationUuid, OrganizationSlug: row.OrganizationSlug,
			UUID: row.Uuid, Provider: row.Provider, Kind: row.Kind, Status: row.Status, Attempts: row.Attempts,
			CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		})
	}
	return out, nil
}
