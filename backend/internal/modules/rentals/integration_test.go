package rentals_test

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
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with a branch, vehicles, a product, settings and
// a customer, created with SQL and removed when the test ends.
type fx struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	q         *db.Queries
	res       *resusecase.Service
	rentals   *usecase.Service
	contracts *contractsusecase.Service
	org       int64
	orgUUID   uuid.UUID
	suffix    string
	segment   int64
	branch    ref
	other     ref
	product   ref
	customer  ref
	driver    ref
	plate     int
	clockMu   sync.Mutex
	shift     time.Duration
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var perms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantResolutionTasksRead, rbac.PermTenantResolutionTasksWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantContractsRead, rbac.PermTenantContractsWrite, rbac.PermTenantContractsActivate,
	rbac.PermTenantDamageApprove, rbac.PermTenantSignatureWaive, rbac.PermTenantDocumentDownload,
}

type fakePDF struct{}

func (fakePDF) RenderPDF(_ context.Context, html []byte) ([]byte, error) {
	return append([]byte("%PDF-1.4\n"), html[:16]...), nil
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
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "rnt-" + f.suffix, Name: "Rnt " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = orgctx.WithScope(ctx, orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "rnt-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms,
	})
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	avail.SetClock(f.now)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	f.res = resusecase.New(pool, q, avail, pricing, settings)
	f.res.SetClock(f.now)
	f.contracts = contractsusecase.New(pool, q, settings, storage.NewMemory())
	f.contracts.SetRenderer(fakePDF{})
	f.contracts.SetClock(f.now)
	f.rentals = usecase.New(pool, q, avail, adapters.NewReservations(f.res), vehicles)
	f.rentals.SetClock(f.now)
	f.rentals.SetDocuments(f.contracts)
	f.res.SetLifecycleListener(f.rentals)

	f.branch = f.newBranch("MRK", 60)
	f.other = f.newBranch("HVL", 30)
	f.product = f.newProduct("EKO")
	f.customer, f.driver = f.newCustomer()
	grace, fee := 30, "150"
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalAuto
		s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
		s.Policies.LateReturnGraceMinutes, s.Policies.ExtraHourFee = &grace, fee
		s.Policies.EarlyReturn = "Erken iadede ücret iadesi yapılmaz."
		s.Company.LegalName = "Test Oto A.Ş."
	})
	return f
}

func intp(v int) *int { return &v }

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) now() time.Time {
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	return time.Now().Add(f.shift)
}

func (f *fx) advance(d time.Duration) {
	f.clockMu.Lock()
	f.shift += d
	f.clockMu.Unlock()
}

func (f *fx) newBranch(code string, prep int) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: int32(prep), IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

func (f *fx) vehicle() ref {
	f.plate++
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 RN %d", f.plate), f.branch.id).Scan(&r.id, &r.uuid))
	// Explicit rental group membership (GROUP guarantees need it).
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
	for _, b := range [][3]any{{1, 3, "1500"}, {4, 7, "1300"}, {8, nil, "1000"}} {
		_, err := f.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, $3, $4, $5)`,
			f.org, versionID, b[0], b[1], b[2])
		f.must(err)
	}
	_, err = f.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planID)
	f.must(err)
	return p
}

func (f *fx) configure(mutate func(s *osmodel.Settings)) {
	s := osmodel.Defaults()
	mutate(&s)
	data, _ := json.Marshal(s)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)
}

func (f *fx) newCustomer() (ref, ref) {
	ctx := context.Background()
	var c, d ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Ayşe', 'Yılmaz', '+905321112233', 'ayse@example.test') RETURNING id, uuid`, f.org).Scan(&c.id, &c.uuid))
	now := time.Now().UTC()
	f.must(f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Ayşe', 'Yılmaz', $3, 'v1:x', $4, '****456', $5) RETURNING id, uuid`,
		f.org, c.id, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return c, d
}

