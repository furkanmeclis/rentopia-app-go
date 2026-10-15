package customerportal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm/netgsmfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Customer portal (TEC-715) against a real database: two organizations
// (A and B), each with the same kind of data. In A, customer X has two
// reservations (a driver D is on the first only) and customer Y one. SMS
// codes go to the Netgsm fake, e-mail codes to a capturing mailer; payments
// use the iyzico fake. Codes are read from what the fakes received.

type ref struct {
	id   int64
	uuid uuid.UUID
}

type capMail struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (c *capMail) Send(_ context.Context, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

func (c *capMail) last(to string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if c.msgs[i].To[0] == to {
			return c.msgs[i].Body
		}
	}
	return ""
}

type fakePDF struct{}

func (fakePDF) RenderPDF(_ context.Context, html []byte) ([]byte, error) {
	return append([]byte("%PDF-1.4\n"), html[:16]...), nil
}

type tenant struct {
	id      int64
	uuid    uuid.UUID
	slug    string
	ctx     context.Context // staff caller with every permission
	segment int64
	branch  ref
	product ref
	plate   int
}

type env struct {
	t        *testing.T
	pool     *pgxpool.Pool
	q        *db.Queries
	res      *resusecase.Service
	docs     *contractsusecase.Service
	svc      *usecase.Service
	sms      *netgsmfake.Fake
	mailer   *capMail
	server   *httptest.Server
	suffix   string
	clockMu  sync.Mutex
	shift    time.Duration
	a, b     *tenant
	settings *orgsettingsusecase.Service
}

var perms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantContractsRead, rbac.PermTenantContractsWrite, rbac.PermTenantContractsActivate,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantDocumentDownload,
	rbac.PermTenantPaymentsRead, rbac.PermTenantPaymentsCreateLink, rbac.PermTenantIntegrationsManage,
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

// stubResolver answers every staff organization lookup with nothing (the
// staff routes are exercised through the use case here; RT-A01 covers
// them over the real server).
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
	e.docs = contractsusecase.New(pool, q, e.settings, storage.NewMemory())
	e.docs.SetRenderer(fakePDF{})
	e.docs.SetClock(e.now)

	fc, err := crypto.NewFieldCipher(strings.Repeat("p", 32))
	e.must(err)
	vault, err := integrations.NewVault(fc)
	e.must(err)
	e.sms = netgsmfake.New()
	t.Cleanup(e.sms.Server.Close)
	smsAdapter := netgsm.New(netgsm.Config{SandboxBaseURL: e.sms.URL()})
	pay := iyzicofake.New()
	t.Cleanup(pay.Server.Close)
	pay.AddAccount("sandbox-api-key-0001", "sandbox-secret-key-0001")
	payAdapter := iyzico.New(iyzico.Config{SandboxBaseURL: pay.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(smsAdapter, payAdapter))
	ops := integrations.NewOperationRegistry()
	runner := jobsusecase.NewRunner(q, accounts, ops)
	smsSvc, err := smsusecase.New(pool, q, fc, accounts, runner, ops, smsAdapter)
	e.must(err)
	smsSvc.SetClock(e.now)
	rec := activity.NewRecorder(q, nil)
	smsSvc.SetActivity(rec)
	e.mailer = &capMail{}
	smsSvc.SetMailer(e.mailer)
	e.must(e.docs.SetRemote(contractsusecase.Remote{SMS: smsSvc, Cipher: fc, PublicBaseURL: "https://app.test", Activity: rec}))
	payments := paymentsusecase.New(pool, q, accounts, e.res, paymentsusecase.Config{PublicBaseURL: "https://app.test"}, payAdapter)
	payments.SetClock(e.now)

	e.svc = usecase.New(pool, q, smsSvc, e.res)
	e.svc.SetPayments(payments, accounts)
	e.svc.SetDocuments(e.docs)
	e.svc.SetActivity(rec)
	e.svc.SetClock(e.now)

	mux := http.NewServeMux()
	customerportal.RegisterRoutes(mux, handler.New(e.svc, rec), nil, nil, stubResolver{})
	e.server = httptest.NewServer(mux)
	t.Cleanup(e.server.Close)

	e.a = e.newTenant("a", accounts, true)
	e.b = e.newTenant("b", accounts, false)
	return e
}

