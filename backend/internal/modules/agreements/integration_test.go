package agreements_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/usecase"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	campaignsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricingmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	rentaladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	resadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with two branches, a product (public tariff
// 1 000 / day), a corporate customer with a driver, and agreements,
// pricing, reservations, rentals and finance wired like the server.
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	svc      *usecase.Service
	pricing  *pricingusecase.Service
	res      *resusecase.Service
	rentals  *rentalusecase.Service
	fin      *financeusecase.Service
	org      int64
	orgUUID  uuid.UUID
	suffix   string
	segment  int64
	branch   ref
	other    ref
	product  ref
	company  ref
	driver   ref
	employee ref
	empDrv   ref
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
	rbac.PermTenantSignatureWaive, rbac.PermTenantPriceOverride, rbac.PermTenantPricingRead,
	rbac.PermTenantPricingWrite, rbac.PermTenantPricingActivate, rbac.PermTenantFinanceAccountsRead,
	rbac.PermTenantAgreementsRead, rbac.PermTenantAgreementsWrite, rbac.PermTenantAgreementsExport,
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
	q := db.New(pool)
	f := &fx{t: t, pool: pool, q: q, suffix: strings.ReplaceAll(uuid.NewString()[:8], "-", "")}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "agr-" + f.suffix, Name: "Agr " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		// Ledger rows and posted periods are append-only: only the
		// organization cascade removes them.
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = f.scoped(nil)
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	f.pricing = pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	campaigns := campaignsusecase.New(pool, q)
	f.pricing.SetCouponEvaluator(pricingadapters.NewCoupons(campaigns))
	f.res = resusecase.New(pool, q, avail, f.pricing, settings)
	f.res.SetCoupons(resadapters.NewCoupons(campaigns))
	f.rentals = rentalusecase.New(pool, q, avail, rentaladapters.NewReservations(f.res), vehicles)
	f.res.SetLifecycleListener(f.rentals)
	f.fin = financeusecase.New(pool, q, nil)
	f.rentals.SetLedger(f.fin)
	f.svc = usecase.New(pool, q, f.fin, f.pricing)

	f.branch = f.newBranch("MRK")
	f.other = f.newBranch("HVL")
	f.product = f.newProduct("EKO")
	f.company, f.driver = f.newCompany("Acme Lojistik A.Ş.")
	f.employee, f.empDrv = f.newIndividual("Ayşe", "Yılmaz")
	f.settings(osmodel.PrepaymentPercent, "20")
	return f
}

// settings writes a settings version (auto approval, prepayment rule).
func (f *fx) settings(rule, value string) {
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	m60, m30 := 60, 30
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = &m60, &m60, &m30
	s.Payment.PrepaymentRule, s.Payment.PrepaymentValue = rule, value
	data, _ := json.Marshal(s)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO organization_settings_versions (organization_id, version, data)
		VALUES ($1, COALESCE((SELECT MAX(version) FROM organization_settings_versions WHERE organization_id = $1), 0) + 1, $2)`, f.org, data)
	f.must(err)
}

func (f *fx) scoped(branches []int64, perms ...string) context.Context {
	if len(perms) == 0 {
		perms = allPerms
	}
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "agr-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms, BranchIDs: branches,
	})
}

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) newBranch(code string) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: 30, IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

func (f *fx) vehicle() ref {
	f.plate++
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 KA %d", f.plate), f.branch.id).Scan(&r.id, &r.uuid))
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
	// The public tariff: 1 000 a day for any length.
	plan, err := f.pricing.CreatePlan(f.ctx, f.org, pricingmodel.RatePlanCreateInput{
		RentalProductUUID: &p.uuid, Scope: pricing.ScopeCommon, Name: "Genel",
		Bands: []pricingmodel.BandInput{{MinDays: 1, DailyPrice: "1000"}},
	})
	f.must(err)
	_, err = f.pricing.ActivatePlan(f.ctx, f.org, plan.UUID, pricingmodel.ActivateInput{})
	f.must(err)
	return p
}

func (f *fx) newCompany(title string) (ref, ref) {
	ctx := context.Background()
	var c, d ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, company_title, phone, email)
		VALUES ($1, 'corporate', $2, '+902121112233', 'filo@acme.test') RETURNING id, uuid`, f.org, title).Scan(&c.id, &c.uuid))
	d = f.newDriver(c, "Mehmet", "Kaya")
	return c, d
}

func (f *fx) newIndividual(first, last string) (ref, ref) {
	ctx := context.Background()
	var c ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', $2, $3, '+905321112233', 'ayse@example.test') RETURNING id, uuid`, f.org, first, last).Scan(&c.id, &c.uuid))
	return c, f.newDriver(c, first, last)
}