// reserve creates a confirmed reservation [from, to) at the main branch
// (returned at ret when set).
func (f *fx) reserve(from, to time.Time, ret *ref) resmodel.Reservation {
	f.t.Helper()
	b := f.branch.uuid
	w := resmodel.Where{PickupBranchUUID: &b, StartsAt: &from, EndsAt: &to}
	if ret != nil {
		rb := ret.uuid
		w.ReturnBranchUUID = &rb
	}
	q, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid})
	if err != nil {
		f.t.Fatalf("quote: %v", err)
	}
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &q.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	if err != nil {
		f.t.Fatalf("reservation: %v", err)
	}
	if r.Status != resmodel.StatusConfirmed {
		f.t.Fatalf("reservation status %s", r.Status)
	}
	return r
}

func (f *fx) tasks(rental uuid.UUID) map[string]string {
	f.t.Helper()
	items, _, err := f.rentals.ListTasks(f.ctx, f.org, model.TaskFilter{RentalUUIDs: []uuid.UUID{rental}, Limit: 50})
	f.must(err)
	out := map[string]string{}
	for _, t := range items {
		key := t.TaskType
		if prev, ok := out[key]; ok {
			out[key] = prev + "," + t.Status
		} else {
			out[key] = t.Status
		}
	}
	return out
}

func (f *fx) allocation(rental model.Rental) (kind string, endsAt time.Time, returnBranch int64, vehicle int64) {
	f.t.Helper()
	f.must(f.pool.QueryRow(context.Background(), `SELECT a.kind, a.ends_at, a.return_branch_id, a.vehicle_id FROM rentals r
		JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
		WHERE r.organization_id = $1 AND r.uuid = $2`, f.org, rental.UUID).Scan(&kind, &endsAt, &returnBranch, &vehicle))
	return
}

func (f *fx) vehicleState(v uuid.UUID) (status string, branch int64, km int32) {
	f.t.Helper()
	f.must(f.pool.QueryRow(context.Background(), `SELECT status, current_branch_id, odometer_km FROM vehicles WHERE organization_id = $1 AND uuid = $2`,
		f.org, v).Scan(&status, &branch, &km))
	return
}

func i32(v int32) *int32 { return &v }
func i16(v int16) *int16 { return &v }

