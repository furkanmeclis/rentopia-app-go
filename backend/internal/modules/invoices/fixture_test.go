package invoices_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	rentaladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice/einvoicefake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with two branches, a product, customers with
// encrypted identity numbers, the rental chain with the finance ledger
// (the handover accrues the rent) and the invoices service with the fake
// integrator on the integration runner (no queue: first steps run inline).
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	cipher   *crypto.FieldCipher
	res      *resusecase.Service
	rentals  *rentalusecase.Service
	fin      *financeusecase.Service
	inv      *usecase.Service
	fake     *einvoicefake.Provider
	accounts *accountsusecase.Service
	runner   *jobsusecase.Runner
	store    *storage.Memory
	org      int64
	orgUUID  uuid.UUID
	suffix   string
	segment  int64
	branch   ref
	other    ref
	product  ref
	person   ref // individual customer (TCKN) → e-Arşiv
	personD  ref
	company  ref // corporate customer (VKN) → e-Fatura
	companyD ref
	plate    int
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var allPerms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantDamageApprove, rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite,
	rbac.PermTenantInvoicesRead, rbac.PermTenantInvoicesWrite, rbac.PermTenantInvoicesCancel,
	rbac.PermTenantInvoicesSettings, rbac.PermTenantIntegrationsManage,
}

// newFx builds the fixture; connect = an einvoice_fake account is connected.
func newFx(t *testing.T, connect bool) *fx {
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
	f := &fx{t: t, pool: pool, q: q, suffix: strings.ReplaceAll(uuid.NewString()[:8], "-", "")}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "inv-" + f.suffix, Name: "Inv " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM integration_jobs WHERE organization_id = $1", f.org)
		// Ledger rows are append-only: only the organization cascade removes them.
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = f.scoped(nil)
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	f.res = resusecase.New(pool, q, avail, pricing, settings)
	f.rentals = rentalusecase.New(pool, q, avail, rentaladapters.NewReservations(f.res), vehicles)
	f.res.SetLifecycleListener(f.rentals)
	f.fin = financeusecase.New(pool, q, nil)
	f.rentals.SetLedger(f.fin)

	f.cipher, err = crypto.NewFieldCipher(strings.Repeat("i", 32))
	f.must(err)
	vault, err := integrations.NewVault(f.cipher)
	f.must(err)
	f.fake = einvoicefake.New()
	providers := []einvoice.Provider{einvoice.Official{}, f.fake}
	f.accounts = accountsusecase.New(q, vault, integrations.NewRegistry(einvoice.Official{}, f.fake))
	ops := integrations.NewOperationRegistry()
	f.runner = jobsusecase.NewRunner(q, f.accounts, ops)
	f.inv, err = usecase.New(pool, q, f.cipher, settings, f.accounts, f.runner, ops, providers...)
	f.must(err)
	f.store = storage.NewMemory()
	f.inv.SetStore(f.store)

	f.branch = f.newBranch("MRK")
	f.other = f.newBranch("HVL")
	f.product = f.newProduct("EKO")
	f.person, f.personD = f.newCustomer(false, "10000000146")
	f.company, f.companyD = f.newCustomer(true, "1234567802")
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
	s.Company = osmodel.Company{LegalName: "Test Oto Kiralama A.Ş.", TaxNumber: "1234567890", TaxOffice: "Kadıköy",
		Address: "Bağdat Cad. No:1", Phone: "+902160000000", Email: "info@example.test"}
	data, _ := json.Marshal(s)
	_, err = pool.Exec(ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)
	_, err = f.inv.PutSettings(f.ctx, f.org, model.SettingsInput{SupplierDistrict: sp("Kadıköy"), SupplierCity: sp("İstanbul")})
	f.must(err)
	if connect {
		f.connect("")
	}
	return f
}

func intp(v int) *int     { return &v }
func sp(s string) *string { return &s }

func (f *fx) scoped(branches []int64, perms ...string) context.Context {
	if len(perms) == 0 {
		perms = allPerms
	}
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "inv-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms, BranchIDs: branches,
	})
}

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

// connect adds an active einvoice_fake account (scenario: "", ok, slow,
// outage, timeout, reject).
func (f *fx) connect(scenario string) uuid.UUID {
	f.t.Helper()
	a, err := f.accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Kind: einvoice.Kind, Provider: einvoicefake.ProviderCode, DisplayName: "Test entegratör",
		Credentials: map[string]string{"username": "u-" + f.suffix, "password": "p-" + f.suffix, "scenario": scenario},
	})
	f.must(err)
	return a.UUID
}

func (f *fx) newBranch(code string) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", City: "İstanbul", Address: code + " Cad. No:1",
		WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"), PrepMinutes: 30, IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