func (e *env) newTenant(name string, accounts *accountsusecase.Service, payments bool) *tenant {
	ctx := context.Background()
	slug := "cp" + name + "-" + e.suffix
	org, err := e.q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: slug, Name: "Portal " + strings.ToUpper(name) + " " + e.suffix})
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
		OrganizationID: org.ID, Code: "MRK", Name: "Merkez Şube", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
		PrepMinutes: 30, IsActive: true,
	})
	e.must(err)
	tn.branch = ref{b.ID, b.Uuid}
	tn.product = e.newProduct(tn)
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	sixty, thirty := 60, 30
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = &sixty, &sixty, &thirty
	s.Policies.Cancellation = "Teslimden 24 saat öncesine kadar ücretsiz iptal."
	s.Company.LegalName = "Portal Oto A.Ş."
	data, _ := json.Marshal(s)
	_, err = e.pool.Exec(ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, org.ID, data)
	e.must(err)
	_, err = accounts.Connect(tn.ctx, org.ID, accountsusecase.ConnectInput{
		Provider: "netgsm", DisplayName: "Netgsm", Mode: "sandbox",
		Credentials: map[string]string{"usercode": "850" + name + e.suffix, "password": "pw", "msgheader": "RENTOPIA"},
	})
	e.must(err)
	e.sms.AddAccount("850"+name+e.suffix, "pw", "RENTOPIA")
	if payments {
		_, err = accounts.Connect(tn.ctx, org.ID, accountsusecase.ConnectInput{
			Provider: iyzico.ProviderCode, DisplayName: "iyzico", Mode: integrations.ModeSandbox,
			Credentials: map[string]string{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "sandbox-secret-key-0001"},
		})
		e.must(err)
	}
	// An active terms template: every reservation can get a document.
	title, body := "Kiralama koşulları", "<p>Kiracı: {{renter.full_name}} — {{reservation.reference}}</p>"
	tpl, err := e.docs.CreateTemplate(tn.ctx, org.ID, contractsmodel.TemplateInput{Kind: contractsmodel.KindTerms, Name: &title, BodyHTML: &body})
	e.must(err)
	_, err = e.docs.ActivateTemplate(tn.ctx, org.ID, tpl.UUID)
	e.must(err)
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
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, tn.id, p.id, group)
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
		tn.id, fmt.Sprintf("34 CP %d", tn.plate), tn.branch.id).Scan(&id))
	_, err := e.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 ON CONFLICT DO NOTHING`, tn.id, id)
	e.must(err)
}

// person is a customer (or a driver of one) with contact values.
type person struct {
	customer ref
	driver   ref
	phone    string
	email    string
}

func (e *env) customer(tn *tenant, first, phone, email string) person {
	ctx := context.Background()
	var c, d ref
	e.must(e.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', $2, 'Yılmaz', $3, $4) RETURNING id, uuid`, tn.id, first, phone, email).Scan(&c.id, &c.uuid))
	d = e.driver(tn, c, first, "")
	return person{customer: c, driver: d, phone: phone, email: email}
}

func (e *env) driver(tn *tenant, c ref, first, phone string) ref {
	var d ref
	now := time.Now().UTC()
	e.must(e.pool.QueryRow(context.Background(), `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, phone, birth_date,
		licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, $3, 'Sürücü', $4, $5, 'v1:x', $6, '****456', $7) RETURNING id, uuid`,
		tn.id, c.id, first, phone, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return d
}

// reserve creates a confirmed reservation of the customer with the
// drivers, and prepares its terms document.
func (e *env) reserve(tn *tenant, p person, days int, drivers ...ref) resmodel.Reservation {
	e.t.Helper()
	e.vehicle(tn)
	from := e.now().Add(time.Duration(days*24) * time.Hour).Truncate(time.Hour)
	to := from.Add(72 * time.Hour)
	b := tn.branch.uuid
	quote, err := e.res.CreateQuote(tn.ctx, tn.id, resmodel.QuoteInput{
		Where: resmodel.Where{PickupBranchUUID: &b, StartsAt: &from, EndsAt: &to}, Channel: "branch", ProductUUID: &tn.product.uuid,
	})
	e.must(err)
	in := []resmodel.DriverInput{{DriverUUID: p.driver.uuid}}
	for _, d := range drivers {
		in = append(in, resmodel.DriverInput{DriverUUID: d.uuid})
	}
	r, err := e.res.Create(tn.ctx, tn.id, resmodel.CreateInput{QuoteUUID: &quote.UUID, CustomerUUID: &p.customer.uuid, Drivers: in})
	e.must(err)
	if r.Status != resmodel.StatusConfirmed {
		e.t.Fatalf("reservation status %s", r.Status)
	}
	_, err = e.docs.PrepareDocument(tn.ctx, tn.id, contractsmodel.PrepareInput{Kind: contractsmodel.KindTerms, ReservationUUID: &r.UUID})
	e.must(err)
	return r
}

// ---------------------------------------------------------------- HTTP

type resp struct {
	code int
	raw  []byte
	data map[string]any
	err  map[string]any
}

func (e *env) call(method, path, token string, body any) resp {
	e.t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.server.URL+path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	if token != "" {
		req.Header.Set(handler.SessionHeader, token)
	}
	res, err := http.DefaultClient.Do(req)
	e.must(err)
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	out := resp{code: res.StatusCode, raw: raw}
	var env struct {
		Data  any            `json:"data"`
		Error map[string]any `json:"error"`
	}
	if json.Unmarshal(raw, &env) == nil {
		out.data, _ = env.Data.(map[string]any)
		out.err = env.Error
	}
	return out
}

func (r resp) code_() string {
	if r.err == nil {
		return ""
	}
	s, _ := r.err["code"].(string)
	return s
}

var smsCodeRE = regexp.MustCompile(`kodunuz: (\d{6})`)

// smsCode is the newest code the fake delivered to the number (national
// form, e.g. 5321112233); "" when none.
func (e *env) smsCode(national string) string {
	msgs := e.sms.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].No != national {
			continue
		}
		if m := smsCodeRE.FindStringSubmatch(msgs[i].Text); m != nil {
			return m[1]
		}
	}
	return ""
}