// TestRentalLifecycle: plate fixed → rental in preparation with tasks;
// handover (km / fuel out, allocation kind rental, vehicle rented); return
// elsewhere (actual branch kept next to the planned one, vehicle moved,
// allocation ends at the actual return); return check → cleaning task.
func TestRentalLifecycle(t *testing.T) {
	f := newFx(t)
	v := f.vehicle()
	start := f.now().Add(2 * time.Hour).Truncate(time.Minute)
	res := f.reserve(start, start.Add(48*time.Hour), nil)

	// Fixing the plate creates the rental (reservation listener).
	created, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	if created.Status != model.StatusPreparation || created.Vehicle.UUID != v.uuid {
		t.Fatalf("created %+v", created)
	}
	again, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &res.UUID})
	f.must(err)
	if again.UUID != created.UUID {
		t.Fatal("a second create must return the same rental")
	}
	if got := f.tasks(created.UUID); got[model.TaskHandover] != model.TaskPlanned || got[model.TaskPreparation] != model.TaskPlanned {
		t.Fatalf("tasks after create: %v", got)
	}

	// Handover an hour early: km below the odometer is refused.
	if _, err := f.rentals.Handover(f.ctx, f.org, created.UUID, model.HandoverInput{Km: i32(10), Fuel: i16(80)}); err == nil {
		t.Fatal("km below the odometer must be refused")
	}
	r, err := f.rentals.Handover(f.ctx, f.org, created.UUID, model.HandoverInput{Km: i32(1200), Fuel: i16(80)})
	f.must(err)
	if r.Status != model.StatusActive || r.ActualPickupAt == nil || len(r.Legs) != 1 || *r.Legs[0].OutKm != 1200 {
		t.Fatalf("after handover %+v", r)
	}
	kind, _, _, _ := f.allocation(r)
	if kind != "rental" {
		t.Fatalf("allocation kind %s", kind)
	}
	if st, _, km := f.vehicleState(v.uuid); st != "rented" || km != 1200 {
		t.Fatalf("vehicle %s %d", st, km)
	}
	if got := f.tasks(r.UUID); got[model.TaskHandover] != model.TaskCompleted || got[model.TaskReturn] != model.TaskPlanned {
		t.Fatalf("tasks after handover: %v", got)
	}
	// The reservation can no longer be cancelled (the vehicle is out).
	if _, err := f.res.Cancel(f.ctx, f.org, res.UUID, "deneme iptal"); err == nil {
		t.Fatal("cancelling a reservation under way must be refused")
	}

	// Early return (a day later) at the other branch.
	f.advance(24 * time.Hour)
	other := f.other.uuid
	r, err = f.rentals.Return(f.ctx, f.org, r.UUID, model.ReturnInput{Km: i32(1650), Fuel: i16(40), BranchUUID: &other})
	f.must(err)
	if r.Status != model.StatusReturnCheck || r.ActualReturn.Branch == nil || r.ActualReturn.Branch.UUID != other ||
		r.PlannedReturn.Branch.UUID != f.branch.uuid || r.LateMinutes != 0 || *r.Legs[0].DrivenKm != 450 {
		t.Fatalf("after return %+v", r)
	}
	_, endsAt, retBranch, _ := f.allocation(r)
	if retBranch != f.other.id || endsAt.Sub(*r.ActualReturnAt).Abs() > time.Second {
		t.Fatalf("allocation end %v branch %d, want the actual return", endsAt, retBranch)
	}
	if st, branch, _ := f.vehicleState(v.uuid); st != "return_check" || branch != f.other.id {
		t.Fatalf("vehicle after return %s %d", st, branch)
	}
	if len(r.Charges) != 0 {
		t.Fatalf("an early return proposes no automatic charge: %+v", r.Charges)
	}

	// Repeat-customer campaigns: a rental in return check is not completed yet.
	if n, err := f.rentals.CompletedRentals(f.ctx, f.org, f.customer.uuid); err != nil || n != 0 {
		t.Fatalf("completed rentals in return check = %d %v", n, err)
	}
	r, err = f.rentals.CompleteReturn(f.ctx, f.org, r.UUID, model.CompleteReturnInput{})
	f.must(err)
	if r.Status != model.StatusReturned {
		t.Fatalf("status %s", r.Status)
	}
	if n, err := f.rentals.CompletedRentals(f.ctx, f.org, f.customer.uuid); err != nil || n != 1 {
		t.Fatalf("completed rentals after the return check = %d %v", n, err)
	}
	if n, err := f.rentals.CompletedRentals(f.ctx, f.org, uuid.New()); err != nil || n != 0 {
		t.Fatalf("completed rentals of another customer = %d %v", n, err)
	}
	if n, err := f.rentals.CompletedRentals(f.ctx, f.org+1_000_000, f.customer.uuid); err != nil || n != 0 {
		t.Fatalf("completed rentals in another organization = %d %v", n, err)
	}
	got := f.tasks(r.UUID)
	if got[model.TaskCleaning] != model.TaskPlanned {
		t.Fatalf("cleaning task: %v", got)
	}
	items, _, err := f.rentals.ListTasks(f.ctx, f.org, model.TaskFilter{RentalUUIDs: []uuid.UUID{r.UUID}, TaskTypes: []string{model.TaskCleaning}, Limit: 5})
	f.must(err)
	if _, err := f.rentals.FinishTask(f.ctx, f.org, items[0].UUID, true, ""); err != nil {
		t.Fatal(err)
	}
	if st, _, _ := f.vehicleState(v.uuid); st != "available" {
		t.Fatalf("vehicle after cleaning %s", st)
	}
}

