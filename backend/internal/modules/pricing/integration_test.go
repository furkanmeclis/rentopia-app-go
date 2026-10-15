package pricing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	osrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	osusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricinghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/handler"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

type fakeCentrifugo struct {
	mu       sync.Mutex
	channels []string
}

func (f *fakeCentrifugo) Publish(_ context.Context, channel string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.channels = append(f.channels, channel)
	return nil
}

func (f *fakeCentrifugo) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.channels
	f.channels = nil
	return out
}

func istanbul(y int, m time.Month, d, h int) string {
	return time.Date(y, m, d, h, 0, 0, 0, rentaltime.Location()).Format(time.RFC3339)
}

// TestPricingEndToEnd drives rate plans, date rules, the matrix, the preview
// and the price list import over HTTP with the real organization resolver
// and SQL: acceptance RT-A07..RT-A11 through the API, versioning and the
// frozen-version triggers, online gap validation from the settings,
// equal-priority activation refusals, organization isolation and live
// channels. Runs with TEST_DATABASE_URL.
func TestPricingEndToEnd(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-price-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, dave := newUser("alice"), newUser("dave")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Pricing " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Other " + suffix, OwnerUserID: dave.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = ANY($1)", []int64{alice.ID, dave.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, other.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, dave.ID})
	})
	mrk, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: org.ID, Code: "MRK"})
	if err != nil {
		t.Fatal(err)
	}
	// Settings: VAT 20 %, 25 % prepayment, web on, online up to 14 days.
	settingsSvc := osusecase.New(osrepository.NewPostgres(pool, q))
	cfg := osmodel.Defaults()
	cfg.Tax.DefaultVATRate = "20"
	cfg.Payment = osmodel.Payment{PrepaymentRule: osmodel.PrepaymentPercent, PrepaymentValue: "25", FullPaymentAllowed: true}
	for i := range cfg.SalesChannels {
		if cfg.SalesChannels[i].Code == "web" {
			cfg.SalesChannels[i].IsActive = true
		}
	}
	maxOnline := 14
	cfg.Reservation.MaxOnlineRentalDays = &maxOnline
	if _, err := settingsSvc.Save(ctx, org.ID, alice.ID, osmodel.SaveInput{Settings: cfg}); err != nil {
		t.Fatal(err)
	}
	// Segments and products (alt grup).
	product := func(orgID int64, seg, code string) uuid.UUID {
		t.Helper()
		var segID int64
		if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, $2, $2)
			ON CONFLICT DO NOTHING RETURNING id`, orgID, seg).Scan(&segID); err != nil {
			if err := pool.QueryRow(ctx, `SELECT id FROM vehicle_segments WHERE organization_id = $1 AND code = $2`, orgID, seg).Scan(&segID); err != nil {
				t.Fatal(err)
			}
		}
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id) VALUES ($1, $2, $2, $3) RETURNING uuid`,
			orgID, code, segID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	eko, suv := product(org.ID, "EKO", "EKO-M"), product(org.ID, "SUV", "SUV-A")
	foreignProduct := product(other.ID, "EKO", "EKO-M")

	fake := &fakeCentrifugo{}
	events := resourceevents.New(fake, nil)
	events.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(q)))
	svc := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settingsSvc)
	h := pricinghandler.New(svc, activity.NewRecorder(q, nil))
	h.SetResourceEvents(events)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	pricing.RegisterRoutes(mux, h, pricing.EngineHandlers{}, tokens, mapLoader{alice.Uuid: alice.ID, dave.Uuid: dave.ID}, orgSvc)

	callIn := func(slug string, user db.User, method, path string, body any) (int, map[string]any) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		if data, ok := env["data"].(map[string]any); ok {
			return rr.Code, data
		}
		errBody, _ := env["error"].(map[string]any)
		return rr.Code, errBody
	}
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		return callIn(org.Slug, alice, method, path, body)
	}
	must := func(want int, step string) func(int, map[string]any) map[string]any {
		return func(code int, res map[string]any) map[string]any {
			t.Helper()
			if code != want {
				t.Fatalf("%s = %d, want %d: %v", step, code, want, res)
			}
			return res
		}
	}
	detailCodes := func(res map[string]any) []string {
		out := []string{}
		details, _ := res["details"].([]any)
		for _, d := range details {
			out = append(out, d.(map[string]any)["code"].(string))
		}
		return out
	}
	hasCode := func(res map[string]any, code string) bool {
		for _, c := range detailCodes(res) {
			if c == code {
				return true
			}
		}
		return false
	}
	ides := []map[string]any{
		{"min_days": 1, "max_days": 3, "daily_price": "1500"},
		{"min_days": 4, "max_days": 7, "daily_price": "1400"},
		{"min_days": 8, "max_days": 14, "daily_price": "1300"},
	}
	preview := func(start, end string, extra map[string]any) map[string]any {
		t.Helper()
		body := map[string]any{"rental_product_uuid": eko, "branch_uuid": mrk.Uuid, "channel": "web", "start_at": start, "end_at": end}
		for k, v := range extra {
			body[k] = v
		}
		return must(http.StatusOK, "preview")(call(http.MethodPost, "/v1/tenant/pricing/preview", body))
	}
	total := func(side map[string]any) string {
		t.Helper()
		b, ok := side["breakdown"].(map[string]any)
		if !ok {
			t.Fatalf("side has no breakdown: %v", side)
		}
		return b["total"].(string)
	}
	side := func(p map[string]any, key string) map[string]any { return p[key].(map[string]any) }

	// --- Rate plan: create (draft v1), validation, activation -------------
	code, res := call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": eko, "scope": "common",
		"bands": []map[string]any{{"min_days": 1, "max_days": 5, "daily_price": "1"}, {"min_days": 4, "daily_price": "1"}},
	})
	if code != http.StatusBadRequest || !hasCode(res, "band_overlap") {
		t.Fatalf("overlapping bands = %d %v", code, res)
	}
	plan := must(http.StatusCreated, "create plan")(call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": eko, "scope": "common", "name": "Genel", "bands": ides,
	}))
	planID := plan["uuid"].(string)
	if plan["status"] != "draft" || plan["draft"] == nil || plan["active"] != nil {
		t.Fatalf("new plan = %v", plan)
	}
	if chans := (func() []string { events.Wait(); return fake.take() })(); len(chans) != 1 || chans[0] != tenantmodule.ResourceChannel(org.UUID, pricing.ResourceRatePlans) {
		t.Fatalf("plan publications = %v", chans)
	}
	check := must(http.StatusOK, "check")(call(http.MethodGet, "/v1/tenant/rate-plans/"+planID+"/activation-check", nil))
	if check["ready"] != true {
		t.Fatalf("activation check = %v", check)
	}
	plan = must(http.StatusOK, "activate")(call(http.MethodPost, "/v1/tenant/rate-plans/"+planID+"/activate", map[string]any{"change_note": "İlk fiyatlar"}))
	if plan["status"] != "active" || plan["active_version"].(float64) != 1 || plan["draft"] != nil {
		t.Fatalf("activated = %v", plan)
	}
	// Frozen: the bands of the active version cannot change in SQL either.
	_, err = pool.Exec(ctx, `UPDATE rate_bands SET daily_price = 1 WHERE organization_id = $1`, org.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("frozen bands = %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE rate_plan_versions SET priority = 9 WHERE organization_id = $1`, org.ID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("frozen version = %v", err)
	}

	// --- Preview: RT-A07 (8 days × 1.300) + VAT + prepayment --------------
	p := preview(istanbul(2026, 4, 10, 10), istanbul(2026, 4, 18, 10), nil)
	cur := side(p, "current")["breakdown"].(map[string]any)
	if cur["total"] != "10400.00" || cur["billing_days"].(float64) != 8 || cur["vat_total"] != "1733.33" ||
		cur["base_total"] != "8666.67" || cur["prepayment"] != "2600.00" {
		t.Fatalf("RT-A07 = %v", cur)
	}
	if w, _ := p["warnings"].([]any); len(w) != 1 || w[0] != "no_changes" {
		t.Fatalf("warnings = %v", p["warnings"])
	}

	// --- Date rule YAZ +20 %: draft preview, activation -------------------
	code, res = call(http.MethodPost, "/v1/tenant/date-rules", map[string]any{
		"name": "YAZ", "start_date": "2026-09-01", "end_date": "2026-06-01", "kind": "percent", "value": 20,
	})
	if code != http.StatusBadRequest || !hasCode(res, "before_start") {
		t.Fatalf("reversed range = %d %v", code, res)
	}
	yaz := must(http.StatusCreated, "create YAZ")(call(http.MethodPost, "/v1/tenant/date-rules", map[string]any{
		"name": "YAZ", "start_date": "2026-06-01", "end_date": "2026-09-01", "kind": "percent", "value": "20",
	}))
	yazID := yaz["uuid"].(string)
	if code, res := call(http.MethodPost, "/v1/tenant/date-rules", map[string]any{
		"name": "yaz", "start_date": "2026-06-01", "end_date": "2026-09-01", "kind": "percent", "value": "5",
	}); code != http.StatusConflict {
		t.Fatalf("duplicate name = %d %v", code, res)
	}
	july := []string{istanbul(2026, 7, 1, 10), istanbul(2026, 7, 9, 10)}
	p = preview(july[0], july[1], map[string]any{"date_rule_uuid": yazID})
	if total(side(p, "current")) != "10400.00" || total(side(p, "proposed")) != "12480.00" || p["difference"] != "2080.00" {
		t.Fatalf("RT-A08 preview = %v", p)
	}
	must(http.StatusOK, "activate YAZ")(call(http.MethodPost, "/v1/tenant/date-rules/"+yazID+"/activate", nil))
	if got := total(side(preview(july[0], july[1], nil), "current")); got != "12480.00" {
		t.Fatalf("RT-A08 active = %s", got)
	}
	// RT-A09: starts May 30 → 2 × 1.300 + 6 × 1.560.
	if got := total(side(preview(istanbul(2026, 5, 30, 10), istanbul(2026, 6, 7, 10), nil), "current")); got != "11960.00" {
		t.Fatalf("RT-A09 = %s", got)
	}
	// RT-A10: an unsaved +100 ₺ rule with a higher priority → 1.400 per day.
	p = preview(july[0], july[1], map[string]any{"date_rule_draft": map[string]any{
		"start_date": "2026-06-01", "end_date": "2026-09-01", "kind": "amount", "value": 100, "priority": 5,
	}})
	if total(side(p, "proposed")) != "11200.00" {
		t.Fatalf("RT-A10 = %v", p)
	}
	// 1 September is off-season (end day excluded): 1.800 + 1.500.
	if got := total(side(preview(istanbul(2026, 8, 31, 10), istanbul(2026, 9, 2, 10), nil), "current")); got != "3300.00" {
		t.Fatalf("Sept 1 = %s", got)
	}
	// Equal-priority overlap with YAZ is refused at activation.
	bayram := must(http.StatusCreated, "create BAYRAM")(call(http.MethodPost, "/v1/tenant/date-rules", map[string]any{
		"name": "BAYRAM", "start_date": "2026-08-20", "end_date": "2026-08-25", "kind": "percent", "value": "50",
		"product_scope": "segments", "segment_uuids": []string{mustSegment(t, ctx, pool, org.ID, "EKO")},
	}))
	bayramID := bayram["uuid"].(string)
	code, res = call(http.MethodPost, "/v1/tenant/date-rules/"+bayramID+"/activate", nil)
	if code != http.StatusUnprocessableEntity || res["code"] != pricinghandler.CodeActivationBlocked || !hasCode(res, "priority_conflict") {
		t.Fatalf("equal priority rule = %d %v", code, res)
	}
	must(http.StatusOK, "BAYRAM priority")(call(http.MethodPatch, "/v1/tenant/date-rules/"+bayramID, map[string]any{"priority": 10}))
	must(http.StatusOK, "activate BAYRAM")(call(http.MethodPost, "/v1/tenant/date-rules/"+bayramID+"/activate", nil))

	// --- Versioning: an edit creates draft v2; activation supersedes v1 ---
	bands2 := []map[string]any{
		{"min_days": 1, "max_days": 3, "daily_price": "1550"},
		{"min_days": 4, "max_days": 7, "daily_price": "1400"},
		{"min_days": 8, "daily_price": "1300"},
	}
	plan = must(http.StatusOK, "edit plan")(call(http.MethodPatch, "/v1/tenant/rate-plans/"+planID, map[string]any{"bands": bands2}))
	if plan["current_version"].(float64) != 2 || plan["active_version"].(float64) != 1 || plan["has_draft"] != true {
		t.Fatalf("draft v2 = %v", plan)
	}
	p = preview(istanbul(2026, 4, 10, 10), istanbul(2026, 4, 11, 10), map[string]any{"rate_plan_uuid": planID})
	if total(side(p, "current")) != "1500.00" || total(side(p, "proposed")) != "1550.00" {
		t.Fatalf("plan draft preview = %v", p)
	}
	must(http.StatusOK, "activate v2")(call(http.MethodPost, "/v1/tenant/rate-plans/"+planID+"/activate", nil))
	versions := must(http.StatusOK, "versions")(call(http.MethodGet, "/v1/tenant/rate-plans/"+planID+"/versions", nil))["items"].([]any)
	if len(versions) != 2 || versions[0].(map[string]any)["status"] != "active" || versions[1].(map[string]any)["status"] != "superseded" {
		t.Fatalf("versions = %v", versions)
	}
	// Discard a draft: back to the active version.
	must(http.StatusOK, "edit again")(call(http.MethodPatch, "/v1/tenant/rate-plans/"+planID, map[string]any{"priority": 3}))
	plan = must(http.StatusOK, "discard")(call(http.MethodDelete, "/v1/tenant/rate-plans/"+planID+"/draft", nil))
	if plan["has_draft"] != false || plan["current_version"].(float64) != 2 || plan["priority"].(float64) != 0 {
		t.Fatalf("discarded = %v", plan)
	}

	// --- RT-A11: precedence and equal-priority refusal --------------------
	branchPlan := must(http.StatusCreated, "branch plan")(call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": eko, "scope": "branch", "branch_uuid": mrk.Uuid,
		"bands": []map[string]any{{"min_days": 1, "daily_price": "1200"}},
	}))
	must(http.StatusOK, "activate branch plan")(call(http.MethodPost, "/v1/tenant/rate-plans/"+branchPlan["uuid"].(string)+"/activate", nil))
	april := []string{istanbul(2026, 4, 10, 10), istanbul(2026, 4, 18, 10)}
	if b := side(preview(april[0], april[1], nil), "current")["breakdown"].(map[string]any); b["plan"].(map[string]any)["scope"] != "branch" || b["total"] != "9600.00" {
		t.Fatalf("branch beats common = %v", b)
	}
	bc := must(http.StatusCreated, "branch+channel plan")(call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": eko, "scope": "branch_channel", "branch_uuid": mrk.Uuid, "channel": "web",
		"bands": []map[string]any{{"min_days": 1, "daily_price": "1000"}},
	}))
	must(http.StatusOK, "activate branch+channel")(call(http.MethodPost, "/v1/tenant/rate-plans/"+bc["uuid"].(string)+"/activate", nil))
	if b := side(preview(april[0], april[1], nil), "current")["breakdown"].(map[string]any); b["plan"].(map[string]any)["scope"] != "branch_channel" || b["total"] != "8000.00" {
		t.Fatalf("branch+channel beats branch = %v", b)
	}
	if b := side(preview(april[0], april[1], map[string]any{"channel": "branch"}), "current")["breakdown"].(map[string]any); b["plan"].(map[string]any)["scope"] != "branch" {
		t.Fatalf("counter sale falls back to the branch plan = %v", b)
	}
	twin := must(http.StatusCreated, "twin plan")(call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": eko, "scope": "common", "bands": ides,
	}))
	twinID := twin["uuid"].(string)
	check = must(http.StatusOK, "twin check")(call(http.MethodGet, "/v1/tenant/rate-plans/"+twinID+"/activation-check", nil))
	if check["ready"] != false {
		t.Fatalf("twin check = %v", check)
	}
	code, res = call(http.MethodPost, "/v1/tenant/rate-plans/"+twinID+"/activate", nil)
	if code != http.StatusUnprocessableEntity || !hasCode(res, "priority_conflict") {
		t.Fatalf("RT-A11 equal priority = %d %v", code, res)
	}
	must(http.StatusOK, "twin priority")(call(http.MethodPatch, "/v1/tenant/rate-plans/"+twinID, map[string]any{"priority": 5}))
	must(http.StatusOK, "activate twin")(call(http.MethodPost, "/v1/tenant/rate-plans/"+twinID+"/activate", nil))

	// --- Online coverage: a web plan with a gap is refused ----------------
	gap := []map[string]any{{"min_days": 1, "max_days": 3, "daily_price": "2000"}, {"min_days": 8, "daily_price": "1800"}}
	web := must(http.StatusCreated, "web plan")(call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": suv, "scope": "channel", "channel": "web", "bands": gap,
	}))
	code, res = call(http.MethodPost, "/v1/tenant/rate-plans/"+web["uuid"].(string)+"/activate", nil)
	if code != http.StatusUnprocessableEntity || !hasCode(res, "coverage_gap") {
		t.Fatalf("online gap = %d %v", code, res)
	}
	counter := must(http.StatusCreated, "counter plan")(call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
		"rental_product_uuid": suv, "scope": "channel", "channel": "branch", "bands": gap,
	}))
	must(http.StatusOK, "counter plan with a gap")(call(http.MethodPost, "/v1/tenant/rate-plans/"+counter["uuid"].(string)+"/activate", nil))

	// --- Matrix -----------------------------------------------------------
	m := must(http.StatusOK, "matrix")(call(http.MethodGet, "/v1/tenant/rate-plans/matrix?scope=common", nil))
	if len(m["rows"].([]any)) != 2 || len(m["bands"].([]any)) != 3 {
		t.Fatalf("matrix = %v", m)
	}
	saved := must(http.StatusOK, "save matrix")(call(http.MethodPut, "/v1/tenant/rate-plans/matrix", map[string]any{
		"scope": "common", "bands": []map[string]any{{"min_days": 1, "max_days": 3}, {"min_days": 4, "max_days": 7}, {"min_days": 8, "max_days": nil}},
		"rows": []map[string]any{
			{"rental_product_uuid": eko, "prices": []any{"1550.00", "1400.00", "1300.00"}},
			{"rental_product_uuid": suv, "prices": []any{"2500", "2300", nil}},
		},
	}))
	if saved["created"].(float64) != 1 || saved["unchanged"].(float64) != 1 || saved["matrix"].(map[string]any)["drafts"].(float64) != 1 {
		t.Fatalf("matrix save = %v", saved)
	}
	// SUV drafts stop at 7 days: online coverage up to 14 is missing.
	code, res = call(http.MethodPost, "/v1/tenant/rate-plans/matrix/activate", map[string]any{"scope": "common"})
	if code != http.StatusUnprocessableEntity || !hasCode(res, "coverage_short") {
		t.Fatalf("matrix activate gap = %d %v", code, res)
	}
	must(http.StatusOK, "fix matrix")(call(http.MethodPut, "/v1/tenant/rate-plans/matrix", map[string]any{
		"scope": "common", "bands": []map[string]any{{"min_days": 1, "max_days": 3}, {"min_days": 4, "max_days": 7}, {"min_days": 8, "max_days": nil}},
		"rows": []map[string]any{{"rental_product_uuid": suv, "prices": []any{"2500", "2300", "2100"}}},
	}))
	if res := must(http.StatusOK, "matrix activate")(call(http.MethodPost, "/v1/tenant/rate-plans/matrix/activate", map[string]any{"scope": "common"})); res["activated"].(float64) != 1 {
		t.Fatalf("matrix activated = %v", res)
	}
	events.Wait()
	fake.take()

	// --- Price list import (adapter, dry-run, apply, revert) -------------
	octx := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, UUID: org.UUID, Slug: org.Slug})
	io := pricingadapters.NewRatePlansIO(svc)
	diff, err := io.PreviewRow(octx, map[string]any{"product_code": "eko-m", "band": "1-3 gün", "daily_price": "1600"}, nil)
	if err != nil || diff.Op != "update" || len(diff.Changes) != 1 || diff.Changes[0].From != "1550.00" || diff.Changes[0].To != "1600.00" {
		t.Fatalf("import preview = %+v %v", diff, err)
	}
	if diff, _ := io.PreviewRow(octx, map[string]any{"product_code": "EKO-M", "band": "2-5", "daily_price": "1"}, nil); diff.Error == "" {
		t.Fatal("overlapping import band must be a row error")
	}
	if diff, _ := io.PreviewRow(octx, map[string]any{"product_code": "NOPE", "band": "1-3", "daily_price": "1"}, nil); diff.Error == "" {
		t.Fatal("unknown product must be a row error")
	}
	row, err := io.ApplyRow(octx, map[string]any{"product_code": "EKO-M", "band": "1-3", "daily_price": "1600"}, nil)
	if err != nil || !row.OK || row.Op != "update" || row.EntityUUID != planID {
		t.Fatalf("import apply = %+v %v", row, err)
	}
	plan = must(http.StatusOK, "plan after import")(call(http.MethodGet, "/v1/tenant/rate-plans/"+planID, nil))
	if plan["draft"] == nil || plan["active"].(map[string]any)["bands"].([]any)[0].(map[string]any)["daily_price"] != "1550.00" {
		t.Fatalf("import writes the draft only = %v", plan)
	}
	if err := io.RevertRow(octx, row.EntityType, row.EntityUUID, row.Previous); err != nil {
		t.Fatal(err)
	}
	plan = must(http.StatusOK, "plan after revert")(call(http.MethodGet, "/v1/tenant/rate-plans/"+planID, nil))
	if plan["draft"].(map[string]any)["bands"].([]any)[0].(map[string]any)["daily_price"] != "1550.00" {
		t.Fatalf("revert = %v", plan)
	}
	created, err := io.ApplyRow(octx, map[string]any{"product_code": "SUV-A", "band": "1+", "daily_price": "3000", "branch_code": "MRK"}, nil)
	if err != nil || !created.OK || created.Op != "create" {
		t.Fatalf("import create = %+v %v", created, err)
	}
	if err := io.RevertRow(octx, created.EntityType, created.EntityUUID, created.Previous); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(http.MethodGet, "/v1/tenant/rate-plans/"+created.EntityUUID, nil); code != http.StatusNotFound {
		t.Fatalf("reverted import plan = %d", code)
	}

	// --- Isolation: another organization ---------------------------------
	if code, _ := callIn(other.Slug, dave, http.MethodGet, "/v1/tenant/rate-plans/"+planID, nil); code != http.StatusNotFound {
		t.Fatalf("foreign plan = %d", code)
	}
	if code, _ := callIn(other.Slug, dave, http.MethodPost, "/v1/tenant/date-rules/"+yazID+"/activate", nil); code != http.StatusNotFound {
		t.Fatalf("foreign rule activate = %d", code)
	}
	code, res = call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{"rental_product_uuid": foreignProduct, "scope": "common"})
	if code != http.StatusBadRequest || !hasCode(res, "not_found") {
		t.Fatalf("foreign product = %d %v", code, res)
	}
	page := must(http.StatusOK, "foreign list")(callIn(other.Slug, dave, http.MethodGet, "/v1/tenant/rate-plans", nil))
	if page["total"].(float64) != 0 {
		t.Fatalf("other organization sees plans: %v", page)
	}
	// The engine never sees another organization's plans: B has none.
	code, res = callIn(other.Slug, dave, http.MethodPost, "/v1/tenant/pricing/preview", map[string]any{
		"rental_product_uuid": foreignProduct, "branch_uuid": mustBranch(t, ctx, q, other.ID), "channel": "web",
		"start_at": april[0], "end_at": april[1],
	})
	if code != http.StatusOK || res["current"].(map[string]any)["error"].(map[string]any)["code"] != "no_plan" {
		t.Fatalf("foreign preview = %d %v", code, res)
	}

	// --- Lists, facets, delete -------------------------------------------
	page = must(http.StatusOK, "list")(call(http.MethodGet, "/v1/tenant/rate-plans?scope=channel&sort=-priority", nil))
	if page["total"].(float64) != 2 {
		t.Fatalf("channel plans = %v", page)
	}
	facets := must(http.StatusOK, "facets")(call(http.MethodGet, "/v1/tenant/rate-plans/facets", nil))
	if len(facets["scope"].([]any)) != 4 {
		t.Fatalf("facets = %v", facets)
	}
	rules := must(http.StatusOK, "rules")(call(http.MethodGet, "/v1/tenant/date-rules?date_from=2026-08-01&date_to=2026-08-31", nil))
	if rules["total"].(float64) != 2 {
		t.Fatalf("rules in August = %v", rules)
	}
	must(http.StatusOK, "deactivate YAZ")(call(http.MethodPost, "/v1/tenant/date-rules/"+yazID+"/deactivate", nil))
	// The branch+channel plan (1.000) applies now; without YAZ: 8 × 1.000.
	if got := total(side(preview(july[0], july[1], nil), "current")); got != "8000.00" {
		t.Fatalf("inactive YAZ still applies: %s", got)
	}
	must(http.StatusOK, "delete twin")(call(http.MethodDelete, "/v1/tenant/rate-plans/"+twinID, nil))
	if code, _ := call(http.MethodGet, "/v1/tenant/rate-plans/"+twinID, nil); code != http.StatusNotFound {
		t.Fatalf("deleted plan = %d", code)
	}
	if chans := (func() []string { events.Wait(); return fake.take() })(); len(chans) < 2 {
		t.Fatalf("rule / plan publications = %v", chans)
	}
}

func mustSegment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID int64, code string) string {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT uuid FROM vehicle_segments WHERE organization_id = $1 AND code = $2`, orgID, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func mustBranch(t *testing.T, ctx context.Context, q *db.Queries, orgID int64) string {
	t.Helper()
	b, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: "MRK"})
	if err != nil {
		t.Fatal(err)
	}
	return b.Uuid.String()
}