func (f *fx) newDriver(c ref, first, last string) ref {
	var d ref
	now := time.Now().UTC()
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, $3, $4, $5, 'v1:x', $6, '****456', $7) RETURNING id, uuid`,
		f.org, c.id, first, last, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return d
}

func ip32(v int32) *int32 { return &v }
func bp(v bool) *bool     { return &v }
func sp(v string) *string { return &v }

func mp(s string) *money.Money {
	v := money.MustParse(s)
	return &v
}

// agreement creates an active agreement with the company.
func (f *fx) agreement(period string, edit func(*model.Input)) model.Agreement {
	f.t.Helper()
	from := rentaltime.LocalDate(time.Now()).AddDays(-30)
	in := model.Input{
		CustomerUUID: &f.company.uuid, Name: sp("Filo kiralama"), ValidFrom: &from, InvoicePeriod: sp(period),
		PaymentTermDays: ip32(30), MonthlyKmAllowance: ip32(3000), MaintenanceIntervalDays: ip32(30), Activate: true,
	}
	if edit != nil {
		edit(&in)
	}
	a, err := f.svc.Create(f.ctx, f.org, in)
	f.must(err)
	return a
}

// tariff gives the agreement its own tariff for the product (daily price).
func (f *fx) tariff(a model.Agreement, price string) {
	f.t.Helper()
	plan, err := f.pricing.CreatePlan(f.ctx, f.org, pricingmodel.RatePlanCreateInput{
		RentalProductUUID: &f.product.uuid, Scope: pricing.ScopeAgreement, AgreementUUID: &a.UUID, Name: "Kurumsal",
		Bands: []pricingmodel.BandInput{{MinDays: 1, DailyPrice: pricingmodel.Decimal(price)}},
	})
	f.must(err)
	_, err = f.pricing.ActivatePlan(f.ctx, f.org, plan.UUID, pricingmodel.ActivateInput{})
	f.must(err)
}

// quote prices a sale [from, to) at the main branch, optionally under an
// agreement and with a coupon.
func (f *fx) quote(a *model.Agreement, from, to time.Time, coupon string) (resmodel.Quote, error) {
	w := resmodel.Where{PickupBranchUUID: &f.branch.uuid, StartsAt: &from, EndsAt: &to}
	in := resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid, CustomerUUID: &f.company.uuid, CouponCode: coupon}
	if a != nil {
		in.AgreementUUID = &a.UUID
	}
	return f.res.CreateQuote(f.ctx, f.org, in)
}

// sell quotes and reserves under the agreement for the company's driver.
func (f *fx) sell(a *model.Agreement, from, to time.Time, edit func(*resmodel.CreateInput)) (resmodel.Reservation, error) {
	q, err := f.quote(a, from, to, "")
	if err != nil {
		return resmodel.Reservation{}, err
	}
	in := resmodel.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &f.company.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}}}
	if edit != nil {
		edit(&in)
	}
	return f.res.Create(f.ctx, f.org, in)
}

// longTerm sells a confirmed long-term rental of days days starting in two
// hours under the agreement and hands it over now (early handover).
func (f *fx) longTerm(a *model.Agreement, days int) (rentalmodel.Rental, resmodel.Reservation) {
	f.t.Helper()
	start := time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	v := f.vehicle()
	res, err := f.sell(a, start, start.Add(time.Duration(days)*rentaltime.Day), func(in *resmodel.CreateInput) { in.ConfirmNow = true })
	f.must(err)
	if res.Status != resmodel.StatusConfirmed {
		f.t.Fatalf("reservation status %s", res.Status)
	}
	r, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	km, fuel := int32(1100), int16(80)
	r, err = f.rentals.Handover(f.ctx, f.org, r.UUID, rentalmodel.HandoverInput{Km: &km, Fuel: &fuel})
	f.must(err)
	return r, res
}

func (f *fx) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	f.must(f.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

func (f *fx) rentalID(id uuid.UUID) int64 {
	f.t.Helper()
	var v int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM rentals WHERE organization_id = $1 AND uuid = $2`, f.org, id).Scan(&v))
	return v
}

// liveRent is the sum and count of a rental's live rent accruals, and how
// many of them are whole-rent (handover) accruals.
func (f *fx) liveRent(rental uuid.UUID) (money.Money, int, int) {
	f.t.Helper()
	var sum string
	var n, whole int
	f.must(f.pool.QueryRow(context.Background(), `SELECT COALESCE(SUM(e.amount), 0)::text, COUNT(*),
		COUNT(*) FILTER (WHERE e.billing_period_id IS NULL)
		FROM ledger_entries e
		JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.kind = 'accrual' AND e.purpose = 'rent'
		  AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id)`,
		f.org, rental).Scan(&sum, &n, &whole))
	return money.MustParse(sum), n, whole
}

// periods are the live (planned / posted) billing periods of a rental.
type periodRow struct {
	no, revision, dayFrom, dayTo int
	status                       string
	amount                       money.Money
}

func (f *fx) periods(rental uuid.UUID, statuses ...string) []periodRow {
	f.t.Helper()
	if len(statuses) == 0 {
		statuses = []string{"planned", "posted"}
	}
	rows, err := f.pool.Query(context.Background(), `SELECT bp.period_no, bp.revision, bp.day_from, bp.day_to, bp.status, bp.amount::text
		FROM rental_billing_periods bp JOIN rentals r ON r.organization_id = bp.organization_id AND r.id = bp.rental_id
		WHERE bp.organization_id = $1 AND r.uuid = $2 AND bp.status = ANY($3) ORDER BY bp.period_no, bp.revision`, f.org, rental, statuses)
	f.must(err)
	defer rows.Close()
	out := []periodRow{}
	for rows.Next() {
		var p periodRow
		var amount string
		f.must(rows.Scan(&p.no, &p.revision, &p.dayFrom, &p.dayTo, &p.status, &amount))
		p.amount = money.MustParse(amount)
		out = append(out, p)
	}
	return out
}

