package usecase_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
)

func (f *fx) groupUUID(id int64) uuid.UUID {
	f.t.Helper()
	var out uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT uuid FROM rental_groups WHERE organization_id = $1 AND id = $2`, f.org, id).Scan(&out); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fx) isMember(v ref, group int64) bool {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM vehicle_rental_groups WHERE organization_id = $1 AND vehicle_id = $2 AND rental_group_id = $3`,
		f.org, v.id, group).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n == 1
}

// A GROUP product sells only the group's members. A group with no criteria
// and no members has no capacity; every membership change (written
// anywhere, here straight into the table) invalidates the cached
// eligibility graph through the fingerprint.
func TestGroupCapacityFollowsMembership(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	v1 := f.vehicle(0, mrk, "MAIN")
	v2 := f.vehicle(0, mrk, "MAIN")
	f.vehicle(0, mrk, "MAIN") // never a member
	group := f.group("OPEN", false)
	p := f.groupProduct("OPEN", group)
	w := where(mrk, mrk, f.at(0), f.at(24))

	got := f.search(w, p)[p.uuid]
	if got.Sellable != 0 || got.Breakdown.Eligible != 0 {
		t.Fatalf("no members: %+v", got)
	}
	if _, err := f.hold(p, w); !isCapacity(err) {
		t.Fatalf("hold without members: %v", err)
	}
	f.member(v1, group)
	if got := f.search(w, p)[p.uuid]; got.Sellable != 1 || got.Breakdown.Eligible != 1 {
		t.Fatalf("one member: %+v", got.Breakdown)
	}
	f.member(v2, group)
	if got := f.search(w, p)[p.uuid]; got.Sellable != 2 {
		t.Fatalf("two members: %+v", got.Breakdown)
	}
	// Remove one and add another in the same statement: the row count stays
	// the same, the fingerprint must still change.
	v4 := f.vehicle(0, mrk, "MAIN")
	if _, err := f.pool.Exec(f.ctx, `WITH gone AS (DELETE FROM vehicle_rental_groups WHERE organization_id = $1 AND vehicle_id = $2)
		INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id) VALUES ($1, $3, $4)`, f.org, v2.id, v4.id, group); err != nil {
		t.Fatal(err)
	}
	res := f.mustHold(p, w)
	if v := f.vehicleOf(res.Allocation.UUID); v != v1.id && v != v4.id {
		t.Fatalf("hold on %d, want a member (%d or %d)", v, v1.id, v4.id)
	}
	if got := f.search(w, p)[p.uuid]; got.Sellable != 1 || got.Breakdown.Eligible != 2 {
		t.Fatalf("after swap and hold: %+v", got.Breakdown)
	}
}

