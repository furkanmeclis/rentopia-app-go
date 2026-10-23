package usecase

import (
	"context"
	"strconv"
	"strings"
	"time"

	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/getsentry/sentry-go"
	"github.com/redis/go-redis/v9"
)

// Failed-work alerts (TEC-744): an Asynq task whose retries are exhausted
// (archived, "dead") or an integration job that reaches failed /
// needs_verification notifies the platform administrators holding
// platform.health.read in the panel and, when configured, ALERT_EMAIL.
// One alert per source + job kind per window (Redis SET NX), so a storm
// of the same failure sends one message.

// Failure sources and statuses.
const (
	FailureSourceQueue           = "queue"
	FailureSourceIntegrationJobs = "integration_jobs"
	FailureStatusDead            = "dead"
)

// DefaultFailureAlertWindow is the dedupe window per job kind.
const DefaultFailureAlertWindow = 15 * time.Minute

// FailureActionURL opens the health dashboard.
const FailureActionURL = "/platform/health"

// Failure is one piece of failed work.
type Failure struct {
	// Source is FailureSourceQueue or FailureSourceIntegrationJobs.
	Source string
	// Kind is the task type or the integration job kind (the dedupe key).
	Kind string
	// Status: dead (queue), failed or needs_verification (jobs).
	Status   string
	Ref      string
	Queue    string
	Provider string
	// OrganizationID is the job's organization (0 = platform work).
	OrganizationID int64
	Error          string
}

// Deduper answers true for the first call per key within ttl.
type Deduper interface {
	First(ctx context.Context, key string, ttl time.Duration) bool
}

// RedisDeduper dedupes across API and worker processes with SET NX. Without
// Redis (or when it fails) it falls back to a per-process window.
type RedisDeduper struct {
	RDB *redis.Client
}

// First implements Deduper.
func (d RedisDeduper) First(ctx context.Context, key string, ttl time.Duration) bool {
	if d.RDB != nil {
		ok, err := d.RDB.SetNX(ctx, key, "1", ttl).Result()
		if err == nil {
			return ok
		}
	}
	return observability.Allow(key, ttl)
}

// FailureAlertConfig configures the alerts.
type FailureAlertConfig struct {
	// Email receives every alert as well (ALERT_EMAIL; empty = in-app only).
	Email string
	// Window is the dedupe window per job kind (0 = 15 minutes).
	Window time.Duration
	Dedupe Deduper
}

// SetFailureAlerts enables the failed-work alerts (needs SetNotifier).
func (s *Service) SetFailureAlerts(c FailureAlertConfig) {
	if c.Window <= 0 {
		c.Window = DefaultFailureAlertWindow
	}
	if c.Dedupe == nil {
		c.Dedupe = RedisDeduper{}
	}
	s.failures = &c
}

// FailureDedupeKey is the Redis key of a failure's dedupe window.
func FailureDedupeKey(f Failure) string {
	return "alerts:failure:" + f.Source + ":" + f.Kind
}

// AlertFailure sends one alert for f unless the same source + kind was
// alerted within the window. It returns whether an alert went out.
func (s *Service) AlertFailure(ctx context.Context, f Failure) (bool, error) {
	if s.failures == nil || s.notifier == nil {
		return false, nil
	}
	if !s.failures.Dedupe.First(ctx, FailureDedupeKey(f), s.failures.Window) {
		return false, nil
	}
	f.Error = trimText(observability.ScrubString(f.Error), 300)
	users, err := s.q.ListPlatformUsersWithPermission(ctx, rbac.PermPlatformHealthRead)
	if err != nil {
		return false, err
	}
	payload := map[string]any{
		"kind": "platform_health.failure", "source": f.Source, "job_kind": f.Kind, "status": f.Status, "ref": f.Ref,
	}
	action := FailureActionURL
	for _, u := range users {
		loc := i18n.Normalize(u.Locale)
		title, body := FailureText(loc, f)
		uid := u.ID
		if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp}, Priority: notifmodel.PriorityHigh,
			Title: title, Body: body, ActionURL: &action, Language: string(loc),
			SourceEvent: "platform_health.failure", Payload: payload,
		}); err != nil {
			s.log.Warn("failure_alert_notify_failed", "user_id", uid, "error", err)
		}
	}
	if to := strings.TrimSpace(s.failures.Email); to != "" {
		title, body := FailureText(i18n.LocaleTR, f)
		if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			Channels: []string{notifmodel.ChannelEmail}, Priority: notifmodel.PriorityHigh, Recipient: &to,
			Title: title, Body: body, ActionURL: &action, Language: string(i18n.LocaleTR),
			SourceEvent: "platform_health.failure", Payload: payload,
		}); err != nil {
			s.log.Warn("failure_alert_email_failed", "error", err)
		}
	}
	s.log.Warn("failure_alert_sent", "source", f.Source, "job_kind", f.Kind, "status", f.Status, "ref", f.Ref, "recipients", len(users))
	return true, nil
}

