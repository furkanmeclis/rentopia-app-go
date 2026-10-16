package reports_test

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
	profitmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	profitusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The fixture (March 2026, Europe/Istanbul; clock 2026-04-10):
//
//	Branches  B1 (MRK), B2 (HVL). Vehicles V1 (B1, G1), V2 (B1, G1 + G2),
//	          V3 (B2, no group), in service since 2026-01-01; V2 blocked
//	          03-20 → 03-25 (5 days).
//	Rentals   R1 V1 03-02 10:00 → 03-06 10:00 (4 days), channel web,
//	          6 000 sold, 300 commission. R3 V3 03-10 → 03-12 (2 days),
//	          branch channel, picked up at B2, 2 000 sold.
//	Other     R4 web cancelled 1 500; R5 yolcu360 confirmed 3 000 with
//	          450 commission collected by the channel; R6 web request
//	          1 000; R7 web booked in April (outside).
//	Ledger    A1 (R1): rent 6 000 (B1, due 03-02), fuel 300 and toll 47.50
//	          (B1, due 03-08), late fee 200 reversed; collected 2 000.
//	          A2 (R3): rent 2 000 (B2, due 03-20), 500 accrued 04-01 due
//	          05-01, 900 posted after the report date (ignored).
//	          A3: 300 collected with nothing owed (credit, B1).
//	          Supplier S1: invoice 1 200 due 04-14, paid 800.
//	Expenses  repair 450 (B1), operations 900 (B1), insurance 600 (head
//	          office), vehicle procurement 100 000 (B2, capex), a void
//	          and an April expense (ignored).

type fx struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	svc    *usecase.Service
	profit *profitusecase.Service
	org    int64
	orgID  uuid.UUID
	slug   string
	n      int
	cat    map[string]int64
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var clock = time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)

func ist(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 0, 0, 0, rentaltime.Location()).UTC()
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
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "rpt-" + suffix, Name: "rpt " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, ctx: ctx, pool: pool, q: q, org: org.ID, orgID: org.Uuid, slug: org.Slug, cat: map[string]int64{}}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.profit = profitusecase.New(q)
	f.profit.SetClock(func() time.Time { return clock })
	f.svc = usecase.New(q, f.profit)
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
		ID: f.org, UUID: f.orgID, Slug: f.slug, Status: orgctx.StatusActive,
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

type world struct {
	b1, b2     ref
	v1, v2, v3 ref
	g1, g2     ref
}

type sale struct {
	branch     ref
	status     string
	channel    string
	collector  string
	commission string
	total      string
	starts     time.Time
	ends       time.Time
	created    time.Time
}