func (f *fx) reservationRange(res uuid.UUID) (time.Time, time.Time, money.Money) {
	f.t.Helper()
	var start, end time.Time
	var total string
	f.must(f.pool.QueryRow(context.Background(), `SELECT starts_at, ends_at, total_amount::text FROM reservations WHERE organization_id = $1 AND uuid = $2`,
		f.org, res).Scan(&start, &end, &total))
	return start.UTC(), end.UTC(), money.MustParse(total)
}

// at sets the finance clock (billing day) to the start of billing day n of
// a rental starting at start, plus one hour.
func (f *fx) at(start time.Time, day int) {
	when := start.Add(time.Duration(day)*rentaltime.Day + time.Hour)
	f.fin.SetClock(func() time.Time { return when })
}

func sum(ps []periodRow) money.Money {
	out := money.Zero()
	for _, p := range ps {
		out = out.Add(p.amount)
	}
	return out
}

// ----------------------------------------------------------------- tests

// TestAgreementCRUDAndAccountTerms: a corporate customer only, KA codes,
// the payment term on the customer's ledger account, contacts and
// permitted drivers of the right customer, status transitions.
func TestAgreementCRUDAndAccountTerms(t *testing.T) {
	f := newFx(t)
	_, err := f.svc.Create(f.ctx, f.org, model.Input{CustomerUUID: &f.employee.uuid, Name: sp("x"), ValidFrom: &rentaltime.Date{Year: 2026, Month: 1, Day: 1}})
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) || ve.Details[0].Code != "not_corporate" {
		t.Fatalf("individual customer = %v", err)
	}
	_, err = f.svc.Create(f.ctx, f.org, model.Input{
		CustomerUUID: &f.company.uuid, Name: sp("x"), ValidFrom: &rentaltime.Date{Year: 2026, Month: 1, Day: 1}, OpenAccount: bp(true),
	})
	if !errors.As(err, &ve) || ve.Details[0].Field != "credit_limit" {
		t.Fatalf("open account without limit = %v", err)
	}
	a := f.agreement(model.InvoiceMonthly, func(in *model.Input) { in.Activate = false })
	if a.Code != "KA-000001" || a.Status != model.StatusDraft || a.Account == nil || a.Account.PaymentTermDays != 30 {
		t.Fatalf("agreement = %+v / account %+v", a, a.Account)
	}
	a, _, err = f.svc.Update(f.ctx, f.org, a.UUID, model.Input{PaymentTermDays: ip32(45)})
	f.must(err)
	if a.Account.PaymentTermDays != 45 {
		t.Fatalf("account term after edit = %d", a.Account.PaymentTermDays)
	}
	var contact uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO customer_contacts (organization_id, customer_id, full_name, phone, email)
		VALUES ($1, $2, 'Zeynep Ak', '+905551112233', 'zeynep@acme.test') RETURNING uuid`, f.org, f.company.id).Scan(&contact))
	var othersContact uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO customer_contacts (organization_id, customer_id, full_name)
		VALUES ($1, $2, 'Başkası') RETURNING uuid`, f.org, f.employee.id).Scan(&othersContact))
	if _, _, err := f.svc.AddContact(f.ctx, f.org, a.UUID, model.ContactInput{ContactUUID: &othersContact}); !errors.As(err, &ve) {
		t.Fatalf("contact of another customer = %v", err)
	}
	a, _, err = f.svc.AddContact(f.ctx, f.org, a.UUID, model.ContactInput{ContactUUID: &contact, Role: "billing"})
	f.must(err)
	if len(a.Contacts) != 1 || a.Contacts[0].PhoneMasked == "+905551112233" || a.Contacts[0].Role != "billing" {
		t.Fatalf("contacts = %+v", a.Contacts)
	}
	a, _, err = f.svc.AddDriver(f.ctx, f.org, a.UUID, model.DriverInput{DriverUUID: &f.driver.uuid})
	f.must(err)
	a, _, err = f.svc.AddDriver(f.ctx, f.org, a.UUID, model.DriverInput{CustomerUUID: &f.employee.uuid})
	f.must(err)
	if len(a.Drivers) != 2 {
		t.Fatalf("drivers = %+v", a.Drivers)
	}
	if _, _, err := f.svc.AddDriver(f.ctx, f.org, a.UUID, model.DriverInput{CustomerUUID: &f.company.uuid}); !errors.As(err, &ve) {
		t.Fatalf("company as permitted customer = %v", err)
	}
	if _, _, err := f.svc.SetStatus(f.ctx, f.org, a.UUID, model.StatusInput{Status: model.StatusSuspended}); err == nil {
		t.Fatal("draft -> suspended accepted")
	}
	a, _, err = f.svc.SetStatus(f.ctx, f.org, a.UUID, model.StatusInput{Status: model.StatusActive})
	f.must(err)
	if _, _, err := f.svc.SetStatus(f.ctx, f.org, a.UUID, model.StatusInput{Status: model.StatusTerminated}); !errors.As(err, &ve) {
		t.Fatalf("termination without reason = %v", err)
	}
	items, total, err := f.svc.List(f.ctx, f.org, model.Filter{Statuses: []string{model.StatusActive}, Validities: []string{model.ValidityCurrent}})
	f.must(err)
	if total != 1 || items[0].UUID != a.UUID {
		t.Fatalf("list = %d %+v", total, items)
	}
}