func (e *env) smsCount(national string) int {
	n := 0
	for _, m := range e.sms.Messages() {
		if m.No == national {
			n++
		}
	}
	return n
}

// signIn runs start + verify with the SMS code and returns the token.
func (e *env) signIn(tn *tenant, reference, contact, national string) string {
	e.t.Helper()
	base := "/v1/customer/" + tn.slug
	if r := e.call(http.MethodPost, base+"/auth/start", "", map[string]any{"reference": reference, "contact": contact}); r.code != http.StatusAccepted {
		e.t.Fatalf("start = %d: %s", r.code, r.raw)
	}
	code := e.smsCode(national)
	if code == "" {
		e.t.Fatalf("no code reached %s", national)
	}
	r := e.call(http.MethodPost, base+"/auth/verify", "", map[string]any{"reference": reference, "contact": contact, "code": code})
	if r.code != http.StatusOK {
		e.t.Fatalf("verify = %d: %s", r.code, r.raw)
	}
	token, _ := r.data["session_token"].(string)
	if len(token) != 43 {
		e.t.Fatalf("token %q", token)
	}
	return token
}

func items(r resp) []any {
	v, _ := r.data["items"].([]any)
	return v
}

func uuidsOf(list []any) []string {
	out := []string{}
	for _, it := range list {
		out = append(out, it.(map[string]any)["uuid"].(string))
	}
	return out
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- tests

// TestPortalHappyPath: sign in with the renter's phone, list and view the
// reservations, download the document, pay remaining (link created, then
// reused), send a cancellation request (policy text from the snapshot),
// staff see and resolve it, sign out.
func TestPortalHappyPath(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "ayse@example.test")
	y := e.customer(e.a, "Mehmet", "+905329998877", "mehmet@example.test")
	r1 := e.reserve(e.a, x, 3)
	r2 := e.reserve(e.a, x, 10)
	ry := e.reserve(e.a, y, 5)
	base := "/v1/customer/" + e.a.slug

	info := e.call(http.MethodGet, base, "", nil)
	if info.code != http.StatusOK || info.data["channels"].(map[string]any)["sms"] != true {
		t.Fatalf("info %d %s", info.code, info.raw)
	}
	// Lower-case reference with spaces and a national number still match.
	token := e.signIn(e.a, " "+strings.ToLower(r1.Reference)+" ", "0532 111 22 33", "5321112233")

	sess := e.call(http.MethodGet, base+"/session", token, nil)
	if sess.code != http.StatusOK || sess.data["role"] != "renter" || sess.data["display_name"] != "Ayşe Y." {
		t.Fatalf("session %s", sess.raw)
	}
	list := uuidsOf(items(e.call(http.MethodGet, base+"/reservations", token, nil)))
	if len(list) != 2 || !has(list, r1.UUID.String()) || !has(list, r2.UUID.String()) || has(list, ry.UUID.String()) {
		t.Fatalf("reservations %v", list)
	}
	detail := e.call(http.MethodGet, base+"/reservations/"+r1.UUID.String(), token, nil)
	if detail.code != http.StatusOK {
		t.Fatalf("detail %d %s", detail.code, detail.raw)
	}
	if detail.data["reference"] != r1.Reference || detail.data["product"] != "Ekonomi Otomatik" ||
		detail.data["cancellation"].(map[string]any)["policy_text"] != "Teslimden 24 saat öncesine kadar ücretsiz iptal." {
		t.Fatalf("detail %s", detail.raw)
	}
	actions := detail.data["actions"].(map[string]any)
	if actions["pay"] != true || actions["pay_amount"] != "4500.00" || actions["request_cancel"] != true {
		t.Fatalf("actions %v", actions)
	}
	docs := detail.data["documents"].([]any)
	if len(docs) != 1 {
		t.Fatalf("documents %v", docs)
	}
	dl := docs[0].(map[string]any)["download_path"].(string)
	if strings.Contains(string(detail.raw), "organizations/") {
		t.Fatalf("a storage key reached the customer: %s", detail.raw)
	}
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/v1/"+dl, nil)
	req.Header.Set(handler.SessionHeader, token)
	pdf, err := http.DefaultClient.Do(req)
	e.must(err)
	raw, _ := io.ReadAll(pdf.Body)
	_ = pdf.Body.Close()
	if pdf.StatusCode != http.StatusOK || !bytes.HasPrefix(raw, []byte("%PDF")) || pdf.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf %d %q", pdf.StatusCode, raw)
	}

	// Pay remaining: a pay page link for the balance, reused on the second
	// click.
	pay := e.call(http.MethodPost, base+"/reservations/"+r1.UUID.String()+"/pay", token, nil)
	if pay.code != http.StatusOK || !strings.HasPrefix(pay.data["url"].(string), "https://app.test/pay/") || pay.data["amount"] != "4500.00" ||
		pay.data["reused"] != false {
		t.Fatalf("pay %d %s", pay.code, pay.raw)
	}
	again := e.call(http.MethodPost, base+"/reservations/"+r1.UUID.String()+"/pay", token, nil)
	if again.code != http.StatusOK || again.data["url"] != pay.data["url"] || again.data["reused"] != true {
		t.Fatalf("pay again %s", again.raw)
	}
	var links int
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM payment_links WHERE organization_id = $1 AND status = 'active'`, e.a.id).Scan(&links))
	if links != 1 {
		t.Fatalf("active links %d", links)
	}

	// Cancellation request: stored with the snapshot's policy text; the
	// reservation is not cancelled (the snapshot does not allow it).
	cancel := e.call(http.MethodPost, base+"/reservations/"+r1.UUID.String()+"/requests", token, map[string]any{
		"type": "cancel", "message": "Planlarım değişti",
	})
	if cancel.code != http.StatusCreated || cancel.data["status"] != "open" || cancel.data["policy_text"] == "" {
		t.Fatalf("cancel request %d %s", cancel.code, cancel.raw)
	}
	if dup := e.call(http.MethodPost, base+"/reservations/"+r1.UUID.String()+"/requests", token, map[string]any{"type": "cancel"}); dup.code != http.StatusConflict ||
		dup.code_() != "REQUEST_EXISTS" {
		t.Fatalf("duplicate %d %s", dup.code, dup.raw)
	}
	if got, err := e.res.Get(e.a.ctx, e.a.id, r1.UUID); err != nil || got.Status != resmodel.StatusConfirmed {
		t.Fatalf("reservation after the request: %v %v", got.Status, err)
	}
	change := e.call(http.MethodPost, base+"/reservations/"+r2.UUID.String()+"/requests", token, map[string]any{
		"type": "change", "message": "Bir gün uzatmak istiyorum", "requested_ends_at": r2.EndsAt.Add(24 * time.Hour),
	})
	if change.code != http.StatusCreated {
		t.Fatalf("change request %d %s", change.code, change.raw)
	}
	if bad := e.call(http.MethodPost, base+"/reservations/"+r2.UUID.String()+"/requests", token, map[string]any{"type": "upgrade"}); bad.code != http.StatusBadRequest {
		t.Fatalf("unknown type %d", bad.code)
	}

	// Staff see both requests (list, stats) and resolve one; B sees none.
	staff, total, err := e.svc.ListRequests(e.a.ctx, e.a.id, model.RequestFilter{SortDesc: true})
	e.must(err)
	if total != 2 || len(staff) != 2 || staff[0].Reservation.UUID != r2.UUID || staff[1].CustomerName != "Ayşe Yılmaz" ||
		staff[1].PolicyText == "" || staff[1].ContactMasked != "+90 5** *** ** 33" {
		t.Fatalf("staff list %d %+v", total, staff)
	}
	if st, err := e.svc.RequestStats(e.a.ctx, e.a.id); err != nil || st.Open != 2 {
		t.Fatalf("stats %+v %v", st, err)
	}
	if _, n, _ := e.svc.ListRequests(e.b.ctx, e.b.id, model.RequestFilter{}); n != 0 {
		t.Fatalf("B sees %d requests", n)
	}
	if _, err := e.svc.GetRequest(e.b.ctx, e.b.id, staff[1].UUID); err != usecase.ErrRequestNotFound {
		t.Fatalf("B reads A's request: %v", err)
	}
	if _, err := e.svc.ResolveRequest(e.a.ctx, e.a.id, staff[1].UUID, model.ResolveInput{Status: "rejected"}); err == nil {
		t.Fatal("reject without a note")
	}
	resolved, err := e.svc.ResolveRequest(e.a.ctx, e.a.id, staff[1].UUID, model.ResolveInput{Status: "rejected", Note: "İptal ücreti uygulanır, sizi arayacağız"})
	e.must(err)
	if resolved.Status != "rejected" {
		t.Fatalf("resolved %+v", resolved)
	}
	after := e.call(http.MethodGet, base+"/reservations/"+r1.UUID.String(), token, nil)
	reqs := after.data["requests"].([]any)
	if len(reqs) != 1 || reqs[0].(map[string]any)["status"] != "rejected" || reqs[0].(map[string]any)["resolution_note"] == "" ||
		after.data["actions"].(map[string]any)["request_cancel"] != true {
		t.Fatalf("customer sees the resolution: %s", after.raw)
	}
	var events int
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM reservation_events WHERE organization_id = $1 AND reservation_id = (
		SELECT id FROM reservations WHERE organization_id = $1 AND uuid = $2) AND action LIKE 'customer_request%'`, e.a.id, r1.UUID).Scan(&events))
	if events != 2 {
		t.Fatalf("timeline entries %d", events)
	}

	// Audit: sign-in events without the code.
	var audits int
	var payloads string
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*), string_agg(payload::text, ' ') FROM activity_events
		WHERE organization_id = $1 AND action LIKE 'customer_portal.%'`, e.a.id).Scan(&audits, &payloads))
	if audits < 4 || !strings.Contains(payloads, "+90 5** *** ** 33") {
		t.Fatalf("audit %d %s", audits, payloads)
	}
	for _, m := range e.sms.Messages() {
		if c := smsCodeRE.FindStringSubmatch(m.Text); c != nil && strings.Contains(payloads, c[1]) {
			t.Fatalf("an OTP code is in the audit: %s", payloads)
		}
	}

	// Sign out: the token is dead.
	if r := e.call(http.MethodPost, base+"/auth/logout", token, nil); r.code != http.StatusOK {
		t.Fatalf("logout %d", r.code)
	}
	if r := e.call(http.MethodGet, base+"/reservations", token, nil); r.code != http.StatusUnauthorized || r.code_() != "CUSTOMER_SESSION_REQUIRED" {
		t.Fatalf("after logout %d %s", r.code, r.raw)
	}
}

// TestPortalEmailSignIn: the e-mail on file gets the code through the
// platform mailer; a driver's own number signs in as that driver and sees
// only the reservations they drive.
func TestPortalEmailAndDriver(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "Ayse@Example.test")
	d := e.driver(e.a, x.customer, "Deniz", "+905551234567")
	r1 := e.reserve(e.a, x, 3, d)
	r2 := e.reserve(e.a, x, 10)
	base := "/v1/customer/" + e.a.slug

	if r := e.call(http.MethodPost, base+"/auth/start", "", map[string]any{"reference": r1.Reference, "contact": "ayse@example.test"}); r.code != http.StatusAccepted ||
		r.data["channel"] != "email" {
		t.Fatalf("email start %d %s", r.code, r.raw)
	}
	body := e.mailer.last("ayse@example.test")
	m := regexp.MustCompile(`kodunuz: (\d{6})`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no e-mail code: %q", body)
	}
	v := e.call(http.MethodPost, base+"/auth/verify", "", map[string]any{"reference": r1.Reference, "contact": "AYSE@example.test", "code": m[1]})
	if v.code != http.StatusOK || v.data["session"].(map[string]any)["channel"] != "email" {
		t.Fatalf("email verify %d %s", v.code, v.raw)
	}
	var stored int
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM sms_otp_challenges WHERE organization_id = $1 AND channel = 'email'
		AND phone_masked = 'a***@e***.test' AND phone_hash <> 'ayse@example.test'`, e.a.id).Scan(&stored))
	if stored != 1 {
		t.Fatalf("email challenge rows %d", stored)
	}

	// One open code per reservation: the driver asks after the cooldown.
	e.advance(61 * time.Second)
	token := e.signIn(e.a, r1.Reference, "+90 555 123 45 67", "5551234567")
	sess := e.call(http.MethodGet, base+"/session", token, nil)
	if sess.data["role"] != "driver" || sess.data["display_name"] != "Deniz S." {
		t.Fatalf("driver session %s", sess.raw)
	}
	list := uuidsOf(items(e.call(http.MethodGet, base+"/reservations", token, nil)))
	if len(list) != 1 || list[0] != r1.UUID.String() {
		t.Fatalf("driver sees %v", list)
	}
	if r := e.call(http.MethodGet, base+"/reservations/"+r2.UUID.String(), token, nil); r.code != http.StatusNotFound {
		t.Fatalf("driver reads another reservation of the customer: %d", r.code)
	}
	// The driver's number does not open the second reservation.
	if r := e.call(http.MethodPost, base+"/auth/start", "", map[string]any{"reference": r2.Reference, "contact": "05551234567"}); r.code != http.StatusAccepted {
		t.Fatalf("start %d", r.code)
	}
}

