package usecase_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	fleetopsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

func quoteCode(err error) string {
	var qe *usecase.QuoteError
	if errors.As(err, &qe) {
		return qe.Code
	}
	return ""
}

// Revalidation at confirmation: an expired quote, a price that changed
// since (superseded by a fresh quote) and capacity that is gone.
func TestQuoteRevalidation(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	drivers := []model.DriverInput{{DriverUUID: d.uuid}}

	// Expired (default validity 30 minutes).
	q := f.quote(f.ctx, f.svc, p, f.at(0), f.at(24))
	if q.Availability == nil || q.Availability.Sellable != 1 {
		t.Fatalf("quote availability %+v", q.Availability)
	}
	if f.activeAllocations() != 0 {
		t.Fatal("a quote consumed capacity")
	}
	f.advance(31 * time.Minute)
	if _, err := f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &c.uuid, Drivers: drivers}); quoteCode(err) != "QUOTE_EXPIRED" {
		t.Fatalf("expired quote: %v", err)
	}

	// Price changed: a more specific (branch) plan became active.
	q2 := f.quote(f.ctx, f.svc, p, f.at(0), f.at(24))
	f.plan(p, "branch", f.mrk.id, "1700", "1600", "1500")
	_, err := f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &q2.UUID, CustomerUUID: &c.uuid, Drivers: drivers})
	var qe *usecase.QuoteError
	if !errors.As(err, &qe) || qe.Code != "QUOTE_PRICE_CHANGED" || qe.Replacement == nil {
		t.Fatalf("changed price: %v", err)
	}
	if !qe.Replacement.Total.Equal(money.MustParse("1700")) || !q2.Total.Equal(money.MustParse("1500")) {
		t.Fatalf("old %s / new %s", q2.Total, qe.Replacement.Total)
	}
	old, _ := f.svc.GetQuote(f.ctx, f.org, q2.UUID)
	if old.Status != model.QuoteSuperseded {
		t.Fatalf("old quote %s, want superseded", old.Status)
	}
	// The replacement goes through.
	r, err := f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &qe.Replacement.UUID, CustomerUUID: &c.uuid, Drivers: drivers})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Price.Total.Equal(money.MustParse("1700")) || r.Price.Snapshot.QuoteUUID != qe.Replacement.UUID {
		t.Fatalf("snapshot %+v", r.Price.Snapshot)
	}
	// A used quote cannot be used twice.
	if _, err := f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &qe.Replacement.UUID, CustomerUUID: &c.uuid, Drivers: drivers}); quoteCode(err) != "QUOTE_USED" {
		t.Fatalf("reused quote: %v", err)
	}

	// Capacity gone: the only vehicle is sold for the range.
	q3 := f.quote(f.ctx, f.svc, p, f.at(2), f.at(20))
	_, err = f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &q3.UUID, CustomerUUID: &c.uuid, Drivers: drivers})
	if _, ok := availusecase.IsCapacity(err); !ok {
		t.Fatalf("capacity gone: %v", err)
	}
	if again, _ := f.svc.GetQuote(f.ctx, f.org, q3.UUID); again.Status != model.QuoteOpen {
		t.Fatalf("a refused sale used the quote: %s", again.Status)
	}
}