// TestAgreementTariffBeatsPublic: an agreement tariff prices sales under the
// agreement (and is shown as applied); the same request without the
// agreement gets the public tariff. Quotes of a monthly agreement carry
// the period plan.
func TestAgreementTariffBeatsPublic(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoiceMonthly, nil)
	f.tariff(a, "700")
	start := time.Now().Add(3 * time.Hour).Truncate(time.Minute)
	end := start.Add(40 * rentaltime.Day)
	pub, err := f.quote(nil, start, end, "")
	f.must(err)
	agr, err := f.quote(&a, start, end, "")
	f.must(err)
	if pub.Total.String() != "40000.00" || agr.Total.String() != "28000.00" {
		t.Fatalf("public %s / agreement %s", pub.Total, agr.Total)
	}
	ap := agr.Breakdown.Agreement
	if ap == nil || !ap.TariffApplied || ap.UUID != a.UUID || agr.Breakdown.Pricing.Plan.Scope != pricing.ScopeAgreement || agr.Agreement == nil {
		t.Fatalf("agreement pricing = %+v / plan %+v", ap, agr.Breakdown.Pricing.Plan)
	}
	if pub.Breakdown.Agreement != nil || len(pub.Breakdown.BillingPlan) != 0 {
		t.Fatal("a public quote shows agreement data")
	}
	// Period plan: what the pure engine says for this range.
	want := pricing.PeriodPlan(start, end, agr.Total)
	got := agr.Breakdown.BillingPlan
	if len(got) != len(want) || len(got) < 2 {
		t.Fatalf("billing plan = %+v", got)
	}
	s := money.Zero()
	for i := range got {
		if got[i].Days != want[i].Days || !got[i].Amount.Equal(want[i].Amount) || got[i].Partial != want[i].Partial {
			t.Fatalf("period %d = %+v want %+v", i, got[i], want[i])
		}
		s = s.Add(got[i].Amount)
	}
	if !s.Equal(agr.Total) {
		t.Fatalf("plan sum %s != total %s", s, agr.Total)
	}
	// An agreement without its own tariff for the product: public price,
	// tariff not applied.
	b := f.agreement(model.InvoicePerRental, nil)
	q, err := f.quote(&b, start, end, "")
	f.must(err)
	if q.Total.String() != "40000.00" || q.Breakdown.Agreement == nil || q.Breakdown.Agreement.TariffApplied {
		t.Fatalf("agreement without tariff = %s %+v", q.Total, q.Breakdown.Agreement)
	}
}

// TestCouponEligibility: an agreement without coupon eligibility refuses
// every code; with it only corporate-eligible campaigns apply.
func TestCouponEligibility(t *testing.T) {
	f := newFx(t)
	for _, c := range []struct {
		code      string
		corporate bool
	}{{"HERKES10", false}, {"KURUM10", true}} {
		_, err := f.pool.Exec(context.Background(), `INSERT INTO campaigns (organization_id, name, campaign_type, code, discount_kind, discount_value, corporate_eligible)
			VALUES ($1, $2, 'coupon', $2, 'percent', 10, $3)`, f.org, c.code, c.corporate)
		f.must(err)
	}
	start := time.Now().Add(3 * time.Hour).Truncate(time.Minute)
	end := start.Add(3 * rentaltime.Day)
	none := f.agreement(model.InvoicePerRental, nil)
	var ve *resusecase.ValidationError
	if _, err := f.quote(&none, start, end, "KURUM10"); !errors.As(err, &ve) || ve.Details[0].Code != "coupon_corporate" {
		t.Fatalf("coupon on an ineligible agreement = %v", err)
	}
	eligible := f.agreement(model.InvoicePerRental, func(in *model.Input) { in.CouponEligible = bp(true) })
	if _, err := f.quote(&eligible, start, end, "HERKES10"); !errors.As(err, &ve) || ve.Details[0].Code != "coupon_corporate" {
		t.Fatalf("non-corporate campaign on an agreement = %v", err)
	}
	q, err := f.quote(&eligible, start, end, "KURUM10")
	f.must(err)
	if q.Total.String() != "2700.00" {
		t.Fatalf("corporate coupon total = %s", q.Total)
	}
	// A public sale still takes the public campaign.
	q, err = f.quote(nil, start, end, "HERKES10")
	f.must(err)
	if q.Total.String() != "2700.00" {
		t.Fatalf("public coupon total = %s", q.Total)
	}
}

