package platformhealth_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/sources"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The clock: 2026-10-15 09:00 UTC (12:00 Istanbul), month = 2026-10.
var clock = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

type fx struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	q    *db.Queries
	tag  string
	org  int64
	org2 int64
	n    int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, ctx: ctx, pool: pool, q: db.New(pool), tag: strings.ReplaceAll(uuid.NewString()[:8], "-", "")}
	f.org = f.id(`INSERT INTO organizations (slug, name) VALUES ($1, $2) RETURNING id`, "ph-"+f.tag, "Sağlık "+f.tag)
	f.org2 = f.id(`INSERT INTO organizations (slug, name, status) VALUES ($1, $2, 'suspended') RETURNING id`, "ph2-"+f.tag, "Askıda "+f.tag)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM organizations WHERE id = ANY($1)`, []int64{f.org, f.org2})
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE email LIKE $1`, "%"+f.tag+"%")
		_, _ = pool.Exec(bg, `DELETE FROM platform_health_alert_cursors WHERE source LIKE $1`, "%"+f.tag)
		pool.Close()
	})
	return f
}

func (f *fx) id(query string, args ...any) int64 {
	f.t.Helper()
	var id int64
	if err := f.pool.QueryRow(f.ctx, query, args...).Scan(&id); err != nil {
		f.t.Fatalf("seed: %v\n%s", err, query)
	}
	return id
}

func (f *fx) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		f.t.Fatalf("exec: %v\n%s", err, query)
	}
}

func (f *fx) next() int { f.n++; return f.n }

func (f *fx) user(locale string, login *time.Time) int64 {
	return f.id(`INSERT INTO users (email, password_hash, name, surname, status, locale, last_login_at)
		VALUES ($1, 'x', 'Test', 'User', 'active', $2, $3) RETURNING id`,
		fmt.Sprintf("u%d-%s@example.test", f.next(), f.tag), locale, login)
}

func (f *fx) member(org, user int64) {
	role := f.id(`SELECT id FROM roles WHERE slug = 'owner' AND scope = 'tenant' AND organization_id IS NULL`)
	f.exec(`INSERT INTO organization_members (organization_id, user_id, role_id) VALUES ($1, $2, $3)`, org, user, role)
}

func (f *fx) seedUsage() {
	branch := f.id(`INSERT INTO branches (organization_id, code, name) VALUES ($1, 'MRK', 'Merkez') RETURNING id`, f.org)
	vehicle := func(plate, status string, deleted bool) {
		var del *time.Time
		if deleted {
			t := clock
			del = &t
		}
		f.exec(`INSERT INTO vehicles (organization_id, plate, model_year, home_branch_id, current_branch_id, status,
				manual_entry, manual_brand, manual_model, deleted_at)
			VALUES ($1, $2, 2024, $3, $3, $4, TRUE, 'Fiat', 'Egea', $5)`, f.org, plate, branch, status, del)
	}
	vehicle("34 PH "+f.tag[:3]+"1", "available", false)
	vehicle("34 PH "+f.tag[:3]+"2", "rented", false)
	vehicle("34 PH "+f.tag[:3]+"3", "out_of_service", false)
	vehicle("34 PH "+f.tag[:3]+"4", "available", true)

	seg := f.id(`INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'ECO', 'Ekonomi') RETURNING id`, f.org)
	product := f.id(`INSERT INTO rental_products (organization_id, code, title, segment_id) VALUES ($1, 'P1', 'Ekonomi', $2) RETURNING id`, f.org, seg)
	f.exec(`INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type) VALUES ($1, $2, 1, 'GROUP')`, f.org, product)
	customer := f.id(`INSERT INTO customers (organization_id, customer_type, first_name, last_name)
		VALUES ($1, 'individual', 'Ada', 'Yılmaz') RETURNING id`, f.org)
	reservation := func(created time.Time) {
		f.exec(`INSERT INTO reservations (organization_id, reference, status, pickup_branch_id, return_branch_id, starts_at, ends_at,
				rental_product_id, product_version, sold_rental_product_id, sold_product_version, guarantee_type, product_snapshot,
				sold_snapshot, customer_id, channel, total_amount, quote_snapshot, created_at)
			VALUES ($1, $2, 'confirmed', $3, $3, $4, $5, $6, 1, $6, 1, 'GROUP', '{}', '{}', $7, 'web', 1000, '{}', $8)`,
			f.org, fmt.Sprintf("PH-%s-%d", f.tag, f.next()), branch, created.Add(48*time.Hour), created.Add(96*time.Hour),
			product, customer, created)
	}
	// Istanbul October: 2026-09-30 21:00 UTC is 2026-10-01 00:00 local.
	reservation(time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC))
	reservation(time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC))
	reservation(time.Date(2026, 9, 30, 20, 30, 0, 0, time.UTC)) // September locally

	f.exec(`INSERT INTO customer_documents (organization_id, customer_id, doc_type, file_object_key, file_content_type, file_size_bytes)
		VALUES ($1, $2, 'identity', 'k1', 'application/pdf', 1500), ($1, $2, 'passport', 'k2', 'image/png', 500)`, f.org, customer)
	f.exec(`INSERT INTO customer_documents (organization_id, customer_id, doc_type, file_object_key, file_content_type, file_size_bytes, deleted_at)
		VALUES ($1, $2, 'other', 'k3', 'image/png', 9000, NOW())`, f.org, customer)

	recent := clock.Add(-2 * time.Hour)
	older := clock.Add(-72 * time.Hour)
	f.member(f.org, f.user("tr", &older))
	f.member(f.org, f.user("en", &recent))
}

