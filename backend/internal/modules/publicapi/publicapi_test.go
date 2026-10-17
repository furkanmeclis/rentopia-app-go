package publicapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	portalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	customerrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Public booking API (TEC-728) against a real database: two organizations
// (A with an online payment account, B without), the same product in
// each, keys of every kind, the iyzico fake for payment links and a
// capturing webhook receiver.

type ref struct {
	id   int64
	uuid uuid.UUID
}

type tenant struct {
	id      int64
	uuid    uuid.UUID
	slug    string
	ctx     context.Context
	segment int64
	branch  ref
	point   ref
	product ref
	plate   int
}

type env struct {
	t        *testing.T
	pool     *pgxpool.Pool
	q        *db.Queries
	res      *resusecase.Service
	svc      *usecase.Service
	server   *httptest.Server
	redis    *miniredis.Miniredis
	suffix   string
	clockMu  sync.Mutex
	shift    time.Duration
	a, b     *tenant
	settings *orgsettingsusecase.Service
}

var perms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantPaymentsRead, rbac.PermTenantPaymentsCreateLink, rbac.PermTenantIntegrationsManage,
	rbac.PermTenantPublicAPIRead, rbac.PermTenantPublicAPIManage,
}

func (e *env) must(err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) now() time.Time {
	e.clockMu.Lock()
	defer e.clockMu.Unlock()
	return time.Now().Add(e.shift)
}

func (e *env) advance(d time.Duration) {
	e.clockMu.Lock()
	e.shift += d
	e.clockMu.Unlock()
}

type stubResolver struct{}

func (stubResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{}, orgctx.ErrNotMember
}

func newEnv(t *testing.T) *env {
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
	q := db.New(pool)
	e := &env{t: t, pool: pool, q: q, suffix: strings.ReplaceAll(uuid.NewString()[:8], "-", "")}
	t.Cleanup(pool.Close)

	e.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, e.settings)
	avail.SetClock(e.now)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), e.settings)
	e.res = resusecase.New(pool, q, avail, pricing, e.settings)
	e.res.SetClock(e.now)

	fc, err := crypto.NewFieldCipher(strings.Repeat("q", 32))
	e.must(err)
	customers := customerusecase.New(customerrepository.NewPostgres(pool, q), fc, nil, nil, slog.Default())
	vault, err := integrations.NewVault(fc)
	e.must(err)
	pay := iyzicofake.New()
	t.Cleanup(pay.Server.Close)
	pay.AddAccount("sandbox-api-key-0001", "sandbox-secret-key-0001")
	payAdapter := iyzico.New(iyzico.Config{SandboxBaseURL: pay.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(payAdapter))
	payments := paymentsusecase.New(pool, q, accounts, e.res, paymentsusecase.Config{PublicBaseURL: "https://app.test"}, payAdapter)
	payments.SetClock(e.now)
	portal := portalusecase.New(pool, q, nil, e.res)
	portal.SetPayments(payments, accounts)
	portal.SetClock(e.now)

	e.svc, err = usecase.New(pool, q, fc, e.settings, e.res, customers, portal)
	e.must(err)
	e.svc.SetClock(e.now)
	e.svc.AllowPrivateWebhooks(true)
	lim := usecase.DefaultLimits()
	lim.WebhookTimeout = 3 * time.Second
	lim.WebhookMaxAttempts = 3
	e.svc.SetLimits(lim)
	e.res.SetLifecycleListener(e.svc)

	e.redis = miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: e.redis.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	mux := http.NewServeMux()
	publicapi.RegisterRoutes(mux, handler.NewPublic(e.svc, ratelimit.New(rdb, "test")), handler.NewPanel(e.svc, nil), nil, nil, stubResolver{})
	e.server = httptest.NewServer(mux)
	t.Cleanup(e.server.Close)

	e.a = e.newTenant("a", accounts, true)
	e.b = e.newTenant("b", accounts, false)
	return e
}

func (e *env) newTenant(name string, accounts *accountsusecase.Service, payments bool) *tenant {
	ctx := context.Background()
	slug := "pa" + name + "-" + e.suffix
	org, err := e.q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: slug, Name: "Public " + strings.ToUpper(name) + " " + e.suffix})
	e.must(err)
	e.t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM activity_events WHERE organization_id = $1", org.ID)
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM integration_jobs WHERE organization_id = $1", org.ID)
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	tn := &tenant{id: org.ID, uuid: org.Uuid, slug: slug}
	tn.ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, UUID: org.Uuid, Slug: slug, Status: orgctx.StatusActive, Permissions: perms})
	e.must(e.pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, org.ID).Scan(&tn.segment))
	b, err := e.q.CreateBranch(ctx, db.CreateBranchParams{
		OrganizationID: org.ID, Code: "MRK", Name: "Merkez Şube", City: "İstanbul", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
		PrepMinutes: 30, IsActive: true,
	})
	e.must(err)
	tn.branch = ref{b.ID, b.Uuid}
	e.must(e.pool.QueryRow(ctx, `INSERT INTO service_points (organization_id, branch_id, type, name) VALUES ($1, $2, 'office', 'Ofis') RETURNING id, uuid`,
		org.ID, b.ID).Scan(&tn.point.id, &tn.point.uuid))
	_, err = e.pool.Exec(ctx, `INSERT INTO service_points (organization_id, branch_id, type, name, online_selectable) VALUES ($1, $2, 'office', 'Gizli', FALSE)`, org.ID, b.ID)
	e.must(err)
	tn.product = e.newProduct(tn)
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	sixty, thirty := 60, 30
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = &sixty, &sixty, &thirty
	s.Policies.Cancellation = "Teslimden 24 saat öncesine kadar ücretsiz iptal."
	for i := range s.SalesChannels {
		switch s.SalesChannels[i].Code {
		case "web", "widget", "api":
			s.SalesChannels[i].IsActive = true
		}
	}
	data, _ := json.Marshal(s)
	_, err = e.pool.Exec(ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, org.ID, data)
	e.must(err)
	if payments {
		_, err = accounts.Connect(tn.ctx, org.ID, accountsusecase.ConnectInput{
			Provider: iyzico.ProviderCode, DisplayName: "iyzico", Mode: integrations.ModeSandbox,
			Credentials: map[string]string{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "sandbox-secret-key-0001"},
		})
		e.must(err)
	}
	e.vehicle(tn)
	return tn
}