// TestOpenAccountCreditLimit: an open account confirms without prepayment
// until the customer's exposure would pass the credit limit; then the
// confirmation is refused (or the sale follows the prepayment flow).
func TestOpenAccountCreditLimit(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoicePerRental, func(in *model.Input) { in.OpenAccount, in.CreditLimit = bp(true), mp("5000") })
	start := time.Now().Add(3 * time.Hour).Truncate(time.Minute)
	sale := func(i int) (resmodel.Reservation, error) {
		from := start.Add(time.Duration(i*5) * rentaltime.Day)
		f.vehicle()
		return f.sell(&a, from, from.Add(2*rentaltime.Day), nil)
	}
	r1, err := sale(0)
	f.must(err)
	if r1.Status != resmodel.StatusConfirmed || !r1.OpenAccount || !r1.Price.Prepayment.IsZero() || r1.Agreement == nil {
		t.Fatalf("open-account sale = %s open %v prepayment %s", r1.Status, r1.OpenAccount, r1.Price.Prepayment)
	}
	if _, err := sale(1); err != nil {
		t.Fatal(err)
	}
	// 2 000 + 2 000 pending; a third 2 000 would make 6 000 > 5 000.
	_, err = sale(2)
	var cl *resusecase.CreditLimitError
	if !errors.As(err, &cl) || cl.Exposure.String() != "6000.00" || cl.Limit.String() != "5000.00" {
		t.Fatalf("over the limit = %v", err)
	}
	// Without the open account the sale follows the prepayment flow.
	from := start.Add(10 * rentaltime.Day)
	r3, err := f.sell(&a, from, from.Add(2*rentaltime.Day), func(in *resmodel.CreateInput) { in.UseOpenAccount = bp(false) })
	f.must(err)
	if r3.Status != resmodel.StatusAwaitingPayment || r3.OpenAccount {
		t.Fatalf("prepayment flow = %s open %v", r3.Status, r3.OpenAccount)
	}
	got, err := f.svc.Get(f.ctx, f.org, a.UUID)
	f.must(err)
	if got.Exposure == nil || got.Exposure.Pending.String() != "6000.00" || got.Exposure.Available.String() != "-1000.00" {
		t.Fatalf("exposure = %+v", got.Exposure)
	}
}

// TestAgreementSaleChecks: the agreement's customer must be the customer or
// the invoice addressee (filled in for an employee), only permitted drivers
// drive, a suspended agreement does not sell.
func TestAgreementSaleChecks(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoicePerRental, nil)
	a, _, err := f.svc.AddDriver(f.ctx, f.org, a.UUID, model.DriverInput{DriverUUID: &f.driver.uuid})
	f.must(err)
	start := time.Now().Add(3 * time.Hour).Truncate(time.Minute)
	q, err := f.quote(&a, start, start.Add(2*rentaltime.Day), "")
	f.must(err)
	_, err = f.res.Create(f.ctx, f.org, resmodel.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &f.employee.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.empDrv.uuid}}})
	var ve *resusecase.ValidationError
	if !errors.As(err, &ve) || ve.Details[0].Code != "not_permitted" {
		t.Fatalf("unlisted driver = %v", err)
	}
	f.vehicle()
	res, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &f.employee.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}}})
	f.must(err)
	if res.InvoiceCustomer == nil || res.InvoiceCustomer.UUID != f.company.uuid {
		t.Fatalf("invoice addressee = %+v", res.InvoiceCustomer)
	}
	_, _, err = f.svc.SetStatus(f.ctx, f.org, a.UUID, model.StatusInput{Status: model.StatusSuspended})
	f.must(err)
	if _, err := f.quote(&a, start, start.Add(2*rentaltime.Day), ""); !errors.As(err, &ve) || ve.Details[0].Code != "agreement_inactive" {
		t.Fatalf("suspended agreement = %v", err)
	}
	rows, err := f.svc.Rentals(f.ctx, f.org, a.UUID)
	f.must(err)
	if len(rows) != 1 || rows[0].ReservationUUID != res.UUID {
		t.Fatalf("agreement rentals = %+v", rows)
	}
}