func TestUsage(t *testing.T) {
	f := newFx(t)
	f.seedUsage()
	svc := usecase.New(f.q, healthcheck.NewRegistry())
	svc.SetClock(func() time.Time { return clock })

	start, end, label, err := svc.MonthRange("")
	if err != nil || label != "2026-10" {
		t.Fatalf("month: %v %s", err, label)
	}
	items, total, err := svc.ListUsage(f.ctx, model.UsageFilter{
		Q: f.tag, MonthStart: start, MonthEnd: end, SortField: "name", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total %d items %d", total, len(items))
	}
	// "Askıda" sorts before "Sağlık".
	got, idle := items[1], items[0]
	if got.Slug != "ph-"+f.tag || idle.Status != "suspended" {
		t.Fatalf("order: %+v / %+v", got, idle)
	}
	if got.ActiveVehicles != 2 || got.TotalVehicles != 3 {
		t.Errorf("vehicles: active %d total %d", got.ActiveVehicles, got.TotalVehicles)
	}
	if got.ReservationsMonth != 2 {
		t.Errorf("reservations this month: %d", got.ReservationsMonth)
	}
	if got.MemberCount != 2 || got.LastLoginAt == nil || !got.LastLoginAt.Equal(clock.Add(-2*time.Hour)) {
		t.Errorf("members: %d last login %v", got.MemberCount, got.LastLoginAt)
	}
	if got.StorageBytes != 2000 {
		t.Errorf("storage: %d", got.StorageBytes)
	}
	if idle.ActiveVehicles != 0 || idle.StorageBytes != 0 || idle.LastLoginAt != nil {
		t.Errorf("empty organization: %+v", idle)
	}

	// Filters, sort and paging.
	act, total, err := svc.ListUsage(f.ctx, model.UsageFilter{
		Q: f.tag, Statuses: []string{"suspended"}, MonthStart: start, MonthEnd: end, SortField: "storage_bytes", SortDesc: true, Limit: 20,
	})
	if err != nil || total != 1 || act[0].Status != "suspended" {
		t.Fatalf("status filter: %v %d", err, total)
	}
	page, total, err := svc.ListUsage(f.ctx, model.UsageFilter{
		Q: f.tag, MonthStart: start, MonthEnd: end, SortField: "active_vehicles", SortDesc: true, Limit: 1,
	})
	if err != nil || total != 2 || len(page) != 1 || page[0].Slug != "ph-"+f.tag {
		t.Fatalf("paging: %v %d %+v", err, total, page)
	}

	// September (local) holds the third reservation.
	ss, se, _, err := svc.MonthRange("2026-09")
	if err != nil {
		t.Fatal(err)
	}
	sep, _, err := svc.ListUsage(f.ctx, model.UsageFilter{Q: "ph-" + f.tag, MonthStart: ss, MonthEnd: se, SortField: "name", Limit: 5})
	if err != nil || len(sep) != 1 || sep[0].ReservationsMonth != 1 {
		t.Fatalf("september: %v %+v", err, sep)
	}

	sum, err := svc.UsageSummary(f.ctx, start, end, label)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Organizations < 2 || sum.ActiveVehicles < 2 || sum.ReservationsMonth < 2 || sum.StorageBytes < 2000 || sum.Members < 2 {
		t.Fatalf("summary: %+v", sum)
	}
}

// orgOnly narrows a source to the test organizations (the database is
// shared with other packages' tests) under a unique code.
type orgOnly struct {
	inner healthcheck.Source
	code  string
	orgs  map[int64]bool
}

func (o orgOnly) Code() string { return o.code }

func (o orgOnly) Collect(ctx context.Context, now time.Time) (healthcheck.Report, error) {
	rep, err := o.inner.Collect(ctx, now)
	if err != nil {
		return rep, err
	}
	kept := rep.Issues[:0]
	for _, is := range rep.Issues {
		if is.OrganizationID != nil && o.orgs[*is.OrganizationID] {
			kept = append(kept, is)
		}
	}
	rep.Issues = kept
	rep.Metrics = nil
	return rep, nil
}

func (f *fx) job(org int64, provider, kind, status, lastErr string, updated time.Time) {
	f.exec(`INSERT INTO integration_jobs (organization_id, direction, provider, kind, idempotency_key, status, last_error, created_at, updated_at)
		VALUES ($1, 'outbound', $2, $3, $4, $5, $6, $7, $7)`,
		org, provider, kind, fmt.Sprintf("ph:%s:%d", f.tag, f.next()), status, lastErr, updated)
}

func (f *fx) account(org int64, provider, kind, check, lastErr string, checked time.Time) {
	f.exec(`INSERT INTO integration_accounts (organization_id, kind, provider, display_name, credentials_ciphertext,
			last_checked_at, last_check_status, last_check_error)
		VALUES ($1, $2, $3, $4, 'v1:x', $5, $6, $7)`, org, kind, provider, provider+" hesabı", checked, check, lastErr)
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []notifmodel.EnqueueInput
}

func (n *fakeNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, in)
	return nil, nil
}