// TestLateReturn: an overdue rental keeps its vehicle busy; the next
// reservation of the (only) vehicle cannot be re-planned and gets a
// late_return task; the return beyond the tolerance proposes the policy's
// late fee, which needs an authorized approval.
func TestLateReturn(t *testing.T) {
	f := newFx(t)
	v := f.vehicle()
	start := f.now().Add(time.Hour).Truncate(time.Minute)
	res := f.reserve(start, start.Add(24*time.Hour), nil)
	rental, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	next := f.reserve(start.Add(28*time.Hour), start.Add(40*time.Hour), nil)
	// The next customer was promised this plate (nothing can move it).
	if _, err := f.res.Plate(f.ctx, f.org, next.UUID, resmodel.PlateInput{VehicleUUID: &v.uuid, Fix: true}); err != nil {
		t.Fatal(err)
	}
	_, err = f.rentals.Handover(f.ctx, f.org, rental.UUID, model.HandoverInput{Km: i32(1100), Fuel: i16(100)})
	f.must(err)

	// 27 hours later the vehicle is 3 hours late (tolerance 30 minutes).
	f.advance(27 * time.Hour)
	n, err := f.rentals.SweepOverdue(f.ctx)
	f.must(err)
	if n < 1 {
		t.Fatalf("overdue sweep handled %d rentals", n)
	}
	r, err := f.rentals.Get(f.ctx, f.org, rental.UUID)
	f.must(err)
	if r.OverdueAt == nil || r.AllocationEndsAt == nil || !r.AllocationEndsAt.After(r.PlannedReturnAt) {
		t.Fatalf("overdue rental %+v", r)
	}
	tasks, _, err := f.res.ListTasks(f.ctx, f.org, resmodel.TaskFilter{ReservationUUIDs: []uuid.UUID{next.UUID}, Limit: 10})
	f.must(err)
	found := false
	for _, tk := range tasks {
		found = found || tk.TaskType == resmodel.TaskLateReturn
	}
	if !found {
		t.Fatalf("next reservation needs a late_return task: %+v", tasks)
	}

	r, err = f.rentals.Return(f.ctx, f.org, rental.UUID, model.ReturnInput{Km: i32(1500), Fuel: i16(90)})
	f.must(err)
	if r.LateMinutes < 110 {
		t.Fatalf("late minutes %d", r.LateMinutes)
	}
	if len(r.Charges) != 1 || r.Charges[0].Kind != model.ChargeLateReturn || r.Charges[0].Status != model.ChargeProposed ||
		r.Charges[0].Source != model.SourcePolicy {
		t.Fatalf("late fee: %+v", r.Charges)
	}
	hours := int64((r.LateMinutes + 59) / 60)
	if !r.Charges[0].Amount.Equal(money.MustParse("150").Mul(hours)) {
		t.Fatalf("late fee %s for %d hours", r.Charges[0].Amount, hours)
	}
	// Approving needs tenant.rentals.close.
	noClose := orgctx.WithScope(context.Background(), orgctx.Scope{ID: f.org, UUID: f.orgUUID, Status: orgctx.StatusActive,
		Permissions: []string{rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite}})
	if _, err := f.rentals.DecideCharge(noClose, f.org, r.UUID, r.Charges[0].UUID, true, ""); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("approve without close = %v", err)
	}
	r, err = f.rentals.DecideCharge(f.ctx, f.org, r.UUID, r.Charges[0].UUID, true, "")
	f.must(err)
	if r.Charges[0].Status != model.ChargeApproved {
		t.Fatalf("charge %+v", r.Charges[0])
	}
}

