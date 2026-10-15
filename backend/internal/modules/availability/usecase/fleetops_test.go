package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	fleetopsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// The fleet calendar (TEC-696 extension point) shows holds as bars for
// members with tenant.availability.read, and a block over a hold lists it
// in affected[] (fleet-availability-inputs §2).
func TestCalendarAndBlocksSeeHolds(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 45)
	v := f.vehicle(0, mrk, "MAIN")
	p := f.product("EKO", "GROUP")
	h := f.mustHold(p, where(mrk, mrk, f.at(10), f.at(20)))

	ops := fleetopsusecase.New(fleetopsrepository.NewPostgres(f.pool, f.q), vehicleusecase.New(vehiclerepository.NewPostgres(f.pool, f.q), nil))
	scope := func(perms ...string) context.Context {
		return orgctx.WithScope(context.Background(), orgctx.Scope{ID: f.org, Permissions: perms})
	}
	window := fleetopsmodel.CalendarFilter{From: f.at(0), To: f.at(48)}
	cal, err := ops.Calendar(scope(rbac.PermTenantVehiclesRead, rbac.PermTenantAvailabilityRead), f.org, window)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Items) != 1 || len(cal.Items[0].Intervals) != 1 {
		t.Fatalf("calendar = %+v", cal.Items)
	}
	bar := cal.Items[0].Intervals[0]
	if bar.Kind != model.KindHold || bar.UUID != h.Allocation.UUID || bar.Allocation == nil ||
		bar.Allocation.Product.UUID != p.uuid || !bar.Allocation.StartsAt.Equal(f.at(10)) ||
		bar.EndsAt == nil || !bar.EndsAt.Equal(f.at(20).Add(45*time.Minute)) || bar.Allocation.ExpiresAt == nil {
		t.Fatalf("hold bar = %+v / %+v", bar, bar.Allocation)
	}
	// Without tenant.availability.read the layer is not there.
	cal, err = ops.Calendar(scope(rbac.PermTenantVehiclesRead), f.org, window)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Items[0].Intervals) != 0 {
		t.Fatalf("hold bar without availability read: %+v", cal.Items[0].Intervals)
	}

	kind := "maintenance"
	start := f.at(15)
	res, err := ops.CreateBlock(scope(rbac.PermTenantBlocksWrite), f.org, v.uuid, fleetopsmodel.BlockInput{Kind: &kind, StartsAt: &start})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Affected) != 1 || res.Affected[0].Kind != fleetopsmodel.AffectedHold || res.Affected[0].UUID != h.Allocation.UUID || res.Affected[0].Label != "EKO" {
		t.Fatalf("affected = %+v", res.Affected)
	}
	// The vehicle is blocked from now on: nothing is sellable after the
	// block started, and the hold itself can still be released.
	if got := f.search(where(mrk, mrk, f.at(30), f.at(40)), p)[p.uuid]; got.Sellable != 0 || got.Breakdown.Blocked != 1 {
		t.Fatalf("after the block: %+v", got.Breakdown)
	}
	if _, err := f.svc.Release(f.ctx, f.org, h.Allocation.UUID, "", nil); err != nil {
		t.Fatal(err)
	}
}
