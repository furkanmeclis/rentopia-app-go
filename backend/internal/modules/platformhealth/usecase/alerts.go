package usecase

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Notifier sends the alert notifications.
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// SetNotifier attaches the notification pipeline (nil: alerts are only
// recorded on the cursors).
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// AlertLookback is how far back the very first sweep of a source looks
// (older failures are history, not news).
const AlertLookback = time.Hour

// AlertActionURL opens the issue list on the dashboard.
const AlertActionURL = "/platform/health?tab=issues"

// AlertResult is the outcome of one sweep.
type AlertResult struct {
	// Fresh counts the new critical issues per source.
	Fresh map[string]int
	// Recipients is the number of administrators notified.
	Recipients int
}

// RunAlerts is the periodic sweep: every critical issue a source reports
// that is newer than the source's cursor is new; the platform
// administrators holding platform.health.read get one in-app notification
// summarizing them, and the cursors move past them so each failure is
// reported once. A source that cannot be read is skipped (its cursor stays).
func (s *Service) RunAlerts(ctx context.Context) (AlertResult, error) {
	now := s.now()
	res := AlertResult{Fresh: map[string]int{}}
	type advance struct {
		source string
		seen   time.Time
		count  int
	}
	var moves []advance
	for _, r := range s.registry.Collect(ctx, now) {
		if r.Err != nil {
			s.log.Warn("platform_health_alert_source_failed", "source", r.Code, "error", r.Err)
			continue
		}
		cursor := now.Add(-AlertLookback)
		known := false
		row, err := s.q.GetPlatformHealthAlertCursor(ctx, r.Code)
		switch {
		case err == nil:
			cursor, known = row.LastSeenAt.Time, true
		case !isNoRows(err):
			return res, err
		}
		newest := cursor
		fresh := 0
		for _, is := range r.Report.Issues {
			if is.Severity != healthcheck.SeverityCritical || !is.OccurredAt.After(cursor) {
				continue
			}
			fresh++
			if is.OccurredAt.After(newest) {
				newest = is.OccurredAt
			}
		}
		if fresh > 0 {
			res.Fresh[r.Code] = fresh
			moves = append(moves, advance{source: r.Code, seen: newest, count: fresh})
		} else if !known {
			// First sweep with nothing new: start the cursor here.
			moves = append(moves, advance{source: r.Code, seen: cursor})
		}
	}
	if len(res.Fresh) > 0 {
		n, err := s.notify(ctx, res.Fresh)
		if err != nil {
			return res, err
		}
		res.Recipients = n
	}
	for _, m := range moves {
		arg := upsertCursor(m.source, m.seen, m.count, now)
		if err := s.q.UpsertPlatformHealthAlertCursor(ctx, arg); err != nil {
			return res, err
		}
	}
	return res, nil
}

// RunAlertsCount adapts RunAlerts to the worker's sweep signature.
func (s *Service) RunAlertsCount(ctx context.Context) (int, error) {
	r, err := s.RunAlerts(ctx)
	total := 0
	for _, n := range r.Fresh {
		total += n
	}
	return total, err
}

func (s *Service) notify(ctx context.Context, fresh map[string]int) (int, error) {
	if s.notifier == nil {
		return 0, nil
	}
	users, err := s.q.ListPlatformUsersWithPermission(ctx, rbac.PermPlatformHealthRead)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, u := range users {
		loc := i18n.Normalize(u.Locale)
		title, body := AlertText(loc, fresh)
		uid := u.ID
		action := AlertActionURL
		if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID:      &uid,
			Channels:    []string{notifmodel.ChannelInapp},
			Priority:    notifmodel.PriorityHigh,
			Title:       title,
			Body:        body,
			ActionURL:   &action,
			Language:    string(loc),
			SourceEvent: "platform_health.alert",
			Payload:     map[string]any{"kind": "platform_health.alert", "sources": fresh},
		}); err != nil {
			s.log.Warn("platform_health_alert_notify_failed", "user_id", uid, "error", err)
			continue
		}
		sent++
	}
	return sent, nil
}

// AlertText renders the alert title and body ("Integration: 3 new failed
// jobs, 1 failing account").
func AlertText(loc i18n.Locale, fresh map[string]int) (string, string) {
	codes := make([]string, 0, len(fresh))
	total := 0
	for c, n := range fresh {
		codes = append(codes, c)
		total += n
	}
	sort.Strings(codes)
	parts := make([]string, 0, len(codes))
	for _, c := range codes {
		parts = append(parts, SourceLabel(loc, c)+": "+strconv.Itoa(fresh[c]))
	}
	// Keys live in i18n/catalog_platform_health.go.
	tl := func(key string) string { return i18n.Translate(loc, key) }
	title := strings.ReplaceAll(tl("platform_health.alert.title"), "{count}", strconv.Itoa(total))
	body := strings.ReplaceAll(tl("platform_health.alert.body"), "{details}", strings.Join(parts, ", "))
	return title, body
}

// SourceLabel is the source's display name from the backend catalog
// ("platform_health.source.<code>"), or the code itself.
func SourceLabel(loc i18n.Locale, code string) string {
	key := "platform_health.source." + code
	if v := i18n.Translate(loc, key); v != key {
		return v
	}
	return code
}