// TestSwapAndExtend: a mid-rental swap closes the old leg and opens a new
// one on another vehicle in the same rental (the reservation follows the
// new allocation); an extension of the rental under way uses the
// reservation's extension rule.
func TestSwapAndExtend(t *testing.T) {
	f := newFx(t)
	v1, v2 := f.vehicle(), f.vehicle()
	start := f.now().Add(time.Hour).Truncate(time.Minute)
	res := f.reserve(start, start.Add(72*time.Hour), nil)
	rental, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v1.uuid})
	f.must(err)
	_, err = f.rentals.Handover(f.ctx, f.org, rental.UUID, model.HandoverInput{Km: i32(1100), Fuel: i16(100)})
	f.must(err)
	f.advance(10 * time.Hour)
	r, err := f.rentals.Swap(f.ctx, f.org, rental.UUID, model.SwapInput{
		VehicleUUID: &v2.uuid, InKm: i32(1300), InFuel: i16(60), OutKm: i32(1000), OutFuel: i16(100), Reason: "arıza",
	})
	f.must(err)
	if r.Vehicle.UUID != v2.uuid || len(r.Legs) != 2 || r.Legs[0].InKm == nil || r.Legs[1].Reason != "swap" {
		t.Fatalf("after swap %+v", r)
	}
	if st, _, _ := f.vehicleState(v1.uuid); st != "return_check" {
		t.Fatalf("old vehicle %s", st)
	}
	if st, _, _ := f.vehicleState(v2.uuid); st != "rented" {
		t.Fatalf("new vehicle %s", st)
	}
	resv, err := f.res.Get(f.ctx, f.org, res.UUID)
	f.must(err)
	if resv.Plan.AssignedVehicle == nil || resv.Plan.AssignedVehicle.UUID != v2.uuid {
		t.Fatalf("reservation plan %+v", resv.Plan)
	}
	newEnd := r.PlannedReturnAt.Add(24 * time.Hour)
	r, err = f.rentals.Extend(f.ctx, f.org, r.UUID, model.ExtendInput{EndsAt: &newEnd, Reason: "müşteri talebi"})
	f.must(err)
	if !r.PlannedReturnAt.Equal(newEnd) {
		t.Fatalf("planned return %v want %v", r.PlannedReturnAt, newEnd)
	}
	_, endsAt, _, vehicle := f.allocation(r)
	if !endsAt.Equal(newEnd) || vehicle != v2.id {
		t.Fatalf("allocation %v %d", endsAt, vehicle)
	}
}

