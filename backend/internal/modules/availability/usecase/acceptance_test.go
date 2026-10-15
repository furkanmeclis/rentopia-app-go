package usecase_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/jackc/pgx/v5/pgconn"
)

// RT-A03: 30 vehicles, one product → the search shows that product with
// capacity 30.
func TestRT_A03_ThirtyVehiclesOneProduct(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 60)
	for i := 0; i < 30; i++ {
		f.vehicle(0, mrk, "MAIN")
	}
	p := f.product("EKO", "GROUP")
	res, err := f.svc.Search(f.ctx, f.org, model.SearchInput{Where: where(mrk, mrk, f.at(0), f.at(72))})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Product.UUID != p.uuid {
		t.Fatalf("items = %+v, want the one product", res.Items)
	}
	b := res.Items[0].Breakdown
	if res.Items[0].Sellable != 30 || b.Eligible != 30 || b.Free != 30 || b.Physical != 30 || b.Reassignable != 0 {
		t.Fatalf("breakdown = %+v", b)
	}
	if res.Request.PrepMinutes != 60 || !res.Request.ReadyAt.Equal(f.at(73)) || res.Request.Channel != "branch" {
		t.Fatalf("request echo = %+v", res.Request)
	}
	// One hold → 29; the breakdown shows it as an option.
	f.mustHold(p, where(mrk, mrk, f.at(10), f.at(20)))
	got := f.search(where(mrk, mrk, f.at(0), f.at(72)))[p.uuid]
	if got.Sellable != 29 || got.Breakdown.Held != 1 || got.Breakdown.Free != 29 {
		t.Fatalf("after a hold: %+v", got.Breakdown)
	}
}