// TestPortalGenericAnswers: an unknown reference, a reference with another
// contact and a matching pair answer the same; only the match sends a code.
// Verify answers one generic error for each failure.
func TestPortalGenericAnswers(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "ayse@example.test")
	y := e.customer(e.a, "Mehmet", "+905329998877", "")
	r1 := e.reserve(e.a, x, 3)
	e.reserve(e.a, y, 5)
	base := "/v1/customer/" + e.a.slug

	start := func(reference, contact string) resp {
		return e.call(http.MethodPost, base+"/auth/start", "", map[string]any{"reference": reference, "contact": contact})
	}
	unknownRef := start("ZZZ-2026-999999", "05321112233")
	otherContact := start(r1.Reference, "05329998877") // Y's number, X's reservation
	if unknownRef.code != http.StatusAccepted || otherContact.code != http.StatusAccepted ||
		string(stripMeta(unknownRef.raw)) != string(stripMeta(otherContact.raw)) {
		t.Fatalf("answers differ:\n%s\n%s", unknownRef.raw, otherContact.raw)
	}
	if e.smsCount("5321112233") != 0 || e.smsCount("5329998877") != 0 {
		t.Fatal("a code went out without a match")
	}
	matched := start(r1.Reference, "05321112233")
	if string(stripMeta(matched.raw)) != string(stripMeta(unknownRef.raw)) || e.smsCount("5321112233") != 1 {
		t.Fatalf("matched answer %s", matched.raw)
	}
	// Resend inside the cooldown: same answer, no second SMS.
	if again := start(r1.Reference, "05321112233"); string(stripMeta(again.raw)) != string(stripMeta(unknownRef.raw)) || e.smsCount("5321112233") != 1 {
		t.Fatalf("cooldown answer %s", again.raw)
	}
	code := e.smsCode("5321112233")
	verify := func(reference, contact, c string) resp {
		return e.call(http.MethodPost, base+"/auth/verify", "", map[string]any{"reference": reference, "contact": contact, "code": c})
	}
	for _, r := range []resp{
		verify("ZZZ-2026-999999", "05321112233", code),
		verify(r1.Reference, "05329998877", code),
		verify(r1.Reference, "ayse@example.test", code), // e-mail on file but the code went by SMS
	} {
		if r.code != http.StatusBadRequest || r.code_() != "OTP_INVALID" {
			t.Fatalf("verify failure %d %s", r.code, r.raw)
		}
	}
	// Malformed input is a plain validation error.
	if r := start("x", "not a contact"); r.code != http.StatusBadRequest {
		t.Fatalf("malformed %d", r.code)
	}
	// Unknown portal.
	if r := e.call(http.MethodPost, "/v1/customer/nope-"+e.suffix+"/auth/start", "", map[string]any{"reference": r1.Reference, "contact": "05321112233"}); r.code != http.StatusNotFound {
		t.Fatalf("unknown portal %d", r.code)
	}
	// The real code still works after the failures above (they never
	// reached the open challenge).
	if r := verify(r1.Reference, "05321112233", code); r.code != http.StatusOK {
		t.Fatalf("verify %d %s", r.code, r.raw)
	}
	// No SMS account in an organization: generic unavailable, before any
	// lookup (B has one: remove it).
	_, err := e.pool.Exec(context.Background(), `UPDATE integration_accounts SET status = 'disabled' WHERE organization_id = $1`, e.b.id)
	e.must(err)
	if r := e.call(http.MethodPost, "/v1/customer/"+e.b.slug+"/auth/start", "", map[string]any{"reference": r1.Reference, "contact": "05321112233"}); r.code != http.StatusServiceUnavailable ||
		r.code_() != "CODE_UNAVAILABLE" {
		t.Fatalf("no channel %d %s", r.code, r.raw)
	}
}