func (e *env) newProduct(tn *tenant) ref {
	ctx := context.Background()
	var p ref
	e.must(e.pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, 'EKO', 'Ekonomi Otomatik', $2, 'published') RETURNING id, uuid`, tn.id, tn.segment).Scan(&p.id, &p.uuid))
	var group int64
	e.must(e.pool.QueryRow(ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, 'GEKO', 'GEKO', $2) RETURNING id`,
		tn.id, tn.segment).Scan(&group))
	_, err := e.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, 21, 2)`, tn.id, p.id, group)
	e.must(err)
	var planID, versionID int64
	e.must(e.pool.QueryRow(ctx, `INSERT INTO rate_plans (organization_id, rental_product_id, scope, name) VALUES ($1, $2, 'common', 'Tarife') RETURNING id`,
		tn.id, p.id).Scan(&planID))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version) VALUES ($1, $2, 1) RETURNING id`,
		tn.id, planID).Scan(&versionID))
	_, err = e.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, 1, NULL, '1500')`,
		tn.id, versionID)
	e.must(err)
	_, err = e.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, tn.id, versionID)
	e.must(err)
	_, err = e.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, tn.id, planID)
	e.must(err)
	return p
}

func (e *env) vehicle(tn *tenant) {
	tn.plate++
	var id int64
	e.must(e.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id`,
		tn.id, fmt.Sprintf("34 PA%s %d", strings.ToUpper(tn.slug[2:3]), tn.plate), tn.branch.id).Scan(&id))
	_, err := e.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 ON CONFLICT DO NOTHING`, tn.id, id)
	e.must(err)
}

func (e *env) key(tn *tenant, kind, channel string, origins ...string) model.IssuedKey {
	e.t.Helper()
	name := kind + " " + channel
	in := model.KeyInput{Kind: &kind, Name: &name, Channel: &channel}
	if len(origins) > 0 {
		in.AllowedOrigins = &origins
	}
	k, err := e.svc.CreateKey(tn.ctx, tn.id, in)
	e.must(err)
	return k
}

// ---------------------------------------------------------------- HTTP

type resp struct {
	code   int
	raw    []byte
	header http.Header
	data   map[string]any
	list   []any
	err    map[string]any
}

type call struct {
	method, path, bearer, origin, idem, token string
	body                                      any
	raw                                       []byte
}

func (e *env) do(c call) resp {
	e.t.Helper()
	var buf io.Reader
	switch {
	case c.raw != nil:
		buf = bytes.NewReader(c.raw)
	case c.body != nil:
		b, _ := json.Marshal(c.body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(c.method, e.server.URL+c.path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	if c.idem != "" {
		req.Header.Set("Idempotency-Key", c.idem)
	}
	if c.token != "" {
		req.Header.Set("X-Reservation-Token", c.token)
	}
	res, err := http.DefaultClient.Do(req)
	e.must(err)
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	out := resp{code: res.StatusCode, raw: raw, header: res.Header}
	var envl struct {
		Data  any            `json:"data"`
		Error map[string]any `json:"error"`
	}
	if json.Unmarshal(raw, &envl) == nil {
		out.data, _ = envl.Data.(map[string]any)
		out.list, _ = envl.Data.([]any)
		out.err = envl.Error
	}
	return out
}

func (r resp) errCode() string {
	if r.err == nil {
		return ""
	}
	s, _ := r.err["code"].(string)
	return s
}

func (e *env) expect(r resp, code int, what string) {
	e.t.Helper()
	if r.code != code {
		e.t.Fatalf("%s: status %d, want %d: %s", what, r.code, code, r.raw)
	}
}

func (e *env) session(publishable, origin string) string {
	e.t.Helper()
	r := e.do(call{method: "POST", path: "/v1/public/sessions", origin: origin, body: map[string]any{"publishable_key": publishable}})
	e.expect(r, http.StatusCreated, "session")
	return r.data["token"].(string)
}

func (e *env) window(days int) (time.Time, time.Time) {
	from := e.now().Add(time.Duration(days*24) * time.Hour).Truncate(time.Hour)
	return from, from.Add(72 * time.Hour)
}

func availabilityPath(tn *tenant, from, to time.Time) string {
	return fmt.Sprintf("/v1/public/availability?pickup_branch_uuid=%s&starts_at=%s&ends_at=%s", tn.branch.uuid,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
}

func quoteBody(tn *tenant, from, to time.Time) map[string]any {
	return map[string]any{
		"subgroup_uuid": tn.product.uuid, "pickup_branch_uuid": tn.branch.uuid,
		"starts_at": from.UTC().Format(time.RFC3339), "ends_at": to.UTC().Format(time.RFC3339),
	}
}

func bookingBody(quote string, birth string) map[string]any {
	return map[string]any{
		"quote_uuid": quote,
		"customer": map[string]any{
			"type": "individual", "first_name": "Ayşe", "last_name": "Demir", "phone": "0532 111 22 33", "email": "ayse@example.com",
		},
		"drivers": []any{map[string]any{
			"same_as_customer": true, "birth_date": birth, "licence_number": "B123456", "licence_issued_on": "2015-04-01",
		}},
	}
}

var idemSeq int

func idem() string {
	idemSeq++
	return "test-" + strconv.Itoa(idemSeq) + "-" + uuid.NewString()
}

// ---------------------------------------------------------------- tests

func TestCredentials(t *testing.T) {
	e := newEnv(t)
	secret := e.key(e.a, model.KindSecret, "api")
	site := e.key(e.a, model.KindPublishable, "web", "https://www.rentacar.test")
	if !strings.HasPrefix(secret.Secret, model.PrefixSecret) || secret.Key.PublishableKey != nil {
		t.Fatalf("secret key: %+v", secret)
	}
	if site.Key.PublishableKey == nil || *site.Key.PublishableKey != site.Secret {
		t.Fatalf("publishable key: %+v", site.Key)
	}
	// Validation: a publishable key needs origins; a secret key has none.
	kind, name := model.KindPublishable, "x"
	if _, err := e.svc.CreateKey(e.a.ctx, e.a.id, model.KeyInput{Kind: &kind, Name: &name}); err == nil {
		t.Fatal("publishable key without origins accepted")
	}
	bad := []string{"http://evil.test", "https://a.test/path"}
	if _, err := e.svc.CreateKey(e.a.ctx, e.a.id, model.KeyInput{Kind: &kind, Name: &name, AllowedOrigins: &bad}); err == nil {
		t.Fatal("invalid origins accepted")
	}

	loc := "/v1/public/locations"
	e.expect(e.do(call{method: "GET", path: loc}), 401, "no bearer")
	e.expect(e.do(call{method: "GET", path: loc, bearer: "rtp_sk_nope"}), 401, "unknown key")
	r := e.do(call{method: "GET", path: loc, bearer: secret.Secret, origin: "https://www.rentacar.test"})
	if r.code != 403 || r.errCode() != handler.CodeSecretKeyInBrowser {
		t.Fatalf("secret key from a browser: %d %s", r.code, r.raw)
	}
	// A publishable key is not a bearer.
	e.expect(e.do(call{method: "GET", path: loc, bearer: site.Secret}), 401, "publishable key as bearer")
	// Sessions: only allowed origins; a session answers its own origin only.
	r = e.do(call{method: "POST", path: "/v1/public/sessions", origin: "https://evil.test", body: map[string]any{"publishable_key": site.Secret}})
	if r.code != 403 || r.errCode() != handler.CodeOriginNotAllowed {
		t.Fatalf("other origin: %d %s", r.code, r.raw)
	}
	r = e.do(call{method: "POST", path: "/v1/public/sessions", origin: "https://www.rentacar.test", body: map[string]any{"publishable_key": secret.Secret}})
	if r.code != 403 || r.errCode() != handler.CodeSecretKeyInBrowser {
		t.Fatalf("secret key minting a session: %d %s", r.code, r.raw)
	}
	tok := e.session(site.Secret, "https://www.rentacar.test")
	r = e.do(call{method: "GET", path: loc, bearer: tok, origin: "https://www.rentacar.test"})
	e.expect(r, 200, "session")
	if r.header.Get("Access-Control-Allow-Origin") != "https://www.rentacar.test" {
		t.Fatalf("cors header %q", r.header.Get("Access-Control-Allow-Origin"))
	}
	e.expect(e.do(call{method: "GET", path: loc, bearer: tok, origin: "https://evil.test"}), 403, "session from another origin")
	tampered := tok[:len(tok)-1] + "0"
	if tampered == tok {
		tampered = tok[:len(tok)-1] + "1"
	}
	e.expect(e.do(call{method: "GET", path: loc, bearer: tampered}), 401, "tampered session")
	// Preflight.
	req, _ := http.NewRequest("OPTIONS", e.server.URL+"/v1/public/quotes", nil)
	req.Header.Set("Origin", "https://www.rentacar.test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	pre, err := http.DefaultClient.Do(req)
	e.must(err)
	_ = pre.Body.Close()
	if pre.StatusCode != 204 || !strings.Contains(pre.Header.Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatalf("preflight %d %v", pre.StatusCode, pre.Header)
	}
	// Removing the origin ends its sessions.
	others := []string{"https://other.test"}
	_, err = e.svc.PatchKey(e.a.ctx, e.a.id, site.Key.UUID, model.KeyInput{AllowedOrigins: &others})
	e.must(err)
	e.expect(e.do(call{method: "GET", path: loc, bearer: tok, origin: "https://www.rentacar.test"}), 403, "origin removed")

	// The channel is the feature gate.
	v, err := e.settings.Current(e.a.ctx, e.a.id)
	e.must(err)
	for i := range v.Settings.SalesChannels {
		if v.Settings.SalesChannels[i].Code == "api" {
			v.Settings.SalesChannels[i].IsActive = false
		}
	}
	data, _ := json.Marshal(v.Settings)
	_, err = e.pool.Exec(context.Background(), `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 2, $2)`, e.a.id, data)
	e.must(err)
	r = e.do(call{method: "GET", path: loc, bearer: secret.Secret})
	if r.code != 403 || r.errCode() != handler.CodeChannelDisabled {
		t.Fatalf("disabled channel: %d %s", r.code, r.raw)
	}
	_, err = e.pool.Exec(context.Background(), `DELETE FROM organization_settings_versions WHERE organization_id = $1 AND version = 2`, e.a.id)
	e.must(err)
	e.expect(e.do(call{method: "GET", path: loc, bearer: secret.Secret}), 200, "channel back on")

	// Rotation: both keys work during the grace period, the old one stops
	// after it; revocation is immediate.
	grace := 1
	rot, err := e.svc.RotateKey(e.a.ctx, e.a.id, secret.Key.UUID, model.RotateInput{GraceHours: &grace})
	e.must(err)
	if rot.Key.RotatedFromUUID == nil || *rot.Key.RotatedFromUUID != secret.Key.UUID {
		t.Fatalf("rotated from %v", rot.Key.RotatedFromUUID)
	}
	e.expect(e.do(call{method: "GET", path: loc, bearer: secret.Secret}), 200, "old key in grace")
	e.expect(e.do(call{method: "GET", path: loc, bearer: rot.Secret}), 200, "new key")
	e.advance(61 * time.Minute)
	e.expect(e.do(call{method: "GET", path: loc, bearer: secret.Secret}), 401, "old key after grace")
	_, err = e.svc.RevokeKey(e.a.ctx, e.a.id, rot.Key.UUID)
	e.must(err)
	e.expect(e.do(call{method: "GET", path: loc, bearer: rot.Secret}), 401, "revoked key")
	if _, err := e.svc.PatchKey(e.a.ctx, e.a.id, rot.Key.UUID, model.KeyInput{Name: &name}); err == nil {
		t.Fatal("revoked key patched")
	}

	// Scopes.
	scopes := []string{model.ScopeCatalogRead}
	ro := e.key(e.a, model.KindSecret, "api")
	_, err = e.svc.PatchKey(e.a.ctx, e.a.id, ro.Key.UUID, model.KeyInput{Scopes: &scopes})
	e.must(err)
	from, to := e.window(3)
	r = e.do(call{method: "POST", path: "/v1/public/quotes", bearer: ro.Secret, idem: idem(), body: quoteBody(e.a, from, to)})
	if r.code != 403 || r.errCode() != handler.CodeScope {
		t.Fatalf("scope: %d %s", r.code, r.raw)
	}
	// Panel isolation: B never sees A's keys.
	if _, err := e.svc.GetKey(e.b.ctx, e.b.id, ro.Key.UUID); err == nil {
		t.Fatal("B read A's key")
	}
	list, total, err := e.svc.ListKeys(e.b.ctx, e.b.id, model.KeyFilter{Limit: 50})
	e.must(err)
	if total != 0 || len(list) != 0 {
		t.Fatalf("B lists %d keys", total)
	}
}

func TestCatalogAndSameAnswerEverywhere(t *testing.T) {
	e := newEnv(t)
	api := e.key(e.a, model.KindSecret, "api")
	site := e.key(e.a, model.KindPublishable, "web", "https://www.rentacar.test")
	widget := e.key(e.a, model.KindPublishable, "widget", "https://partner.test")
	siteTok := e.session(site.Secret, "https://www.rentacar.test")
	widgetTok := e.session(widget.Secret, "https://partner.test")

	r := e.do(call{method: "GET", path: "/v1/public/locations", bearer: api.Secret})
	e.expect(r, 200, "locations")
	if len(r.list) != 1 {
		t.Fatalf("locations: %s", r.raw)
	}
	loc := r.list[0].(map[string]any)
	points := loc["points"].([]any)
	if loc["uuid"] != e.a.branch.uuid.String() || len(points) != 1 || points[0].(map[string]any)["name"] != "Ofis" {
		t.Fatalf("location: %s", r.raw)
	}
	r = e.do(call{method: "GET", path: "/v1/public/vehicle-subgroups", bearer: api.Secret})
	e.expect(r, 200, "subgroups")
	sg := r.list[0].(map[string]any)
	cond := sg["conditions"].(map[string]any)
	if sg["uuid"] != e.a.product.uuid.String() || cond["min_driver_age"].(float64) != 21 {
		t.Fatalf("subgroup: %s", r.raw)
	}
	if strings.Contains(string(r.raw), e.b.product.uuid.String()) {
		t.Fatal("B's product leaked")
	}
	e.expect(e.do(call{method: "GET", path: "/v1/public/extras", bearer: api.Secret}), 200, "extras")

	// RT-A27: the same request answers the same price and capacity on the
	// API, the site and the widget, and matches the panel's quote search.
	from, to := e.window(5)
	path := availabilityPath(e.a, from, to)
	answers := []resp{
		e.do(call{method: "GET", path: path, bearer: api.Secret}),
		e.do(call{method: "GET", path: path, bearer: siteTok, origin: "https://www.rentacar.test"}),
		e.do(call{method: "GET", path: path, bearer: widgetTok, origin: "https://partner.test"}),
	}
	type item struct {
		Available bool `json:"available"`
		UnitsLeft int  `json:"units_left"`
		Price     *struct {
			Total      string `json:"total"`
			Prepayment string `json:"prepayment"`
		} `json:"price"`
	}
	var got []item
	for i, a := range answers {
		e.expect(a, 200, "availability "+strconv.Itoa(i))
		var body struct {
			Data struct {
				Items []item `json:"items"`
			} `json:"data"`
		}
		e.must(json.Unmarshal(a.raw, &body))
		if len(body.Data.Items) != 1 || body.Data.Items[0].Price == nil || !body.Data.Items[0].Available {
			t.Fatalf("availability %d: %s", i, a.raw)
		}
		got = append(got, body.Data.Items[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].UnitsLeft != got[0].UnitsLeft || got[i].Price.Total != got[0].Price.Total || got[i].Price.Prepayment != got[0].Price.Prepayment {
			t.Fatalf("answer %d differs: %+v vs %+v", i, got[i], got[0])
		}
	}
	panel, err := e.res.Search(e.a.ctx, e.a.id, resmodel.QuoteSearchInput{
		Where: resmodel.Where{PickupBranchUUID: &e.a.branch.uuid, StartsAt: &from, EndsAt: &to}, Channel: "api",
	})
	e.must(err)
	if panel.Items[0].Price.Total.String() != got[0].Price.Total || panel.Items[0].Sellable != got[0].UnitsLeft {
		t.Fatalf("panel %s / %d vs public %s / %d", panel.Items[0].Price.Total, panel.Items[0].Sellable, got[0].Price.Total, got[0].UnitsLeft)
	}
	// Past pick-up and malformed input.
	past := e.now().Add(-48 * time.Hour)
	e.expect(e.do(call{method: "GET", path: availabilityPath(e.a, past, past.Add(24*time.Hour)), bearer: api.Secret}), 400, "past")
	e.expect(e.do(call{method: "GET", path: "/v1/public/availability?pickup_branch_uuid=x", bearer: api.Secret}), 400, "bad uuid")
	// B's branch through A's key is refused.
	e.expect(e.do(call{method: "GET", path: availabilityPath(e.b, from, to), bearer: api.Secret}), 400, "B's branch")
}

// TestStorefrontPublishState: show_online = false (TEC-730) hides a branch,
// subgroup or extra from the storefront (publishable key / booking
// session: branded site, widget) only. Integrations with a secret key keep
// seeing and quoting them.
func TestStorefrontPublishState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	origin := "https://www.rentacar.test"
	site := e.key(e.a, model.KindPublishable, "web", origin)
	api := e.key(e.a, model.KindSecret, "web")
	tok := e.session(site.Secret, origin)
	var extra uuid.UUID
	e.must(e.pool.QueryRow(ctx, `INSERT INTO extras (organization_id, code, name, extra_type, charge_type, status, show_online)
		VALUES ($1, 'BBK', 'Bebek koltuğu', 'child_seat', 'per_day', 'published', FALSE) RETURNING uuid`, e.a.id).Scan(&extra))
	_, err := e.pool.Exec(ctx, `UPDATE branches SET show_online = FALSE WHERE organization_id = $1 AND id = $2`, e.a.id, e.a.branch.id)
	e.must(err)
	_, err = e.pool.Exec(ctx, `UPDATE rental_products SET show_online = FALSE WHERE organization_id = $1 AND id = $2`, e.a.id, e.a.product.id)
	e.must(err)
	from, to := e.window(5)
	has := func(r resp, id uuid.UUID) bool { return strings.Contains(string(r.raw), id.String()) }

	// Storefront: hidden everywhere.
	sf := func(method, path string, body any) resp {
		return e.do(call{method: method, path: path, bearer: tok, origin: origin, idem: idem(), body: body})
	}
	for path, id := range map[string]uuid.UUID{
		"/v1/public/locations": e.a.branch.uuid, "/v1/public/vehicle-subgroups": e.a.product.uuid, "/v1/public/extras": extra,
	} {
		if r := sf("GET", path, nil); r.code != 200 || has(r, id) {
			t.Errorf("storefront %s lists a hidden item: %d %s", path, r.code, r.raw)
		}
	}
	if r := sf("GET", availabilityPath(e.a, from, to), nil); r.code != 400 || !strings.Contains(string(r.raw), "pickup_branch_uuid") {
		t.Errorf("storefront availability at a hidden branch = %d %s", r.code, r.raw)
	}
	if r := sf("POST", "/v1/public/quotes", quoteBody(e.a, from, to)); r.code != 400 ||
		!strings.Contains(string(r.raw), "subgroup_uuid") || !strings.Contains(string(r.raw), "pickup_branch_uuid") {
		t.Errorf("storefront quote of hidden items = %d %s", r.code, r.raw)
	}
	withExtra := quoteBody(e.a, from, to)
	withExtra["extras"] = []any{map[string]any{"extra_uuid": extra, "quantity": 1}}
	if r := sf("POST", "/v1/public/quotes", withExtra); r.code != 400 || !strings.Contains(string(r.raw), "extras.0.extra_uuid") {
		t.Errorf("storefront quote with a hidden extra = %d %s", r.code, r.raw)
	}

	// Secret key (integration): the flag does not apply.
	sk := func(method, path string, body any) resp {
		return e.do(call{method: method, path: path, bearer: api.Secret, idem: idem(), body: body})
	}
	for path, id := range map[string]uuid.UUID{
		"/v1/public/locations": e.a.branch.uuid, "/v1/public/vehicle-subgroups": e.a.product.uuid, "/v1/public/extras": extra,
	} {
		if r := sk("GET", path, nil); r.code != 200 || !has(r, id) {
			t.Errorf("secret key %s misses a storefront-hidden item: %d %s", path, r.code, r.raw)
		}
	}
	if r := sk("GET", availabilityPath(e.a, from, to), nil); r.code != 200 || !has(r, e.a.product.uuid) {
		t.Errorf("secret key availability = %d %s", r.code, r.raw)
	}
	e.expect(sk("POST", "/v1/public/quotes", quoteBody(e.a, from, to)), 201, "secret key quote of storefront-hidden items")
}

func TestQuoteIdempotency(t *testing.T) {
	e := newEnv(t)
	api := e.key(e.a, model.KindSecret, "api")
	from, to := e.window(4)
	body := quoteBody(e.a, from, to)
	r := e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, body: body})
	if r.code != 400 || r.errCode() != handler.CodeIdempotencyRequired {
		t.Fatalf("no key: %d %s", r.code, r.raw)
	}
	key := idem()
	first := e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, idem: key, body: body})
	e.expect(first, 201, "quote")
	again := e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, idem: key, body: body})
	e.expect(again, 201, "replay")
	if again.header.Get(handler.HeaderReplayed) != "true" || again.data["uuid"] != first.data["uuid"] {
		t.Fatalf("replay: %s vs %s", again.raw, first.raw)
	}
	var count int
	e.must(e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM public_quotes WHERE organization_id = $1`, e.a.id).Scan(&count))
	if count != 1 {
		t.Fatalf("%d quotes stored", count)
	}
	other := quoteBody(e.a, from.Add(time.Hour), to)
	r = e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, idem: key, body: other})
	if r.code != 422 || r.errCode() != handler.CodeIdempotencyKeyReused {
		t.Fatalf("reused key: %d %s", r.code, r.raw)
	}
	// The same key from another key is another request.
	api2 := e.key(e.a, model.KindSecret, "api")
	e.expect(e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api2.Secret, idem: key, body: body}), 201, "other key")
	// Read back; B cannot.
	id := first.data["uuid"].(string)
	e.expect(e.do(call{method: "GET", path: "/v1/public/quotes/" + id, bearer: api.Secret}), 200, "get quote")
	bkey := e.key(e.b, model.KindSecret, "api")
	e.expect(e.do(call{method: "GET", path: "/v1/public/quotes/" + id, bearer: bkey.Secret}), 404, "B reads A's quote")
	if first.data["total"] == nil || len(first.data["lines"].([]any)) == 0 {
		t.Fatalf("quote view: %s", first.raw)
	}
	if strings.Contains(string(first.raw), "rate_plan") || strings.Contains(string(first.raw), "created_by") {
		t.Fatalf("internal fields leaked: %s", first.raw)
	}
	// Past pick-up.
	past := e.now().Add(-time.Hour)
	e.expect(e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, idem: idem(), body: quoteBody(e.a, past, past.Add(48*time.Hour))}), 400, "past")
}