// A member that no longer meets the group's criteria (an attribute edit
// after it was added) does not count.
func TestStaleMembershipDoesNotCount(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	egea, egeaType := f.model("Egea")
	_, clioType := f.model("Clio")
	v1 := f.vehicle(egeaType, mrk, "MAIN")
	group := f.group("EGEA", false, egea)
	p := f.groupProduct("EGEA", group)
	f.member(v1, group)
	w := where(mrk, mrk, f.at(0), f.at(24))
	if got := f.search(w, p)[p.uuid]; got.Sellable != 1 {
		t.Fatalf("member: %+v", got.Breakdown)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE vehicles SET catalog_type_id = $3, updated_at = now() WHERE organization_id = $1 AND id = $2`,
		f.org, v1.id, clioType); err != nil {
		t.Fatal(err)
	}
	if got := f.search(w, p)[p.uuid]; got.Sellable != 0 || got.Breakdown.Eligible != 0 {
		t.Fatalf("stale member counted: %+v", got.Breakdown)
	}
}

// Removing a member whose future commitment relied on the membership
// re-plans the commitment onto another member in the same transaction.
func TestRemoveMemberReplansCommitments(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	v1 := f.vehicle(0, mrk, "MAIN")
	v2 := f.vehicle(0, mrk, "MAIN")
	group := f.group("OPEN", false)
	p := f.groupProduct("OPEN", group)
	f.member(v1, group)
	f.member(v2, group)
	res := f.mustHold(p, where(mrk, mrk, f.at(0), f.at(24)))
	on := f.vehicleOf(res.Allocation.UUID)
	leaving, staying := v1, v2
	if on == v2.id {
		leaving, staying = v2, v1
	}
	if err := f.vehicles.RemoveGroup(f.ctx, f.org, leaving.uuid, f.groupUUID(group)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if f.isMember(leaving, group) {
		t.Fatal("membership kept")
	}
	if got := f.vehicleOf(res.Allocation.UUID); got != staying.id {
		t.Fatalf("hold on %d, want %d", got, staying.id)
	}
	eligible := map[int64]map[int64]bool{p.id: {staying.id: true}}
	f.invariants(eligible)
	// The engine sees the removal at once (the leaving vehicle is gone).
	if got := f.search(where(mrk, mrk, f.at(30), f.at(40)), p)[p.uuid]; got.Breakdown.Eligible != 1 {
		t.Fatalf("after removal: %+v", got.Breakdown)
	}
}

// Removing a member is refused (nothing changes) when a commitment that
// relies on it cannot move: no other member, or a promised plate. A bulk
// replace that drops the group is refused the same way.
func TestRemoveMemberRefusedWhenCommitmentCannotMove(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 0)
	v1 := f.vehicle(0, mrk, "MAIN")
	v2 := f.vehicle(0, mrk, "MAIN")
	group := f.group("OPEN", false)
	other := f.group("OTHER", false)
	p := f.groupProduct("OPEN", group)
	f.member(v1, group)
	res := f.mustHold(p, where(mrk, mrk, f.at(0), f.at(24)))
	if f.vehicleOf(res.Allocation.UUID) != v1.id {
		t.Fatal("hold must sit on the only member")
	}
	gid := f.groupUUID(group)
	err := f.vehicles.RemoveGroup(f.ctx, f.org, v1.uuid, gid)
	var inUse *vehicleusecase.MembershipInUseError
	if !errors.As(err, &inUse) || len(inUse.Allocations) != 1 {
		t.Fatalf("remove = %v", err)
	}
	a := inUse.Allocations[0]
	if a.UUID != res.Allocation.UUID || a.Kind != model.KindHold || a.Reason != vehicleusecase.BlockNoVehicle || a.Reservation != nil {
		t.Fatalf("blocking = %+v", a)
	}
	if !f.isMember(v1, group) || f.vehicleOf(res.Allocation.UUID) != v1.id {
		t.Fatal("a refused removal changed something")
	}
	if _, err := f.vehicles.SetGroups(f.ctx, f.org, v1.uuid, []uuid.UUID{f.groupUUID(other)}, true, false); !errors.As(err, &inUse) {
		t.Fatalf("bulk replace = %v", err)
	}
	if !f.isMember(v1, group) || f.isMember(v1, other) {
		t.Fatal("a refused replace changed memberships")
	}
	// A second member exists, but the plate is promised: still refused.
	f.member(v2, group)
	if _, err := f.svc.AssignPlate(f.ctx, f.org, res.Allocation.UUID, v1.uuid); err != nil {
		t.Fatal(err)
	}
	err = f.vehicles.RemoveGroup(f.ctx, f.org, v1.uuid, gid)
	if !errors.As(err, &inUse) || inUse.Allocations[0].Reason != vehicleusecase.BlockPlateFixed {
		t.Fatalf("remove under a promised plate = %v", err)
	}
	// Without the commitment the removal goes through.
	if _, err := f.svc.Release(f.ctx, f.org, res.Allocation.UUID, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.vehicles.RemoveGroup(f.ctx, f.org, v1.uuid, gid); err != nil {
		t.Fatalf("remove after release: %v", err)
	}
	// Adding a member never needs the engine: v1 joins again.
	if _, err := f.vehicles.AddGroup(f.ctx, f.org, v1.uuid, gid); err != nil {
		t.Fatal(err)
	}
}