func TestHealthIssuesAndAlerts(t *testing.T) {
	f := newFx(t)
	now := time.Now().UTC().Truncate(time.Second)
	f.account(f.org, "netgsm", "sms", "failed", "401 unauthorized", now.Add(-10*time.Minute))
	f.account(f.org2, "iyzico", "payment", "ok", "", now.Add(-10*time.Minute))
	f.job(f.org, "kabis", "kabis.notify", "waiting_dependency", "branch code missing", now.Add(-30*time.Minute))
	f.job(f.org, "einvoice", "einvoice.issue", "failed", "rejected by GIB", now.Add(-20*time.Minute))
	f.job(f.org2, "hgs_fake", "toll.fetch", "succeeded", "", now.Add(-20*time.Minute))
	f.job(f.org2, "netgsm", "sms.send", "failed", "too old", now.Add(-10*24*time.Hour)) // outside the window

	orgs := map[int64]bool{f.org: true, f.org2: true}
	reg := healthcheck.NewRegistry(
		orgOnly{inner: sources.NewIntegrationAccounts(f.q), code: "acc_" + f.tag, orgs: orgs},
		orgOnly{inner: sources.NewIntegrationJobs(f.q), code: "jobs_" + f.tag, orgs: orgs},
	)
	svc := usecase.New(f.q, reg)

	// The built-in registry wires both sources and reports platform metrics.
	builtin := platformhealth.NewRegistry(f.q).Collect(f.ctx, now)
	if len(builtin) != 2 || builtin[0].Code != sources.CodeIntegrationAccounts || builtin[1].Code != sources.CodeIntegrationJobs {
		t.Fatalf("built-in sources: %+v", builtin)
	}
	for _, r := range builtin {
		if r.Err != nil || r.Status != healthcheck.StatusFailing {
			t.Fatalf("%s: %s %v", r.Code, r.Status, r.Err)
		}
	}

	sum, err := svc.Health(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Status != healthcheck.StatusFailing || len(sum.Sources) != 2 {
		t.Fatalf("summary: %+v", sum)
	}
	if s := sum.Sources[0]; s.Critical != 1 || s.Warning != 0 {
		t.Errorf("accounts: %+v", s)
	}
	if s := sum.Sources[1]; s.Critical != 1 || s.Warning != 1 {
		t.Errorf("jobs: %+v", s)
	}

	items, total, err := svc.ListIssues(f.ctx, model.IssueFilter{SortField: "occurred_at", SortDesc: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("issues: %d %+v", total, items)
	}
	if items[0].Kind != sources.KindAccountCheckFailed || items[0].Organization == nil || items[0].Organization.Slug != "ph-"+f.tag {
		t.Errorf("newest: %+v", items[0])
	}
	if items[2].Kind != sources.KindJobWaitingDependency || items[2].Message != "branch code missing" {
		t.Errorf("oldest: %+v", items[2])
	}
	crit, total, err := svc.ListIssues(f.ctx, model.IssueFilter{Severities: []string{"critical"}, Q: "gib", Limit: 20})
	if err != nil || total != 1 || crit[0].Provider != "einvoice" {
		t.Fatalf("filtered: %v %d %+v", err, total, crit)
	}
	bySource, total, err := svc.ListIssues(f.ctx, model.IssueFilter{Sources: []string{"acc_" + f.tag}, SortField: "severity", Limit: 1, Offset: 0})
	if err != nil || total != 1 || len(bySource) != 1 {
		t.Fatalf("source filter: %v %d", err, total)
	}

	// Alerts: the first sweep reports the two critical issues of the last
	// hour once; a second sweep reports nothing; a new failure is reported.
	n := &fakeNotifier{}
	svc.SetNotifier(n)
	admin := f.user("en", nil)
	f.exec(`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE slug = 'super_admin' AND scope = 'platform'`, admin)

	res, err := svc.RunAlerts(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fresh["acc_"+f.tag] != 1 || res.Fresh["jobs_"+f.tag] != 1 || res.Recipients < 1 {
		t.Fatalf("first sweep: %+v", res)
	}
	var mine *notifmodel.EnqueueInput
	for i := range n.sent {
		if n.sent[i].UserID != nil && *n.sent[i].UserID == admin {
			mine = &n.sent[i]
		}
	}
	if mine == nil {
		t.Fatal("admin not notified")
	}
	if !strings.Contains(mine.Title, "2 new critical") || mine.ActionURL == nil || *mine.ActionURL != usecase.AlertActionURL {
		t.Errorf("alert: %q %v", mine.Title, mine.ActionURL)
	}

	res, err = svc.RunAlerts(f.ctx)
	if err != nil || len(res.Fresh) != 0 {
		t.Fatalf("second sweep: %v %+v", err, res)
	}
	f.job(f.org2, "netgsm", "sms.send", "failed", "provider down", time.Now().UTC())
	res, err = svc.RunAlerts(f.ctx)
	if err != nil || res.Fresh["jobs_"+f.tag] != 1 || len(res.Fresh) != 1 {
		t.Fatalf("third sweep: %v %+v", err, res)
	}

	sum, err = svc.Health(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range sum.Alerts {
		if a.Source == "jobs_"+f.tag && a.LastAlertCount == 1 && a.LastAlertAt != nil {
			found = true
		}
	}
	if !found {
		t.Errorf("alert cursors: %+v", sum.Alerts)
	}
}

func TestAlertText(t *testing.T) {
	title, body := usecase.AlertText(i18n.LocaleTR, map[string]int{sources.CodeIntegrationJobs: 2, "channel_sync": 1})
	if !strings.Contains(title, "3 yeni kritik") {
		t.Fatalf("title: %s", title)
	}
	if !strings.Contains(body, "Entegrasyon işleri: 2") || !strings.Contains(body, "channel_sync: 1") {
		t.Fatalf("body: %s", body)
	}
}
