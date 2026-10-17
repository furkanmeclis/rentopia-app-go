// Package usecase computes platform usage per organization and the
// connection health of every registered health source (TEC-739).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Queries is the read / write side the service needs.
type Queries interface {
	ListOrganizationUsage(ctx context.Context, arg db.ListOrganizationUsageParams) ([]db.ListOrganizationUsageRow, error)
	PlatformUsageTotals(ctx context.Context, arg db.PlatformUsageTotalsParams) (db.PlatformUsageTotalsRow, error)
	ListHealthOrganizations(ctx context.Context, ids []int64) ([]db.ListHealthOrganizationsRow, error)
	GetPlatformHealthAlertCursor(ctx context.Context, source string) (db.GetPlatformHealthAlertCursorRow, error)
	UpsertPlatformHealthAlertCursor(ctx context.Context, arg db.UpsertPlatformHealthAlertCursorParams) error
	ListPlatformHealthAlertCursors(ctx context.Context) ([]db.ListPlatformHealthAlertCursorsRow, error)
	ListPlatformUsersWithPermission(ctx context.Context, slug string) ([]db.ListPlatformUsersWithPermissionRow, error)
}

// ValidationError carries field details for the handler.
type ValidationError struct {
	Field, Code, Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Service is the platform health use case.
type Service struct {
	q        Queries
	registry *healthcheck.Registry
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service over the health source registry.
func New(q Queries, registry *healthcheck.Registry) *Service {
	return &Service{q: q, registry: registry, log: slog.Default(), now: time.Now}
}

// SetClock overrides the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetLogger sets the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// Registry exposes the source registry.
func (s *Service) Registry() *healthcheck.Registry { return s.registry }

// MonthRange parses "YYYY-MM" (empty = the current month) as an Istanbul
// calendar month and returns [start, end) plus the normalized label.
func (s *Service) MonthRange(raw string) (time.Time, time.Time, string, error) {
	loc := rentaltime.Location()
	var start time.Time
	raw = strings.TrimSpace(raw)
	if raw == "" {
		n := s.now().In(loc)
		start = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
	} else {
		t, err := time.ParseInLocation("2006-01", raw, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", &ValidationError{Field: "month", Code: "invalid", Message: "month must be YYYY-MM"}
		}
		start = t
	}
	end := start.AddDate(0, 1, 0)
	return start.UTC(), end.UTC(), start.Format("2006-01"), nil
}

// ListUsage returns one page of the per-organization usage list.
func (s *Service) ListUsage(ctx context.Context, f model.UsageFilter) ([]model.OrganizationUsage, int64, error) {
	arg := db.ListOrganizationUsageParams{
		SortField:  f.SortField,
		SortDesc:   f.SortDesc,
		RowLimit:   f.Limit,
		RowOffset:  f.Offset,
		MonthStart: ts(f.MonthStart),
		MonthEnd:   ts(f.MonthEnd),
	}
	if len(f.Statuses) > 0 {
		arg.Statuses = f.Statuses
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		arg.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListOrganizationUsage(ctx, arg)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.OrganizationUsage, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		out = append(out, model.OrganizationUsage{
			UUID:              r.Uuid,
			Slug:              r.Slug,
			Name:              r.Name,
			Status:            r.Status,
			ActiveVehicles:    r.ActiveVehicles,
			TotalVehicles:     r.TotalVehicles,
			ReservationsMonth: r.ReservationsMonth,
			MemberCount:       r.MemberCount,
			StorageBytes:      r.StorageBytes,
			LastLoginAt:       timePtr(r.LastLoginAt),
			CreatedAt:         r.CreatedAt.Time,
		})
	}
	return out, total, nil
}

// UsageSummary returns the platform-wide totals of the month.
func (s *Service) UsageSummary(ctx context.Context, start, end time.Time, label string) (model.UsageSummary, error) {
	t, err := s.q.PlatformUsageTotals(ctx, db.PlatformUsageTotalsParams{MonthStart: ts(start), MonthEnd: ts(end)})
	if err != nil {
		return model.UsageSummary{}, err
	}
	return model.UsageSummary{
		Month:               label,
		Organizations:       t.Organizations,
		ActiveOrganizations: t.ActiveOrganizations,
		ActiveVehicles:      t.ActiveVehicles,
		ReservationsMonth:   t.ReservationsMonth,
		Members:             t.Members,
		StorageBytes:        t.StorageBytes,
	}, nil
}

// Health collects every source and summarizes it for the dashboard.
func (s *Service) Health(ctx context.Context) (model.HealthSummary, error) {
	now := s.now()
	results := s.registry.Collect(ctx, now)
	out := model.HealthSummary{CheckedAt: now.UTC(), Sources: make([]model.SourceHealth, 0, len(results)), Alerts: []model.AlertState{}}
	statuses := make([]string, 0, len(results))
	for _, r := range results {
		sh := model.SourceHealth{
			Code:       r.Code,
			Status:     r.Status,
			Metrics:    r.Report.Metrics,
			IssueCount: len(r.Report.Issues),
			Truncated:  r.Report.Truncated,
			DurationMs: r.Duration.Milliseconds(),
		}
		if sh.Metrics == nil {
			sh.Metrics = []healthcheck.Metric{}
		}
		if r.Err != nil {
			s.log.Warn("platform_health_source_failed", "source", r.Code, "error", r.Err)
			sh.Error = "source unavailable"
		}
		for _, is := range r.Report.Issues {
			switch is.Severity {
			case healthcheck.SeverityCritical:
				sh.Critical++
			case healthcheck.SeverityWarning:
				sh.Warning++
			}
		}
		out.Critical += sh.Critical
		out.Warning += sh.Warning
		statuses = append(statuses, r.Status)
		out.Sources = append(out.Sources, sh)
	}
	out.Status = healthcheck.WorstStatus(statuses...)
	cursors, err := s.q.ListPlatformHealthAlertCursors(ctx)
	if err != nil {
		return model.HealthSummary{}, err
	}
	for _, c := range cursors {
		out.Alerts = append(out.Alerts, model.AlertState{
			Source: c.Source, LastSeenAt: c.LastSeenAt.Time, LastAlertAt: timePtr(c.LastAlertAt), LastAlertCount: c.LastAlertCount,
		})
	}
	return out, nil
}

// ListIssues collects every source and returns one filtered, sorted page
// of their issues.
func (s *Service) ListIssues(ctx context.Context, f model.IssueFilter) ([]model.Issue, int64, error) {
	results := s.registry.Collect(ctx, s.now())
	var all []healthcheck.Issue
	for _, r := range results {
		if len(f.Sources) > 0 && !contains(f.Sources, r.Code) {
			continue
		}
		for _, is := range r.Report.Issues {
			if len(f.Severities) > 0 && !contains(f.Severities, is.Severity) {
				continue
			}
			all = append(all, is)
		}
	}
	orgs, err := s.organizations(ctx, all)
	if err != nil {
		return nil, 0, err
	}
	items := make([]model.Issue, 0, len(all))
	q := strings.ToLower(strings.TrimSpace(f.Q))
	for i, is := range all {
		var ref *model.OrganizationRef
		if is.OrganizationID != nil {
			if o, ok := orgs[*is.OrganizationID]; ok {
				ref = &o
			}
		}
		item := model.Issue{
			Key:          issueKey(is, i),
			Source:       is.Source,
			Kind:         is.Kind,
			Severity:     is.Severity,
			Organization: ref,
			Provider:     is.Provider,
			Subject:      is.Subject,
			Message:      is.Message,
			RefType:      is.RefType,
			RefUUID:      is.RefUUID,
			OccurredAt:   is.OccurredAt.UTC(),
		}
		if q != "" && !matches(item, q) {
			continue
		}
		items = append(items, item)
	}
	sortIssues(items, f.SortField, f.SortDesc)
	total := int64(len(items))
	start := int(f.Offset)
	if start > len(items) {
		start = len(items)
	}
	end := start + int(f.Limit)
	if end > len(items) || f.Limit <= 0 {
		end = len(items)
	}
	return items[start:end], total, nil
}

func (s *Service) organizations(ctx context.Context, issues []healthcheck.Issue) (map[int64]model.OrganizationRef, error) {
	seen := map[int64]bool{}
	ids := []int64{}
	for _, is := range issues {
		if is.OrganizationID != nil && !seen[*is.OrganizationID] {
			seen[*is.OrganizationID] = true
			ids = append(ids, *is.OrganizationID)
		}
	}
	out := map[int64]model.OrganizationRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListHealthOrganizations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = model.OrganizationRef{UUID: r.Uuid, Slug: r.Slug, Name: r.Name}
	}
	return out, nil
}

func issueKey(is healthcheck.Issue, i int) string {
	if is.RefUUID != "" {
		return is.Source + ":" + is.Kind + ":" + is.RefUUID
	}
	return fmt.Sprintf("%s:%s:%d", is.Source, is.Kind, i)
}

func matches(it model.Issue, q string) bool {
	fields := []string{it.Provider, it.Subject, it.Message, it.Source, it.Kind}
	if it.Organization != nil {
		fields = append(fields, it.Organization.Name, it.Organization.Slug)
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func sortIssues(items []model.Issue, field string, desc bool) {
	less := func(a, b model.Issue) int {
		switch field {
		case "severity":
			return healthcheck.SeverityRank(b.Severity) - healthcheck.SeverityRank(a.Severity)
		case "source":
			return strings.Compare(a.Source, b.Source)
		case "organization":
			return strings.Compare(orgName(a), orgName(b))
		default:
			return a.OccurredAt.Compare(b.OccurredAt)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		c := less(items[i], items[j])
		if c == 0 {
			// Ties: newest first.
			return items[i].OccurredAt.After(items[j].OccurredAt)
		}
		if desc {
			return c > 0
		}
		return c < 0
	})
}

func orgName(it model.Issue) string {
	if it.Organization == nil {
		return ""
	}
	return strings.ToLower(it.Organization.Name)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