// TestRT_A18_ContractReapproval: in an approved contract a vehicle change
// (swap) or an amount change (extension) creates a new version; the
// previous approval is invalid for the new content and the handover-style
// check reports the contract as not approved until it is approved again.
func TestRT_A18_ContractReapproval(t *testing.T) {
	f := newFx(t)
	v1, v2 := f.vehicle(), f.vehicle()
	name := "Kiralama sözleşmesi"
	body := "<p>Kiracı: {{renter.full_name}}</p><p>Araç: {{vehicle.plate}}</p><p>Toplam: {{amounts.total}}</p>"
	tpl, err := f.contracts.CreateTemplate(f.ctx, f.org, contractsmodel.TemplateInput{Kind: contractsmodel.KindRentalContract, Name: &name, BodyHTML: &body})
	f.must(err)
	_, err = f.contracts.ActivateTemplate(f.ctx, f.org, tpl.UUID)
	f.must(err)

	start := f.now().Add(time.Hour).Truncate(time.Minute)
	res := f.reserve(start, start.Add(72*time.Hour), nil)
	rental, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v1.uuid})
	f.must(err)
	if !rental.Contract.Required || rental.Contract.Approved || rental.Contract.DocumentUUID == nil {
		t.Fatalf("contract state %+v", rental.Contract)
	}
	docID := *rental.Contract.DocumentUUID
	doc, err := f.contracts.GetDocument(f.ctx, f.org, docID)
	f.must(err)
	if doc.Current == nil || doc.Current.Version != 1 {
		t.Fatalf("document %+v", doc)
	}
	v1Version := *doc.Current
	ver, err := f.contracts.GetVersion(f.ctx, f.org, docID, v1Version.UUID)
	f.must(err)
	if !strings.Contains(ver.HTML, "34 RN 1") || !strings.Contains(ver.HTML, "Ayşe Yılmaz") {
		t.Fatalf("rendered html: %s", ver.HTML)
	}
	// Handover without approval needs a waiver.
	if _, err := f.rentals.Handover(f.ctx, f.org, rental.UUID, model.HandoverInput{Km: i32(1100), Fuel: i16(100)}); err == nil {
		t.Fatal("handover without an approved contract must be refused")
	}
	_, err = f.contracts.ApproveVersion(f.ctx, f.org, docID, v1Version.UUID, contractsmodel.ApproveInput{
		ContentHash: v1Version.ContentHash, ApproverName: "Ayşe Yılmaz",
	})
	f.must(err)
	r, err := f.rentals.Handover(f.ctx, f.org, rental.UUID, model.HandoverInput{Km: i32(1100), Fuel: i16(100)})
	f.must(err)
	if !r.Contract.Approved {
		t.Fatal("contract should be approved before the swap")
	}

	// Vehicle change → v2, v1 superseded, the approval invalid.
	f.advance(5 * time.Hour)
	r, err = f.rentals.Swap(f.ctx, f.org, rental.UUID, model.SwapInput{
		VehicleUUID: &v2.uuid, InKm: i32(1200), InFuel: i16(70), OutKm: i32(1000), OutFuel: i16(100), Reason: "arıza",
	})
	f.must(err)
	if r.Contract.Approved {
		t.Fatal("a vehicle change must require a new approval")
	}
	doc, err = f.contracts.GetDocument(f.ctx, f.org, docID)
	f.must(err)
	if doc.Current.Version != 2 || doc.Status != contractsmodel.DocumentDraft || !contains(doc.Current.ChangedKeys, "vehicle.plate") {
		t.Fatalf("after swap %+v", doc.Current)
	}
	if doc.Versions[1].Status != contractsmodel.VersionSuperseded || len(doc.Approvals) != 1 || doc.Approvals[0].Valid {
		t.Fatalf("v1 / approvals after swap: %+v %+v", doc.Versions, doc.Approvals)
	}
	// The old content's hash cannot approve the new version.
	var cc *contractsusecase.ContentChangedError
	if _, err := f.contracts.ApproveVersion(f.ctx, f.org, docID, doc.Current.UUID, contractsmodel.ApproveInput{
		ContentHash: v1Version.ContentHash, ApproverName: "Ayşe Yılmaz",
	}); !errors.As(err, &cc) {
		t.Fatalf("approving with the old hash = %v", err)
	}
	// Approving the superseded version is refused as well.
	if _, err := f.contracts.ApproveVersion(f.ctx, f.org, docID, v1Version.UUID, contractsmodel.ApproveInput{
		ContentHash: v1Version.ContentHash, ApproverName: "Ayşe Yılmaz",
	}); !errors.As(err, &cc) {
		t.Fatalf("approving the superseded version = %v", err)
	}
	_, err = f.contracts.ApproveVersion(f.ctx, f.org, docID, doc.Current.UUID, contractsmodel.ApproveInput{
		ContentHash: doc.Current.ContentHash, ApproverName: "Ayşe Yılmaz",
	})
	f.must(err)

	// Amount change (extension) → v3 needs a new approval again.
	newEnd := r.PlannedReturnAt.Add(24 * time.Hour)
	_, err = f.rentals.Extend(f.ctx, f.org, rental.UUID, model.ExtendInput{EndsAt: &newEnd})
	f.must(err)
	doc, err = f.contracts.GetDocument(f.ctx, f.org, docID)
	f.must(err)
	if doc.Current.Version != 3 || doc.Status != contractsmodel.DocumentDraft || !contains(doc.Current.ChangedKeys, "amounts.total") {
		t.Fatalf("after extension %+v", doc.Current)
	}
	// Nothing changed: a refresh renders no new version.
	out, err := f.contracts.RefreshDocument(f.ctx, f.org, docID)
	f.must(err)
	if out.Created {
		t.Fatal("a refresh without changes must not render a new version")
	}
	// The rendered version is immutable in the database.
	if _, err := f.pool.Exec(context.Background(), `UPDATE document_versions SET html = 'x' WHERE organization_id = $1`, f.org); err == nil {
		t.Fatal("document versions must be immutable")
	}
	// The PDF of the current version is stored privately and streamed.
	pdf, err := f.contracts.OpenPDF(f.ctx, f.org, docID, doc.Current.UUID)
	f.must(err)
	_ = pdf.Body.Close()
	if pdf.Size == 0 {
		t.Fatal("empty pdf")
	}
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
