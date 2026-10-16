// Package sources holds the built-in health sources of the platform
// health dashboard (TEC-739): the organizations' connected provider
// accounts (iyzico, Netgsm, HGS, KABİS, e-invoice …) and the integration
// job runner's work. Other modules register their own sources in the
// healthcheck.Registry (see platformhealth.NewRegistry).
package sources

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/jackc/pgx/v5/pgtype"
)

// Source codes.
const (
	CodeIntegrationAccounts = "integration_accounts"
	CodeIntegrationJobs     = "integration_jobs"
)

// Issue kinds.
const (
	KindAccountCheckFailed    = "account_check_failed"
	KindJobFailed             = "job_failed"
	KindJobNeedsVerification  = "job_needs_verification"
	KindJobWaitingDependency  = "job_waiting_dependency"
	KindJobBlocked            = "job_blocked"
	RefTypeIntegrationAccount = "integration_account"
	RefTypeIntegrationJob     = "integration_job"
)

// IssueLimit caps the issues one source lists (newest first).
const IssueLimit = 500

// Defaults of the job source.
const (
	// DefaultFailureWindow: failed / succeeded jobs counted since now - window.
	DefaultFailureWindow = 7 * 24 * time.Hour
	// DefaultOverdueAfter: pending / submitted work this late counts as overdue.
	DefaultOverdueAfter = 30 * time.Minute
)

// AccountQueries is the read side of the account source.
type AccountQueries interface {
	IntegrationAccountHealthCounts(ctx context.Context) (db.IntegrationAccountHealthCountsRow, error)
	ListFailingIntegrationAccounts(ctx context.Context, rowLimit int32) ([]db.ListFailingIntegrationAccountsRow, error)
}

// IntegrationAccounts reports the connection tests of every organization's
// provider accounts (integration_accounts.last_check_*).
type IntegrationAccounts struct {
	q AccountQueries
}

// NewIntegrationAccounts builds the source.
func NewIntegrationAccounts(q AccountQueries) *IntegrationAccounts {
	return &IntegrationAccounts{q: q}
}

// Code implements healthcheck.Source.
func (s *IntegrationAccounts) Code() string { return CodeIntegrationAccounts }

// Collect implements healthcheck.Source.
func (s *IntegrationAccounts) Collect(ctx context.Context, _ time.Time) (healthcheck.Report, error) {
	c, err := s.q.IntegrationAccountHealthCounts(ctx)
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep := healthcheck.Report{Metrics: []healthcheck.Metric{
		{Key: "active", Value: c.Active},
		{Key: "healthy", Value: c.Healthy},
		{Key: "failing", Value: c.Failing, Severity: healthcheck.SeverityCritical},
		{Key: "unchecked", Value: c.Unchecked, Severity: healthcheck.SeverityInfo},
		{Key: "disabled", Value: c.Disabled, Severity: healthcheck.SeverityInfo},
	}}
	if c.Failing == 0 {
		return rep, nil
	}
	rows, err := s.q.ListFailingIntegrationAccounts(ctx, IssueLimit)
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep.Truncated = len(rows) >= IssueLimit
	for _, r := range rows {
		org := r.OrganizationID
		rep.Issues = append(rep.Issues, healthcheck.Issue{
			Kind:           KindAccountCheckFailed,
			Severity:       healthcheck.SeverityCritical,
			OrganizationID: &org,
			Provider:       r.Provider,
			Subject:        r.DisplayName,
			Message:        r.LastCheckError,
			RefType:        RefTypeIntegrationAccount,
			RefUUID:        r.Uuid.String(),
			OccurredAt:     firstTime(r.LastCheckedAt, r.UpdatedAt),
		})
	}
	return rep, nil
}

// JobQueries is the read side of the job source.
type JobQueries interface {
	IntegrationJobHealthCounts(ctx context.Context, arg db.IntegrationJobHealthCountsParams) (db.IntegrationJobHealthCountsRow, error)
	ListProblemIntegrationJobs(ctx context.Context, arg db.ListProblemIntegrationJobsParams) ([]db.ListProblemIntegrationJobsRow, error)
}

// IntegrationJobs reports the integration job runner (integration_jobs):
// final failures in the window, jobs waiting on verification, on a
// dependency (e.g. a KABİS branch code) or blocked, and overdue work.
type IntegrationJobs struct {
	q             JobQueries
	failureWindow time.Duration
	overdueAfter  time.Duration
}

// NewIntegrationJobs builds the source with the default windows.
func NewIntegrationJobs(q JobQueries) *IntegrationJobs {
	return &IntegrationJobs{q: q, failureWindow: DefaultFailureWindow, overdueAfter: DefaultOverdueAfter}
}

// Code implements healthcheck.Source.
func (s *IntegrationJobs) Code() string { return CodeIntegrationJobs }

// Collect implements healthcheck.Source.
func (s *IntegrationJobs) Collect(ctx context.Context, now time.Time) (healthcheck.Report, error) {
	since := ts(now.Add(-s.failureWindow))
	c, err := s.q.IntegrationJobHealthCounts(ctx, db.IntegrationJobHealthCountsParams{
		SinceAt: since, StaleBefore: ts(now.Add(-s.overdueAfter)),
	})
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep := healthcheck.Report{Metrics: []healthcheck.Metric{
		{Key: "succeeded", Value: c.Succeeded},
		{Key: "failed", Value: c.Failed, Severity: healthcheck.SeverityCritical},
		{Key: "needs_verification", Value: c.NeedsVerification, Severity: healthcheck.SeverityWarning},
		{Key: "waiting_dependency", Value: c.WaitingDependency, Severity: healthcheck.SeverityWarning},
		{Key: "blocked", Value: c.Blocked, Severity: healthcheck.SeverityWarning},
		{Key: "overdue", Value: c.Overdue, Severity: healthcheck.SeverityWarning},
	}}
	if c.Failed+c.NeedsVerification+c.WaitingDependency+c.Blocked == 0 {
		return rep, nil
	}
	rows, err := s.q.ListProblemIntegrationJobs(ctx, db.ListProblemIntegrationJobsParams{SinceAt: since, RowLimit: IssueLimit})
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep.Truncated = len(rows) >= IssueLimit
	for _, r := range rows {
		kind, sev := jobKind(r.Status)
		var org *int64
		if r.OrganizationID.Valid {
			id := r.OrganizationID.Int64
			org = &id
		}
		rep.Issues = append(rep.Issues, healthcheck.Issue{
			Kind:           kind,
			Severity:       sev,
			OrganizationID: org,
			Provider:       r.Provider,
			Subject:        r.Kind,
			Message:        r.LastError,
			RefType:        RefTypeIntegrationJob,
			RefUUID:        r.Uuid.String(),
			OccurredAt:     firstTime(r.UpdatedAt, r.CreatedAt),
		})
	}
	return rep, nil
}

func jobKind(status string) (string, string) {
	switch status {
	case "failed":
		return KindJobFailed, healthcheck.SeverityCritical
	case "needs_verification":
		return KindJobNeedsVerification, healthcheck.SeverityWarning
	case "waiting_dependency":
		return KindJobWaitingDependency, healthcheck.SeverityWarning
	default:
		return KindJobBlocked, healthcheck.SeverityWarning
	}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func firstTime(vals ...pgtype.Timestamptz) time.Time {
	for _, v := range vals {
		if v.Valid {
			return v.Time
		}
	}
	return time.Time{}
}