// Date change is atomic: into a full period the old plan stays (dates,
// allocation, price), into a free one it moves.
func TestChangeDatesAtomic(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r1 := f.reserve(p, c, d, f.at(0), f.at(24))
	r2 := f.reserve(p, c, d, f.at(48), f.at(72))
	_, _, before := f.allocation(r1.UUID)
	cq, err := f.svc.ChangeQuote(f.ctx, f.org, r1.UUID, model.ChangeQuoteInput{Where: model.Where{StartsAt: ptr(f.at(40)), EndsAt: ptr(f.at(60))}})
	if err != nil {
		t.Fatal(err)
	}
	if cq.Purpose != model.PurposeChange || cq.Difference == nil {
		t.Fatalf("change quote %+v", cq.Purpose)
	}
	_, err = f.svc.ApplyChange(f.ctx, f.org, r1.UUID, model.ApplyChangeInput{QuoteUUID: &cq.UUID})
	if _, ok := availusecase.IsCapacity(err); !ok {
		t.Fatalf("change into r2's range: %v", err)
	}
	got, _ := f.svc.Get(f.ctx, f.org, r1.UUID)
	if !got.StartsAt.Equal(f.at(0)) || !got.EndsAt.Equal(f.at(24)) || !got.Price.Total.Equal(r1.Price.Total) {
		t.Fatalf("old plan changed: %v - %v %s", got.StartsAt, got.EndsAt, got.Price.Total)
	}
	if _, status, v := f.allocation(r1.UUID); status != "active" || v != before {
		t.Fatalf("allocation %s on %d", status, v)
	}
	r2now, _ := f.svc.Get(f.ctx, f.org, r2.UUID)
	if !r2now.StartsAt.Equal(f.at(48)) {
		t.Fatal("r2 moved")
	}
	ok, err := f.svc.ChangeQuote(f.ctx, f.org, r1.UUID, model.ChangeQuoteInput{Where: model.Where{StartsAt: ptr(f.at(100)), EndsAt: ptr(f.at(172))}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.ApplyChange(f.ctx, f.org, r1.UUID, model.ApplyChangeInput{QuoteUUID: &ok.UUID, Reason: "müşteri talebi"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reservation.StartsAt.Equal(f.at(100)) || !res.Reservation.Price.Total.Equal(money.MustParse("4500")) {
		t.Fatalf("after change %v total %s", res.Reservation.StartsAt, res.Reservation.Price.Total)
	}
	if res.Difference == nil || !res.Difference.Equal(money.MustParse("3000")) {
		t.Fatalf("difference %v, want 3000", res.Difference)
	}
	if f.activeAllocations() != 2 {
		t.Fatalf("%d active allocations, want 2", f.activeAllocations())
	}
}

// Extension: past lines are kept as sold; the added days are priced with
// the band of the NEW total length (3 days at 1500, extended to 5 days →
// the 4-7 band, 1300, applies to the 2 added days: +2600).
func TestExtensionUsesNewTotalBand(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21, "1500", "1300", "1000")
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(72))
	if !r.Price.Total.Equal(money.MustParse("4500")) {
		t.Fatalf("3 days = %s, want 4500", r.Price.Total)
	}
	q, err := f.svc.ChangeQuote(f.ctx, f.org, r.UUID, model.ChangeQuoteInput{Where: model.Where{EndsAt: ptr(f.at(120))}})
	if err != nil {
		t.Fatal(err)
	}
	if q.Purpose != model.PurposeExtension || q.Breakdown.Extension == nil {
		t.Fatalf("purpose %s", q.Purpose)
	}
	ext := q.Breakdown.Extension
	if ext.PreviousBillingDays != 3 || ext.BillingDays != 5 || ext.AddedDays != 2 {
		t.Fatalf("days %d → %d (+%d)", ext.PreviousBillingDays, ext.BillingDays, ext.AddedDays)
	}
	if !ext.Band.DailyPrice.Equal(money.MustParse("1300")) || !ext.Amount.Equal(money.MustParse("2600")) {
		t.Fatalf("band %s amount %s, want 1300 / 2600", ext.Band.DailyPrice, ext.Amount)
	}
	if !q.Total.Equal(money.MustParse("2600")) || q.Difference == nil || !q.Difference.Equal(money.MustParse("2600")) {
		t.Fatalf("quote total %s diff %v", q.Total, q.Difference)
	}
	res, err := f.svc.ApplyChange(f.ctx, f.org, r.UUID, model.ApplyChangeInput{QuoteUUID: &q.UUID})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Reservation
	if !got.Price.Total.Equal(money.MustParse("7100")) || !got.EndsAt.Equal(f.at(120)) {
		t.Fatalf("after extension total %s ends %v", got.Price.Total, got.EndsAt)
	}
	snap := got.Price.Snapshot
	if snap.Pricing == nil || len(snap.Pricing.Days) != 3 || !snap.Pricing.Days[0].UnitPrice.Equal(money.MustParse("1500")) {
		t.Fatalf("past lines re-priced: %+v", snap.Pricing)
	}
	if len(snap.Extensions) != 1 || snap.Extensions[0].Days[0].Index != 4 {
		t.Fatalf("extensions %+v", snap.Extensions)
	}
	// A whole re-price of 5 days would be 6500: the band rule is not a
	// re-price of the past days.
	if got.Price.Total.Equal(money.MustParse("6500")) {
		t.Fatal("extension re-priced the past days")
	}
}

// RT-A31: an extension that collides with the next (promised) reservation
// on the same vehicle is refused; another vehicle with a difference quote
// is offered and applying it moves the reservation and extends it.
func TestRT_A31_ConflictingExtensionOffersAlternative(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21)
	v1 := f.vehicle(f.mrk)
	v2 := f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r1 := f.reserve(p, c, d, f.at(0), f.at(48))
	r2 := f.reserve(p, c, d, f.at(50), f.at(96))
	for _, r := range []model.Reservation{r1, r2} {
		if _, err := f.svc.Plate(f.ctx, f.org, r.UUID, model.PlateInput{VehicleUUID: &v1.uuid, Fix: true, Reason: "müşteri bu aracı istedi"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.svc.Extend(f.ctx, f.org, r1.UUID, model.ExtendInput{EndsAt: ptr(f.at(72)), Reason: "uzatma"})
	var xe *usecase.ExtensionConflictError
	if !errors.As(err, &xe) {
		t.Fatalf("conflicting extension: %v", err)
	}
	alts := xe.Alternatives
	if len(alts.Vehicles) != 1 || alts.Vehicles[0].Vehicle.UUID != v2.uuid {
		t.Fatalf("vehicle alternatives %+v", alts.Vehicles)
	}
	alt := alts.Vehicles[0].Quote
	if alt.Difference == nil || !alt.Difference.Equal(money.MustParse("1500")) || alt.Vehicle == nil {
		t.Fatalf("difference quote %+v", alt)
	}
	got, _ := f.svc.Get(f.ctx, f.org, r1.UUID)
	if !got.EndsAt.Equal(f.at(48)) || got.Plan.AssignedVehicle.UUID != v1.uuid {
		t.Fatal("refused extension changed the reservation")
	}
	if got.OpenTasks != 1 {
		t.Fatalf("open tasks %d, want the extension_conflict task", got.OpenTasks)
	}
	res, err := f.svc.ApplyChange(f.ctx, f.org, r1.UUID, model.ApplyChangeInput{QuoteUUID: &alt.UUID, Reason: "alternatif araç"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reservation.EndsAt.Equal(f.at(72)) || res.Reservation.Plan.AssignedVehicle == nil || res.Reservation.Plan.AssignedVehicle.UUID != v2.uuid {
		t.Fatalf("after alternative: ends %v plan %+v", res.Reservation.EndsAt, res.Reservation.Plan.AssignedVehicle)
	}
	if !res.Reservation.Price.Total.Equal(money.MustParse("4500")) {
		t.Fatalf("total %s, want 3000 + 1500", res.Reservation.Price.Total)
	}
	r2now, _ := f.svc.Get(f.ctx, f.org, r2.UUID)
	if r2now.Plan.AssignedVehicle.UUID != v1.uuid {
		t.Fatal("r2's promised plate moved")
	}
}

// A block over a reservation (fleetops affected[]): re-planned onto another
// vehicle when possible (recorded), otherwise a resolution task — never an
// automatic cancellation.
func TestBlockReplansOrOpensResolutionTask(t *testing.T) {
	f := newFx(t)
	fleetOps := fleetopsusecase.New(fleetopsrepository.NewPostgres(f.pool, f.q), f.vehicles)
	fleetOps.SetAffectedHandler(adapters.NewAffected(f.svc))
	p := f.product("EKO", 21)
	v1 := f.vehicle(f.mrk)
	v2 := f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r1 := f.reserve(p, c, d, f.at(0), f.at(24))
	_, _, planned := f.allocation(r1.UUID)
	blocked, free := v1, v2
	if planned == v2.id {
		blocked, free = v2, v1
	}
	kind := "maintenance"
	from, to := f.at(-2), f.at(30)
	if _, err := fleetOps.CreateBlock(f.ctx, f.org, blocked.uuid, fleetopsmodel.BlockInput{Kind: &kind, StartsAt: &from, EndsAt: fleetopsmodel.OptionalTime{Set: true, Value: &to}}); err != nil {
		t.Fatal(err)
	}
	if _, _, now := f.allocation(r1.UUID); now != free.id {
		t.Fatalf("re-plan: on %d, want %d", now, free.id)
	}
	ev := f.events(r1.UUID)
	if ev[len(ev)-1] != usecase.EventReplanned {
		t.Fatalf("events %v", ev)
	}
	// Now block the other vehicle too: nothing can carry it → task.
	if _, err := fleetOps.CreateBlock(f.ctx, f.org, free.uuid, fleetopsmodel.BlockInput{Kind: &kind, StartsAt: &from, EndsAt: fleetopsmodel.OptionalTime{Set: true, Value: &to}}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(f.ctx, f.org, r1.UUID)
	if got.Status != model.StatusConfirmed || got.OpenTasks != 1 {
		t.Fatalf("after second block: %s, %d tasks", got.Status, got.OpenTasks)
	}
	tasks, total, err := f.svc.ListTasks(f.ctx, f.org, model.TaskFilter{ReservationUUIDs: []uuid.UUID{r1.UUID}})
	if err != nil || total != 1 || tasks[0].TaskType != model.TaskBlockConflict || tasks[0].Status != model.TaskOpen {
		t.Fatalf("tasks %+v %v", tasks, err)
	}
	// Retrying the re-plan still fails; resolving closes the task.
	if out, err := f.svc.ReplanTask(f.ctx, f.org, tasks[0].UUID); err != nil || out.Replanned {
		t.Fatalf("replan task: %+v %v", out, err)
	}
	if done, err := f.svc.ResolveTask(f.ctx, f.org, tasks[0].UUID, "müşteri ile görüşüldü"); err != nil || done.Status != model.TaskResolved {
		t.Fatalf("resolve: %+v %v", done, err)
	}
}

// Driver eligibility: an under-age driver blocks the sale; the override
// needs the permission and a reason, and is recorded.
func TestDriverEligibilityAndOverride(t *testing.T) {
	f := newFx(t)
	p := f.product("PRM", 25)
	f.vehicle(f.mrk)
	c, young := f.customer(22, 3)
	q := f.quote(f.ctx, f.svc, p, f.at(0), f.at(24))
	in := model.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &c.uuid, Drivers: []model.DriverInput{{DriverUUID: young.uuid}}}
	_, err := f.svc.Create(f.ctx, f.org, in)
	var ee *usecase.EligibilityError
	if !errors.As(err, &ee) || len(ee.Drivers) != 1 || ee.Drivers[0].Reasons[0] != "under_min_age" {
		t.Fatalf("under-age driver: %v", err)
	}
	in.EligibilityOverride = &model.OverrideInput{Reason: "kurumsal müşteri onayı"}
	sales := f.scoped(nil, rbac.PermTenantReservationsWrite, rbac.PermTenantQuotesWrite, rbac.PermTenantReservationsRead)
	if _, err := f.svc.Create(sales, f.org, in); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("override without permission: %v", err)
	}
	r, err := f.svc.Create(f.ctx, f.org, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Drivers) != 1 || r.Drivers[0].Eligible || r.Drivers[0].OverrideReason == "" || len(r.EligibilityOverride) == 0 {
		t.Fatalf("drivers %+v override %s", r.Drivers, r.EligibilityOverride)
	}
}

// Branch scope: visible when the pick-up OR the return branch is in the
// member's scope; otherwise not found everywhere.
func TestBranchScope(t *testing.T) {
	f := newFx(t)
	other := f.branch("ANK", 60)
	third := f.branch("IZM", 60)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(24))
	for name, branches := range map[string][]int64{"other": {other.id}, "third": {third.id}} {
		ctx := f.scoped(branches, allPerms...)
		if _, err := f.svc.Get(ctx, f.org, r.UUID); !errors.Is(err, usecase.ErrNotFound) {
			t.Errorf("%s: get = %v", name, err)
		}
		if items, total, _ := f.svc.List(ctx, f.org, model.ListFilter{}); total != 0 || len(items) != 0 {
			t.Errorf("%s: list shows %d", name, total)
		}
		if _, err := f.svc.Cancel(ctx, f.org, r.UUID, "kapsam dışı"); !errors.Is(err, usecase.ErrNotFound) {
			t.Errorf("%s: cancel = %v", name, err)
		}
		facets, _ := f.svc.Facets(ctx, f.org)
		if len(facets.Status) != 0 {
			t.Errorf("%s: facets %+v", name, facets.Status)
		}
	}
	mine := f.scoped([]int64{f.mrk.id}, allPerms...)
	if _, err := f.svc.Get(mine, f.org, r.UUID); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := f.svc.List(mine, f.org, model.ListFilter{}); total != 1 {
		t.Fatalf("own branch list %d", total)
	}
}

// Another organization's quote / reservation uuid is not found.
func TestIsolationAcrossOrganizations(t *testing.T) {
	a, b := newFx(t), newFx(t)
	p := b.product("EKO", 21)
	b.vehicle(b.mrk)
	c, d := b.customer(30, 5)
	r := b.reserve(p, c, d, b.at(0), b.at(24))
	q := b.quote(b.ctx, b.svc, p, b.at(30), b.at(40))
	if _, err := a.svc.Get(a.ctx, a.org, r.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign reservation: %v", err)
	}
	if _, err := a.svc.GetQuote(a.ctx, a.org, q.UUID); !errors.Is(err, usecase.ErrQuoteNotFound) {
		t.Fatalf("foreign quote: %v", err)
	}
	ac, _ := a.customer(30, 5)
	if _, err := a.svc.Create(a.ctx, a.org, model.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &ac.uuid}); err == nil {
		t.Fatal("created from another organization's quote")
	}
}

// Two reservations racing for the last car: exactly one wins (run with
// -race -count=10).
func TestLastCarConcurrency(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	const n = 4
	quotes := make([]model.Quote, n)
	services := make([]*usecase.Service, n)
	for i := range quotes {
		_, services[i] = f.services()
		quotes[i] = f.quote(f.ctx, services[i], p, f.at(0), f.at(24))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, refused := 0, 0
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := services[i].Create(f.ctx, f.org, model.CreateInput{
				QuoteUUID: &quotes[i].UUID, CustomerUUID: &c.uuid, Drivers: []model.DriverInput{{DriverUUID: d.uuid}},
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else if _, ok := availusecase.IsCapacity(err); ok {
				refused++
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if wins != 1 || refused != n-1 {
		t.Fatalf("wins %d refused %d", wins, refused)
	}
	if f.activeAllocations() != 1 {
		t.Fatalf("%d active allocations", f.activeAllocations())
	}
	var count int
	_ = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM reservations WHERE organization_id = $1`, f.org).Scan(&count)
	if count != 1 {
		t.Fatalf("%d reservations, want 1", count)
	}
}

// Taking the planned vehicle out of the product's rental group re-plans the
// reservation onto another member (plan_moved, cause
// rental_group_membership); when no member is left the removal is refused
// with the reservation listed.
func TestGroupMembershipRemovalReplansReservation(t *testing.T) {
	f := newFx(t)
	f.vehicles.SetMembershipGuard(f.avail)
	f.avail.SetPlanMovedHandler(f.svc)
	p := f.product("EKO", 21)
	v1 := f.vehicle(f.mrk)
	v2 := f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r1 := f.reserve(p, c, d, f.at(0), f.at(24))
	var group uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT uuid FROM rental_groups WHERE organization_id = $1 AND code = 'GEKO'`, f.org).Scan(&group); err != nil {
		t.Fatal(err)
	}
	_, _, planned := f.allocation(r1.UUID)
	leaving, staying := v1, v2
	if planned == v2.id {
		leaving, staying = v2, v1
	}
	if err := f.vehicles.RemoveGroup(f.ctx, f.org, leaving.uuid, group); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, _, now := f.allocation(r1.UUID); now != staying.id {
		t.Fatalf("re-plan: on %d, want %d", now, staying.id)
	}
	if ev := f.events(r1.UUID); ev[len(ev)-1] != usecase.EventPlanMoved {
		t.Fatalf("events %v", ev)
	}
	err := f.vehicles.RemoveGroup(f.ctx, f.org, staying.uuid, group)
	var inUse *vehicleusecase.MembershipInUseError
	if !errors.As(err, &inUse) || len(inUse.Allocations) != 1 || inUse.Allocations[0].Reservation == nil ||
		inUse.Allocations[0].Reservation.UUID != r1.UUID || inUse.Allocations[0].Reservation.Reference != r1.Reference {
		t.Fatalf("remove last member = %v", err)
	}
	if _, _, now := f.allocation(r1.UUID); now != staying.id {
		t.Fatal("a refused removal moved the reservation")
	}
}

func ptr(t time.Time) *time.Time { return &t }
