package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// now is 12:00 in Istanbul; "today" is 2026-10-14T21:00Z .. 2026-10-15T21:00Z.
var now = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

func at(hours float64) time.Time { return now.Add(time.Duration(hours * float64(time.Hour))) }

var allPerms = []string{
	rbac.PermTenantRentalsRead, rbac.PermTenantVehiclesRead, rbac.PermTenantPaymentsRead,
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsApprove, rbac.PermTenantResolutionTasksRead,
	rbac.PermTenantIntegrationsManage, rbac.PermTenantContractsRead,
}

type seeder struct {
	t    *testing.T
	pool *pgxpool.Pool
	org  int64
	n    int
	tag  string
}

func (s *seeder) id(query string, args ...any) int64 {
	s.t.Helper()
	var id int64
	if err := s.pool.QueryRow(context.Background(), query, args...).Scan(&id); err != nil {
		s.t.Fatalf("seed: %v\n%s", err, query)
	}
	return id
}

func (s *seeder) next() int { s.n++; return s.n }

type branch struct {
	id   int64
	uuid uuid.UUID
}

func (s *seeder) branch(code string) branch {
	s.t.Helper()
	var b branch
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO branches (organization_id, code, name) VALUES ($1, $2, $3) RETURNING id, uuid`,
		s.org, code, code+" şube").Scan(&b.id, &b.uuid)
	if err != nil {
		s.t.Fatal(err)
	}
	return b
}

func (s *seeder) customer(first, last string) int64 {
	return s.id(`INSERT INTO customers (organization_id, customer_type, first_name, last_name)
		VALUES ($1, 'individual', $2, $3) RETURNING id`, s.org, first, last)
}

func (s *seeder) product() int64 {
	seg := s.id(`INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'ECO', 'Ekonomi') RETURNING id`, s.org)
	p := s.id(`INSERT INTO rental_products (organization_id, code, title, segment_id) VALUES ($1, 'P1', 'Ekonomi', $2) RETURNING id`, s.org, seg)
	s.id(`INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type)
		VALUES ($1, $2, 1, 'GROUP') RETURNING id`, s.org, p)
	return p
}

func (s *seeder) vehicle(plate string, home, current int64, status string) int64 {
	return s.id(`INSERT INTO vehicles (organization_id, plate, model_year, home_branch_id, current_branch_id, status,
			manual_entry, manual_brand, manual_model)
		VALUES ($1, $2, 2024, $3, $4, $5, TRUE, 'Fiat', 'Egea') RETURNING id`, s.org, plate, home, current, status)
}

type resOpts struct {
	status                  string
	pickup, ret             int64
	starts, ends            time.Time
	review, payment         *time.Time
	prepayment, paid, total string
}

func (s *seeder) reservation(product, customer int64, o resOpts) (int64, uuid.UUID) {
	s.t.Helper()
	if o.prepayment == "" {
		o.prepayment = "0"
	}
	if o.paid == "" {
		o.paid = "0"
	}
	if o.total == "" {
		o.total = "4500.00"
	}
	var id int64
	var u uuid.UUID
	err := s.pool.QueryRow(context.Background(), `INSERT INTO reservations (organization_id, reference, status, pickup_branch_id,
			return_branch_id, starts_at, ends_at, rental_product_id, product_version, sold_rental_product_id, sold_product_version,
			guarantee_type, product_snapshot, sold_snapshot, customer_id, channel, total_amount, quote_snapshot,
			review_deadline, payment_deadline, prepayment_amount, paid_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $8, 1, 'GROUP', '{}', '{}', $9, 'web', $10, '{}', $11, $12, $13, $14)
		RETURNING id, uuid`,
		s.org, fmt.Sprintf("%s-R%03d", s.tag, s.next()), o.status, o.pickup, o.ret, o.starts, o.ends, product, customer,
		o.total, o.review, o.payment, o.prepayment, o.paid).Scan(&id, &u)
	if err != nil {
		s.t.Fatal(err)
	}
	return id, u
}

func (s *seeder) rental(reservation, vehicle int64, status string, overdueAt *time.Time) int64 {
	var pickup *time.Time
	if status != "preparation" {
		p := at(-48)
		pickup = &p
	}
	return s.id(`INSERT INTO rentals (organization_id, reference, reservation_id, vehicle_id, status, actual_pickup_at, overdue_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		s.org, fmt.Sprintf("%s-K%03d", s.tag, s.next()), reservation, vehicle, status, pickup, overdueAt)
}

