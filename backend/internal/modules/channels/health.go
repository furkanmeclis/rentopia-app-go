package channels

import (
	"context"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/jackc/pgx/v5/pgtype"
)

// HealthSourceCode is the platform health source of the channel manager.
const HealthSourceCode = "channels"

// Issue kinds of the channel health source.
const (
	HealthKindSyncFailed  = "channel_sync_failed"
	HealthKindPushExpired = "channel_push_expired"
	HealthKindErrorQueue  = "channel_error_queue"
	healthRefType         = "integration_account"
	healthIssueLimit      = 500
)

// HealthQueries is the read side of the health source.
type HealthQueries interface {
	ChannelHealthCounts(ctx context.Context, now pgtype.Timestamptz) (db.ChannelHealthCountsRow, error)
	ListChannelHealthIssues(ctx context.Context, arg db.ListChannelHealthIssuesParams) ([]db.ListChannelHealthIssuesRow, error)
}

// HealthSource reports the channel accounts of every organization to the
// platform health dashboard (TEC-739): failed pushes / pulls, pushes whose
// validity ran out (the channel applies its offline policy) and messages
// waiting in the error queue.
type HealthSource struct{ q HealthQueries }

// NewHealthSource builds the source.
func NewHealthSource(q HealthQueries) *HealthSource { return &HealthSource{q: q} }

// Code implements healthcheck.Source.
func (s *HealthSource) Code() string { return HealthSourceCode }

func at(ts ...pgtype.Timestamptz) time.Time {
	for _, t := range ts {
		if t.Valid {
			return t.Time.UTC()
		}
	}
	return time.Time{}
}

// Collect implements healthcheck.Source.
func (s *HealthSource) Collect(ctx context.Context, now time.Time) (healthcheck.Report, error) {
	n := pgtype.Timestamptz{Time: now, Valid: true}
	c, err := s.q.ChannelHealthCounts(ctx, n)
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep := healthcheck.Report{Metrics: []healthcheck.Metric{
		{Key: "active", Value: c.Active},
		{Key: "failing", Value: c.Failing, Severity: healthcheck.SeverityCritical},
		{Key: "expired", Value: c.Expired, Severity: healthcheck.SeverityCritical},
		{Key: "error_queue", Value: c.ErrorQueue, Severity: healthcheck.SeverityWarning},
		{Key: "disabled", Value: c.Disabled, Severity: healthcheck.SeverityInfo},
	}}
	if c.Failing+c.Expired+c.ErrorQueue == 0 {
		return rep, nil
	}
	rows, err := s.q.ListChannelHealthIssues(ctx, db.ListChannelHealthIssuesParams{Now: n, RowLimit: healthIssueLimit})
	if err != nil {
		return healthcheck.Report{}, err
	}
	rep.Truncated = len(rows) >= healthIssueLimit
	for _, r := range rows {
		org := r.OrganizationID
		issue := func(kind, severity, msg string, when time.Time) {
			rep.Issues = append(rep.Issues, healthcheck.Issue{
				Kind: kind, Severity: severity, OrganizationID: &org, Provider: r.Provider, Subject: r.DisplayName,
				Message: msg, RefType: healthRefType, RefUUID: r.Uuid.String(), OccurredAt: when,
			})
		}
		if r.LastPushStatus == "failed" {
			issue(HealthKindSyncFailed, healthcheck.SeverityCritical, "push: "+r.LastPushError, at(r.LastPushAt, r.UpdatedAt))
		}
		if r.LastPullStatus == "failed" {
			issue(HealthKindSyncFailed, healthcheck.SeverityCritical, "pull: "+r.LastPullError, at(r.LastPullAt, r.UpdatedAt))
		}
		if r.PushEnabled && r.PushedValidUntil.Valid && r.PushedValidUntil.Time.Before(now) {
			issue(HealthKindPushExpired, healthcheck.SeverityCritical, "pushed capacity expired; the channel applies its offline policy",
				at(r.PushedValidUntil))
		}
		if r.OpenErrors > 0 {
			issue(HealthKindErrorQueue, healthcheck.SeverityWarning, strconv.FormatInt(r.OpenErrors, 10)+" failed booking messages",
				at(r.UpdatedAt))
		}
	}
	return rep, nil
}