// AlertDeadTask is the worker's hook for tasks whose retries are exhausted
// (queue.Worker.WithFailureAlerts).
func (s *Service) AlertDeadTask(ctx context.Context, t queue.TaskFailure) {
	f := Failure{
		Source: FailureSourceQueue, Kind: t.Type, Status: FailureStatusDead, Ref: t.TaskID, Queue: t.Queue,
		OrganizationID: t.IDs["organization_id"],
	}
	if t.Err != nil {
		f.Error = t.Err.Error()
	}
	if _, err := s.AlertFailure(ctx, f); err != nil {
		s.log.Warn("failure_alert_failed", "source", f.Source, "job_kind", f.Kind, "error", err)
	}
}

// IntegrationJobChanged implements the integration runner's listener: a
// job that reaches failed or needs_verification is reported to the error
// tracker (job kind, job uuid, organization) and alerted.
func (s *Service) IntegrationJobChanged(ctx context.Context, orgID int64, job jobsusecase.State, _ integrations.Result) {
	if job.Status != jobsusecase.StatusFailed && job.Status != jobsusecase.StatusNeedsVerification {
		return
	}
	f := Failure{
		Source: FailureSourceIntegrationJobs, Kind: job.Kind, Status: job.Status, Ref: job.UUID.String(),
		Provider: job.Provider, OrganizationID: orgID, Error: job.LastError,
	}
	if observability.Enabled() && observability.Allow("integration-job:"+job.Kind+":"+job.Status, time.Minute) {
		observability.CaptureMessage(ctx, "integration job "+job.Status+": "+job.Kind, observability.Options{
			Level:       sentry.LevelWarning,
			Transaction: "integration " + job.Kind,
			Fingerprint: []string{"integration-job", job.Kind, job.Status},
			Tags: map[string]string{
				"job_kind": job.Kind, "job_uuid": job.UUID.String(), "job_status": job.Status,
				"provider": job.Provider, "organization_id": strconv.FormatInt(orgID, 10),
			},
			Extra: map[string]any{"attempts": job.Attempts, "operation": job.Operation, "error": job.LastError},
		})
	}
	if _, err := s.AlertFailure(ctx, f); err != nil {
		s.log.Warn("failure_alert_failed", "source", f.Source, "job_kind", f.Kind, "error", err)
	}
}

// FailureText renders the alert title and body in loc.
func FailureText(loc i18n.Locale, f Failure) (string, string) {
	tl := func(key string) string { return i18n.Translate(loc, key) }
	status := tl("platform_health.failure_status." + f.Status)
	if strings.HasPrefix(status, "platform_health.") {
		status = f.Status
	}
	title := strings.NewReplacer("{kind}", f.Kind, "{status}", status).Replace(tl("platform_health.failure_alert.title"))
	ref := f.Ref
	if f.Provider != "" {
		ref = f.Provider + " · " + ref
	}
	if f.OrganizationID > 0 {
		ref += " · org " + strconv.FormatInt(f.OrganizationID, 10)
	}
	errText := f.Error
	if errText == "" {
		errText = "-"
	}
	body := strings.NewReplacer(
		"{source}", SourceLabel(loc, f.Source), "{kind}", f.Kind, "{status}", status, "{ref}", ref, "{error}", errText,
	).Replace(tl("platform_health.failure_alert.body"))
	return title, body
}

func trimText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