// TestRT_A24_MonthlyAccrualNeverTwice (RT-A24): the handover accrues no
// whole rent; each period is accrued once — running the job twice, eight
// runs at once and a run after an extension post nothing twice, and the
// periods add up to the contract total.
func TestRT_A24_MonthlyAccrualNeverTwice(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoiceMonthly, nil)
	f.tariff(a, "500")
	r, res := f.longTerm(&a, 75)
	start, end, total := f.reservationRange(res.UUID)

	// At the handover: the plan exists, the first period is due today
	// (Istanbul day of the pick-up), no whole-rent accrual.
	plan := f.periods(r.UUID)
	want := pricing.PeriodPlan(start, end, total)
	if len(plan) != len(want) {
		t.Fatalf("plan = %+v, want %d periods", plan, len(want))
	}
	for i := range want {
		if plan[i].dayFrom != want[i].FromDay || plan[i].dayTo != want[i].ToDay || !plan[i].amount.Equal(want[i].Amount) {
			t.Fatalf("period %d = %+v want %+v", i+1, plan[i], want[i])
		}
	}
	if !sum(plan).Equal(total) {
		t.Fatalf("plan sum %s != total %s", sum(plan), total)
	}
	live, n, whole := f.liveRent(r.UUID)
	if whole != 0 || n > 1 || f.count(`SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND accrual_key = $2`,
		f.org, fmt.Sprintf("rental:%d:rent", f.rentalID(r.UUID))) != 0 {
		t.Fatalf("handover accrued %s in %d entries (%d whole)", live, n, whole)
	}

	// The job, twice, on the second period's first day.
	f.at(start, want[1].FromDay)
	for i := 0; i < 2; i++ {
		_, err := f.fin.AccruePeriods(f.ctx)
		f.must(err)
	}
	live, n, whole = f.liveRent(r.UUID)
	if n != 2 || whole != 0 || !live.Equal(want[0].Amount.Add(want[1].Amount)) {
		t.Fatalf("after two runs: %s in %d entries", live, n)
	}

	// Eight concurrent runs on the third period's first day.
	f.at(start, want[2].FromDay)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.fin.AccruePeriods(f.ctx); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	live, n, _ = f.liveRent(r.UUID)
	if n != 3 || !live.Equal(want[0].Amount.Add(want[1].Amount).Add(want[2].Amount)) {
		t.Fatalf("after concurrent runs: %s in %d entries", live, n)
	}
	if got := f.count(`SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND rental_id = $2 AND kind = 'accrual'`,
		f.org, f.rentalID(r.UUID)); got != 3 {
		t.Fatalf("accrual rows = %d", got)
	}

	// Extension by 30 days: posted periods stay, only what follows is
	// re-planned; the job after the extension posts nothing twice.
	posted := f.periods(r.UUID, "posted")
	_, err := f.rentals.Extend(f.ctx, f.org, r.UUID, rentalmodel.ExtendInput{EndsAt: ptrTime(end.Add(30 * rentaltime.Day))})
	f.must(err)
	_, newEnd, newTotal := f.reservationRange(res.UUID)
	if !newTotal.Equal(total.Add(money.MustParse("15000"))) {
		t.Fatalf("extended total = %s", newTotal)
	}
	after := f.periods(r.UUID, "posted")
	if len(after) != len(posted) {
		t.Fatalf("posted periods changed: %+v -> %+v", posted, after)
	}
	for i := range posted {
		if posted[i] != after[i] {
			t.Fatalf("posted period %d changed: %+v -> %+v", i+1, posted[i], after[i])
		}
	}
	if f.count(`SELECT COUNT(*) FROM rental_billing_periods bp JOIN rentals r ON r.organization_id = bp.organization_id AND r.id = bp.rental_id
		WHERE bp.organization_id = $1 AND r.uuid = $2 AND bp.status IN ('superseded', 'reversed')`, f.org, r.UUID) != 0 {
		t.Fatal("an extension superseded a posted period")
	}
	full := f.periods(r.UUID)
	if !sum(full).Equal(newTotal) {
		t.Fatalf("plan sum after extension %s != total %s", sum(full), newTotal)
	}
	// Run to the end of the extended rental, twice.
	rng, _ := rentaltime.NewRange(start, newEnd)
	f.at(start, rng.BillingDays())
	for i := 0; i < 2; i++ {
		_, err := f.fin.AccruePeriods(f.ctx)
		f.must(err)
	}
	live, n, whole = f.liveRent(r.UUID)
	if !live.Equal(newTotal) || n != len(full) || whole != 0 {
		t.Fatalf("at the end: %s in %d entries (%d periods, total %s)", live, n, len(full), newTotal)
	}
	keys := f.count(`SELECT COUNT(DISTINCT accrual_key) FROM ledger_entries WHERE organization_id = $1 AND rental_id = $2 AND kind = 'accrual'`,
		f.org, f.rentalID(r.UUID))
	if keys != len(full) {
		t.Fatalf("distinct period keys = %d, periods %d", keys, len(full))
	}
	// The agreement's billing view exposes each posted period's entry.
	bill, err := f.svc.Billing(f.ctx, f.org, a.UUID, nil)
	f.must(err)
	for _, p := range bill.Periods {
		if p.Status == "posted" && p.EntryUUID == nil {
			t.Fatalf("posted period without entry: %+v", p)
		}
	}
	if !bill.Posted.Equal(newTotal) || !bill.Planned.IsZero() {
		t.Fatalf("billing totals posted %s planned %s", bill.Posted, bill.Planned)
	}
	st, err := f.svc.Statement(f.ctx, f.org, a.UUID)
	f.must(err)
	if !st.Balance.Equal(newTotal) {
		t.Fatalf("statement balance %s", st.Balance)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestEarlyTerminationRecomputesFuture: an early return cuts the plan at the
// return; posted periods after it are reversed, the period containing it is
// superseded by a prorated revision; nothing posted is deleted.
func TestEarlyTerminationRecomputesFuture(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoiceMonthly, nil)
	r, res := f.longTerm(&a, 70)
	start, end, total := f.reservationRange(res.UUID)
	want := pricing.PeriodPlan(start, end, total)
	if len(want) < 3 {
		t.Fatalf("plan too short: %+v", want)
	}
	// Post the first three periods (the clock runs ahead of the return).
	f.at(start, want[2].FromDay)
	_, err := f.fin.AccruePeriods(f.ctx)
	f.must(err)
	if got := len(f.periods(r.UUID, "posted")); got != 3 {
		t.Fatalf("posted = %d", got)
	}
	rowsBefore := f.count(`SELECT COUNT(*) FROM rental_billing_periods WHERE organization_id = $1 AND rental_id = $2`, f.org, f.rentalID(r.UUID))

	// The vehicle comes back now: billing stops after the first billing day.
	km, fuel := int32(1200), int16(70)
	_, err = f.rentals.Return(f.ctx, f.org, r.UUID, rentalmodel.ReturnInput{Km: &km, Fuel: &fuel})
	f.must(err)
	kept := pricing.Truncate(want, start, 1)
	live, n, _ := f.liveRent(r.UUID)
	if !live.Equal(kept[0].Amount) || n != 1 {
		t.Fatalf("after the early return: %s in %d entries, want %s", live, n, kept[0].Amount)
	}
	if f.count(`SELECT COUNT(*) FROM rental_billing_periods WHERE organization_id = $1 AND rental_id = $2 AND status = 'reversed'`,
		f.org, f.rentalID(r.UUID)) != 2 {
		t.Fatal("periods 2 and 3 are not reversed")
	}
	if want[0].Days > 1 {
		if f.count(`SELECT COUNT(*) FROM rental_billing_periods WHERE organization_id = $1 AND rental_id = $2 AND status = 'superseded' AND period_no = 1`,
			f.org, f.rentalID(r.UUID)) != 1 {
			t.Fatal("period 1 is not superseded")
		}
		cut := f.periods(r.UUID, "posted")
		if len(cut) != 1 || cut[0].revision != 2 || cut[0].dayTo != 1 || !cut[0].amount.Equal(pricing.Prorate(want[0].Amount, 1, want[0].Days)) {
			t.Fatalf("cut period = %+v", cut)
		}
	}
	if after := f.count(`SELECT COUNT(*) FROM rental_billing_periods WHERE organization_id = $1 AND rental_id = $2 AND status <> 'planned'`,
		f.org, f.rentalID(r.UUID)); after < 3 {
		t.Fatalf("posted rows disappeared: %d (had %d rows)", after, rowsBefore)
	}
	// Posted history cannot be rewritten or deleted.
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM rental_billing_periods WHERE organization_id = $1 AND rental_id = $2 AND status <> 'planned'`,
		f.org, f.rentalID(r.UUID)); err == nil {
		t.Fatal("deleted a posted period")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE rental_billing_periods SET amount = 1 WHERE organization_id = $1 AND rental_id = $2 AND status = 'posted'`,
		f.org, f.rentalID(r.UUID)); err == nil {
		t.Fatal("edited a posted period")
	}
	// Running the job again changes nothing.
	_, err = f.fin.AccruePeriods(f.ctx)
	f.must(err)
	if again, m, _ := f.liveRent(r.UUID); !again.Equal(live) || m != n {
		t.Fatalf("job after the return: %s in %d", again, m)
	}
}