func stripMeta(raw []byte) []byte {
	var env map[string]any
	_ = json.Unmarshal(raw, &env)
	delete(env, "meta")
	b, _ := json.Marshal(env)
	return b
}

// TestPortalOTPLock: five wrong codes lock the challenge; the right code
// then fails with the same generic answer; a new code (after the cooldown)
// signs in.
func TestPortalOTPLock(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "")
	r1 := e.reserve(e.a, x, 3)
	base := "/v1/customer/" + e.a.slug
	body := map[string]any{"reference": r1.Reference, "contact": "05321112233"}
	if r := e.call(http.MethodPost, base+"/auth/start", "", body); r.code != http.StatusAccepted {
		t.Fatal(r.code)
	}
	code := e.smsCode("5321112233")
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		r := e.call(http.MethodPost, base+"/auth/verify", "", map[string]any{"reference": r1.Reference, "contact": "05321112233", "code": wrong})
		if r.code != http.StatusBadRequest || r.code_() != "OTP_INVALID" {
			t.Fatalf("wrong %d: %d %s", i, r.code, r.raw)
		}
	}
	r := e.call(http.MethodPost, base+"/auth/verify", "", map[string]any{"reference": r1.Reference, "contact": "05321112233", "code": code})
	if r.code != http.StatusBadRequest || r.code_() != "OTP_INVALID" {
		t.Fatalf("locked challenge accepted the code: %d %s", r.code, r.raw)
	}
	var locked int
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM sms_otp_challenges WHERE organization_id = $1 AND purpose = 'customer_portal' AND status = 'locked'`,
		e.a.id).Scan(&locked))
	if locked != 1 {
		t.Fatalf("locked challenges %d", locked)
	}
	var failed int
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM activity_events WHERE organization_id = $1 AND action = 'customer_portal.sign_in_failed'`,
		e.a.id).Scan(&failed))
	if failed != 6 {
		t.Fatalf("failure audits %d", failed)
	}
	e.advance(61 * time.Second)
	if token := e.signIn(e.a, r1.Reference, "05321112233", "5321112233"); token == "" {
		t.Fatal("no session after a new code")
	}
}

