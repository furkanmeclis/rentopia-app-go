package profitability_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The fixture (March 2026, Europe/Istanbul) — every figure below is
// asserted to the kuruş for both bases and the three allocation methods.
//
//	Branches  B1 (MRK), B2 (HVL). Vehicles V1, V2 (home B1), V3 (home B2),
//	          in service since 2026-01-01. Rental group G1 = {V1}.
//	R1        6 000.00 sold (600.00 of it an extra), channel commission
//	          300.00. Handed over on V1 03-02 10:00, swapped to V2 03-06
//	          10:00, returned 03-08 10:00 (4 days V1, 2 days V2).
//	          Accruals: rent 6 000, fuel 300, late fee 200 (reversed 03-09),
//	          toll 47.50 (pass-through) + toll fee 10.00.
//	          Cash: 2 000 at the desk (03-02, rental link), 4 657.50 online
//	          (03-08, reservation link only, commission 120.00 → expense of
//	          category payment_commission, left out of the overhead),
//	          500.00 refunded (03-20). Deposit: 2 000 held 03-02, returned
//	          03-08.
//	R2        V1 03-15 → 03-18, 3 000.00 accrued, 3 000.00 collected 03-18.
//	R3        V3 03-10 → 03-12, 2 000.00 accrued, nothing collected.
//	Costs     V1 repair 450.00 (03-12); V2 supplier invoice 1 200.00
//	          (03-14), paid 800.00 on 03-25; V2 maintenance 250.00 (03-20);
//	          V3 transfer 150.00 (03-05); V3 procurement 100 000.00 (capex);
//	          a void expense and an April expense (ignored).
//	Overhead  B1 operations 900.00 (03-31), head office insurance 600.00
//	          (03-01).
//	Blocks    V2 maintenance block 03-20 → 03-25 (5 days).

type fx struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	svc     *usecase.Service
	org     int64
	orgUUID uuid.UUID
	slug    string
	n       int
	cat     map[string]int64
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var clock = time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)

func ist(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 0, 0, 0, rentaltime.Location()).UTC()
}

func newFx(t *testing.T, label string) *fx {
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
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: label + "-" + suffix, Name: label + " " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, ctx: ctx, pool: pool, q: q, org: org.ID, orgUUID: org.Uuid, slug: org.Slug, cat: map[string]int64{}}
	t.Cleanup(func() {
		// Ledger rows are append-only: only the organization cascade
		// removes them.
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.svc = usecase.New(q)
	f.svc.SetClock(func() time.Time { return clock })
	f.must(financeusecase.SeedCategories(ctx, q, f.org))
	rows, err := pool.Query(ctx, `SELECT code, id FROM expense_categories WHERE organization_id = $1`, f.org)
	f.must(err)
	for rows.Next() {
		var code string
		var id int64
		f.must(rows.Scan(&code, &id))
		f.cat[code] = id
	}
	rows.Close()
	return f
}

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) scope(branches []int64) context.Context {
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: f.slug, Status: orgctx.StatusActive,
		Permissions: []string{rbac.PermTenantFinanceReportsRead, rbac.PermTenantReportExport}, BranchIDs: branches,
	})
}

func (f *fx) row(sql string, args ...any) ref {
	f.t.Helper()
	var r ref
	f.must(f.pool.QueryRow(f.ctx, sql+" RETURNING id, uuid", args...).Scan(&r.id, &r.uuid))
	return r
}

func (f *fx) id(sql string, args ...any) int64 {
	f.t.Helper()
	var id int64
	f.must(f.pool.QueryRow(f.ctx, sql+" RETURNING id", args...).Scan(&id))
	return id
}

func (f *fx) exec(sql string, args ...any) {
	f.t.Helper()
	_, err := f.pool.Exec(f.ctx, sql, args...)
	f.must(err)
}

func (f *fx) branch(code string) ref {
	return f.row(`INSERT INTO branches (organization_id, code, name, working_hours, holiday_exceptions, prep_minutes, is_active)
		VALUES ($1, $2, $3, '{}', '[]', 30, TRUE)`, f.org, code, code+" Şube")
}

func (f *fx) vehicle(plate string, home ref) ref {
	return f.row(`INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km, in_service_date)
		VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000, '2026-01-01')`, f.org, plate, home.id)
}

// world is the seeded organization.
type world struct {
	b1, b2     ref
	v1, v2, v3 ref
	g1         ref
	r1, r2, r3 ref
}

type sale struct {
	vehicle  ref
	branch   ref
	starts   time.Time
	ends     time.Time
	total    string
	extras   string
	channel  string
	snapshot string
}