// TestVehicleSwapKeepsAgreementRunning: a swap returns the old vehicle and
// hands over the new one as separate legs; the rental, its agreement and
// its billing plan continue.
func TestVehicleSwapKeepsAgreementRunning(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoiceMonthly, nil)
	r, res := f.longTerm(&a, 60)
	start, end, total := f.reservationRange(res.UUID)
	nv := f.vehicle()
	reason := "periodic maintenance"
	inKm, outKm, fuel := int32(1500), int32(5000), int16(90)
	sw, err := f.rentals.Swap(f.ctx, f.org, r.UUID, rentalmodel.SwapInput{
		VehicleUUID: &nv.uuid, InKm: &inKm, InFuel: &fuel, OutKm: &outKm, OutFuel: &fuel, Reason: reason,
	})
	f.must(err)
	if sw.Status != rentalmodel.StatusActive || f.count(`SELECT COUNT(*) FROM rental_vehicles WHERE organization_id = $1 AND rental_id = $2`,
		f.org, f.rentalID(r.UUID)) != 2 {
		t.Fatalf("swap = %s", sw.Status)
	}
	got, err := f.res.Get(f.ctx, f.org, res.UUID)
	f.must(err)
	if got.Agreement == nil || got.Agreement.UUID != a.UUID {
		t.Fatalf("reservation agreement after swap = %+v", got.Agreement)
	}
	rng, _ := rentaltime.NewRange(start, end)
	f.at(start, rng.BillingDays())
	_, err = f.fin.AccruePeriods(f.ctx)
	f.must(err)
	live, n, whole := f.liveRent(r.UUID)
	if !live.Equal(total) || whole != 0 || n != len(pricing.PeriodPlan(start, end, total)) {
		t.Fatalf("after the swap: %s in %d entries", live, n)
	}
	rows, err := f.svc.Rentals(f.ctx, f.org, a.UUID)
	f.must(err)
	if len(rows) != 1 || rows[0].Rental == nil || rows[0].Rental.RentBilling != "periodic" || rows[0].VehicleUUID == nil || *rows[0].VehicleUUID != nv.uuid {
		t.Fatalf("agreement rentals after swap = %+v", rows)
	}
}