// TestPortalSessionExpiry: the idle expiry slides with use, the hard cap
// ends the session whatever the activity.
func TestPortalSessionExpiry(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "")
	r1 := e.reserve(e.a, x, 3)
	base := "/v1/customer/" + e.a.slug
	token := e.signIn(e.a, r1.Reference, "05321112233", "5321112233")
	// 25 min idle, used, 25 min idle again: still alive (slid).
	e.advance(25 * time.Minute)
	if r := e.call(http.MethodGet, base+"/session", token, nil); r.code != http.StatusOK {
		t.Fatalf("after 25 min %d", r.code)
	}
	e.advance(25 * time.Minute)
	if r := e.call(http.MethodGet, base+"/reservations", token, nil); r.code != http.StatusOK {
		t.Fatalf("after 50 min (slid) %d", r.code)
	}
	// 31 idle minutes: expired.
	e.advance(31 * time.Minute)
	if r := e.call(http.MethodGet, base+"/session", token, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("idle-expired session %d", r.code)
	}
	// Hard cap: used every 20 minutes, dead after 2 hours.
	token = e.signIn(e.a, r1.Reference, "05321112233", "5321112233")
	alive := 0
	for i := 0; i < 7; i++ {
		e.advance(20 * time.Minute)
		if r := e.call(http.MethodGet, base+"/session", token, nil); r.code == http.StatusOK {
			alive++
		}
	}
	if alive != 5 {
		t.Fatalf("alive %d of 7 checks (hard cap 2 h)", alive)
	}
	// A malformed token, an unknown one.
	for _, tok := range []string{"x", strings.Repeat("A", 43)} {
		if r := e.call(http.MethodGet, base+"/session", tok, nil); r.code != http.StatusUnauthorized {
			t.Fatalf("token %q = %d", tok, r.code)
		}
	}
}