func (f *fx) seed() world {
	t := f.t
	t.Helper()
	var w world
	w.b1, w.b2 = f.branch("MRK"), f.branch("HVL")
	w.v1, w.v2, w.v3 = f.vehicle("34 PRF 1", w.b1), f.vehicle("34 PRF 2", w.b1), f.vehicle("06 PRF 3", w.b2)
	segment := f.id(`INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi')`, f.org)
	w.g1 = f.row(`INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, 'G1', 'Grup 1', $2)`, f.org, segment)
	f.exec(`INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id) VALUES ($1, $2, $3)`, f.org, w.v1.id, w.g1.id)
	product := f.id(`INSERT INTO rental_products (organization_id, code, title, segment_id, status) VALUES ($1, 'EKO', 'Ekonomi', $2, 'published')`,
		f.org, segment)
	f.exec(`INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, f.org, product, w.g1.id)
	customer := f.id(`INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Ayşe', 'Yılmaz', '+905321112233', 'ayse@example.test')`, f.org)
	account := f.id(`INSERT INTO accounts (organization_id, code, party_kind, customer_id, name) VALUES ($1, 'CR-000001', 'customer', $2, 'Ayşe Yılmaz')`,
		f.org, customer)
	till := f.id(`INSERT INTO cash_accounts (organization_id, code, name, kind, branch_id, opening_balance) VALUES ($1, 'KASA', 'Kasa', 'cash', $2, 100000)`,
		f.org, w.b1.id)

	rental := func(s sale) (ref, int64) {
		f.n++
		res := f.id(`INSERT INTO reservations (organization_id, reference, status, pickup_branch_id, return_branch_id, starts_at, ends_at,
			rental_product_id, product_version, sold_rental_product_id, sold_product_version, guarantee_type, product_snapshot, sold_snapshot,
			customer_id, channel, collecting_party, commission_amount, currency, total_amount, quote_snapshot)
			VALUES ($1, $2, 'confirmed', $3, $3, $4, $5, $6, 1, $6, 1, 'GROUP', '{}', '{}', $7, 'branch', 'us', $8, 'TRY', $9, $10)`,
			f.org, fmt.Sprintf("PRF-%d", f.n), s.branch.id, s.starts, s.ends, product, customer, s.channel, s.total, s.snapshot)
		r := f.row(`INSERT INTO rentals (organization_id, reference, reservation_id, status, vehicle_id, actual_pickup_at, actual_return_at)
			VALUES ($1, $2, $3, 'returned', $4, $5, $6)`, f.org, fmt.Sprintf("PRF-%d", f.n), res, s.vehicle.id, s.starts, s.ends)
		return r, res
	}
	leg := func(r ref, v ref, out, in time.Time, reason string) {
		f.exec(`INSERT INTO rental_vehicles (organization_id, rental_id, vehicle_id, reason, out_at, in_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			f.org, r.id, v.id, reason, out, in)
	}
	entry := func(kind, direction, purpose, amount, day, source string, extra map[string]any) int64 {
		cols := []string{"organization_id", "account_id", "kind", "direction", "purpose", "amount", "entry_date", "due_date", "source_type"}
		args := []any{f.org, account, kind, direction, purpose, amount, day, day, source}
		for k, v := range extra {
			cols = append(cols, k)
			args = append(args, v)
		}
		ph := make([]string, len(args))
		for i := range args {
			ph[i] = fmt.Sprintf("$%d", i+1)
		}
		return f.id("INSERT INTO ledger_entries ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(ph, ", ")+")", args...)
	}
	charge := func(r ref, kind, source, amount string) int64 {
		return f.id(`INSERT INTO rental_charges (organization_id, rental_id, kind, source, status, amount, decided_at, reason)
			VALUES ($1, $2, $3, $4, 'approved', $5, $6, 'fixture')`, f.org, r.id, kind, source, amount, ist(3, 8, 12))
	}
	snapshot := func(day, extra string) string {
		lines := `[{"kind":"day","amount":"` + day + `"}`
		if extra != "" {
			lines += `,{"kind":"extra","amount":"` + extra + `"}`
		}
		return `{"lines":` + lines + `],"extensions":null}`
	}

	// R1: swap V1 → V2.
	var res1 int64
	w.r1, res1 = rental(sale{vehicle: w.v2, branch: w.b1, starts: ist(3, 2, 10), ends: ist(3, 8, 10), total: "6000.00", channel: "300.00",
		snapshot: snapshot("5400.00", "600.00")})
	leg(w.r1, w.v1, ist(3, 2, 10), ist(3, 6, 10), "handover")
	leg(w.r1, w.v2, ist(3, 6, 10), ist(3, 8, 10), "swap")
	r1 := map[string]any{"rental_id": w.r1.id, "reservation_id": res1}
	entry("accrual", "debit", "rent", "6000.00", "2026-03-02", "rental", map[string]any{"rental_id": w.r1.id, "accrual_key": "rental:r1:rent"})
	for i, c := range []struct{ kind, source, amount string }{
		{"fuel", "manual", "300.00"}, {"late_return", "manual", "200.00"}, {"toll", "toll", "47.50"}, {"toll_fee", "toll", "10.00"},
	} {
		ch := charge(w.r1, c.kind, c.source, c.amount)
		e := entry("accrual", "debit", "charge", c.amount, "2026-03-08", "rental_charge", map[string]any{
			"rental_id": w.r1.id, "rental_charge_id": ch, "accrual_key": fmt.Sprintf("rental_charge:%d", ch),
		})
		if i == 1 { // the late fee is no longer due
			entry("reversal", "credit", "charge", c.amount, "2026-03-09", "rental_charge", map[string]any{
				"rental_id": w.r1.id, "rental_charge_id": ch, "reverses_entry_id": e, "reason": "system:no_longer_due",
			})
		}
	}
	desk := entry("collection", "credit", "general", "2000.00", "2026-03-02", "collection", r1)
	deskDoc := f.id(`INSERT INTO collections (organization_id, code, account_id, method, provider, amount, gross_amount, net_amount, refunded_amount,
		status, collection_date, branch_id, cash_account_id, rental_id, reservation_id, ledger_entry_id)
		VALUES ($1, 'TH-000001', $2, 'cash', 'manual', 2000, 2000, 2000, 500, 'partially_refunded', '2026-03-02', $3, $4, $5, $6, $7)`,
		f.org, account, w.b1.id, till, w.r1.id, res1, desk)
	commission := f.id(`INSERT INTO expenses (organization_id, category_id, expense_date, net_amount, total_amount, branch_id, description)
		VALUES ($1, $2, '2026-03-08', 120, 120, $3, 'iyzico komisyon')`, f.org, f.cat["payment_commission"], w.b1.id)
	online := entry("collection", "credit", "general", "4657.50", "2026-03-08", "payment", map[string]any{"reservation_id": res1})
	f.exec(`INSERT INTO collections (organization_id, code, account_id, method, provider, amount, gross_amount, commission_amount, net_amount,
		collection_date, reservation_id, ledger_entry_id, commission_expense_id, settlement_status, settled_at)
		VALUES ($1, 'TH-000002', $2, 'online', 'iyzico', 4657.50, 4657.50, 120, 4537.50, '2026-03-08', $3, $4, $5, 'settled', $6)`,
		f.org, account, res1, online, commission, ist(3, 8, 12))
	entry("refund", "debit", "general", "500.00", "2026-03-20", "collection", map[string]any{"collection_id": deskDoc})
	deposit := f.id(`INSERT INTO security_deposits (organization_id, rental_id, reservation_id, customer_id, account_id, branch_id, amount, method,
		status, cash_account_id, returned_amount, held_at, resolved_at)
		VALUES ($1, $2, $3, $4, $5, $6, 2000, 'cash', 'refunded', $7, 2000, $8, $9)`,
		f.org, w.r1.id, res1, customer, account, w.b1.id, till, ist(3, 2, 10), ist(3, 8, 10))
	entry("deposit_hold", "credit", "deposit", "2000.00", "2026-03-02", "security_deposit", map[string]any{"rental_id": w.r1.id, "security_deposit_id": deposit})
	entry("deposit_return", "debit", "deposit", "2000.00", "2026-03-08", "security_deposit", map[string]any{"rental_id": w.r1.id, "security_deposit_id": deposit})

	// R2: V1, paid in full. R3: V3, unpaid.
	var res2, res3 int64
	w.r2, res2 = rental(sale{vehicle: w.v1, branch: w.b1, starts: ist(3, 15, 10), ends: ist(3, 18, 10), total: "3000.00", channel: "0",
		snapshot: snapshot("3000.00", "")})
	leg(w.r2, w.v1, ist(3, 15, 10), ist(3, 18, 10), "handover")
	entry("accrual", "debit", "rent", "3000.00", "2026-03-15", "rental", map[string]any{"rental_id": w.r2.id, "accrual_key": "rental:r2:rent"})
	cash2 := entry("collection", "credit", "general", "3000.00", "2026-03-18", "collection", map[string]any{"rental_id": w.r2.id})
	f.exec(`INSERT INTO collections (organization_id, code, account_id, method, amount, gross_amount, net_amount, collection_date, rental_id,
		reservation_id, ledger_entry_id) VALUES ($1, 'TH-000003', $2, 'cash', 3000, 3000, 3000, '2026-03-18', $3, $4, $5)`,
		f.org, account, w.r2.id, res2, cash2)
	w.r3, res3 = rental(sale{vehicle: w.v3, branch: w.b2, starts: ist(3, 10, 10), ends: ist(3, 12, 10), total: "2000.00", channel: "0",
		snapshot: snapshot("2000.00", "")})
	_ = res3
	leg(w.r3, w.v3, ist(3, 10, 10), ist(3, 12, 10), "handover")
	entry("accrual", "debit", "rent", "2000.00", "2026-03-10", "rental", map[string]any{"rental_id": w.r3.id, "accrual_key": "rental:r3:rent"})

	// Costs.
	expense := func(cat string, day string, total string, branch, vehicle *ref, status string) {
		var b, v any
		if branch != nil {
			b = branch.id
		}
		if vehicle != nil {
			v = vehicle.id
		}
		voided := any(nil)
		if status == "void" {
			voided = clock
		}
		f.exec(`INSERT INTO expenses (organization_id, category_id, expense_date, net_amount, total_amount, branch_id, vehicle_id, status, voided_at)
			VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8)`, f.org, f.cat[cat], day, total, b, v, status, voided)
	}
	expense("repair", "2026-03-12", "450.00", &w.b1, &w.v1, "recorded")
	expense("repair", "2026-03-10", "999.00", &w.b1, &w.v1, "void")
	expense("repair", "2026-04-02", "777.00", &w.b1, &w.v1, "recorded")
	expense("vehicle_procurement", "2026-03-03", "100000.00", &w.b2, &w.v3, "recorded")
	expense("operations", "2026-03-31", "900.00", &w.b1, nil, "recorded")
	expense("insurance", "2026-03-01", "600.00", nil, nil, "recorded")

	supplier := f.id(`INSERT INTO suppliers (organization_id, code, title) VALUES ($1, 'TED-000001', 'Servis A.Ş.')`, f.org)
	supAcc := f.id(`INSERT INTO accounts (organization_id, code, party_kind, supplier_id, name) VALUES ($1, 'TD-000001', 'supplier', $2, 'Servis A.Ş.')`,
		f.org, supplier)
	invoice := f.id(`INSERT INTO supplier_invoices (organization_id, supplier_id, account_id, invoice_no, invoice_date, due_date, category_id,
		branch_id, vehicle_id, net_amount, vat_amount, total_amount, paid_amount, status)
		VALUES ($1, $2, $3, 'F-1', '2026-03-14', '2026-04-14', $4, $5, $6, 1000, 200, 1200, 800, 'partially_paid')`,
		f.org, supplier, supAcc, f.cat["maintenance"], w.b1.id, w.v2.id)
	f.exec(`INSERT INTO expenses (organization_id, category_id, expense_date, net_amount, vat_rate, vat_amount, total_amount, branch_id, vehicle_id,
		supplier_id, supplier_invoice_id) VALUES ($1, $2, '2026-03-14', 1000, 20, 200, 1200, $3, $4, $5, $6)`,
		f.org, f.cat["maintenance"], w.b1.id, w.v2.id, supplier, invoice)
	payment := f.id(`INSERT INTO supplier_payments (organization_id, supplier_id, account_id, cash_account_id, amount, payment_date, branch_id)
		VALUES ($1, $2, $3, $4, 800, '2026-03-25', $5)`, f.org, supplier, supAcc, till, w.b1.id)
	f.exec(`INSERT INTO supplier_payment_allocations (organization_id, payment_id, invoice_id, amount) VALUES ($1, $2, $3, 800)`, f.org, payment, invoice)

	f.exec(`INSERT INTO vehicle_maintenance (organization_id, vehicle_id, kind, maintenance_type, done_date, cost) VALUES
		($1, $2, 'done', 'periodic', '2026-03-20', 250), ($1, $2, 'done', 'oil', '2026-03-21', NULL)`, f.org, w.v2.id)
	f.exec(`INSERT INTO vehicle_transfers (organization_id, vehicle_id, from_branch_id, to_branch_id, planned, actual_departure, actual_arrival, cost, status)
		VALUES ($1, $2, $3, $4, tstzrange($5, $6, '[)'), $5, $6, 150, 'completed')`, f.org, w.v3.id, w.b2.id, w.b1.id, ist(3, 5, 9), ist(3, 5, 12))
	f.exec(`INSERT INTO vehicle_blocks (organization_id, vehicle_id, kind, during, reason)
		VALUES ($1, $2, 'maintenance', tstzrange($3, $4, '[)'), 'periyodik bakım')`, f.org, w.v2.id, ist(3, 20, 0), ist(3, 25, 0))
	return w
}

func march(basis, allocation string) model.Filter {
	from, to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	return model.Filter{From: &from, To: &to, Basis: basis, Allocation: allocation}
}

// want are the expected figures of one vehicle (kuruş).
type want struct {
	rent, extras, charges, fees, gross, pay, chan_, direct, over, contrib int64
	refunded, held, returned, pass, capex                                 int64
}

func check(t *testing.T, label string, got model.Figures, w want) {
	t.Helper()
	pairs := []struct {
		name string
		got  int64
		want int64
	}{
		{"rent", got.Rent.Minor(), w.rent}, {"extras", got.Extras.Minor(), w.extras}, {"charges", got.Charges.Minor(), w.charges},
		{"toll_fees", got.TollFees.Minor(), w.fees}, {"gross", got.GrossIncome.Minor(), w.gross},
		{"payment_commission", got.PaymentCommission.Minor(), w.pay}, {"channel_commission", got.ChannelCommission.Minor(), w.chan_},
		{"direct_costs", got.DirectCosts.Minor(), w.direct}, {"overhead", got.Overhead.Minor(), w.over},
		{"contribution", got.Contribution.Minor(), w.contrib}, {"refunded", got.Refunded.Minor(), w.refunded},
		{"deposits_held", got.DepositsHeld.Minor(), w.held}, {"deposits_returned", got.DepositsReturned.Minor(), w.returned},
		{"pass_through", got.PassThrough.Minor(), w.pass}, {"capex", got.Capex.Minor(), w.capex},
	}
	for _, p := range pairs {
		if p.got != p.want {
			t.Errorf("%s %s = %d, want %d", label, p.name, p.got, p.want)
		}
	}
}

func rowOf(t *testing.T, rep model.Report, v ref) model.Figures {
	t.Helper()
	for _, r := range rep.Rows {
		if r.Vehicle.UUID == v.uuid {
			return r.Figures
		}
	}
	t.Fatalf("vehicle %s is not in the report", v.uuid)
	return model.Figures{}
}

// Income, commissions and kept-apart classes do not depend on the
// allocation method; the overhead (and so the contribution) does.
var (
	accrualV1 = want{rent: 660000, extras: 40000, charges: 20000, fees: 667, gross: 720667, pay: 8000, chan_: 20000, direct: 45000,
		refunded: 33333, held: 133333, returned: 133333, pass: 3167}
	accrualV2 = want{rent: 180000, extras: 20000, charges: 10000, fees: 333, gross: 210333, pay: 4000, chan_: 10000, direct: 145000,
		refunded: 16667, held: 66667, returned: 66667, pass: 1583}
	accrualV3 = want{rent: 200000, gross: 200000, direct: 15000, capex: 10000000}
	cashV1    = want{rent: 648675, extras: 38741, charges: 19371, fees: 646, gross: 707433, pay: 7748, chan_: 19371, direct: 45000,
		refunded: 33333, held: 133333, returned: 133333, pass: 3067}
	cashV2 = want{rent: 174337, extras: 19371, charges: 9685, fees: 323, gross: 203716, pay: 3874, chan_: 9685, direct: 105000,
		refunded: 16667, held: 66667, returned: 66667, pass: 1534}
	cashV3 = want{direct: 15000, capex: 10000000}
)

func with(w want, over int64) want {
	w.over = over
	w.contrib = w.gross - w.pay - w.chan_ - w.direct - over
	return w
}

func TestProfitabilityBasesAndAllocations(t *testing.T) {
	f := newFx(t, "prf")
	w := f.seed()
	ctx := f.scope(nil)
	cases := []struct {
		basis, allocation string
		v1, v2, v3        want
	}{
		{model.BasisAccrual, model.AllocationRentalDays, with(accrualV1, 108182), with(accrualV2, 30909), with(accrualV3, 10909)},
		{model.BasisAccrual, model.AllocationVehicleCount, with(accrualV1, 65000), with(accrualV2, 65000), with(accrualV3, 20000)},
		{model.BasisAccrual, model.AllocationRevenue, with(accrualV1, 107899), with(accrualV2, 31491), with(accrualV3, 10610)},
		{model.BasisCash, model.AllocationRentalDays, with(cashV1, 108182), with(cashV2, 30909), with(cashV3, 10909)},
		{model.BasisCash, model.AllocationVehicleCount, with(cashV1, 65000), with(cashV2, 65000), with(cashV3, 20000)},
		// V3 has no cash income: under revenue share it carries no
		// head-office overhead.
		{model.BasisCash, model.AllocationRevenue, with(cashV1, 116463), with(cashV2, 33537), with(cashV3, 0)},
	}
	for _, c := range cases {
		label := c.basis + "/" + c.allocation
		rep, err := f.svc.Report(ctx, f.org, march(c.basis, c.allocation))
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if rep.Method.Basis != c.basis || rep.Method.Allocation != c.allocation || rep.From != "2026-03-01" || rep.To != "2026-03-31" ||
			rep.Method.Timezone != "Europe/Istanbul" || !rep.Method.VATInclusive || rep.Method.OverheadScope != model.OverheadAll {
			t.Errorf("%s: method %+v from %s to %s", label, rep.Method, rep.From, rep.To)
		}
		if len(rep.Rows) != 3 {
			t.Fatalf("%s: %d rows", label, len(rep.Rows))
		}
		check(t, label+" V1", rowOf(t, rep, w.v1), c.v1)
		check(t, label+" V2", rowOf(t, rep, w.v2), c.v2)
		check(t, label+" V3", rowOf(t, rep, w.v3), c.v3)
		// The shared overhead is allocated in full (900 + 600).
		if got := rep.Totals.Overhead.Minor(); got != 150000 {
			t.Errorf("%s: total overhead %d, want 150000", label, got)
		}
		var sum int64
		for _, r := range rep.Rows {
			sum += r.Contribution.Minor()
		}
		if rep.Totals.Contribution.Minor() != sum {
			t.Errorf("%s: totals row %d != sum of rows %d", label, rep.Totals.Contribution.Minor(), sum)
		}
		if len(rep.Trend) != 1 || rep.Trend[0].Month != "2026-03" || rep.Trend[0].Contribution.Minor() != sum {
			t.Errorf("%s: trend %+v", label, rep.Trend)
		}
	}
}

func TestProfitabilityOccupancyAndDetail(t *testing.T) {
	f := newFx(t, "prf")
	w := f.seed()
	ctx := f.scope(nil)
	rep, err := f.svc.Report(ctx, f.org, march("", ""))
	if err != nil {
		t.Fatal(err)
	}
	// Defaults: accrual, rental days.
	if rep.Method.Basis != model.BasisAccrual || rep.Method.Allocation != model.AllocationRentalDays {
		t.Fatalf("defaults %+v", rep.Method)
	}
	pct := func(v *float64) float64 {
		if v == nil {
			return -1
		}
		return *v
	}
	for _, c := range []struct {
		v                  ref
		rented, avail, occ float64
		rentals            int
	}{
		// V1: 4 days of R1 + 3 days of R2 over 31 days.
		{w.v1, 7, 31, 22.6, 2},
		// V2: 2 days of R1; the 5-day maintenance block is not available.
		{w.v2, 2, 26, 7.7, 1},
		{w.v3, 2, 31, 6.5, 1},
	} {
		got := rowOf(t, rep, c.v)
		if got.RentalDays != c.rented || got.AvailableDays != c.avail || pct(got.Occupancy) != c.occ || got.RentalCount != c.rentals {
			t.Errorf("%s: days %.2f / %.2f occupancy %.1f rentals %d, want %.2f / %.2f %.1f %d", c.v.uuid,
				got.RentalDays, got.AvailableDays, pct(got.Occupancy), got.RentalCount, c.rented, c.avail, c.occ, c.rentals)
		}
	}
	if rep.Totals.RentalDays != 11 || rep.Totals.AvailableDays != 88 || pct(rep.Totals.Occupancy) != 12.5 || rep.Totals.RentalCount != 4 {
		t.Errorf("totals days %+v", rep.Totals)
	}

	// The vehicle tab: the same figures, the trend and the expense
	// categories of the direct costs.
	d, err := f.svc.Vehicle(ctx, f.org, w.v2.uuid, march(model.BasisAccrual, model.AllocationRentalDays))
	if err != nil {
		t.Fatal(err)
	}
	check(t, "detail V2", d.Figures, with(accrualV2, 30909))
	if d.Figures.Maintenance.Minor() != 25000 || d.Figures.Expenses.Minor() != 120000 {
		t.Errorf("detail V2 maintenance %s expenses %s", d.Figures.Maintenance, d.Figures.Expenses)
	}
	if len(d.Categories) != 1 || d.Categories[0].Code != "maintenance" || d.Categories[0].Amount.Minor() != 120000 {
		t.Errorf("detail V2 categories %+v", d.Categories)
	}
	if len(d.Trend) != 1 || d.Vehicle.Plate != "34 PRF 2" {
		t.Errorf("detail V2 trend %+v vehicle %+v", d.Trend, d.Vehicle)
	}

	// Filters: rental group G1 = V1; home branch B2 = V3; plate search.
	rep, err = f.svc.Report(ctx, f.org, model.Filter{From: march("", "").From, To: march("", "").To, RentalGroupUUIDs: []uuid.UUID{w.g1.uuid}})
	if err != nil || len(rep.Rows) != 1 || rep.Rows[0].Vehicle.UUID != w.v1.uuid {
		t.Errorf("group filter: %v %+v", err, rep.Rows)
	}
	rep, err = f.svc.Report(ctx, f.org, model.Filter{From: march("", "").From, To: march("", "").To, BranchUUIDs: []uuid.UUID{w.b2.uuid}})
	if err != nil || len(rep.Rows) != 1 || rep.Rows[0].Vehicle.UUID != w.v3.uuid {
		t.Errorf("branch filter: %v %+v", err, rep.Rows)
	}
	rep, err = f.svc.Report(ctx, f.org, model.Filter{From: march("", "").From, To: march("", "").To, Q: "34prf2"})
	if err != nil || len(rep.Rows) != 1 || rep.Rows[0].Vehicle.UUID != w.v2.uuid {
		t.Errorf("q filter: %v %+v", err, rep.Rows)
	}

	// A two-month period: February is empty, the trend has both months
	// and the totals equal March.
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	f2 := march(model.BasisAccrual, model.AllocationRentalDays)
	f2.From = &feb
	rep, err = f.svc.Report(ctx, f.org, f2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Trend) != 2 || rep.Trend[0].Month != "2026-02" || rep.Trend[0].GrossIncome.Minor() != 0 ||
		rep.Trend[1].GrossIncome.Minor() != 720667+210333+200000 || rep.Totals.GrossIncome.Minor() != 720667+210333+200000 {
		t.Errorf("two months: %+v totals %s", rep.Trend, rep.Totals.GrossIncome)
	}
	// February: 28 days available each, none rented.
	if got := rowOf(t, rep, w.v1); got.AvailableDays != 59 {
		t.Errorf("V1 available over Feb–Mar = %.2f", got.AvailableDays)
	}

	// Invalid filters.
	var ve *usecase.ValidationError
	if _, err := f.svc.Report(ctx, f.org, model.Filter{Basis: "invoice"}); !errors.As(err, &ve) || ve.Details[0].Field != "basis" {
		t.Errorf("bad basis: %v", err)
	}
	if _, err := f.svc.Report(ctx, f.org, model.Filter{Allocation: "km"}); !errors.As(err, &ve) || ve.Details[0].Field != "allocation" {
		t.Errorf("bad allocation: %v", err)
	}
}

// TestProfitabilityBranchScope: a member of B2 sees V3 only, no head-office
// overhead (stated in the method), never B1's vehicles.
func TestProfitabilityBranchScope(t *testing.T) {
	f := newFx(t, "prf")
	w := f.seed()
	ctx := f.scope([]int64{w.b2.id})
	rep, err := f.svc.Report(ctx, f.org, march(model.BasisAccrual, model.AllocationRentalDays))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.Rows[0].Vehicle.UUID != w.v3.uuid {
		t.Fatalf("rows %+v", rep.Rows)
	}
	if rep.Method.OverheadScope != model.OverheadBranches {
		t.Errorf("overhead scope %s", rep.Method.OverheadScope)
	}
	// B2 has no overhead of its own; the head-office share is left out.
	check(t, "B2 V3", rep.Rows[0].Figures, with(accrualV3, 0))
	if _, err := f.svc.Vehicle(ctx, f.org, w.v1.uuid, march("", "")); !errors.Is(err, usecase.ErrVehicleNotFound) {
		t.Errorf("V1 outside the scope: %v", err)
	}
	var ve *usecase.ValidationError
	if _, err := f.svc.Report(ctx, f.org, model.Filter{BranchUUIDs: []uuid.UUID{w.b1.uuid}}); !errors.As(err, &ve) ||
		ve.Details[0].Field != "branch" || ve.Details[0].Code != "not_found" {
		t.Errorf("branch outside the scope as a filter: %v", err)
	}
	// A member of B1 gets B1's overhead but no head-office share.
	rep, err = f.svc.Report(f.scope([]int64{w.b1.id}), f.org, march(model.BasisAccrual, model.AllocationRentalDays))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 2 || rep.Totals.Overhead.Minor() != 90000 {
		t.Errorf("B1 member rows %d overhead %s", len(rep.Rows), rep.Totals.Overhead)
	}
	check(t, "B1 V1", rowOf(t, rep, w.v1), with(accrualV1, 70000))
	check(t, "B1 V2", rowOf(t, rep, w.v2), with(accrualV2, 20000))
}

// TestProfitabilityCrossOrganization: another organization's vehicles,
// branches, groups and money never reach the report.
func TestProfitabilityCrossOrganization(t *testing.T) {
	a := newFx(t, "prfa")
	wa := a.seed()
	b := newFx(t, "prfb")
	wb := b.seed()
	// B books more money on its own vehicle.
	b.exec(`INSERT INTO expenses (organization_id, category_id, expense_date, net_amount, total_amount, vehicle_id)
		VALUES ($1, $2, '2026-03-05', 5000, 5000, $3)`, b.org, b.cat["repair"], wb.v1.id)

	ctx := a.scope(nil)
	rep, err := a.svc.Report(ctx, a.org, march(model.BasisAccrual, model.AllocationRentalDays))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 3 {
		t.Fatalf("A rows %d", len(rep.Rows))
	}
	for _, r := range rep.Rows {
		if r.Vehicle.UUID == wb.v1.uuid || r.Vehicle.UUID == wb.v2.uuid || r.Vehicle.UUID == wb.v3.uuid {
			t.Fatalf("B's vehicle in A's report")
		}
	}
	check(t, "A V1", rowOf(t, rep, wa.v1), with(accrualV1, 108182))
	if _, err := a.svc.Vehicle(ctx, a.org, wb.v1.uuid, march("", "")); !errors.Is(err, usecase.ErrVehicleNotFound) {
		t.Errorf("B's vehicle in A: %v", err)
	}
	var ve *usecase.ValidationError
	if _, err := a.svc.Report(ctx, a.org, model.Filter{BranchUUIDs: []uuid.UUID{wb.b1.uuid}}); !errors.As(err, &ve) || ve.Details[0].Code != "not_found" {
		t.Errorf("B's branch in A: %v", err)
	}
	if _, err := a.svc.Report(ctx, a.org, model.Filter{RentalGroupUUIDs: []uuid.UUID{wb.g1.uuid}}); !errors.As(err, &ve) ||
		ve.Details[0].Field != "rental_group" {
		t.Errorf("B's group in A: %v", err)
	}
	// B's own report carries its extra expense.
	repB, err := b.svc.Report(b.scope(nil), b.org, march(model.BasisAccrual, model.AllocationRentalDays))
	if err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, repB, wb.v1).DirectCosts.Minor(); got != 545000 {
		t.Errorf("B V1 direct costs %d", got)
	}
}

// TestProfitabilityExport: the io engine export carries one row per
// vehicle, a totals row, the method in every row and the method notes; it
// renders as CSV, XLSX and (landscape) PDF.
func TestProfitabilityExport(t *testing.T) {
	f := newFx(t, "prf")
	f.seed()
	io := adapters.NewIO(f.svc)
	ds, err := io.Export(f.scope(nil), ioengine.ExportQuery{
		"from": "2026-03-01", "to": "2026-03-31", "basis": "cash", "allocation": "vehicle_count",
	}, i18n.LocaleTR)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 4 || !ds.Landscape || len(ds.Notes) < 4 {
		t.Fatalf("rows %d landscape %v notes %v", len(ds.Rows), ds.Landscape, ds.Notes)
	}
	total := ds.Rows[3]
	if total["plate"] != "TOPLAM" || total["basis"] != "Nakit esaslı" || total["allocation"] != "Araç sayısına göre" ||
		total["period"] != "2026-03-01 – 2026-03-31" || total["overhead"] != "1500.00" {
		t.Errorf("totals row %+v", total)
	}
	if !strings.Contains(ds.Notes[0], "Nakit esaslı") || !strings.Contains(ds.Notes[0], "2026-03-01") {
		t.Errorf("notes %v", ds.Notes)
	}
	for _, format := range []ioengine.ExportFormat{ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF, ioengine.ExportJSON} {
		out, err := ioengine.EncodeExport(format, ds, "tr", &ioengine.Letterhead{CompanyName: "Rentopia"}, "Araç kârlılığı raporu")
		if err != nil || len(out) == 0 {
			t.Errorf("%s: %v (%d bytes)", format, err, len(out))
		}
		if format == ioengine.ExportCSV && !strings.Contains(string(out), "Nakit esaslı") {
			t.Errorf("csv does not state the basis")
		}
		if format == ioengine.ExportJSON && !strings.Contains(string(out), `"notes"`) {
			t.Errorf("json has no notes")
		}
	}
	// A bad filter fails the job; an organization is required.
	if _, err := io.Export(f.scope(nil), ioengine.ExportQuery{"basis": "x"}, i18n.LocaleTR); err == nil {
		t.Error("bad basis exported")
	}
	if _, err := io.Export(context.Background(), ioengine.ExportQuery{}, i18n.LocaleTR); err == nil {
		t.Error("export without an organization")
	}
}