func (s *seeder) task(kind, status string, branch, rental, vehicle int64, due time.Time) {
	var completed *time.Time
	if status == "completed" || status == "cancelled" {
		c := due
		completed = &c
	}
	s.id(`INSERT INTO operation_tasks (organization_id, task_type, status, branch_id, rental_id, vehicle_id, due_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, s.org, kind, status, branch, rental, vehicle, due, completed)
}

func (s *seeder) resolution(reservation int64, kind, status string) {
	var resolved *time.Time
	if status == "resolved" {
		r := at(-1)
		resolved = &r
	}
	s.id(`INSERT INTO resolution_tasks (organization_id, reservation_id, task_type, status, reason, resolved_at)
		VALUES ($1, $2, $3, $4, 'test', $5) RETURNING id`, s.org, reservation, kind, status, resolved)
}

func (s *seeder) payment(reservation int64, amount, status string, verified *time.Time) int64 {
	return s.id(`INSERT INTO payments (organization_id, reservation_id, provider, method, status, amount, conversation_id, verified_at)
		VALUES ($1, $2, 'manual', 'cash', $3, $4, $5, $6) RETURNING id`,
		s.org, reservation, status, amount, uuid.NewString(), verified)
}

func (s *seeder) refund(payment int64, amount string) {
	s.id(`INSERT INTO payment_refunds (organization_id, payment_id, amount, status) VALUES ($1, $2, $3, 'succeeded') RETURNING id`,
		s.org, payment, amount)
	if _, err := s.pool.Exec(context.Background(), `UPDATE payment_refunds SET updated_at = $3
		WHERE organization_id = $1 AND payment_id = $2`, s.org, payment, at(-1)); err != nil {
		s.t.Fatal(err)
	}
}

func (s *seeder) job(status string) {
	s.id(`INSERT INTO integration_jobs (organization_id, direction, provider, kind, idempotency_key, status, last_error)
		VALUES ($1, 'outbound', 'netgsm', 'sms.send', $2, $3, 'timeout') RETURNING id`,
		s.org, fmt.Sprintf("org:%d:test:%s", s.org, uuid.NewString()), status)
}

func (s *seeder) contractTemplate() {
	s.id(`INSERT INTO contract_templates (organization_id, kind, name, version, status, activated_at)
		VALUES ($1, 'rental_contract', 'Kira sözleşmesi', 1, 'active', NOW()) RETURNING id`, s.org)
}

func (s *seeder) document(reservation int64, status string) {
	s.id(`INSERT INTO documents (organization_id, kind, reservation_id, status) VALUES ($1, 'rental_contract', $2, $3) RETURNING id`,
		s.org, reservation, status)
}

func ptr(t time.Time) *time.Time { return &t }

type fixture struct {
	orgA, orgB             int64
	a1, a2, b1             branch
	res4, res6, res3, res7 uuid.UUID
}

func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	tag := uuid.NewString()[:8]
	org := func(slug string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, $1) RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id) })
		return id
	}
	var f fixture
	f.orgA, f.orgB = org("ov-a-"+tag), org("ov-b-"+tag)

	// Organization A: two branches.
	a := &seeder{t: t, pool: pool, org: f.orgA, tag: "A" + tag}
	f.a1, f.a2 = a.branch("A1"), a.branch("A2")
	prod := a.product()
	cust := a.customer("Ayşe", "Yılmaz")
	v1 := a.vehicle("34 A 001", f.a1.id, f.a1.id, "rented")
	v2 := a.vehicle("34 A 002", f.a1.id, f.a1.id, "available")
	v3 := a.vehicle("35 A 003", f.a2.id, f.a2.id, "rented")
	a.vehicle("35 A 004", f.a2.id, f.a2.id, "maintenance")
	a.vehicle("34 A 005", f.a1.id, f.a1.id, "out_of_service")

	// R1: A1, active, planned return 3 h ago (overdue).
	res1, _ := a.reservation(prod, cust, resOpts{status: "confirmed", pickup: f.a1.id, ret: f.a1.id, starts: at(-48), ends: at(-3)})
	r1 := a.rental(res1, v1, "active", nil)
	// R2: A2, active, return tomorrow.
	res2, _ := a.reservation(prod, cust, resOpts{status: "confirmed", pickup: f.a2.id, ret: f.a2.id, starts: at(-4), ends: at(30)})
	r2 := a.rental(res2, v3, "active", nil)
	// R3: A1, preparation, pick-up today at 15:00 Istanbul.
	var res3 int64
	res3, f.res3 = a.reservation(prod, cust, resOpts{status: "confirmed", pickup: f.a1.id, ret: f.a1.id, starts: at(3), ends: at(51)})
	r3 := a.rental(res3, v2, "preparation", nil)

	a.task("handover", "planned", f.a1.id, r3, v2, at(3))      // today
	a.task("return", "planned", f.a1.id, r1, v1, at(-3))       // today, late
	a.task("handover", "completed", f.a2.id, r2, v3, at(-4))     // today, done
	a.task("return", "planned", f.a2.id, r2, v3, at(30))       // tomorrow
	a.task("preparation", "planned", f.a1.id, r3, v2, at(1))   // not a handover / return
	a.task("handover", "cancelled", f.a1.id, r3, v2, at(2)) // cancelled

	// Approval queue: A1 request past its SLA, A2 review within it.
	_, f.res4 = a.reservation(prod, cust, resOpts{status: "request", pickup: f.a1.id, ret: f.a1.id, starts: at(72), ends: at(96), review: ptr(at(-1))})
	a.reservation(prod, cust, resOpts{status: "awaiting_review", pickup: f.a2.id, ret: f.a2.id, starts: at(72), ends: at(96), review: ptr(at(2))})
	// Waiting for payment: A2, 1000 prepayment, 250 paid.
	var res6 int64
	res6, f.res6 = a.reservation(prod, cust, resOpts{status: "awaiting_payment", pickup: f.a2.id, ret: f.a2.id, starts: at(80), ends: at(100),
		payment: ptr(at(1)), prepayment: "1000.00", paid: "250.00"})

	a.resolution(res2, "late_return", "open")
	a.resolution(res6, "late_payment", "open")
	a.resolution(res1, "block_conflict", "resolved")

	p1 := a.payment(res1, "1500.00", "partially_refunded", ptr(at(-2)))
	a.refund(p1, "100.00")
	a.payment(res6, "250.00", "succeeded", ptr(at(-1)))
	a.payment(res2, "999.00", "succeeded", ptr(at(-24)))
	a.payment(res2, "50.00", "pending", nil)

	a.job("failed")
	a.job("needs_verification")
	a.job("succeeded")

	a.contractTemplate()
	a.document(res3, "draft")
	// Res7: A2, tomorrow, no contract prepared yet.
	_, f.res7 = a.reservation(prod, cust, resOpts{status: "confirmed", pickup: f.a2.id, ret: f.a2.id, starts: at(25), ends: at(50)})
	// Outside the 48 h window, and an approved one: not listed.
	a.reservation(prod, cust, resOpts{status: "confirmed", pickup: f.a1.id, ret: f.a1.id, starts: at(72), ends: at(90)})
	res9, _ := a.reservation(prod, cust, resOpts{status: "confirmed", pickup: f.a1.id, ret: f.a1.id, starts: at(5), ends: at(20)})
	a.document(res9, "approved")

	// Organization B: the same kinds of records, none may reach A.
	b := &seeder{t: t, pool: pool, org: f.orgB, tag: "B" + tag}
	f.b1 = b.branch("B1")
	bprod := b.product()
	bcust := b.customer("Mehmet", "Kaya")
	bv := b.vehicle("06 B 001", f.b1.id, f.b1.id, "rented")
	bres, _ := b.reservation(bprod, bcust, resOpts{status: "confirmed", pickup: f.b1.id, ret: f.b1.id, starts: at(-48), ends: at(-5)})
	br := b.rental(bres, bv, "active", ptr(at(-4)))
	b.task("return", "planned", f.b1.id, br, bv, at(-5))
	b.reservation(bprod, bcust, resOpts{status: "request", pickup: f.b1.id, ret: f.b1.id, starts: at(72), ends: at(96), review: ptr(at(-2))})
	b.resolution(bres, "late_return", "open")
	b.payment(bres, "7777.00", "succeeded", ptr(at(-1)))
	b.job("failed")
	b.contractTemplate()
	b.reservation(bprod, bcust, resOpts{status: "confirmed", pickup: f.b1.id, ret: f.b1.id, starts: at(6), ends: at(30)})
	return f
}

func scoped(orgID int64, branches []int64, perms ...string) context.Context {
	return orgctx.WithScope(context.Background(), orgctx.Scope{ID: orgID, Slug: "x", Permissions: perms, BranchIDs: branches})
}

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newService(pool *pgxpool.Pool) *usecase.Service {
	svc := usecase.New(db.New(pool))
	svc.SetClock(func() time.Time { return now })
	return svc
}

// Every KPI and section for a member who sees every branch.
func TestOverviewAllBranches(t *testing.T) {
	pool := openPool(t)
	f := seed(t, pool)
	svc := newService(pool)
	ctx := scoped(f.orgA, nil, allPerms...)

	out, err := svc.Overview(ctx, f.orgA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Today.Start != time.Date(2026, 10, 14, 21, 0, 0, 0, time.UTC) || out.BranchScope != "all" || len(out.Branches) != 2 {
		t.Fatalf("header: %+v", out)
	}
	if o := out.KPIs.Occupancy; o == nil || o.Rentable != 3 || o.Rented != 2 || o.Rate != 66.7 {
		t.Errorf("occupancy = %+v, want 2 / 3 = 66.7", o)
	}
	if a := out.KPIs.ActiveRentals; a == nil || a.Active != 2 || a.Overdue != 1 {
		t.Errorf("active rentals = %+v", a)
	}
	if c := out.KPIs.Collections; c == nil || c.Count != 2 || c.Amount.String() != "1750.00" ||
		c.Refunded.String() != "100.00" || c.Net.String() != "1650.00" || c.Currency != "TRY" {
		t.Errorf("collections = %+v", c)
	}
	today := out.TodayOperations
	if today == nil || today.Total != 3 || len(today.Items) != 3 ||
		today.Handovers != (usecase.TaskCounts{Total: 2, Done: 1, Open: 1}) || today.Returns != (usecase.TaskCounts{Total: 1, Open: 1}) {
		t.Fatalf("today = %+v", today)
	}
	if first := today.Items[0]; first.TaskType != "handover" || first.Status != "completed" || first.Plate != "35 A 003" ||
		first.CustomerName != "Ayşe Yılmaz" || first.Branch.UUID != f.a2.uuid {
		t.Errorf("today first row = %+v", first)
	}
	if od := out.OverdueReturns; od == nil || od.Total != 1 || od.Items[0].LateMinutes != 180 || od.Items[0].Plate != "34 A 001" {
		t.Errorf("overdue = %+v", od)
	}
	if ap := out.Approvals; ap == nil || ap.Total != 2 || ap.Overdue != 1 || ap.Items[0].UUID != f.res4 || ap.Items[0].Deadline == nil {
		t.Errorf("approvals = %+v", ap)
	}
	if wp := out.AwaitingPayment; wp == nil || wp.Total != 1 || wp.Items[0].UUID != f.res6 || wp.Items[0].AmountDue.String() != "750.00" {
		t.Errorf("awaiting payment = %+v", wp)
	}
	if av := out.Availability; av == nil || av.Total != 2 || av.ByType["late_return"] != 1 || av.ByType["late_payment"] != 1 || len(av.Items) != 2 {
		t.Errorf("availability = %+v", av)
	}
	if j := out.IntegrationJobs; j == nil || j.Total != 2 || len(j.Items) != 2 {
		t.Errorf("jobs = %+v", j)
	}
	d := out.Documents
	if d == nil || !d.Required || d.Total != 2 || d.WindowHours != 48 {
		t.Fatalf("documents = %+v", d)
	}
	if d.Items[0].Reservation.UUID != f.res3 || d.Items[0].Document == nil || d.Items[0].Document.Status != "draft" || d.Items[0].Rental == nil {
		t.Errorf("documents[0] = %+v", d.Items[0])
	}
	if d.Items[1].Reservation.UUID != f.res7 || d.Items[1].Document != nil || d.Items[1].Rental != nil {
		t.Errorf("documents[1] = %+v", d.Items[1])
	}

	badges, err := svc.Badges(ctx, f.orgA)
	if err != nil {
		t.Fatal(err)
	}
	if badges.Approvals == nil || *badges.Approvals != (usecase.ApprovalBadge{Pending: 2, Overdue: 1}) ||
		badges.ResolutionTasks == nil || badges.ResolutionTasks.Open != 2 {
		t.Errorf("badges = %+v", badges)
	}
}

// A member limited to branch A1 sees A1's data only, everywhere; A2 as a
// filter is refused like an unknown branch.
func TestOverviewBranchScope(t *testing.T) {
	pool := openPool(t)
	f := seed(t, pool)
	svc := newService(pool)
	ctx := scoped(f.orgA, []int64{f.a1.id}, allPerms...)

	out, err := svc.Overview(ctx, f.orgA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.BranchScope != "selected" || len(out.Branches) != 1 || out.Branches[0].UUID != f.a1.uuid {
		t.Errorf("branches = %+v (%s)", out.Branches, out.BranchScope)
	}
	if o := out.KPIs.Occupancy; o.Rentable != 2 || o.Rented != 1 || o.Rate != 50 {
		t.Errorf("occupancy = %+v", o)
	}
	if a := out.KPIs.ActiveRentals; a.Active != 1 || a.Overdue != 1 {
		t.Errorf("active = %+v", a)
	}
	if c := out.KPIs.Collections; c.Count != 1 || c.Amount.String() != "1500.00" || c.Net.String() != "1400.00" {
		t.Errorf("collections = %+v", c)
	}
	if out.TodayOperations.Total != 2 || out.OverdueReturns.Total != 1 || out.Approvals.Total != 1 || out.Approvals.Overdue != 1 ||
		out.AwaitingPayment.Total != 0 || out.Availability.Total != 0 || out.Documents.Total != 1 {
		t.Errorf("sections: today %d overdue %d approvals %d/%d payment %d tasks %d documents %d",
			out.TodayOperations.Total, out.OverdueReturns.Total, out.Approvals.Total, out.Approvals.Overdue,
			out.AwaitingPayment.Total, out.Availability.Total, out.Documents.Total)
	}
	for _, it := range out.TodayOperations.Items {
		if it.Branch.UUID != f.a1.uuid {
			t.Errorf("today row of another branch: %+v", it)
		}
	}
	// Organization-wide jobs are not branch-bound.
	if out.IntegrationJobs.Total != 2 {
		t.Errorf("jobs = %d", out.IntegrationJobs.Total)
	}
	if _, err := svc.Overview(ctx, f.orgA, &f.a2.uuid); !errors.Is(err, usecase.ErrBranchNotFound) {
		t.Errorf("A2 filter outside the scope: err = %v", err)
	}
	b, err := svc.Badges(ctx, f.orgA)
	if err != nil {
		t.Fatal(err)
	}
	if b.Approvals.Pending != 1 || b.ResolutionTasks.Open != 0 {
		t.Errorf("badges = %+v %+v", b.Approvals, b.ResolutionTasks)
	}

	// A member of every branch narrowing to A2.
	all := scoped(f.orgA, nil, allPerms...)
	out, err = svc.Overview(all, f.orgA, &f.a2.uuid)
	if err != nil {
		t.Fatal(err)
	}
	if out.Branch == nil || out.Branch.UUID != f.a2.uuid || len(out.Branches) != 2 {
		t.Errorf("filter = %+v", out.Branch)
	}
	if out.TodayOperations.Total != 1 || out.KPIs.ActiveRentals.Active != 1 || out.KPIs.ActiveRentals.Overdue != 0 ||
		out.Approvals.Total != 1 || out.AwaitingPayment.Total != 1 || out.Availability.Total != 2 || out.Documents.Total != 1 ||
		out.KPIs.Collections.Amount.String() != "250.00" || out.KPIs.Occupancy.Rentable != 1 {
		t.Errorf("A2: today %d active %+v approvals %d payment %d tasks %d documents %d collections %s occupancy %+v",
			out.TodayOperations.Total, out.KPIs.ActiveRentals, out.Approvals.Total, out.AwaitingPayment.Total,
			out.Availability.Total, out.Documents.Total, out.KPIs.Collections.Amount, out.KPIs.Occupancy)
	}
}

// Sections and KPIs without their permission are nil.
func TestOverviewPermissionHiding(t *testing.T) {
	pool := openPool(t)
	f := seed(t, pool)
	svc := newService(pool)

	out, err := svc.Overview(scoped(f.orgA, nil, rbac.PermTenantReservationsRead), f.orgA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.KPIs.Occupancy != nil || out.KPIs.ActiveRentals != nil || out.KPIs.Collections != nil ||
		out.TodayOperations != nil || out.OverdueReturns != nil || out.Approvals != nil || out.Availability != nil ||
		out.IntegrationJobs != nil || out.Documents != nil {
		t.Errorf("reservations.read only sees more than it may: %+v", out)
	}
	if out.AwaitingPayment == nil || out.AwaitingPayment.Total != 1 {
		t.Errorf("awaiting payment = %+v", out.AwaitingPayment)
	}
	// rentals.read without vehicles.read: no occupancy, but active rentals.
	out, err = svc.Overview(scoped(f.orgA, nil, rbac.PermTenantRentalsRead), f.orgA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.KPIs.Occupancy != nil || out.KPIs.ActiveRentals == nil || out.TodayOperations == nil || out.AwaitingPayment != nil {
		t.Errorf("rentals.read only: %+v", out.KPIs)
	}
	b, err := svc.Badges(scoped(f.orgA, nil, rbac.PermTenantReservationsRead), f.orgA)
	if err != nil {
		t.Fatal(err)
	}
	if b.Approvals != nil || b.ResolutionTasks != nil {
		t.Errorf("badges without approve / tasks read: %+v", b)
	}
}

// Organization B's records never reach A, and B's branch is not a filter
// A can use.
func TestOverviewNoCrossOrganization(t *testing.T) {
	pool := openPool(t)
	f := seed(t, pool)
	svc := newService(pool)

	if _, err := svc.Overview(scoped(f.orgA, nil, allPerms...), f.orgA, &f.b1.uuid); !errors.Is(err, usecase.ErrBranchNotFound) {
		t.Errorf("B's branch as A's filter: err = %v", err)
	}
	// B sees exactly its own records.
	out, err := svc.Overview(scoped(f.orgB, nil, allPerms...), f.orgB, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.KPIs.Occupancy.Rentable != 1 || out.KPIs.ActiveRentals.Active != 1 || out.KPIs.Collections.Amount.String() != "7777.00" ||
		out.TodayOperations.Total != 1 || out.OverdueReturns.Total != 1 || out.Approvals.Total != 1 || out.AwaitingPayment.Total != 0 ||
		out.Availability.Total != 1 || out.IntegrationJobs.Total != 1 || out.Documents.Total != 1 || len(out.Branches) != 1 {
		t.Errorf("B overview: %+v", out)
	}
}
