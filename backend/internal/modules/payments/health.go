package payments

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrphanHealthSourceCode is the platform health source of provider
// notifications that named no payment of ours (TEC-962).
const OrphanHealthSourceCode = "payment_webhook_orphans"

// Issue kinds of the orphan source.
const (
	HealthKindOrphanUnmatched  = "payment_orphan_unmatched"
	HealthKindOrphanUnresolved = "payment_orphan_unresolved"
	orphanRefType              = "payment_webhook_orphan"
	orphanIssueLimit           = 200
	orphanMatchedWindow        = 7 * 24 * time.Hour
)

// OrphanHealthQueries is the read side of the orphan source.
type OrphanHealthQueries interface {
	PaymentWebhookOrphanHealthCounts(ctx context.Context, since pgtype.Timestamptz) (db.PaymentWebhookOrphanHealthCountsRow, error)
	ListUnmatchedPaymentWebhookOrphans(ctx context.Context, rowLimit int32) ([]db.ListUnmatchedPaymentWebhookOrphansRow, error)
}

// OrphanHealthSource reports unmatched orphan notifications to the
// platform health dashboard: money the provider took for a checkout we
// have no record of (a restore), per organization when the signature
// named one, platform-level otherwise.
type OrphanHealthSource struct{ q OrphanHealthQueries }

// NewOrphanHealthSource builds the source.
func NewOrphanHealthSource(q OrphanHealthQueries) *OrphanHealthSource {
	return &OrphanHealthSource{q: q}
}

// Code implements healthcheck.Source.
func (s *OrphanHealthSource) Code() string { return OrphanHealthSourceCode }

// Collect implements healthcheck.Source.
func (s *OrphanHealthSource) Collect(ctx context.Context, now time.Time) (healthcheck.Report, error) {
	c, err := s.q.PaymentWebhookOrphanHealthCounts(ctx, pgtype.Timestamptz{Time: now.Add(-orphanMatchedWindow), Valid: true})
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep := healthcheck.Report{Metrics: []healthcheck.Metric{
		{Key: "unmatched", Value: c.Unmatched, Severity: healthcheck.SeverityCritical},
		{Key: "unresolved", Value: c.Unresolved, Severity: healthcheck.SeverityWarning},
		{Key: "matched", Value: c.Matched, Severity: healthcheck.SeverityInfo},
	}}
	if c.Unmatched+c.Unresolved == 0 {
		return rep, nil
	}
	rows, err := s.q.ListUnmatchedPaymentWebhookOrphans(ctx, orphanIssueLimit)
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep.Truncated = len(rows) >= orphanIssueLimit
	for _, r := range rows {
		issue := healthcheck.Issue{
			Kind: HealthKindOrphanUnmatched, Severity: healthcheck.SeverityCritical, Provider: r.Provider,
			Subject: r.ProviderPaymentID, Message: r.EventType + " " + r.ProviderStatus + " (conversation " + r.ConversationID + ")",
			RefType: orphanRefType, RefUUID: r.Uuid.String(), OccurredAt: r.CreatedAt.Time.UTC(),
		}
		if r.OrganizationID.Valid {
			org := r.OrganizationID.Int64
			issue.OrganizationID = &org
		} else {
			issue.Kind, issue.Severity = HealthKindOrphanUnresolved, healthcheck.SeverityWarning
			issue.Message += "; signature matched no account"
		}
		rep.Issues = append(rep.Issues, issue)
	}
	return rep, nil
}