func TestBookingFlow(t *testing.T) {
	e := newEnv(t)
	site := e.key(e.a, model.KindPublishable, "web", "https://www.rentacar.test")
	api := e.key(e.a, model.KindSecret, "api")
	tok := e.session(site.Secret, "https://www.rentacar.test")
	origin := "https://www.rentacar.test"
	from, to := e.window(6)

	q := e.do(call{method: "POST", path: "/v1/public/quotes", bearer: tok, origin: origin, idem: idem(), body: quoteBody(e.a, from, to)})
	e.expect(q, 201, "quote")
	quote := q.data["uuid"].(string)

	// A quote of another channel cannot be booked here.
	apiQuote := e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, idem: idem(), body: quoteBody(e.a, from, to)})
	e.expect(apiQuote, 201, "api quote")
	r := e.do(call{method: "POST", path: "/v1/public/reservations", bearer: tok, origin: origin, idem: idem(),
		body: bookingBody(apiQuote.data["uuid"].(string), "1990-05-05")})
	e.expect(r, 400, "other channel's quote")

	// Too young for the product (min age 21): refused, and the customer
	// card is removed again.
	var before int
	e.must(e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM customers WHERE organization_id = $1`, e.a.id).Scan(&before))
	young := e.now().AddDate(-19, 0, 0).Format("2006-01-02")
	r = e.do(call{method: "POST", path: "/v1/public/reservations", bearer: tok, origin: origin, idem: idem(), body: bookingBody(quote, young)})
	if r.code != 422 || r.errCode() != handler.CodeDriverNotEligible {
		t.Fatalf("young driver: %d %s", r.code, r.raw)
	}
	var after int
	e.must(e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM customers WHERE organization_id = $1`, e.a.id).Scan(&after))
	if after != before {
		t.Fatalf("customers %d → %d after a refused booking", before, after)
	}
	// A browser cannot set the external reference.
	body := bookingBody(quote, "1990-05-05")
	body["external_reference"] = "X-1"
	e.expect(e.do(call{method: "POST", path: "/v1/public/reservations", bearer: tok, origin: origin, idem: idem(), body: body}), 400, "external ref")

	key := idem()
	booked := e.do(call{method: "POST", path: "/v1/public/reservations", bearer: tok, origin: origin, idem: key, body: bookingBody(quote, "1990-05-05")})
	e.expect(booked, 201, "booking")
	reference := booked.data["reference"].(string)
	access := booked.data["access_token"].(string)
	if reference == "" || access == "" || booked.data["status"] != resmodel.StatusConfirmed {
		t.Fatalf("booked: %s", booked.raw)
	}
	resv := booked.data["reservation"].(map[string]any)
	if resv["reference"] != reference || resv["price"].(map[string]any)["total"] == nil {
		t.Fatalf("reservation view: %s", booked.raw)
	}
	if strings.Contains(string(booked.raw), "download_path") || strings.Contains(string(booked.raw), "documents") {
		t.Fatalf("portal-only fields leaked: %s", booked.raw)
	}
	// A retry with the same key answers the same booking (no second one).
	replay := e.do(call{method: "POST", path: "/v1/public/reservations", bearer: tok, origin: origin, idem: key, body: bookingBody(quote, "1990-05-05")})
	e.expect(replay, 201, "booking replay")
	if replay.data["reference"] != reference || replay.data["access_token"] != access {
		t.Fatalf("replay differs: %s", replay.raw)
	}
	var n int
	e.must(e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM reservations WHERE organization_id = $1`, e.a.id).Scan(&n))
	if n != 1 {
		t.Fatalf("%d reservations", n)
	}
	// The quote is used now.
	r = e.do(call{method: "POST", path: "/v1/public/reservations", bearer: tok, origin: origin, idem: idem(), body: bookingBody(quote, "1990-05-05")})
	if r.code != 409 {
		t.Fatalf("used quote: %d %s", r.code, r.raw)
	}
	// The reservation carries the channel.
	var channel string
	e.must(e.pool.QueryRow(context.Background(), `SELECT channel FROM reservations WHERE organization_id = $1 AND reference = $2`, e.a.id, reference).Scan(&channel))
	if channel != "web" {
		t.Fatalf("channel %s", channel)
	}

	// Status: a session needs the access token; a wrong one reads as not
	// found; the creating key needs none; another key does.
	path := "/v1/public/reservations/" + reference
	e.expect(e.do(call{method: "GET", path: path, bearer: tok, origin: origin}), 404, "no token")
	e.expect(e.do(call{method: "GET", path: path, bearer: tok, origin: origin, token: "wrong"}), 404, "wrong token")
	st := e.do(call{method: "GET", path: path, bearer: tok, origin: origin, token: access})
	e.expect(st, 200, "status")
	if st.data["status"] != resmodel.StatusConfirmed || st.data["actions"].(map[string]any)["request_cancellation"] != true {
		t.Fatalf("status: %s", st.raw)
	}
	e.expect(e.do(call{method: "GET", path: path, bearer: api.Secret}), 404, "other key without token")
	e.expect(e.do(call{method: "GET", path: path, bearer: api.Secret, token: access}), 200, "other key with token")
	bkey := e.key(e.b, model.KindSecret, "api")
	e.expect(e.do(call{method: "GET", path: path, bearer: bkey.Secret, token: access}), 404, "B with A's token")
	e.expect(e.do(call{method: "GET", path: path + "X", bearer: tok, origin: origin, token: access}), 404, "unknown reference")

	// Payment link of the balance (A has an iyzico account), reused.
	pay := e.do(call{method: "POST", path: path + "/payment-links", bearer: tok, origin: origin, token: access, idem: idem()})
	e.expect(pay, 201, "payment link")
	if !strings.HasPrefix(pay.data["url"].(string), "https://app.test/pay/") {
		t.Fatalf("pay link: %s", pay.raw)
	}
	pay2 := e.do(call{method: "POST", path: path + "/payment-links", bearer: tok, origin: origin, token: access, idem: idem()})
	e.expect(pay2, 201, "payment link again")
	if pay2.data["url"] != pay.data["url"] || pay2.data["reused"] != true {
		t.Fatalf("pay link not reused: %s", pay2.raw)
	}
	e.expect(e.do(call{method: "POST", path: path + "/payment-links", bearer: tok, origin: origin, idem: idem()}), 404, "pay without token")

	// Requests.
	ch := e.do(call{method: "POST", path: path + "/change-requests", bearer: tok, origin: origin, token: access, idem: idem(),
		body: map[string]any{"message": "Bir gün uzatmak istiyorum"}})
	e.expect(ch, 201, "change request")
	r = e.do(call{method: "POST", path: path + "/change-requests", bearer: tok, origin: origin, token: access, idem: idem(),
		body: map[string]any{"message": "Tekrar"}})
	if r.code != 409 || r.errCode() != handler.CodeRequestExists {
		t.Fatalf("duplicate request: %d %s", r.code, r.raw)
	}
	cancel := e.do(call{method: "POST", path: path + "/cancellation-requests", bearer: tok, origin: origin, token: access, idem: idem(),
		body: map[string]any{"message": "Planım değişti"}})
	e.expect(cancel, 201, "cancellation request")
	if cancel.data["policy_text"] != "Teslimden 24 saat öncesine kadar ücretsiz iptal." || cancel.data["status"] != "open" {
		t.Fatalf("cancellation: %s", cancel.raw)
	}
	var reqs int
	var contact string
	e.must(e.pool.QueryRow(context.Background(), `SELECT COUNT(*), MAX(contact_masked) FROM customer_requests WHERE organization_id = $1`, e.a.id).Scan(&reqs, &contact))
	if reqs != 2 || contact != "Web sitesi" {
		t.Fatalf("requests %d contact %q", reqs, contact)
	}
	st = e.do(call{method: "GET", path: path, bearer: tok, origin: origin, token: access})
	if len(st.data["requests"].([]any)) != 2 {
		t.Fatalf("requests in status: %s", st.raw)
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t)
	api := e.key(e.a, model.KindSecret, "api")
	limit := 10
	_, err := e.svc.PatchKey(e.a.ctx, e.a.id, api.Key.UUID, model.KeyInput{RateLimitPerMinute: &limit})
	e.must(err)
	for i := 0; i < limit; i++ {
		e.expect(e.do(call{method: "GET", path: "/v1/public/extras", bearer: api.Secret}), 200, "within the limit")
	}
	r := e.do(call{method: "GET", path: "/v1/public/extras", bearer: api.Secret})
	if r.code != 429 || r.header.Get("Retry-After") == "" {
		t.Fatalf("over the limit: %d %v", r.code, r.header)
	}
	// Another key has its own window.
	other := e.key(e.a, model.KindSecret, "api")
	e.expect(e.do(call{method: "GET", path: "/v1/public/extras", bearer: other.Secret}), 200, "other key")
	e.redis.FastForward(time.Minute + time.Second)
	e.expect(e.do(call{method: "GET", path: "/v1/public/extras", bearer: api.Secret}), 200, "next window")
}

type hook struct {
	mu     sync.Mutex
	status int
	got    []http.Header
	bodies [][]byte
}

func (h *hook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	h.got = append(h.got, r.Header.Clone())
	h.bodies = append(h.bodies, b)
	status := h.status
	h.mu.Unlock()
	w.WriteHeader(status)
}

func (h *hook) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.got)
}

func TestWebhooks(t *testing.T) {
	e := newEnv(t)
	ok := &hook{status: 200}
	okSrv := httptest.NewServer(ok)
	t.Cleanup(okSrv.Close)
	bad := &hook{status: 500}
	badSrv := httptest.NewServer(bad)
	t.Cleanup(badSrv.Close)

	name, url := "ERP", okSrv.URL+"/hooks"
	created, err := e.svc.CreateEndpoint(e.a.ctx, e.a.id, model.EndpointInput{Name: &name, URL: &url})
	e.must(err)
	if !strings.HasPrefix(created.Secret, "whsec_") || len(created.Endpoint.Events) != len(model.WebhookEvents) {
		t.Fatalf("endpoint: %+v", created)
	}
	badName, badURL := "Down", badSrv.URL
	events := []string{model.EventReservationCreated}
	down, err := e.svc.CreateEndpoint(e.a.ctx, e.a.id, model.EndpointInput{Name: &badName, URL: &badURL, Events: &events})
	e.must(err)
	// Ping now.
	ping, err := e.svc.TestEndpoint(e.a.ctx, e.a.id, created.Endpoint.UUID)
	e.must(err)
	if ping.Status != model.DeliverySucceeded || ok.count() != 1 {
		t.Fatalf("ping %+v (%d received)", ping, ok.count())
	}
	// B sees none of it.
	if _, err := e.svc.GetEndpoint(e.b.ctx, e.b.id, created.Endpoint.UUID); err == nil {
		t.Fatal("B read A's endpoint")
	}
	if _, err := e.svc.GetDelivery(e.b.ctx, e.b.id, ping.UUID); err == nil {
		t.Fatal("B read A's delivery")
	}

	// A booking through the API queues reservation.created for both.
	api := e.key(e.a, model.KindSecret, "api")
	from, to := e.window(8)
	q := e.do(call{method: "POST", path: "/v1/public/quotes", bearer: api.Secret, idem: idem(), body: quoteBody(e.a, from, to)})
	e.expect(q, 201, "quote")
	body := bookingBody(q.data["uuid"].(string), "1988-01-01")
	body["external_reference"] = "ERP-77"
	booked := e.do(call{method: "POST", path: "/v1/public/reservations", bearer: api.Secret, idem: idem(), body: body})
	e.expect(booked, 201, "booking")
	n, err := e.svc.DeliverDue(context.Background())
	e.must(err)
	if n != 2 {
		t.Fatalf("%d attempts", n)
	}
	if ok.count() != 2 {
		t.Fatalf("receiver got %d", ok.count())
	}
	ok.mu.Lock()
	hdr, raw := ok.got[1], ok.bodies[1]
	ok.mu.Unlock()
	stamp, _ := strconv.ParseInt(hdr.Get(usecase.HeaderWebhookTimestamp), 10, 64)
	if hdr.Get(usecase.HeaderWebhookSignature) != usecase.SignWebhook(created.Secret, stamp, raw) {
		t.Fatal("signature does not verify")
	}
	var ev struct {
		Type string `json:"type"`
		Data struct {
			Reservation struct {
				Reference string `json:"reference"`
				Status    string `json:"status"`
				Channel   string `json:"channel"`
				External  string `json:"external_reference"`
			} `json:"reservation"`
		} `json:"data"`
	}
	e.must(json.Unmarshal(raw, &ev))
	if ev.Type != model.EventReservationCreated || ev.Data.Reservation.Reference != booked.data["reference"] ||
		ev.Data.Reservation.Channel != "api" || ev.Data.Reservation.External != "ERP-77" {
		t.Fatalf("event: %s", raw)
	}
	if strings.Contains(string(raw), "ayse@example.com") || strings.Contains(string(raw), "Demir") {
		t.Fatalf("personal data in the webhook: %s", raw)
	}
	// The failing endpoint retries with backoff, then fails.
	list, _, err := e.svc.ListDeliveries(e.a.ctx, e.a.id, model.DeliveryFilter{EndpointUUIDs: []uuid.UUID{down.Endpoint.UUID}, Limit: 10})
	e.must(err)
	if len(list) != 1 || list[0].Status != model.DeliveryPending || list[0].Attempts != 1 || *list[0].LastStatusCode != 500 {
		t.Fatalf("failing delivery: %+v", list)
	}
	n, err = e.svc.DeliverDue(context.Background())
	e.must(err)
	if n != 0 {
		t.Fatalf("retried before the backoff: %d", n)
	}
	for i := 0; i < 2; i++ {
		e.advance(6 * time.Minute)
		_, err = e.svc.DeliverDue(context.Background())
		e.must(err)
	}
	failed, err := e.svc.GetDelivery(e.a.ctx, e.a.id, list[0].UUID)
	e.must(err)
	if failed.Status != model.DeliveryFailed || failed.Attempts != 3 || bad.count() != 3 {
		t.Fatalf("after max attempts: %+v (%d received)", failed, bad.count())
	}
	// The receiver recovers; a manual retry delivers.
	bad.mu.Lock()
	bad.status = 204
	bad.mu.Unlock()
	retried, err := e.svc.RetryDelivery(e.a.ctx, e.a.id, failed.UUID)
	e.must(err)
	if retried.Status != model.DeliverySucceeded {
		t.Fatalf("retry: %+v", retried)
	}
	ep, err := e.svc.GetEndpoint(e.a.ctx, e.a.id, down.Endpoint.UUID)
	e.must(err)
	if ep.ConsecutiveFailures != 0 || ep.LastSuccessAt == nil {
		t.Fatalf("endpoint health: %+v", ep)
	}

	// A lifecycle change of the reservation (staff cancel) is an event too;
	// panel reservations are never sent.
	r, err := e.res.Get(e.a.ctx, e.a.id, uuid.MustParse(mustRes(e, booked.data["reference"].(string))))
	e.must(err)
	_, err = e.res.Cancel(e.a.ctx, e.a.id, r.UUID, "Müşteri aradı")
	e.must(err)
	_, err = e.svc.DeliverDue(context.Background())
	e.must(err)
	ok.mu.Lock()
	last := ok.bodies[len(ok.bodies)-1]
	ok.mu.Unlock()
	var cancelled struct {
		Type string `json:"type"`
		Data struct {
			Reservation struct {
				Status string `json:"status"`
			} `json:"reservation"`
		} `json:"data"`
	}
	e.must(json.Unmarshal(last, &cancelled))
	if cancelled.Type != model.EventReservationCancelled || cancelled.Data.Reservation.Status != resmodel.StatusCancelled {
		t.Fatalf("cancel event: %s", last)
	}
	// Secret rotation: the new secret signs.
	rot, err := e.svc.RotateEndpointSecret(e.a.ctx, e.a.id, created.Endpoint.UUID)
	e.must(err)
	ping, err = e.svc.TestEndpoint(e.a.ctx, e.a.id, created.Endpoint.UUID)
	e.must(err)
	ok.mu.Lock()
	hdr, raw = ok.got[len(ok.got)-1], ok.bodies[len(ok.bodies)-1]
	ok.mu.Unlock()
	stamp, _ = strconv.ParseInt(hdr.Get(usecase.HeaderWebhookTimestamp), 10, 64)
	if ping.Status != model.DeliverySucceeded || hdr.Get(usecase.HeaderWebhookSignature) != usecase.SignWebhook(rot.Secret, stamp, raw) {
		t.Fatal("rotated secret does not sign")
	}
	// Disabled endpoints get nothing; deleting removes the deliveries.
	disabled := model.EndpointDisabled
	_, err = e.svc.PatchEndpoint(e.a.ctx, e.a.id, created.Endpoint.UUID, model.EndpointInput{Status: &disabled})
	e.must(err)
	e.must(e.svc.DeleteEndpoint(e.a.ctx, e.a.id, down.Endpoint.UUID))
	if _, err := e.svc.GetDelivery(e.a.ctx, e.a.id, failed.UUID); err == nil {
		t.Fatal("delivery survived its endpoint")
	}
	// URL rules in production mode.
	e.svc.AllowPrivateWebhooks(false)
	for _, u := range []string{"http://example.com/h", "https://127.0.0.1/h", "https://10.0.0.5/h", "https://localhost/h", "ftp://x.test"} {
		u := u
		if _, err := e.svc.CreateEndpoint(e.a.ctx, e.a.id, model.EndpointInput{Name: &name, URL: &u}); err == nil {
			t.Fatalf("%s accepted", u)
		}
	}
}

func mustRes(e *env, reference string) string {
	var id uuid.UUID
	e.must(e.pool.QueryRow(context.Background(), `SELECT uuid FROM reservations WHERE organization_id = $1 AND reference = $2`, e.a.id, reference).Scan(&id))
	return id.String()
}