// TestPortalScoping (RT-A01 style for the portal): a session of customer X
// of organization A reads nothing of customer Y of A and nothing of
// organization B. Every customer route is called with Y's and B's ids in
// the path (and X's reservation with Y's document); X's token is refused on
// B's portal. Nothing of Y or B appears in any answer.
func TestPortalScoping(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "ayse@example.test")
	y := e.customer(e.a, "Mehmet", "+905329998877", "mehmet@example.test")
	// B has a customer with X's very number.
	xb := e.customer(e.b, "Ayşe", "+905321112233", "ayse@example.test")
	rx := e.reserve(e.a, x, 3)
	ry := e.reserve(e.a, y, 5)
	rb := e.reserve(e.b, xb, 3)
	docOf := func(tn *tenant, r resmodel.Reservation) string {
		list, _, err := e.docs.ListDocuments(tn.ctx, tn.id, contractsmodel.DocumentFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, Limit: 5})
		e.must(err)
		return list[0].UUID.String()
	}
	dx, dy, dbB := docOf(e.a, rx), docOf(e.a, ry), docOf(e.b, rb)
	_, err := e.svc.Info(context.Background(), e.a.slug)
	e.must(err)
	token := e.signIn(e.a, rx.Reference, "05321112233", "5321112233")
	base := "/v1/customer/" + e.a.slug
	leaks := []string{ry.UUID.String(), ry.Reference, rb.UUID.String(), rb.Reference, dy, dbB, "Mehmet", e.b.uuid.String(), e.b.slug}

	type route struct{ method, path string }
	routes := []route{}
	for _, res := range []string{ry.UUID.String(), rb.UUID.String(), uuid.NewString(), "not-a-uuid"} {
		routes = append(routes,
			route{http.MethodGet, base + "/reservations/" + res},
			route{http.MethodPost, base + "/reservations/" + res + "/pay"},
			route{http.MethodPost, base + "/reservations/" + res + "/requests"},
			route{http.MethodGet, base + "/reservations/" + res + "/documents/" + dx + "/pdf"},
		)
	}
	// X's own reservation with someone else's document.
	for _, d := range []string{dy, dbB} {
		routes = append(routes, route{http.MethodGet, base + "/reservations/" + rx.UUID.String() + "/documents/" + d + "/pdf"})
	}
	for _, rt := range routes {
		var body any
		if rt.method == http.MethodPost {
			body = map[string]any{"type": "cancel", "message": "x"}
			if strings.HasSuffix(rt.path, "/pay") {
				body = nil
			}
		}
		r := e.call(rt.method, rt.path, token, body)
		if r.code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404: %s", rt.method, rt.path, r.code, r.raw)
		}
		for _, l := range leaks {
			if strings.Contains(string(r.raw), l) {
				t.Errorf("%s %s leaked %q", rt.method, rt.path, l)
			}
		}
	}
	// Lists and the session hold X's data only.
	for _, p := range []string{"/session", "/reservations", "/reservations/" + rx.UUID.String()} {
		r := e.call(http.MethodGet, base+p, token, nil)
		if r.code != http.StatusOK {
			t.Fatalf("%s = %d", p, r.code)
		}
		for _, l := range leaks {
			if strings.Contains(string(r.raw), l) {
				t.Errorf("%s leaked %q", p, l)
			}
		}
	}
	// X's A-token on B's portal (B has the same number, the same kind of
	// data): refused on every route.
	bBase := "/v1/customer/" + e.b.slug
	for _, p := range []string{"/session", "/reservations", "/reservations/" + rb.UUID.String(), "/reservations/" + rb.UUID.String() + "/documents/" + dbB + "/pdf"} {
		if r := e.call(http.MethodGet, bBase+p, token, nil); r.code != http.StatusUnauthorized {
			t.Errorf("A's token on B %s = %d", p, r.code)
		}
	}
	for _, p := range []string{"/reservations/" + rb.UUID.String() + "/pay", "/reservations/" + rb.UUID.String() + "/requests", "/auth/logout"} {
		if r := e.call(http.MethodPost, bBase+p, token, map[string]any{"type": "cancel"}); r.code != http.StatusUnauthorized {
			t.Errorf("A's token on B POST %s = %d", p, r.code)
		}
	}
	// A's reference on B's portal with the shared number: no code in B.
	before := e.smsCount("5321112233")
	if r := e.call(http.MethodPost, bBase+"/auth/start", "", map[string]any{"reference": rx.Reference, "contact": "05321112233"}); r.code != http.StatusAccepted {
		t.Fatalf("start on B %d", r.code)
	}
	if e.smsCount("5321112233") != before {
		t.Fatal("B sent a code for A's reference")
	}
	// Nothing changed in B or for Y.
	var n int
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM customer_requests WHERE organization_id = $1 OR reservation_id = (
		SELECT id FROM reservations WHERE organization_id = $2 AND uuid = $3)`, e.b.id, e.a.id, ry.UUID).Scan(&n))
	if n != 0 {
		t.Fatalf("requests written for B / Y: %d", n)
	}
	e.must(e.pool.QueryRow(context.Background(), `SELECT count(*) FROM payment_links WHERE organization_id = $1`, e.b.id).Scan(&n))
	if n != 0 {
		t.Fatalf("payment links in B: %d", n)
	}
}

// TestPortalSelfCancel: a policy snapshot that explicitly allows
// self-cancellation cancels at once (the request is recorded resolved);
// without it the reservation stays as it was (happy path).
func TestPortalSelfCancel(t *testing.T) {
	e := newEnv(t)
	x := e.customer(e.a, "Ayşe", "+905321112233", "")
	r1 := e.reserve(e.a, x, 3)
	_, err := e.pool.Exec(context.Background(), `UPDATE reservations SET policy_snapshot = jsonb_set(policy_snapshot, '{policies,self_cancel_allowed}', 'true')
		|| '{"cancellation_fee": "250.00"}'::jsonb WHERE organization_id = $1 AND uuid = $2`, e.a.id, r1.UUID)
	e.must(err)
	base := "/v1/customer/" + e.a.slug
	token := e.signIn(e.a, r1.Reference, "05321112233", "5321112233")
	detail := e.call(http.MethodGet, base+"/reservations/"+r1.UUID.String(), token, nil)
	c := detail.data["cancellation"].(map[string]any)
	if c["self_cancel"] != true || c["fee"] != "250.00" {
		t.Fatalf("cancellation %v", c)
	}
	r := e.call(http.MethodPost, base+"/reservations/"+r1.UUID.String()+"/requests", token, map[string]any{"type": "cancel"})
	if r.code != http.StatusCreated || r.data["status"] != "resolved" || r.data["outcome"] != "self_cancelled" || r.data["cancellation_fee"] != "250.00" {
		t.Fatalf("self cancel %d %s", r.code, r.raw)
	}
	got, err := e.res.Get(e.a.ctx, e.a.id, r1.UUID)
	e.must(err)
	if got.Status != resmodel.StatusCancelled {
		t.Fatalf("status %s", got.Status)
	}
	if r := e.call(http.MethodPost, base+"/reservations/"+r1.UUID.String()+"/pay", token, nil); r.code != http.StatusConflict || r.code_() != "NOTHING_DUE" {
		t.Fatalf("pay a cancelled reservation %d %s", r.code, r.raw)
	}
}