// RT-A04: two (and more) concurrent requests for the last unit, each on its
// own connection: exactly one succeeds. Run once with the component lock
// and once without it (the EXCLUDE constraint plus retries alone must keep
// the answer correct).
func TestRT_A04_LastUnitConcurrency(t *testing.T) {
	for _, mode := range []string{"lock", "exclude-only"} {
		t.Run(mode, func(t *testing.T) {
			f := newFx(t)
			mrk := f.branch("MRK", 30)
			f.vehicle(0, mrk, "MAIN")
			f.vehicle(0, mrk, "MAIN")
			p := f.product("EKO", "GROUP")
			w := where(mrk, mrk, f.at(0), f.at(24))
			f.mustHold(p, w) // one of two vehicles taken: one unit left
			for _, n := range []int{2, 6} {
				// Each round competes for one free unit: release what the
				// previous round sold.
				services := make([]*usecase.Service, n)
				for i := range services {
					services[i] = f.newService()
					if mode == "exclude-only" {
						services[i].DisableComponentLock()
					}
				}
				var wg sync.WaitGroup
				results := make([]error, n)
				start := make(chan struct{})
				for i := 0; i < n; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						<-start
						res, err := services[i].Reserve(f.ctx, f.org, model.ReserveInput{Where: w, ProductUUID: p.uuid, Kind: model.KindHold})
						if err == nil && res.Allocation.UUID.String() == "" {
							err = errors.New("empty allocation")
						}
						results[i] = err
					}(i)
				}
				close(start)
				wg.Wait()
				ok, capacity := 0, 0
				for _, err := range results {
					switch {
					case err == nil:
						ok++
					case isCapacity(err):
						capacity++
					default:
						t.Fatalf("unexpected error: %v", err)
					}
				}
				if ok != 1 || capacity != n-1 {
					t.Fatalf("%d concurrent requests for the last unit: %d succeeded, %d refused", n, ok, capacity)
				}
				f.invariants(nil)
				// Free the unit for the next round.
				var last string
				if err := f.pool.QueryRow(f.ctx, `SELECT uuid::text FROM vehicle_allocations WHERE organization_id = $1 AND status = 'active' ORDER BY id DESC LIMIT 1`, f.org).Scan(&last); err != nil {
					t.Fatal(err)
				}
				if _, err := f.pool.Exec(f.ctx, `UPDATE vehicle_allocations SET status = 'released', released_at = now() WHERE uuid = $1`, last); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// RT-A05: picked up at A, returned at B → after the preparation time the
// vehicle is sellable at B, not at A.
func TestRT_A05_OneWayReturnMovesCapacity(t *testing.T) {
	f := newFx(t)
	a, b := f.branch("AAA", 30), f.branch("BBB", 90)
	f.allowOneWay(a, b)
	f.vehicle(0, a, "MAIN")
	p := f.product("EKO", "GROUP")
	res := f.mustHold(p, where(a, b, f.at(0), f.at(10)))
	if !res.Allocation.ReadyAt.Equal(f.at(10).Add(90 * time.Minute)) {
		t.Fatalf("ready_at = %v, want return + 90 min (return branch preparation)", res.Allocation.ReadyAt)
	}
	afterPrep := f.search(where(b, b, f.at(12), f.at(20)), p)[p.uuid]
	if afterPrep.Sellable != 1 {
		t.Fatalf("at B after preparation: %+v", afterPrep.Breakdown)
	}
	atA := f.search(where(a, a, f.at(12), f.at(20)), p)[p.uuid]
	if atA.Sellable != 0 || atA.Breakdown.Elsewhere != 1 {
		t.Fatalf("at A after the one-way return: %+v", atA.Breakdown)
	}
	duringPrep := f.search(where(b, b, f.at(11), f.at(20)), p)[p.uuid]
	if duringPrep.Sellable != 0 || duringPrep.Breakdown.Held != 1 {
		t.Fatalf("at B inside the preparation buffer: %+v", duringPrep.Breakdown)
	}
	// Without a one-way rule the route is not sellable at all.
	c := f.branch("CCC", 0)
	back := f.search(where(b, c, f.at(30), f.at(40)), p)[p.uuid]
	if back.Sellable != 0 || len(back.Reasons) != 1 || back.Reasons[0] != model.ReasonOneWayNotAllowed {
		t.Fatalf("one-way without a rule: %+v", back)
	}
	f.invariants(nil)
}

// RT-A06: a block in the middle of the range → not sellable; the database
// refuses an allocation over a block even when the engine is bypassed.
func TestRT_A06_BlockInTheMiddle(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 60)
	v := f.vehicle(0, mrk, "MAIN")
	p := f.product("EKO", "GROUP")
	f.block(v, f.at(20), f.at(22))
	got := f.search(where(mrk, mrk, f.at(10), f.at(40)), p)[p.uuid]
	if got.Sellable != 0 || got.Breakdown.Blocked != 1 {
		t.Fatalf("breakdown = %+v", got.Breakdown)
	}
	if _, err := f.hold(p, where(mrk, mrk, f.at(10), f.at(40))); !isCapacity(err) {
		t.Fatalf("hold over a block = %v, want CAPACITY_UNAVAILABLE", err)
	}
	if got := f.search(where(mrk, mrk, f.at(23), f.at(40)), p)[p.uuid]; got.Sellable != 1 {
		t.Fatalf("after the block: %+v", got.Breakdown)
	}
	_, err := f.pool.Exec(f.ctx, `INSERT INTO vehicle_allocations (organization_id, vehicle_id, rental_product_id, product_version, kind,
		starts_at, ends_at, period, pickup_branch_id, return_branch_id, fleet_pool, channel)
		VALUES ($1, $2, $3, 1, 'reservation', $4, $5, tstzrange($4, $5, '[)'), $6, $6, 'MAIN', 'branch')`,
		f.org, v.id, p.id, f.at(10), f.at(40), mrk.id)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "23P01" {
		t.Fatalf("direct insert over a block = %v, want exclusion_violation", err)
	}
}

// The EXCLUDE constraint refuses double booking at the database level.
func TestExcludeConstraintRefusesDoubleBooking(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	v := f.vehicle(0, mrk, "MAIN")
	p := f.product("EKO", "GROUP")
	insert := func(from, to time.Time) error {
		_, err := f.pool.Exec(f.ctx, `INSERT INTO vehicle_allocations (organization_id, vehicle_id, rental_product_id, product_version, kind,
			starts_at, ends_at, period, pickup_branch_id, return_branch_id, fleet_pool, channel)
			VALUES ($1, $2, $3, 1, 'reservation', $4, $5, tstzrange($4, $5, '[)'), $6, $6, 'MAIN', 'branch')`,
			f.org, v.id, p.id, from, to, mrk.id)
		return err
	}
	if err := insert(f.at(0), f.at(10)); err != nil {
		t.Fatal(err)
	}
	var pe *pgconn.PgError
	if err := insert(f.at(5), f.at(15)); !errors.As(err, &pe) || pe.Code != "23P01" || pe.ConstraintName != "ex_vehicle_allocations_overlap" {
		t.Fatalf("overlapping insert = %v", err)
	}
	if err := insert(f.at(10), f.at(15)); err != nil {
		t.Fatalf("adjacent (half-open) insert refused: %v", err)
	}
}

// Report §15: a model-guaranteed product is not sold when only a different
// model is free.
func TestModelGuaranteeNotSoldOnOtherModel(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	egeaModel, egeaType := f.model("Egea")
	clioModel, clioType := f.model("Clio")
	egea := f.vehicle(egeaType, mrk, "MAIN")
	f.vehicle(clioType, mrk, "MAIN")
	exact := f.product("EGEA", "EXACT_VARIANT", egeaModel)
	compact := f.product("KOMPAKT", "GROUP", egeaModel, clioModel)
	f.block(egea, f.at(0), f.at(100))
	got := f.search(where(mrk, mrk, f.at(10), f.at(20)))
	if got[exact.uuid].Sellable != 0 || got[exact.uuid].Breakdown.Eligible != 1 || got[exact.uuid].Breakdown.Blocked != 1 {
		t.Fatalf("exact model with only the other model free: %+v", got[exact.uuid].Breakdown)
	}
	if got[compact.uuid].Sellable != 1 || got[compact.uuid].Breakdown.Eligible != 2 {
		t.Fatalf("group product: %+v", got[compact.uuid].Breakdown)
	}
	if _, err := f.hold(exact, where(mrk, mrk, f.at(10), f.at(20))); !isCapacity(err) {
		t.Fatalf("exact model hold = %v, want CAPACITY_UNAVAILABLE", err)
	} else if ce, _ := usecase.IsCapacity(err); ce.Alternatives == nil || len(ce.Alternatives.Products) != 1 || ce.Alternatives.Products[0].Product.UUID != compact.uuid {
		t.Fatalf("alternatives = %+v", ce.Alternatives)
	}
}

// Report §15: a general group and an exact model share cars. A new sale is
// accepted when all commitments stay satisfiable (it needs a reassignment)
// and refused when they would not.
func TestSharedCarsReassignment(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	egeaModel, egeaType := f.model("Egea")
	clioModel, clioType := f.model("Clio")
	egea := f.vehicle(egeaType, mrk, "MAIN")
	clio := f.vehicle(clioType, mrk, "MAIN")
	exact := f.product("EGEA", "EXACT_VARIANT", egeaModel)
	compact := f.product("KOMPAKT", "GROUP", egeaModel, clioModel)
	eligible := map[int64]map[int64]bool{
		exact.id: {egea.id: true}, compact.id: {egea.id: true, clio.id: true},
	}
	// The Clio is in maintenance when the compact is sold: it lands on the
	// Egea. Then the maintenance is cancelled.
	blockID := f.block(clio, f.at(0), f.at(50))
	sold := f.mustHold(compact, where(mrk, mrk, f.at(10), f.at(20)))
	if f.vehicleOf(sold.Allocation.UUID) != egea.id {
		t.Fatal("compact did not land on the Egea")
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE vehicle_blocks SET deleted_at = now() WHERE uuid = $1`, blockID); err != nil {
		t.Fatal(err)
	}
	// The Egea product needs the Egea: accepted by moving the compact.
	before := f.search(where(mrk, mrk, f.at(12), f.at(18)), exact)[exact.uuid]
	if before.Sellable != 1 || before.Breakdown.Free != 0 || before.Breakdown.Reassignable != 1 {
		t.Fatalf("search before the reassignment: %+v", before.Breakdown)
	}
	res := f.mustHold(exact, where(mrk, mrk, f.at(12), f.at(18)))
	if res.Reassigned != 1 || f.vehicleOf(res.Allocation.UUID) != egea.id || f.vehicleOf(sold.Allocation.UUID) != clio.id {
		t.Fatalf("reassignment: %+v (compact on %d)", res, f.vehicleOf(sold.Allocation.UUID))
	}
	f.invariants(eligible)
	// Every car is committed: neither product can take another overlapping
	// sale (the compact would need a third car).
	if _, err := f.hold(exact, where(mrk, mrk, f.at(11), f.at(13))); !isCapacity(err) {
		t.Fatalf("second exact sale = %v", err)
	}
	if _, err := f.hold(compact, where(mrk, mrk, f.at(14), f.at(16))); !isCapacity(err) {
		t.Fatalf("second compact sale = %v", err)
	}
	f.invariants(eligible)
	// A promised plate is never moved: fix the compact on the Clio, free
	// the Egea, then another exact sale fits only on the Egea.
	if _, err := f.svc.AssignPlate(f.ctx, f.org, sold.Allocation.UUID, clio.uuid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Release(f.ctx, f.org, res.Allocation.UUID, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AssignPlate(f.ctx, f.org, sold.Allocation.UUID, egea.uuid); err == nil {
		t.Fatal("re-assigned a promised plate")
	}
	f.mustHold(exact, where(mrk, mrk, f.at(12), f.at(18)))
	f.invariants(eligible)
}

// Date change into a full period keeps the old reservation; into a free one
// it swaps atomically (overlapping its own old range is fine).
func TestChangeDates(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	f.vehicle(0, mrk, "MAIN")
	p := f.product("EKO", "GROUP")
	first := f.mustHold(p, where(mrk, mrk, f.at(0), f.at(10)))
	f.mustHold(p, where(mrk, mrk, f.at(20), f.at(30)))
	_, err := f.svc.ChangeDates(f.ctx, f.org, first.Allocation.UUID, model.ChangeInput{Where: where(mrk, mrk, f.at(15), f.at(25))})
	if !isCapacity(err) {
		t.Fatalf("change into a full period = %v", err)
	}
	a, err := f.svc.GetAllocation(f.ctx, f.org, first.Allocation.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !a.StartsAt.Equal(f.at(0)) || !a.EndsAt.Equal(f.at(10)) || a.Status != model.StatusActive {
		t.Fatalf("old reservation changed: %+v", a)
	}
	moved, err := f.svc.ChangeDates(f.ctx, f.org, first.Allocation.UUID, model.ChangeInput{Where: where(mrk, mrk, f.at(5), f.at(19))})
	if err != nil {
		t.Fatal(err)
	}
	if !moved.Allocation.StartsAt.Equal(f.at(5)) || !moved.Allocation.EndsAt.Equal(f.at(19)) {
		t.Fatalf("after change: %+v", moved.Allocation)
	}
	f.invariants(nil)
}

// Holds: a quote consumes nothing; a hold consumes; confirming and
// assigning a plate never consume twice; release / expiry free exactly once.
func TestHoldLifecycle(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	v1 := f.vehicle(0, mrk, "MAIN")
	f.vehicle(0, mrk, "MAIN")
	p := f.product("EKO", "GROUP")
	w := where(mrk, mrk, f.at(0), f.at(10))
	sellable := func() int { return f.search(w, p)[p.uuid].Sellable }
	for i := 0; i < 3; i++ {
		if sellable() != 2 {
			t.Fatal("a quote consumed capacity")
		}
	}
	minutes := 5
	h, err := f.svc.Reserve(f.ctx, f.org, model.ReserveInput{Where: w, ProductUUID: p.uuid, Kind: model.KindHold, HoldMinutes: &minutes})
	if err != nil {
		t.Fatal(err)
	}
	if h.Allocation.ExpiresAt == nil || h.Allocation.Kind != model.KindHold || sellable() != 1 {
		t.Fatalf("hold: %+v sellable %d", h.Allocation, sellable())
	}
	// Confirm (TEC-706) and assign the plate: still one unit.
	if _, err := f.svc.Confirm(f.ctx, f.org, h.Allocation.UUID, "reservation", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Confirm(f.ctx, f.org, h.Allocation.UUID, "reservation", nil); !errors.Is(err, usecase.ErrNotHold) {
		t.Fatalf("second confirm = %v", err)
	}
	if _, err := f.svc.AssignPlate(f.ctx, f.org, h.Allocation.UUID, v1.uuid); err != nil {
		t.Fatal(err)
	}
	if got := f.search(w, p)[p.uuid]; got.Sellable != 1 || got.Breakdown.Reserved != 1 {
		t.Fatalf("after confirm + plate: %+v", got.Breakdown)
	}
	// Release frees exactly once.
	if _, err := f.svc.Release(f.ctx, f.org, h.Allocation.UUID, "cancelled", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Release(f.ctx, f.org, h.Allocation.UUID, "cancelled", nil); !errors.Is(err, usecase.ErrNotActive) {
		t.Fatalf("second release = %v", err)
	}
	if sellable() != 2 {
		t.Fatal("release did not free the unit")
	}
	// Expiry: the lapsed hold stops counting at expires_at (before any
	// sweep) and the sweep expires it exactly once.
	h2, err := f.svc.Reserve(f.ctx, f.org, model.ReserveInput{Where: w, ProductUUID: p.uuid, Kind: model.KindHold, HoldMinutes: &minutes})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ExtendHold(f.ctx, f.org, h2.Allocation.UUID, 10); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(11 * time.Minute)
	f.svc.SetClock(func() time.Time { return later })
	if sellable() != 2 {
		t.Fatal("an expired hold still counts")
	}
	if _, err := f.svc.ExtendHold(f.ctx, f.org, h2.Allocation.UUID, 10); !errors.Is(err, usecase.ErrHoldExpired) {
		t.Fatalf("extend after expiry = %v", err)
	}
	if _, err := f.svc.Confirm(f.ctx, f.org, h2.Allocation.UUID, "", nil); !errors.Is(err, usecase.ErrHoldExpired) {
		t.Fatalf("confirm after expiry = %v", err)
	}
	n, err := f.svc.ExpireDue(f.ctx)
	if err != nil || n < 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	a, _ := f.svc.GetAllocation(f.ctx, f.org, h2.Allocation.UUID)
	if a.Status != model.StatusExpired || a.ReleaseReason != "expired" {
		t.Fatalf("after the sweep: %+v", a)
	}
	var again int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM vehicle_allocations WHERE organization_id = $1 AND status = 'active' AND kind = 'hold' AND expires_at <= $2`, f.org, later).Scan(&again); err != nil || again != 0 {
		t.Fatalf("due holds left: %d %v", again, err)
	}
	// A lapsed hold is also expired lazily by the next writer: two new
	// holds fit on two vehicles.
	h3, err := f.svc.Reserve(f.ctx, f.org, model.ReserveInput{Where: w, ProductUUID: p.uuid, Kind: model.KindHold, HoldMinutes: &minutes})
	if err != nil {
		t.Fatal(err)
	}
	evenLater := later.Add(6 * time.Minute)
	f.svc.SetClock(func() time.Time { return evenLater })
	f.mustHold(p, w)
	f.mustHold(p, w)
	a3, _ := f.svc.GetAllocation(f.ctx, f.org, h3.Allocation.UUID)
	if a3.Status != model.StatusExpired {
		t.Fatalf("lazily expired hold: %+v", a3)
	}
	f.invariants(nil)
}

// Releasing a one-way commitment re-plans the later commitment that relied
// on its return branch.
func TestReleaseReplansDependents(t *testing.T) {
	f := newFx(t)
	a, b := f.branch("AAA", 0), f.branch("BBB", 0)
	f.allowOneWay(a, b)
	f.vehicle(0, a, "MAIN") // v1 at A
	v2 := f.vehicle(0, b, "MAIN")
	p := f.product("EKO", "GROUP")
	// v2 is busy early; the one-way A→B goes on v1, then a B pick-up later
	// can use v1 (now at B) or v2.
	f.block(v2, f.at(0), f.at(40))
	oneWay := f.mustHold(p, where(a, b, f.at(0), f.at(10)))
	later := f.mustHold(p, where(b, b, f.at(20), f.at(30)))
	if f.vehicleOf(later.Allocation.UUID) != f.vehicleOf(oneWay.Allocation.UUID) {
		t.Fatal("the B pick-up should ride on the one-way return (v2 is blocked)")
	}
	// Releasing the one-way strands the B pick-up on v1 (still at A); v2 is
	// blocked until 40, so it cannot be repaired: reported as affected.
	res, err := f.svc.Release(f.ctx, f.org, oneWay.Allocation.UUID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Affected) != 1 || res.Affected[0].UUID != later.Allocation.UUID {
		t.Fatalf("affected = %+v", res.Affected)
	}
	// With v2 free the dependent is moved instead.
	f2 := newFx(t)
	a, b = f2.branch("AAA", 0), f2.branch("BBB", 0)
	f2.allowOneWay(a, b)
	v1 := f2.vehicle(0, a, "MAIN")
	v2 = f2.vehicle(0, b, "MAIN")
	p = f2.product("EKO", "GROUP")
	blk := f2.block(v2, f2.at(0), f2.at(15))
	oneWay = f2.mustHold(p, where(a, b, f2.at(0), f2.at(10)))
	later = f2.mustHold(p, where(b, b, f2.at(12), f2.at(30)))
	if f2.vehicleOf(later.Allocation.UUID) != v1.id {
		t.Fatal("setup: the B pick-up should be on v1")
	}
	if _, err := f2.pool.Exec(f2.ctx, `UPDATE vehicle_blocks SET deleted_at = now() WHERE uuid = $1`, blk); err != nil {
		t.Fatal(err)
	}
	res, err = f2.svc.Release(f2.ctx, f2.org, oneWay.Allocation.UUID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Affected) != 0 || res.Reassigned != 1 || f2.vehicleOf(later.Allocation.UUID) != v2.id {
		t.Fatalf("release result %+v; dependent on %d", res, f2.vehicleOf(later.Allocation.UUID))
	}
	f2.invariants(nil)
}

// Independent pools never share: a commitment stays in its pool.
func TestPoolsNeverShare(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	f.vehicle(0, mrk, "MAIN")
	f.vehicle(0, mrk, "PARTNER")
	p := f.product("EKO", "GROUP")
	main := where(mrk, mrk, f.at(0), f.at(10))
	main.FleetPool = "MAIN"
	if got := f.search(main, p)[p.uuid]; got.Sellable != 1 || got.Breakdown.Eligible != 1 {
		t.Fatalf("MAIN pool: %+v", got.Breakdown)
	}
	h := f.mustHold(p, main)
	if h.Allocation.FleetPool != "MAIN" {
		t.Fatalf("pool = %s", h.Allocation.FleetPool)
	}
	if _, err := f.hold(p, main); !isCapacity(err) {
		t.Fatalf("second MAIN hold = %v", err)
	}
	if got := f.search(where(mrk, mrk, f.at(0), f.at(10)), p)[p.uuid]; got.Sellable != 1 {
		t.Fatalf("any pool: %+v", got.Breakdown)
	}
}

// Delivery points of a branch are fed by the same stock: searching at the
// point or at the branch counts the same vehicles once.
func TestServicePointsShareStock(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	f.vehicle(0, mrk, "MAIN")
	p := f.product("EKO", "GROUP")
	var point ref
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO service_points (organization_id, branch_id, type, name) VALUES ($1, $2, 'airport', 'SAW') RETURNING id, uuid`,
		f.org, mrk.id).Scan(&point.id, &point.uuid); err != nil {
		t.Fatal(err)
	}
	atPoint := model.Where{PickupPointUUID: &point.uuid, StartsAt: f.at(0), EndsAt: f.at(10)}
	if got := f.search(atPoint, p)[p.uuid]; got.Sellable != 1 {
		t.Fatalf("at the point: %+v", got)
	}
	h, err := f.svc.Reserve(f.ctx, f.org, model.ReserveInput{Where: atPoint, ProductUUID: p.uuid, Kind: model.KindHold})
	if err != nil {
		t.Fatal(err)
	}
	if h.Allocation.PickupPoint == nil || h.Allocation.PickupBranch.UUID != mrk.uuid {
		t.Fatalf("hold places: %+v", h.Allocation)
	}
	if got := f.search(where(mrk, mrk, f.at(0), f.at(10)), p)[p.uuid]; got.Sellable != 0 {
		t.Fatalf("the branch still sells the unit held at its point: %+v", got)
	}
}

// Channel rules: safety margin, guaranteed quota (never above the real
// capacity) and sales limit.
func TestChannelRules(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	for i := 0; i < 4; i++ {
		f.vehicle(0, mrk, "MAIN")
	}
	p := f.product("EKO", "GROUP")
	// Activate the web channel.
	settings := `{"sales_channels":[{"code":"web","is_active":true},{"code":"branch","is_active":true}]}`
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, settings); err != nil {
		t.Fatal(err)
	}
	branchW := where(mrk, mrk, f.at(0), f.at(10))
	webW := branchW
	webW.Channel = "web"
	web := "web"
	mustRule := func(in model.CapacityRuleInput) model.CapacityRule {
		r, err := f.svc.CreateRule(f.ctx, f.org, in, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	margin := mustRule(model.CapacityRuleInput{RuleType: model.RuleSafetyMargin, Quantity: 1, ProductUUID: &p.uuid})
	quota := mustRule(model.CapacityRuleInput{RuleType: model.RuleGuaranteedQuota, Channel: &web, Quantity: 2})
	if _, err := f.svc.CreateRule(f.ctx, f.org, model.CapacityRuleInput{RuleType: model.RuleGuaranteedQuota, Channel: &web, Quantity: 5, ProductUUID: &p.uuid}, nil); err == nil {
		t.Fatal("quotas above the eligible vehicles accepted")
	}
	// physical 4, web quota 2 unused, margin 1: branch sells 4-2-1 = 1, web
	// sells 2 + (4-2-1) = 3.
	b := f.search(branchW, p)[p.uuid].Breakdown
	if b.Sellable != 1 || b.ReservedForOtherChannels != 2 || b.SafetyMargin != 1 {
		t.Fatalf("branch: %+v", b)
	}
	wb := f.search(webW, p)[p.uuid].Breakdown
	if wb.Sellable != 3 || wb.OwnQuotaRemaining == nil || *wb.OwnQuotaRemaining != 2 {
		t.Fatalf("web: %+v", wb)
	}
	f.mustHold(p, branchW)
	if _, err := f.hold(p, branchW); !isCapacity(err) {
		t.Fatalf("branch beyond quota + margin = %v", err)
	}
	// Web uses its quota, then the shared rest; the margin is never sold.
	f.mustHold(p, webW)
	f.mustHold(p, webW)
	if _, err := f.hold(p, webW); !isCapacity(err) {
		t.Fatalf("web into the safety margin = %v", err)
	}
	if got := f.search(webW, p)[p.uuid]; got.Breakdown.Physical != 1 || got.Sellable != 0 {
		t.Fatalf("margin: %+v", got.Breakdown)
	}
	// A sales limit caps a channel without reserving anything.
	if err := f.svc.DeleteRule(f.ctx, f.org, margin.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteRule(f.ctx, f.org, quota.UUID); err != nil {
		t.Fatal(err)
	}
	mustRule(model.CapacityRuleInput{RuleType: model.RuleSalesLimit, Channel: &web, Quantity: 2})
	if got := f.search(webW, p)[p.uuid].Breakdown; got.Sellable != 0 || got.ChannelLimitRemaining == nil || *got.ChannelLimitRemaining != 0 {
		t.Fatalf("web at its limit: %+v", got)
	}
	if got := f.search(branchW, p)[p.uuid]; got.Sellable != 1 {
		t.Fatalf("branch is not limited: %+v", got.Breakdown)
	}
}