func (f *fx) vehicle(b ref) ref {
	f.plate++
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 IV %d%s", f.plate, strings.ToUpper(f.suffix[:3])), b.id).Scan(&r.id, &r.uuid))
	_, err := f.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 ON CONFLICT DO NOTHING`, f.org, r.id)
	f.must(err)
	return r
}

func (f *fx) newProduct(code string) ref {
	ctx := context.Background()
	var p ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, $2, $3, $4, 'published') RETURNING id, uuid`, f.org, code, code+" ürün", f.segment).Scan(&p.id, &p.uuid))
	var group int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, $2, $2, $3) RETURNING id`,
		f.org, "G"+code, f.segment).Scan(&group))
	_, err := f.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, f.org, p.id, group)
	f.must(err)
	var planID, versionID int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rate_plans (organization_id, rental_product_id, scope, name) VALUES ($1, $2, 'common', 'Tarife') RETURNING id`,
		f.org, p.id).Scan(&planID))
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version) VALUES ($1, $2, 1) RETURNING id`,
		f.org, planID).Scan(&versionID))
	_, err = f.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, 1, NULL, 1199.99)`,
		f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planID)
	f.must(err)
	return p
}

func (f *fx) enc(column, v string) string {
	out, err := f.cipher.Encrypt(v, column+"|org:"+strconv.FormatInt(f.org, 10))
	f.must(err)
	return out
}

// newCustomer adds a customer with a driver: corporate with a VKN, or an
// individual with a TCKN; both with a billing address.
func (f *fx) newCustomer(corporate bool, identity string) (ref, ref) {
	ctx := context.Background()
	var c, d ref
	typ, kind, first, last, title := "individual", "tckn", "Deniz", "Kaya", ""
	if corporate {
		typ, kind, first, last, title = "corporate", "vkn", "", "", "Kaya Lojistik Ltd. Şti."
	}
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, company_title, phone, email,
		identity_type, identity_enc, identity_bidx, identity_hint, nationality, tax_office, billing_address, billing_city, billing_district,
		billing_country)
		VALUES ($1, $2, $3, $4, $5, '+905321112244', 'musteri@example.test', $6, $7, $8, '***', 'TR', 'Çankaya', 'Atatürk Blv. 10', 'Ankara', 'Çankaya', 'TR')
		RETURNING id, uuid`,
		f.org, typ, first, last, title, kind, f.enc("customers.identity", identity), uuid.NewString()).Scan(&c.id, &c.uuid))
	now := time.Now().UTC()
	f.must(f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Deniz', 'Kaya', $3, 'v1:x', $4, '****456', $5) RETURNING id, uuid`,
		f.org, c.id, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return c, d
}

// rental books a two day rental of customer c at branch b and hands it
// over (the ledger accrues the rent).
func (f *fx) rental(b ref, c, d ref) rentalmodel.Rental {
	f.t.Helper()
	v := f.vehicle(b)
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	w := resmodel.Where{PickupBranchUUID: &b.uuid, StartsAt: &start, EndsAt: ptrTime(start.Add(48 * time.Hour))}
	quote, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid})
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &c.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: d.uuid}},
	})
	f.must(err)
	rental, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &r.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	km, fuel := int32(1100), int16(80)
	out, err := f.rentals.Handover(f.ctx, f.org, rental.UUID, rentalmodel.HandoverInput{Km: &km, Fuel: &fuel})
	f.must(err)
	return out
}

func ptrTime(t time.Time) *time.Time { return &t }

func (f *fx) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	f.must(f.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

// ledger is a fingerprint of the organization's ledger: the number of
// entries and the debit / credit sums.
type ledger struct {
	entries       int
	debit, credit string
}

func (f *fx) ledger() ledger {
	f.t.Helper()
	var l ledger
	f.must(f.pool.QueryRow(context.Background(), `SELECT count(*),
		COALESCE(SUM(amount) FILTER (WHERE direction = 'debit'), 0)::text,
		COALESCE(SUM(amount) FILTER (WHERE direction = 'credit'), 0)::text
		FROM ledger_entries WHERE organization_id = $1`, f.org).Scan(&l.entries, &l.debit, &l.credit))
	return l
}

// rentAccrual is the live rent accrual of a rental (amount, count).
func (f *fx) rentAccrual(rental uuid.UUID) (money.Money, int) {
	f.t.Helper()
	var sum string
	var n int
	f.must(f.pool.QueryRow(context.Background(), `SELECT COALESCE(SUM(e.amount), 0)::text, COUNT(*) FROM ledger_entries e
		JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.kind = 'accrual' AND e.purpose = 'rent'
		  AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id)`,
		f.org, rental).Scan(&sum, &n))
	return money.MustParse(sum), n
}

func (f *fx) draft(rental uuid.UUID) model.Detail {
	f.t.Helper()
	d, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &rental})
	f.must(err)
	return d
}

func (f *fx) get(id uuid.UUID) model.Detail {
	f.t.Helper()
	d, err := f.inv.Get(f.ctx, f.org, id)
	f.must(err)
	return d
}

// step runs one runner step of the invoice's send job (the sweep).
func (f *fx) step(d model.Detail) {
	f.t.Helper()
	var jobID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM integration_jobs WHERE organization_id = $1 AND uuid = $2`, f.org, d.Job.UUID).Scan(&jobID))
	_, err := f.runner.Process(context.Background(), f.org, jobID, jobsusecase.TriggerSweep)
	f.must(err)
}

// dueNow makes the invoice's jobs due now (the runner's backoff).
func (f *fx) dueNow(id uuid.UUID) {
	_, err := f.pool.Exec(context.Background(), `UPDATE integration_jobs SET next_attempt_at = NOW() - interval '1 second'
		WHERE organization_id = $1 AND ref_uuid = $2`, f.org, id)
	f.must(err)
}