func (f *fx) seed() world {
	var w world
	branch := func(code string) ref {
		return f.row(`INSERT INTO branches (organization_id, code, name, working_hours, holiday_exceptions, prep_minutes, is_active)
			VALUES ($1, $2, $3, '{}', '[]', 30, TRUE)`, f.org, code, code+" Şube")
	}
	vehicle := func(plate string, home ref) ref {
		return f.row(`INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
			home_branch_id, current_branch_id, odometer_km, in_service_date)
			VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000, '2026-01-01')`, f.org, plate, home.id)
	}
	w.b1, w.b2 = branch("MRK"), branch("HVL")
	w.v1, w.v2, w.v3 = vehicle("34 RPT 1", w.b1), vehicle("34 RPT 2", w.b1), vehicle("06 RPT 3", w.b2)
	segment := f.id(`INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi')`, f.org)
	w.g1 = f.row(`INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, 'G1', 'Grup 1', $2)`, f.org, segment)
	w.g2 = f.row(`INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, 'G2', 'Grup 2', $2)`, f.org, segment)
	f.exec(`INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id) VALUES ($1, $2, $4), ($1, $3, $4), ($1, $3, $5)`,
		f.org, w.v1.id, w.v2.id, w.g1.id, w.g2.id)
	f.exec(`INSERT INTO vehicle_blocks (organization_id, vehicle_id, kind, during, reason)
		VALUES ($1, $2, 'maintenance', tstzrange($3, $4, '[)'), 'periyodik bakım')`, f.org, w.v2.id, ist(3, 20, 0), ist(3, 25, 0))
	product := f.id(`INSERT INTO rental_products (organization_id, code, title, segment_id, status) VALUES ($1, 'EKO', 'Ekonomi', $2, 'published')`,
		f.org, segment)
	f.exec(`INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, f.org, product, w.g1.id)
	customer := func(name string) int64 {
		return f.id(`INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
			VALUES ($1, 'individual', $2, 'Test', '+905321112233', 'c@example.test')`, f.org, name)
	}
	c1, c2, c3 := customer("Ayşe"), customer("Mehmet"), customer("Zeynep")
	account := func(code string, c int64, name string) int64 {
		return f.id(`INSERT INTO accounts (organization_id, code, party_kind, customer_id, name) VALUES ($1, $2, 'customer', $3, $4)`,
			f.org, code, c, name)
	}
	a1, a2, a3 := account("CR-000001", c1, "Ayşe Test"), account("CR-000002", c2, "Mehmet Test"), account("CR-000003", c3, "Zeynep Test")

	reservation := func(s sale) int64 {
		f.n++
		var cancelled any
		if s.status == "cancelled" {
			cancelled = s.created
		}
		return f.id(`INSERT INTO reservations (organization_id, reference, status, pickup_branch_id, return_branch_id, starts_at, ends_at,
			rental_product_id, product_version, sold_rental_product_id, sold_product_version, guarantee_type, product_snapshot, sold_snapshot,
			customer_id, channel, collecting_party, commission_amount, currency, total_amount, quote_snapshot, created_at, cancelled_at)
			VALUES ($1, $2, $3, $4, $4, $5, $6, $7, 1, $7, 1, 'GROUP', '{}', '{}', $8, $9, $10, $11, 'TRY', $12, '{"lines":[]}', $13, $14)`,
			f.org, fmt.Sprintf("RPT-%d", f.n), s.status, s.branch.id, s.starts, s.ends, product, c1, s.channel, s.collector, s.commission,
			s.total, s.created, cancelled)
	}
	rental := func(res int64, v ref, s sale) ref {
		r := f.row(`INSERT INTO rentals (organization_id, reference, reservation_id, status, vehicle_id, actual_pickup_at, actual_return_at)
			VALUES ($1, $2, $3, 'returned', $4, $5, $6)`, f.org, fmt.Sprintf("RPT-R%d", res), res, v.id, s.starts, s.ends)
		f.exec(`INSERT INTO rental_vehicles (organization_id, rental_id, vehicle_id, reason, out_at, in_at) VALUES ($1, $2, $3, 'handover', $4, $5)`,
			f.org, r.id, v.id, s.starts, s.ends)
		return r
	}
	s1 := sale{branch: w.b1, status: "confirmed", channel: "web", collector: "us", commission: "300", total: "6000",
		starts: ist(3, 2, 10), ends: ist(3, 6, 10), created: ist(3, 1, 12)}
	r1 := rental(reservation(s1), w.v1, s1)
	s3 := sale{branch: w.b2, status: "confirmed", channel: "branch", collector: "us", commission: "0", total: "2000",
		starts: ist(3, 10, 10), ends: ist(3, 12, 10), created: ist(3, 9, 12)}
	r3 := rental(reservation(s3), w.v3, s3)
	reservation(sale{branch: w.b1, status: "cancelled", channel: "web", collector: "us", commission: "0", total: "1500",
		starts: ist(4, 20, 10), ends: ist(4, 22, 10), created: ist(3, 5, 12)})
	reservation(sale{branch: w.b1, status: "confirmed", channel: "yolcu360", collector: "channel", commission: "450", total: "3000",
		starts: ist(4, 1, 10), ends: ist(4, 4, 10), created: ist(3, 15, 12)})
	reservation(sale{branch: w.b1, status: "request", channel: "web", collector: "us", commission: "0", total: "1000",
		starts: ist(4, 25, 10), ends: ist(4, 26, 10), created: ist(3, 30, 12)})
	reservation(sale{branch: w.b1, status: "confirmed", channel: "web", collector: "us", commission: "0", total: "800",
		starts: ist(4, 25, 10), ends: ist(4, 26, 10), created: ist(4, 2, 12)})

	keys := 0
	entry := func(account int64, kind, direction, purpose, amount, day, due string, branch *ref, extra map[string]any) int64 {
		cols := []string{"organization_id", "account_id", "kind", "direction", "purpose", "amount", "entry_date", "due_date", "source_type"}
		args := []any{f.org, account, kind, direction, purpose, amount, day, due, "manual"}
		if branch != nil {
			cols = append(cols, "branch_id")
			args = append(args, branch.id)
		}
		if kind == "accrual" {
			keys++
			cols = append(cols, "accrual_key")
			args = append(args, fmt.Sprintf("fixture:%d", keys))
		}
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
	entry(a1, "accrual", "debit", "rent", "6000.00", "2026-03-02", "2026-03-02", &w.b1, map[string]any{"rental_id": r1.id})
	fuel := charge(r1, "fuel", "manual", "300.00")
	entry(a1, "accrual", "debit", "charge", "300.00", "2026-03-08", "2026-03-08", &w.b1, map[string]any{"rental_id": r1.id, "rental_charge_id": fuel})
	toll := charge(r1, "toll", "toll", "47.50")
	entry(a1, "accrual", "debit", "charge", "47.50", "2026-03-08", "2026-03-08", &w.b1, map[string]any{"rental_id": r1.id, "rental_charge_id": toll})
	late := charge(r1, "late_return", "manual", "200.00")
	lateEntry := entry(a1, "accrual", "debit", "charge", "200.00", "2026-03-08", "2026-03-08", &w.b1,
		map[string]any{"rental_id": r1.id, "rental_charge_id": late})
	entry(a1, "reversal", "credit", "charge", "200.00", "2026-03-09", "2026-03-09", &w.b1,
		map[string]any{"rental_id": r1.id, "rental_charge_id": late, "reverses_entry_id": lateEntry, "reason": "fixture reversal"})
	entry(a1, "collection", "credit", "general", "2000.00", "2026-03-02", "2026-03-02", &w.b1, nil)
	// An April accrual: outside March income, inside the aging.
	entry(a2, "accrual", "debit", "rent", "2000.00", "2026-03-10", "2026-03-20", &w.b2, map[string]any{"rental_id": r3.id})
	entry(a2, "accrual", "debit", "general", "500.00", "2026-04-01", "2026-05-01", &w.b2, nil)
	entry(a2, "accrual", "debit", "general", "900.00", "2026-04-20", "2026-04-20", &w.b2, nil)
	entry(a3, "collection", "credit", "general", "300.00", "2026-03-15", "2026-03-15", &w.b1, nil)

	supplier := f.id(`INSERT INTO suppliers (organization_id, code, title) VALUES ($1, 'TED-000001', 'Servis A.Ş.')`, f.org)
	sAcc := f.id(`INSERT INTO accounts (organization_id, code, party_kind, supplier_id, name) VALUES ($1, 'TD-000001', 'supplier', $2, 'Servis A.Ş.')`,
		f.org, supplier)
	entry(sAcc, "accrual", "credit", "general", "1200.00", "2026-03-14", "2026-04-14", &w.b1, nil)
	entry(sAcc, "payment", "debit", "general", "800.00", "2026-03-25", "2026-03-25", &w.b1, nil)

	expense := func(cat, day, total string, branch *ref, status string) {
		var b any
		if branch != nil {
			b = branch.id
		}
		voided := any(nil)
		if status == "void" {
			voided = clock
		}
		f.exec(`INSERT INTO expenses (organization_id, category_id, expense_date, net_amount, total_amount, branch_id, status, voided_at)
			VALUES ($1, $2, $3, $4, $4, $5, $6, $7)`, f.org, f.cat[cat], day, total, b, status, voided)
	}
	expense("repair", "2026-03-12", "450.00", &w.b1, "recorded")
	expense("repair", "2026-03-10", "999.00", &w.b1, "void")
	expense("repair", "2026-04-02", "777.00", &w.b1, "recorded")
	expense("operations", "2026-03-31", "900.00", &w.b1, "recorded")
	expense("insurance", "2026-03-01", "600.00", nil, "recorded")
	expense("vehicle_procurement", "2026-03-03", "100000.00", &w.b2, "recorded")
	return w
}

func march() model.Filter {
	from, to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	return model.Filter{From: &from, To: &to}
}

func occ(v *float64) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%.1f", *v)
}

func TestReports(t *testing.T) {
	f := newFx(t)
	w := f.seed()
	all := f.scope(nil)

	t.Run("occupancy by rental group", func(t *testing.T) {
		rep, err := f.svc.Occupancy(all, f.org, march())
		f.must(err)
		if rep.GroupBy != model.GroupRentalGroup || rep.MultiGroupVehicles != 1 || len(rep.Trend) != 1 {
			t.Fatalf("report = %+v", rep)
		}
		// V1 4/31 days, V2 0/26 (5 days blocked), V3 2/31.
		if rep.Totals.Vehicles != 3 || rep.Totals.RentalDays != 6 || rep.Totals.AvailableDays != 88 || occ(rep.Totals.Occupancy) != "6.8" {
			t.Errorf("totals = %+v occupancy %s", rep.Totals, occ(rep.Totals.Occupancy))
		}
		want := map[string]struct {
			vehicles   int
			rented, av float64
			occupancy  string
		}{
			w.g1.uuid.String(): {2, 4, 57, "7.0"}, w.g2.uuid.String(): {1, 0, 26, "0.0"}, usecase.UngroupedKey: {1, 2, 31, "6.5"},
		}
		if len(rep.Rows) != len(want) {
			t.Fatalf("rows = %+v", rep.Rows)
		}
		for _, r := range rep.Rows {
			e, ok := want[r.Key]
			if !ok || r.Vehicles != e.vehicles || r.RentalDays != e.rented || r.AvailableDays != e.av || occ(r.Occupancy) != e.occupancy {
				t.Errorf("row %s = %+v occupancy %s", r.Key, r.OccupancyFigures, occ(r.Occupancy))
			}
		}
		if rep.Rows[len(rep.Rows)-1].Key != usecase.UngroupedKey {
			t.Errorf("ungrouped row is not last")
		}
	})

	t.Run("occupancy by branch and month, scoped", func(t *testing.T) {
		fl := march()
		fl.GroupBy = model.GroupBranch
		rep, err := f.svc.Occupancy(all, f.org, fl)
		f.must(err)
		if len(rep.Rows) != 2 || rep.Rows[0].Group.Code != "HVL" || occ(rep.Rows[0].Occupancy) != "6.5" ||
			rep.Rows[1].Group.Code != "MRK" || occ(rep.Rows[1].Occupancy) != "7.0" {
			t.Errorf("rows = %+v", rep.Rows)
		}
		rep, err = f.svc.Occupancy(f.scope([]int64{w.b2.id}), f.org, fl)
		f.must(err)
		if len(rep.Rows) != 1 || rep.Rows[0].Group.Code != "HVL" || rep.Method.BranchScope != model.ScopeBranches {
			t.Errorf("scoped rows = %+v", rep.Rows)
		}
		fl.GroupBy = model.GroupMonth
		rep, err = f.svc.Occupancy(all, f.org, fl)
		f.must(err)
		if len(rep.Rows) != 1 || rep.Rows[0].Key != "2026-03" || occ(rep.Rows[0].Occupancy) != "6.8" {
			t.Errorf("month rows = %+v", rep.Rows)
		}
	})

	t.Run("income and expense", func(t *testing.T) {
		rep, err := f.svc.IncomeExpense(all, f.org, march())
		f.must(err)
		tot := rep.Totals
		if tot.Income.String() != "8300.00" || tot.Expense.String() != "1950.00" || tot.Net.String() != "6350.00" ||
			tot.PassThrough.String() != "47.50" || tot.Capex.String() != "100000.00" {
			t.Errorf("totals = %+v", tot)
		}
		got := map[string]string{}
		for _, r := range rep.Rows {
			switch r.Kind {
			case model.KindIncome:
				got[r.Key] = r.Income.String()
			case model.KindExpense:
				got[r.Key] = r.Expense.String()
			case model.KindPassThrough:
				got[r.Key] = r.PassThrough.String()
			case model.KindCapex:
				got[r.Key] = r.Capex.String()
			}
		}
		want := map[string]string{
			"income:rent": "8000.00", "income:charge.fuel": "300.00", "pass_through:charge.toll": "47.50",
			"expense:repair": "450.00", "expense:operations": "900.00", "expense:insurance": "600.00",
			"capex:vehicle_procurement": "100000.00",
		}
		if len(got) != len(want) {
			t.Errorf("rows = %v", got)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %s", k, got[k], v)
			}
		}
		if rep.Rows[0].Key != "income:rent" || occ(rep.Rows[0].Share) != "96.4" {
			t.Errorf("first row = %+v", rep.Rows[0])
		}

		fl := march()
		fl.GroupBy = model.GroupBranch
		rep, err = f.svc.IncomeExpense(all, f.org, fl)
		f.must(err)
		byKey := map[string]model.IncomeExpenseRow{}
		for _, r := range rep.Rows {
			byKey[r.Key] = r
		}
		if r := byKey[w.b1.uuid.String()]; r.Income.String() != "6300.00" || r.Expense.String() != "1350.00" {
			t.Errorf("B1 = %+v", r)
		}
		if r := byKey[w.b2.uuid.String()]; r.Income.String() != "2000.00" || r.Capex.String() != "100000.00" {
			t.Errorf("B2 = %+v", r)
		}
		if r := byKey[usecase.HeadOfficeKey]; r.Expense.String() != "600.00" || rep.Rows[len(rep.Rows)-1].Key != usecase.HeadOfficeKey {
			t.Errorf("head office = %+v", r)
		}

		// A member of B1 sees B1 only: no head-office expense.
		rep, err = f.svc.IncomeExpense(f.scope([]int64{w.b1.id}), f.org, march())
		f.must(err)
		if rep.Totals.Income.String() != "6300.00" || rep.Totals.Expense.String() != "1350.00" || rep.Method.BranchScope != model.ScopeBranches {
			t.Errorf("scoped totals = %+v", rep.Totals)
		}
		// B2 as a filter for a B1 member: not found.
		fl = march()
		fl.BranchUUIDs = []uuid.UUID{w.b2.uuid}
		var ve *usecase.ValidationError
		if _, err := f.svc.IncomeExpense(f.scope([]int64{w.b1.id}), f.org, fl); !errors.As(err, &ve) || ve.Details[0].Code != "not_found" {
			t.Errorf("out-of-scope branch filter = %v", err)
		}
		fl.GroupBy = model.GroupMonth
		fl.BranchUUIDs = nil
		rep, err = f.svc.IncomeExpense(all, f.org, fl)
		f.must(err)
		if len(rep.Rows) != 1 || rep.Rows[0].Net.String() != "6350.00" || len(rep.Trend) != 1 {
			t.Errorf("month rows = %+v", rep.Rows)
		}
	})

	t.Run("channels", func(t *testing.T) {
		rep, err := f.svc.Channels(all, f.org, march())
		f.must(err)
		if rep.Source != usecase.ChannelSource || rep.DateBasis != model.DateBooking {
			t.Errorf("method = %+v", rep)
		}
		tot := rep.Totals
		if tot.Reservations != 5 || tot.Confirmed != 3 || tot.Cancelled != 1 || tot.Pending != 1 || tot.Revenue.String() != "11000.00" ||
			tot.Commission.String() != "750.00" || occ(tot.CancellationRate) != "20.0" {
			t.Errorf("totals = %+v", tot)
		}
		if len(rep.Rows) != 3 || rep.Rows[0].Channel != "web" || rep.Rows[1].Channel != "yolcu360" || rep.Rows[2].Channel != "branch" {
			t.Fatalf("rows = %+v", rep.Rows)
		}
		web := rep.Rows[0]
		if web.Reservations != 3 || web.Revenue.String() != "6000.00" || web.NetRevenue.String() != "5700.00" ||
			occ(web.CancellationRate) != "33.3" || occ(web.CommissionRate) != "5.0" || occ(web.RevenueShare) != "54.5" || web.AverageDays != 4 {
			t.Errorf("web = %+v", web)
		}
		if y := rep.Rows[1]; y.ChannelCollected.String() != "3000.00" || y.Commission.String() != "450.00" {
			t.Errorf("yolcu360 = %+v", y)
		}
		// Pick-up basis: only R1 and R3 start in March.
		fl := march()
		fl.DateBasis = model.DatePickup
		rep, err = f.svc.Channels(all, f.org, fl)
		f.must(err)
		if rep.Totals.Reservations != 2 || rep.Totals.Revenue.String() != "8000.00" {
			t.Errorf("pickup totals = %+v", rep.Totals)
		}
		// A B2 member sees R3 only.
		rep, err = f.svc.Channels(f.scope([]int64{w.b2.id}), f.org, march())
		f.must(err)
		if len(rep.Rows) != 1 || rep.Rows[0].Channel != "branch" {
			t.Errorf("scoped rows = %+v", rep.Rows)
		}
	})

	t.Run("aging", func(t *testing.T) {
		rep, err := f.svc.Aging(all, f.org, model.Filter{})
		f.must(err)
		if rep.AsOf != "2026-04-10" || rep.Party != model.PartyCustomer {
			t.Errorf("header = %+v", rep)
		}
		tot := rep.Totals
		if tot.Current.String() != "500.00" || tot.Days30.String() != "2000.00" || tot.Days60.String() != "4347.50" ||
			tot.Days90.String() != "0.00" || tot.Over90.String() != "0.00" || tot.Open.String() != "6847.50" ||
			tot.Overdue.String() != "6347.50" || tot.Credit.String() != "300.00" {
			t.Errorf("totals = %+v", tot)
		}
		if len(rep.Rows) != 3 || rep.Rows[0].Code != "CR-000001" || rep.Rows[1].Code != "CR-000002" || rep.Rows[2].Code != "CR-000003" {
			t.Fatalf("rows = %+v", rep.Rows)
		}
		if r := rep.Rows[1]; r.Current.String() != "500.00" || r.Days30.String() != "2000.00" || r.Balance.String() != "2500.00" {
			t.Errorf("A2 = %+v", r)
		}
		// As of 2026-03-31: the April accrual is not posted yet; A2's rent
		// is 11 days late.
		asOf := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
		rep, err = f.svc.Aging(all, f.org, model.Filter{AsOf: &asOf})
		f.must(err)
		if rep.Totals.Open.String() != "6347.50" || rep.Totals.Current.String() != "0.00" || rep.Totals.Days30.String() != "6347.50" {
			t.Errorf("as of March totals = %+v", rep.Totals)
		}
		// A B1 member: A1 and A3 only.
		rep, err = f.svc.Aging(f.scope([]int64{w.b1.id}), f.org, model.Filter{})
		f.must(err)
		if len(rep.Rows) != 2 || rep.Totals.Open.String() != "4347.50" {
			t.Errorf("scoped = %+v", rep.Rows)
		}
		rep, err = f.svc.Aging(all, f.org, model.Filter{Party: model.PartySupplier})
		f.must(err)
		if len(rep.Rows) != 1 || rep.Rows[0].Current.String() != "400.00" || rep.Rows[0].Balance.String() != "400.00" ||
			rep.Totals.Overdue.String() != "0.00" {
			t.Errorf("supplier = %+v", rep.Rows)
		}
	})

	t.Run("branches match the profitability report", func(t *testing.T) {
		rep, err := f.svc.Branches(all, f.org, march())
		f.must(err)
		from, to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
		prof, err := f.profit.Report(all, f.org, profitmodel.Filter{From: &from, To: &to})
		f.must(err)
		if rep.Totals.Contribution.String() != prof.Totals.Contribution.String() ||
			rep.Totals.GrossIncome.String() != prof.Totals.GrossIncome.String() || rep.Totals.Vehicles != 3 {
			t.Errorf("totals %+v vs profitability %+v", rep.Totals, prof.Totals)
		}
		if len(rep.Rows) != 2 || rep.Rows[0].Branch.Code != "HVL" || rep.Rows[0].Vehicles != 1 || rep.Rows[1].Vehicles != 2 ||
			occ(rep.Rows[1].Occupancy) != "7.0" {
			t.Errorf("rows = %+v", rep.Rows)
		}
		if rep.Method.Basis != "accrual" || rep.Method.Allocation != "rental_days" {
			t.Errorf("method = %+v", rep.Method)
		}
	})

	t.Run("exports", func(t *testing.T) {
		ctx := orgctx.WithOrganizationID(all, f.org)
		q := ioengine.ExportQuery{"from": "2026-03-01", "to": "2026-03-31"}
		for _, a := range []interface {
			Resource() string
			Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error)
		}{
			adapters.NewOccupancyIO(f.svc), adapters.NewIncomeExpenseIO(f.svc), adapters.NewChannelsIO(f.svc),
			adapters.NewAgingIO(f.svc), adapters.NewBranchesIO(f.svc),
		} {
			ds, err := a.Export(ctx, q, i18n.LocaleTR)
			if err != nil {
				t.Fatalf("%s: %v", a.Resource(), err)
			}
			if len(ds.Rows) < 2 || len(ds.Notes) < 2 || ds.Resource != a.Resource() {
				t.Errorf("%s: %d rows, notes %v", a.Resource(), len(ds.Rows), ds.Notes)
				continue
			}
			last := ds.Rows[len(ds.Rows)-1]
			found := false
			for _, v := range last {
				if v == "TOPLAM" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: last row is not the total: %v", a.Resource(), last)
			}
			for _, c := range ds.Columns {
				if i18n.Translate(i18n.LocaleTR, c.LabelKey) == c.LabelKey {
					t.Errorf("%s: column %s has no label", a.Resource(), c.LabelKey)
				}
			}
		}
		if _, err := adapters.NewChannelsIO(f.svc).Export(ctx, ioengine.ExportQuery{"date_basis": "x"}, i18n.LocaleTR); err == nil {
			t.Error("invalid export query accepted")
		}
	})
}
