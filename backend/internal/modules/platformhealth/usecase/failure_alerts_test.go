package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type alertQueries struct {
	Queries
	slug string
}

func (q *alertQueries) ListPlatformUsersWithPermission(_ context.Context, slug string) ([]db.ListPlatformUsersWithPermissionRow, error) {
	q.slug = slug
	return []db.ListPlatformUsersWithPermissionRow{{ID: 1, Locale: "tr"}, {ID: 2, Locale: "en"}}, nil
}

type captureNotifier struct {
	mu  sync.Mutex
	ins []notifmodel.EnqueueInput
}

func (n *captureNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ins = append(n.ins, in)
	return nil, nil
}

func newAlertService(t *testing.T) (*Service, *alertQueries, *captureNotifier, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	q := &alertQueries{}
	n := &captureNotifier{}
	s := New(q, healthcheck.NewRegistry())
	s.SetNotifier(n)
	s.SetFailureAlerts(FailureAlertConfig{Email: "ops@example.test", Dedupe: RedisDeduper{RDB: rdb}})
	return s, q, n, mr
}

// A dead task alerts the platform.health.read audience in the panel and
// ALERT_EMAIL once; the same kind is deduped for 15 minutes.
func TestAlertDeadTaskDedupesPerKind(t *testing.T) {
	s, q, n, mr := newAlertService(t)
	ctx := context.Background()
	dead := queue.TaskFailure{Type: "app:export:process", TaskID: "t-1", Queue: "exports", Final: true,
		IDs: map[string]int64{"organization_id": 9}, Err: errors.New("render failed for ada@example.com")}
	s.AlertDeadTask(ctx, dead)
	s.AlertDeadTask(ctx, dead) // same kind within the window

	if q.slug != rbac.PermPlatformHealthRead {
		t.Fatalf("audience permission %q", q.slug)
	}
	if len(n.ins) != 3 {
		t.Fatalf("want 2 in-app + 1 e-mail, got %d", len(n.ins))
	}
	var email *notifmodel.EnqueueInput
	for i, in := range n.ins {
		if in.OrganizationID != nil {
			t.Fatal("platform alerts carry no organization")
		}
		if in.Channels[0] == notifmodel.ChannelEmail {
			email = &n.ins[i]
		}
	}
	if email == nil || email.Recipient == nil || *email.Recipient != "ops@example.test" || email.UserID != nil {
		t.Fatalf("e-mail alert: %+v", email)
	}
	if strings.Contains(email.Body, "ada@example.com") || !strings.Contains(email.Body, "app:export:process") {
		t.Fatalf("body must name the kind and scrub personal data: %s", email.Body)
	}
	if ttl := mr.TTL("alerts:failure:queue:app:export:process"); ttl <= 0 || ttl > 15*time.Minute {
		t.Fatalf("dedupe key ttl %v", ttl)
	}

	// Another kind alerts at once; after the window the first one again.
	s.AlertDeadTask(ctx, queue.TaskFailure{Type: "app:bulk:process", Final: true, Err: errors.New("x")})
	if len(n.ins) != 6 {
		t.Fatalf("other kind: %d", len(n.ins))
	}
	mr.FastForward(16 * time.Minute)
	s.AlertDeadTask(ctx, dead)
	if len(n.ins) != 9 {
		t.Fatalf("after the window: %d", len(n.ins))
	}
}

// Integration jobs alert on failed / needs_verification only.
func TestIntegrationJobChangedAlerts(t *testing.T) {
	s, _, n, _ := newAlertService(t)
	ctx := context.Background()
	job := jobsusecase.State{UUID: uuid.New(), Kind: "kabis.rental_start", Provider: "kabis_fake", Status: jobsusecase.StatusSucceeded}
	s.IntegrationJobChanged(ctx, 5, job, integrations.Result{})
	job.Status = jobsusecase.StatusPending
	s.IntegrationJobChanged(ctx, 5, job, integrations.Result{})
	if len(n.ins) != 0 {
		t.Fatalf("no alert for normal states: %d", len(n.ins))
	}
	job.Status = jobsusecase.StatusNeedsVerification
	job.LastError = "timeout"
	s.IntegrationJobChanged(ctx, 5, job, integrations.Result{})
	if len(n.ins) != 3 {
		t.Fatalf("needs_verification must alert: %d", len(n.ins))
	}
	if !strings.Contains(n.ins[0].Title, "kabis.rental_start") || !strings.Contains(n.ins[0].Body, job.UUID.String()) {
		t.Fatalf("alert text: %q / %q", n.ins[0].Title, n.ins[0].Body)
	}
}

// Without SetFailureAlerts nothing is sent.
func TestFailureAlertsOff(t *testing.T) {
	n := &captureNotifier{}
	s := New(&alertQueries{}, healthcheck.NewRegistry())
	s.SetNotifier(n)
	if sent, err := s.AlertFailure(context.Background(), Failure{Source: FailureSourceQueue, Kind: "x"}); sent || err != nil || len(n.ins) != 0 {
		t.Fatalf("off: sent=%v err=%v n=%d", sent, err, len(n.ins))
	}
}