// TestHandoverAndPeriodicAccrualExclusive: the database refuses a whole-rent
// accrual on a periodic rental, a period accrual on a handover rental, and
// a billing mode change while rent is accrued.
func TestHandoverAndPeriodicAccrualExclusive(t *testing.T) {
	f := newFx(t)
	monthly := f.agreement(model.InvoiceMonthly, nil)
	perRental := f.agreement(model.InvoicePerRental, nil)
	periodic, _ := f.longTerm(&monthly, 40)
	handover, hres := f.longTerm(&perRental, 5)
	ctx := context.Background()
	pid, hid := f.rentalID(periodic.UUID), f.rentalID(handover.UUID)

	// Service side: the per-rental agreement rental accrued its whole rent
	// once, with no billing plan; the periodic one has no whole-rent accrual.
	_, _, total := f.reservationRange(hres.UUID)
	live, n, whole := f.liveRent(handover.UUID)
	if !live.Equal(total) || n != 1 || whole != 1 || len(f.periods(handover.UUID)) != 0 {
		t.Fatalf("handover rental: %s in %d (%d whole), %d periods", live, n, whole, len(f.periods(handover.UUID)))
	}
	if _, _, w := f.liveRent(periodic.UUID); w != 0 {
		t.Fatal("periodic rental has a whole-rent accrual")
	}

	var account int64
	f.must(f.pool.QueryRow(ctx, `SELECT id FROM accounts WHERE organization_id = $1 AND customer_id = $2`, f.org, f.company.id).Scan(&account))
	insert := func(rental int64, key string, period any) error {
		_, err := f.pool.Exec(ctx, `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, purpose, amount, entry_date, due_date,
			source_type, rental_id, accrual_key, billing_period_id) VALUES ($1, $2, 'accrual', 'debit', 'rent', 10, CURRENT_DATE, CURRENT_DATE,
			'rental', $3, $4, $5)`, f.org, account, rental, key, period)
		return err
	}
	if err := insert(pid, fmt.Sprintf("rental:%d:rent", pid), nil); err == nil || !strings.Contains(err.Error(), "billed per period") {
		t.Fatalf("whole rent on a periodic rental = %v", err)
	}
	var foreign int64
	f.must(f.pool.QueryRow(ctx, `SELECT id FROM rental_billing_periods WHERE organization_id = $1 AND rental_id = $2 ORDER BY id LIMIT 1`, f.org, pid).Scan(&foreign))
	if err := insert(hid, fmt.Sprintf("rental:%d:p1", hid), foreign); err == nil {
		t.Fatal("period accrual on a handover rental accepted")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE rentals SET rent_billing = 'handover' WHERE organization_id = $1 AND id = $2`, f.org, pid); err == nil {
		t.Fatal("periodic rental switched to handover with live accruals")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE rentals SET rent_billing = 'periodic' WHERE organization_id = $1 AND id = $2`, f.org, hid); err == nil {
		t.Fatal("handover rental switched to periodic with a live accrual")
	}
	// SyncRental again (an extension, a charge decision) keeps the modes.
	f.must(f.fin.SyncRental(f.ctx, nil, f.org, pid))
	f.must(f.fin.SyncRental(f.ctx, nil, f.org, hid))
	if _, n2, w2 := f.liveRent(handover.UUID); n2 != 1 || w2 != 1 {
		t.Fatal("handover rental changed")
	}
}

// TestBranchScopeAndIsolation: a branch agreement is invisible outside the
// branch scope; another organization's agreement is not found anywhere
// (agreements API, quotes, rate plans).
func TestBranchScopeAndIsolation(t *testing.T) {
	f := newFx(t)
	a := f.agreement(model.InvoicePerRental, func(in *model.Input) { in.BranchUUID = &f.other.uuid })
	head := f.agreement(model.InvoicePerRental, nil)
	scoped := f.scoped([]int64{f.branch.id})
	if _, err := f.svc.Get(scoped, f.org, a.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("branch agreement outside the scope = %v", err)
	}
	items, total, err := f.svc.List(scoped, f.org, model.Filter{})
	f.must(err)
	if total != 1 || items[0].UUID != head.UUID {
		t.Fatalf("scoped list = %+v", items)
	}
	if _, err := f.svc.Create(scoped, f.org, model.Input{
		CustomerUUID: &f.company.uuid, Name: sp("x"), ValidFrom: &rentaltime.Date{Year: 2026, Month: 1, Day: 1},
	}); err == nil {
		t.Fatal("a branch-scoped member opened a head office agreement")
	}

	g := newFx(t)
	other := g.agreement(model.InvoicePerRental, nil)
	if _, err := f.svc.Get(f.ctx, f.org, other.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign agreement = %v", err)
	}
	for _, call := range []func() error{
		func() error {
			_, _, err := f.svc.Update(f.ctx, f.org, other.UUID, model.Input{Name: sp("x")})
			return err
		},
		func() error { _, err := f.svc.Billing(f.ctx, f.org, other.UUID, nil); return err },
		func() error { _, err := f.svc.Statement(f.ctx, f.org, other.UUID); return err },
		func() error { _, err := f.svc.Rentals(f.ctx, f.org, other.UUID); return err },
	} {
		if err := call(); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("foreign agreement call = %v", err)
		}
	}
	start := time.Now().Add(3 * time.Hour).Truncate(time.Minute)
	var ve *resusecase.ValidationError
	if _, err := f.quote(&other, start, start.Add(rentaltime.Day), ""); !errors.As(err, &ve) || ve.Details[0].Field != "agreement_uuid" {
		t.Fatalf("quote with a foreign agreement = %v", err)
	}
	_, err = f.pricing.CreatePlan(f.ctx, f.org, pricingmodel.RatePlanCreateInput{
		RentalProductUUID: &f.product.uuid, Scope: pricing.ScopeAgreement, AgreementUUID: &other.UUID,
		Bands: []pricingmodel.BandInput{{MinDays: 1, DailyPrice: "1"}},
	})
	var pve *pricingusecase.ValidationError
	if !errors.As(err, &pve) || pve.Details[0].Field != "agreement_uuid" {
		t.Fatalf("tariff for a foreign agreement = %v", err)
	}
	if _, _, err := f.svc.AddDriver(f.ctx, f.org, head.UUID, model.DriverInput{DriverUUID: &g.driver.uuid}); !errors.As(err, new(*usecase.ValidationError)) {
		t.Fatalf("foreign driver = %v", err)
	}
	if _, err := f.svc.Create(f.ctx, f.org, model.Input{
		CustomerUUID: &g.company.uuid, Name: sp("x"), ValidFrom: &rentaltime.Date{Year: 2026, Month: 1, Day: 1},
	}); !errors.As(err, new(*usecase.ValidationError)) {
		t.Fatalf("foreign customer = %v", err)
	}
}
