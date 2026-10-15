package usecase

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// The commitment API (docs/design/availability-engine.md §5). Holds use it
// through HTTP today; reservations (TEC-706) will call Reserve(kind
// reservation) or Confirm(hold), ChangeDates, AssignPlate and Release.

// Reserve sells one unit: in one transaction under the component lock it
// expires due holds, plans (a free eligible vehicle, or an augmenting path
// that moves flexible commitments), checks the channel rules and writes the
// moves and the new allocation. CapacityError when no plan exists (nothing
// is written).
func (s *Service) Reserve(ctx context.Context, orgID int64, in model.ReserveInput) (model.PlanResult, error) {
	if in.Kind != model.KindHold && in.Kind != model.KindReservation {
		return model.PlanResult{}, validation("kind", "invalid", "kind must be hold or reservation")
	}
	r, err := s.resolve(ctx, orgID, in.Where)
	if err != nil {
		return model.PlanResult{}, err
	}
	product, err := s.q.GetAvailabilityProduct(ctx, db.GetAvailabilityProductParams{OrganizationID: orgID, Uuid: in.ProductUUID})
	if notFound(err) {
		return model.PlanResult{}, validation("product_uuid", "not_found", "rental product was not found")
	}
	if err != nil {
		return model.PlanResult{}, err
	}
	if product.Status != "published" || !product.IsActive {
		return model.PlanResult{}, &CapacityError{Reason: model.ReasonNotSellable}
	}
	var expires pgtype.Timestamptz
	if in.Kind == model.KindHold && in.HoldUntil != nil {
		if !in.HoldUntil.After(s.clock()) {
			return model.PlanResult{}, validation("hold_until", "in_past", "the hold must expire in the future")
		}
		expires = ts(in.HoldUntil.UTC())
	} else if in.Kind == model.KindHold {
		minutes, err := holdMinutes(r, in.HoldMinutes)
		if err != nil {
			return model.PlanResult{}, err
		}
		expires = ts(s.clock().Add(time.Duration(minutes) * time.Minute))
	}
	key := versionKey(product.ID, product.CurrentVersion)
	var only int64
	if in.VehicleUUID != nil {
		v, err := s.q.GetAvailabilityVehicle(ctx, db.GetAvailabilityVehicleParams{OrganizationID: orgID, Uuid: *in.VehicleUUID})
		if err != nil {
			return model.PlanResult{}, validation("vehicle_uuid", "not_found", "vehicle was not found")
		}
		only = v.ID
	}

	var created uuid.UUID
	reassigned := 0
	err = s.write(ctx, orgID, func(q *db.Queries, tx pgx.Tx) error {
		snap, err := s.snapshot(ctx, orgID)
		if err != nil {
			return err
		}
		version := snap.versions[key]
		if version == nil {
			return errStale // the product changed since it was read
		}
		if only != 0 && !snap.eligible[key][only] {
			return validation("vehicle_uuid", "not_eligible", "the vehicle does not keep the product's guarantee")
		}
		comp := snap.componentOf(key)
		if comp == 0 {
			return &CapacityError{Reason: model.ReasonNoEligibleVehicle}
		}
		allowed, err := s.oneWayAllowed(ctx, q, orgID, r, product.ID)
		if err != nil {
			return err
		}
		if !allowed {
			return &CapacityError{Reason: model.ReasonOneWayNotAllowed}
		}
		if err := s.lockFresh(ctx, q, orgID, snap, comp); err != nil {
			return err
		}
		now := s.clock()
		cs, err := s.loadComponent(ctx, q, orgID, snap, comp, now)
		if err != nil {
			return err
		}
		w := &writer{ctx: ctx, q: q, tx: tx, orgID: orgID, cs: cs}
		if _, err := w.expireDue(now, s.opts); err != nil {
			return err
		}
		req := r.request(key)
		req.Hold = in.Kind == model.KindHold
		// EXACT_VEHICLE products promise their plate.
		req.Fixed = only != 0 || version.GuaranteeType == "EXACT_VEHICLE"
		if only != 0 {
			req.Vehicles = []int64{only}
		}
		plan, ok := cs.st.Place(req, s.opts)
		if !ok {
			return &CapacityError{Reason: model.ReasonNoVehicle}
		}
		if err := s.checkChannel(ctx, q, orgID, cs, req, product.ID, r, now); err != nil {
			return err
		}
		if err := w.applyMoves(plan.Moves); err != nil {
			return err
		}
		vehicle := cs.st.Vehicles[plan.Vehicle]
		var refUUID pgtype.UUID
		if in.RefUUID != nil {
			refUUID = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
		}
		row, err := q.InsertAllocation(ctx, db.InsertAllocationParams{
			OrganizationID: orgID, VehicleID: plan.Vehicle, RentalProductID: product.ID, ProductVersion: product.CurrentVersion,
			Kind: in.Kind, StartsAt: ts(r.start), EndsAt: ts(r.end), PrepMinutes: int32(r.prep), PeriodEnd: ts(r.readyAt()),
			PickupBranchID: r.pickup.ID, ReturnBranchID: r.ret.ID, PickupPointID: pointID(r.pickupPoint), ReturnPointID: pointID(r.retPoint),
			FleetPool: vehicle.Pool, Channel: r.channel, PlateFixed: req.Fixed, ExpiresAt: expires,
			RefType: textArg(in.RefType), RefUuid: refUUID, Note: in.Note, CreatedBy: int8Arg(in.ActorID),
		})
		if err != nil {
			return err
		}
		created = row.Uuid
		reassigned = len(plan.Moves)
		if in.Hook != nil {
			return in.Hook(ctx, tx, model.HookResult{
				AllocationID: row.ID, AllocationUUID: row.Uuid, VehicleID: plan.Vehicle, ProductID: product.ID,
				ProductVersion: product.CurrentVersion, Kind: in.Kind, PlateFixed: req.Fixed, Moved: w.movedAllocations(row.ID),
			})
		}
		return nil
	})
	if ce, ok := IsCapacity(err); ok && ce.Reason != model.ReasonNotSellable {
		ce.Alternatives = s.alternatives(ctx, orgID, in.Where, in.ProductUUID)
		return model.PlanResult{}, ce
	}
	if err != nil {
		return model.PlanResult{}, err
	}
	a, err := s.GetAllocation(ctx, orgID, created)
	return model.PlanResult{Allocation: a, Reassigned: reassigned}, err
}

// checkChannel applies the channel rules to one more unit of the request.
func (s *Service) checkChannel(ctx context.Context, q *db.Queries, orgID int64, cs *componentState, req engine.Request, productID int64, r resolved, now time.Time) error {
	rules, err := q.ListActiveCapacityRules(ctx, orgID)
	if err != nil {
		return err
	}
	picked := pickRules(rules, productID, r.pickup.ID)
	if picked.empty() {
		return nil
	}
	u, err := s.productUsage(ctx, q, orgID, productID, r, now)
	if err != nil {
		return err
	}
	e := evaluate(picked, u, r.channel)
	need := e.needed()
	if need < 0 {
		return &CapacityError{Reason: model.ReasonChannelRules}
	}
	physical := 1
	if need > 1 {
		probe := req
		probe.Vehicles, probe.Fixed = nil, false
		physical = cs.st.Capacity(probe, need, s.opts).Physical
	}
	b := model.Breakdown{}
	e.apply(physical, &b)
	if b.Sellable < 1 {
		return &CapacityError{Reason: model.ReasonChannelRules}
	}
	return nil
}

// target is an allocation loaded for a commitment call.
type target struct {
	ref db.GetAllocationRefRow
}

func (s *Service) target(ctx context.Context, orgID int64, id uuid.UUID) (target, error) {
	ref, err := s.q.GetAllocationRef(ctx, db.GetAllocationRefParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return target{}, ErrAllocationNotFound
	}
	if err != nil {
		return target{}, err
	}
	if err := s.checkScope(ctx, ref.PickupBranchID, ref.ReturnBranchID); err != nil {
		return target{}, err
	}
	return target{ref: ref}, nil
}

// checkScope: an allocation is visible when its pick-up OR return branch is
// in the caller's branch scope (404 otherwise).
func (s *Service) checkScope(ctx context.Context, pickup, ret int64) error {
	if orgctx.CheckBranch(ctx, pickup) != nil && orgctx.CheckBranch(ctx, ret) != nil {
		return ErrAllocationNotFound
	}
	return nil
}

// ErrOtherComponent: a product change to a product whose vehicles share
// nothing with the allocation's (the caller reserves the new product and
// releases the old allocation instead).
var ErrOtherComponent = errors.New("the product shares no vehicles with the allocation")

// replanned is what a change function decides: the new places / range and
// (product change) the new product version.
type replanned struct {
	r         resolved
	productID int64
	version   int32
}

// replan re-plans an existing allocation inside the lock: ChangeDates,
// AssignPlate, MovePlan, Replan and ChangeProduct. The old plan stays
// untouched unless a new one is found and written (one transaction: the
// swap is atomic). hook runs in the same transaction.
func (s *Service) replan(ctx context.Context, orgID int64, id uuid.UUID, hook model.TxHook, change func(ref db.GetAllocationRefRow, cs *componentState, req *engine.Request) (replanned, error)) (model.PlanResult, error) {
	t, err := s.target(ctx, orgID, id)
	if err != nil {
		return model.PlanResult{}, err
	}
	reassigned := 0
	err = s.write(ctx, orgID, func(q *db.Queries, tx pgx.Tx) error {
		ref, err := q.GetAllocationRef(ctx, db.GetAllocationRefParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		if ref.Status != model.StatusActive {
			return ErrNotActive
		}
		snap, err := s.snapshot(ctx, orgID)
		if err != nil {
			return err
		}
		comp := snap.component[ref.VehicleID]
		if comp == 0 {
			return &CapacityError{Reason: model.ReasonNoEligibleVehicle}
		}
		if err := s.lockFresh(ctx, q, orgID, snap, comp); err != nil {
			return err
		}
		now := s.clock()
		cs, err := s.loadComponent(ctx, q, orgID, snap, comp, now)
		if err != nil {
			return err
		}
		w := &writer{ctx: ctx, q: q, tx: tx, orgID: orgID, cs: cs}
		if _, err := w.expireDue(now, s.opts); err != nil {
			return err
		}
		c := cs.st.Commitments[ref.ID]
		if c == nil {
			return ErrNotActive // expired just now, or its vehicle left the fleet
		}
		req := engine.Request{Self: ref.ID, Key: c.Key, Period: c.Period, End: c.End, Pickup: c.Pickup, Return: c.Return}
		if c.Fixed {
			req.Vehicles, req.Fixed = []int64{c.Vehicle}, true
		}
		next, err := change(ref, cs, &req)
		if err != nil {
			return err
		}
		if next.productID == 0 {
			next.productID, next.version = ref.RentalProductID, ref.ProductVersion
		}
		r := next.r
		plan, ok := cs.st.Place(req, s.opts)
		if !ok {
			return &CapacityError{Reason: model.ReasonNoVehicle}
		}
		if err := w.applyMoves(plan.Moves); err != nil {
			return err
		}
		if plan.Vehicle != ref.VehicleID {
			if err := w.deferConstraint(); err != nil {
				return err
			}
		}
		n, err := q.ReplanAllocation(ctx, db.ReplanAllocationParams{
			OrganizationID: orgID, ID: ref.ID, FromVehicleID: ref.VehicleID, ToVehicleID: plan.Vehicle,
			RentalProductID: next.productID, ProductVersion: next.version,
			StartsAt: ts(r.start), EndsAt: ts(r.end), PrepMinutes: int32(r.prep), PeriodEnd: ts(r.readyAt()),
			PickupBranchID: r.pickup.ID, ReturnBranchID: r.ret.ID, PickupPointID: pointID(r.pickupPoint), ReturnPointID: pointID(r.retPoint),
			PlateFixed: req.Fixed,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return errStale
		}
		reassigned = len(plan.Moves)
		if hook != nil {
			return hook(ctx, tx, model.HookResult{
				AllocationID: ref.ID, AllocationUUID: ref.Uuid, VehicleID: plan.Vehicle, ProductID: next.productID,
				ProductVersion: next.version, Kind: ref.Kind, PlateFixed: req.Fixed, Moved: w.movedAllocations(ref.ID),
			})
		}
		return nil
	})
	if err != nil {
		return model.PlanResult{}, err
	}
	a, err := s.GetAllocation(ctx, orgID, t.ref.Uuid)
	return model.PlanResult{Allocation: a, Reassigned: reassigned}, err
}

// ChangeDates moves an allocation to a new range (and optionally new
// places). Not release-then-search: the new plan is searched with the old
// allocation still in place (it only ignores itself) and swapped in
// atomically; without a plan the old allocation stays as it was
// (CapacityError).
//
// With in.VehicleUUID the new range is planned on that vehicle only (an
// explicit plate change, allowed over a promised plate); FixPlate promises
// it. Both are internal options of reservations (extension onto an
// alternative vehicle).
func (s *Service) ChangeDates(ctx context.Context, orgID int64, id uuid.UUID, in model.ChangeInput) (model.PlanResult, error) {
	var only int64
	if in.VehicleUUID != nil {
		v, err := s.q.GetAvailabilityVehicle(ctx, db.GetAvailabilityVehicleParams{OrganizationID: orgID, Uuid: *in.VehicleUUID})
		if err != nil {
			return model.PlanResult{}, validation("vehicle_uuid", "not_found", "vehicle was not found")
		}
		only = v.ID
	}
	return s.replan(ctx, orgID, id, in.Hook, func(ref db.GetAllocationRefRow, cs *componentState, req *engine.Request) (replanned, error) {
		r, err := s.changedWhere(ctx, orgID, ref, in.Where)
		if err != nil {
			return replanned{}, err
		}
		req.Period, req.End, req.Pickup, req.Return = r.period(), r.end, r.pickup.ID, r.ret.ID
		if only != 0 {
			if !cs.st.Eligible[req.Key][only] {
				return replanned{}, validation("vehicle_uuid", "not_eligible", "the vehicle does not keep the product's guarantee")
			}
			req.Vehicles = []int64{only}
			req.Fixed = in.FixPlate || ref.PlateFixed
		} else if in.FixPlate {
			req.Fixed = true
		}
		return replanned{r: r}, nil
	})
}

// changedWhere resolves the places / range of a change request; missing
// places keep the allocation's, the channel never changes.
func (s *Service) changedWhere(ctx context.Context, orgID int64, ref db.GetAllocationRefRow, in model.Where) (resolved, error) {
	where := in
	if where.PickupBranchUUID == nil && where.PickupPointUUID == nil {
		if err := s.fillPlaces(ctx, orgID, ref, &where); err != nil {
			return resolved{}, err
		}
	}
	where.Channel = ref.Channel
	where.FleetPool = ""
	return s.resolve(ctx, orgID, where)
}

// ChangeProduct moves an allocation to another product (an upgrade) and
// optionally a new range / places, in one atomic re-plan. The new product
// must share vehicles with the allocation's component (ErrOtherComponent
// otherwise: the caller reserves the new product, then releases this one).
// A promised plate is kept when it keeps the new guarantee, else re-planned.
func (s *Service) ChangeProduct(ctx context.Context, orgID int64, id, productUUID uuid.UUID, in model.ChangeInput) (model.PlanResult, error) {
	product, err := s.q.GetAvailabilityProduct(ctx, db.GetAvailabilityProductParams{OrganizationID: orgID, Uuid: productUUID})
	if notFound(err) {
		return model.PlanResult{}, validation("product_uuid", "not_found", "rental product was not found")
	}
	if err != nil {
		return model.PlanResult{}, err
	}
	if product.Status != "published" || !product.IsActive {
		return model.PlanResult{}, &CapacityError{Reason: model.ReasonNotSellable}
	}
	key := versionKey(product.ID, product.CurrentVersion)
	return s.replan(ctx, orgID, id, in.Hook, func(ref db.GetAllocationRefRow, cs *componentState, req *engine.Request) (replanned, error) {
		if len(cs.st.Eligible[key]) == 0 {
			return replanned{}, ErrOtherComponent
		}
		r, err := s.changedWhere(ctx, orgID, ref, in.Where)
		if err != nil {
			return replanned{}, err
		}
		allowed, err := s.oneWayAllowed(ctx, s.q, orgID, r, product.ID)
		if err != nil {
			return replanned{}, err
		}
		if !allowed {
			return replanned{}, &CapacityError{Reason: model.ReasonOneWayNotAllowed}
		}
		req.Key = key
		req.Period, req.End, req.Pickup, req.Return = r.period(), r.end, r.pickup.ID, r.ret.ID
		if req.Fixed && !cs.st.Eligible[key][ref.VehicleID] {
			req.Vehicles, req.Fixed = nil, false
		}
		return replanned{r: r, productID: product.ID, version: product.CurrentVersion}, nil
	})
}

// fillPlaces copies the allocation's current places into a change request.
func (s *Service) fillPlaces(ctx context.Context, orgID int64, ref db.GetAllocationRefRow, w *model.Where) error {
	rows, err := s.q.ListAvailabilityBranchesByIDs(ctx, db.ListAvailabilityBranchesByIDsParams{
		OrganizationID: orgID, Ids: []int64{ref.PickupBranchID, ref.ReturnBranchID},
	})
	if err != nil {
		return err
	}
	for _, b := range rows {
		id := b.Uuid
		if b.ID == ref.PickupBranchID {
			w.PickupBranchUUID = &id
		}
		if b.ID == ref.ReturnBranchID {
			w.ReturnBranchUUID = &id
		}
	}
	return nil
}

// AssignPlate promises a plate to an allocation (plate_fixed). The
// allocation already consumes capacity; assigning only re-plans it onto the
// vehicle (moving flexible commitments off it when needed), never adds a
// second unit.
func (s *Service) AssignPlate(ctx context.Context, orgID int64, id, vehicleUUID uuid.UUID) (model.PlanResult, error) {
	return s.movePlan(ctx, orgID, id, vehicleUUID, true, false, nil)
}

// MovePlan re-plans an allocation onto one vehicle (staff plate change,
// calendar drag). With fix the plate is promised; without it the vehicle is
// the new provisional plate (the engine may move it again later). Unlike
// AssignPlate it may replace a promised plate: it is an explicit staff
// decision. Never consumes a second unit.
func (s *Service) MovePlan(ctx context.Context, orgID int64, id uuid.UUID, opts model.PlanOptions) (model.PlanResult, error) {
	if opts.VehicleUUID == nil {
		return model.PlanResult{}, validation("vehicle_uuid", "required", "vehicle_uuid is required")
	}
	return s.movePlan(ctx, orgID, id, *opts.VehicleUUID, opts.FixPlate, true, opts.Hook)
}

func (s *Service) movePlan(ctx context.Context, orgID int64, id, vehicleUUID uuid.UUID, fix, overrideFixed bool, hook model.TxHook) (model.PlanResult, error) {
	v, err := s.q.GetAvailabilityVehicle(ctx, db.GetAvailabilityVehicleParams{OrganizationID: orgID, Uuid: vehicleUUID})
	if err != nil {
		return model.PlanResult{}, validation("vehicle_uuid", "not_found", "vehicle was not found")
	}
	return s.replan(ctx, orgID, id, hook, func(ref db.GetAllocationRefRow, cs *componentState, req *engine.Request) (replanned, error) {
		if ref.PlateFixed && ref.VehicleID != v.ID && !overrideFixed {
			return replanned{}, validation("vehicle_uuid", "plate_fixed", "the allocation already has another promised plate")
		}
		if !cs.st.Eligible[req.Key][v.ID] {
			return replanned{}, validation("vehicle_uuid", "not_eligible", "the vehicle does not keep the product's guarantee")
		}
		req.Vehicles, req.Fixed = []int64{v.ID}, fix
		r, err := s.currentResolved(ctx, orgID, ref)
		return replanned{r: r}, err
	})
}

// Replan re-plans an allocation as it is (same range and places): after a
// block or another operational change broke its plan, it moves to another
// eligible vehicle (moving flexible commitments when needed). A promised
// plate only stays on its vehicle. CapacityError when nothing fits (the
// allocation stays where it was).
func (s *Service) Replan(ctx context.Context, orgID int64, id uuid.UUID, hook model.TxHook) (model.PlanResult, error) {
	return s.replan(ctx, orgID, id, hook, func(ref db.GetAllocationRefRow, _ *componentState, _ *engine.Request) (replanned, error) {
		r, err := s.currentResolved(ctx, orgID, ref)
		return replanned{r: r}, err
	})
}

// CandidateVehicles lists the vehicles (other than the current one) that
// could carry the allocation over a changed range / places, each checked
// with the planner (read-only; nothing is written). Up to limit vehicles,
// fewest moves first by id.
func (s *Service) CandidateVehicles(ctx context.Context, orgID int64, id uuid.UUID, in model.Where, limit int) ([]model.VehicleRef, error) {
	t, err := s.target(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if t.ref.Status != model.StatusActive {
		return nil, ErrNotActive
	}
	r, err := s.changedWhere(ctx, orgID, t.ref, in)
	if err != nil {
		return nil, err
	}
	snap, err := s.snapshot(ctx, orgID)
	if err != nil {
		return nil, err
	}
	comp := snap.component[t.ref.VehicleID]
	out := []model.VehicleRef{}
	if comp == 0 {
		return out, nil
	}
	err = s.read(ctx, func(q *db.Queries) error {
		cs, err := s.loadComponent(ctx, q, orgID, snap, comp, s.clock())
		if err != nil {
			return err
		}
		cs.dropDue()
		c := cs.st.Commitments[t.ref.ID]
		if c == nil {
			return ErrNotActive
		}
		ids := []int64{}
		for vid := range cs.st.Eligible[c.Key] {
			if vid != c.Vehicle {
				ids = append(ids, vid)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, vid := range ids {
			if len(out) >= limit {
				break
			}
			req := engine.Request{
				Self: c.ID, Key: c.Key, Period: r.period(), End: r.end, Pickup: r.pickup.ID, Return: r.ret.ID,
				Vehicles: []int64{vid}, Fixed: true,
			}
			if _, ok := cs.st.Place(req, s.opts); ok {
				info := snap.vehicles[vid]
				out = append(out, model.VehicleRef{UUID: info.UUID, Plate: info.Plate})
			}
		}
		return nil
	})
	return out, err
}

// SetHoldExpiry moves the expiry of an active, unexpired hold to until
// (reservation review window → payment window). ErrHoldExpired / ErrNotHold
// / ErrNotActive otherwise.
func (s *Service) SetHoldExpiry(ctx context.Context, orgID int64, id uuid.UUID, until time.Time) error {
	t, err := s.target(ctx, orgID, id)
	if err != nil {
		return err
	}
	n, err := s.q.SetHoldExpiry(ctx, db.SetHoldExpiryParams{
		OrganizationID: orgID, ID: t.ref.ID, ExpiresAt: ts(until.UTC()), NowAt: ts(s.clock()),
	})
	if err != nil {
		return err
	}
	if n != 1 {
		if until.After(s.clock()) {
			return s.whyNot(t.ref)
		}
		return validation("expires_at", "in_past", "the hold must expire in the future")
	}
	return nil
}

// currentResolved rebuilds the resolved request of an allocation as it is.
func (s *Service) currentResolved(ctx context.Context, orgID int64, ref db.GetAllocationRefRow) (resolved, error) {
	rows, err := s.q.ListAvailabilityBranchesByIDs(ctx, db.ListAvailabilityBranchesByIDsParams{
		OrganizationID: orgID, Ids: []int64{ref.PickupBranchID, ref.ReturnBranchID},
	})
	if err != nil {
		return resolved{}, err
	}
	r := resolved{start: ref.StartsAt.Time, end: ref.EndsAt.Time, channel: ref.Channel}
	for _, b := range rows {
		row := db.GetAvailabilityBranchRow(b)
		if b.ID == ref.PickupBranchID {
			r.pickup = row
		}
		if b.ID == ref.ReturnBranchID {
			r.ret = row
		}
	}
	if ref.PickupPointID.Valid {
		r.pickupPoint = &db.GetAvailabilityServicePointRow{ID: ref.PickupPointID.Int64}
	}
	if ref.ReturnPointID.Valid {
		r.retPoint = &db.GetAvailabilityServicePointRow{ID: ref.ReturnPointID.Int64}
	}
	r.prep = int(r.ret.PrepMinutes)
	return r, nil
}

// Confirm turns an active, unexpired hold into a confirmed reservation (the
// same row: capacity is not consumed twice).
func (s *Service) Confirm(ctx context.Context, orgID int64, id uuid.UUID, refType string, refUUID *uuid.UUID) (model.Allocation, error) {
	return s.ConfirmWith(ctx, orgID, id, refType, refUUID, nil)
}

// ConfirmWith is Confirm with a hook in the same transaction.
func (s *Service) ConfirmWith(ctx context.Context, orgID int64, id uuid.UUID, refType string, refUUID *uuid.UUID, hook model.TxHook) (model.Allocation, error) {
	t, err := s.target(ctx, orgID, id)
	if err != nil {
		return model.Allocation{}, err
	}
	var ref pgtype.UUID
	if refUUID != nil {
		ref = pgtype.UUID{Bytes: *refUUID, Valid: true}
	}
	err = s.tx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		n, err := q.ConfirmAllocation(ctx, db.ConfirmAllocationParams{
			OrganizationID: orgID, ID: t.ref.ID, RefType: textArg(refType), RefUuid: ref, NowAt: ts(s.clock()),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return s.whyNot(t.ref)
		}
		if hook != nil {
			return hook(ctx, tx, model.HookResult{
				AllocationID: t.ref.ID, AllocationUUID: t.ref.Uuid, VehicleID: t.ref.VehicleID, ProductID: t.ref.RentalProductID,
				ProductVersion: t.ref.ProductVersion, Kind: model.KindReservation, PlateFixed: t.ref.PlateFixed,
				Moved: []model.MovedAllocation{},
			})
		}
		return nil
	})
	if err != nil {
		return model.Allocation{}, err
	}
	return s.GetAllocation(ctx, orgID, id)
}

// whyNot explains a hold mutation that matched no row.
func (s *Service) whyNot(ref db.GetAllocationRefRow) error {
	switch {
	case ref.Status != model.StatusActive:
		return ErrNotActive
	case ref.Kind != model.KindHold:
		return ErrNotHold
	default:
		return ErrHoldExpired
	}
}

// ExtendHold moves the expiry of an active, unexpired hold to now + minutes
// (capacity is already held; nothing is re-planned).
func (s *Service) ExtendHold(ctx context.Context, orgID int64, id uuid.UUID, minutes int) (model.Allocation, error) {
	t, err := s.target(ctx, orgID, id)
	if err != nil {
		return model.Allocation{}, err
	}
	if minutes < 1 || minutes > model.MaxHoldMinutes {
		return model.Allocation{}, validation("minutes", "out_of_range", "minutes must be between 1 and 1440")
	}
	now := s.clock()
	n, err := s.q.ExtendHold(ctx, db.ExtendHoldParams{
		OrganizationID: orgID, ID: t.ref.ID, ExpiresAt: ts(now.Add(time.Duration(minutes) * time.Minute)), NowAt: ts(now),
	})
	if err != nil {
		return model.Allocation{}, err
	}
	if n != 1 {
		return model.Allocation{}, s.whyNot(t.ref)
	}
	return s.GetAllocation(ctx, orgID, id)
}

// Release cancels an allocation (exactly once: ErrNotActive afterwards).
// Commitments that relied on its return branch are re-planned in the same
// transaction; the ones that cannot be are reported as affected.
func (s *Service) Release(ctx context.Context, orgID int64, id uuid.UUID, reason string, actorID *int64) (model.ReleaseResult, error) {
	return s.ReleaseWith(ctx, orgID, id, reason, actorID, nil)
}

// ReleaseWith is Release with a hook in the same transaction (the hook sees
// the moves and the affected items).
func (s *Service) ReleaseWith(ctx context.Context, orgID int64, id uuid.UUID, reason string, actorID *int64, hook model.TxHook) (model.ReleaseResult, error) {
	t, err := s.target(ctx, orgID, id)
	if err != nil {
		return model.ReleaseResult{}, err
	}
	if reason == "" {
		reason = "released"
	}
	out := model.ReleaseResult{Affected: []model.Affected{}}
	err = s.write(ctx, orgID, func(q *db.Queries, tx pgx.Tx) error {
		out = model.ReleaseResult{Affected: []model.Affected{}}
		ref, err := q.GetAllocationRef(ctx, db.GetAllocationRefParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		if ref.Status != model.StatusActive {
			return ErrNotActive
		}
		now := s.clock()
		snap, err := s.snapshot(ctx, orgID)
		if err != nil {
			return err
		}
		var cs *componentState
		var res engine.ReleaseResult
		lapsed := false
		w := &writer{ctx: ctx, q: q, tx: tx, orgID: orgID, cs: &componentState{allocs: map[int64]db.ListAvailabilityAllocationsRow{}}}
		if comp := snap.component[ref.VehicleID]; comp != 0 {
			if err := s.lockFresh(ctx, q, orgID, snap, comp); err != nil {
				return err
			}
			if cs, err = s.loadComponent(ctx, q, orgID, snap, comp, now); err != nil {
				return err
			}
			w.cs = cs
			for _, id := range cs.due {
				if id == ref.ID {
					// The allocation is a lapsed hold: expiring it below is
					// its one transition out of "active".
					lapsed = true
				}
			}
			affected, err := w.expireDue(now, s.opts)
			if err != nil {
				return err
			}
			if cs.st.Commitments[ref.ID] != nil {
				res = cs.st.Release(ref.ID, s.opts)
				if err := w.applyMoves(res.Moves); err != nil {
					return err
				}
			}
			res.Affected = append(res.Affected, affected...)
		}
		n, err := q.ReleaseAllocation(ctx, db.ReleaseAllocationParams{
			OrganizationID: orgID, ID: ref.ID, Status: model.StatusReleased, ReleasedAt: ts(now),
			ReleasedBy: int8Arg(actorID), ReleaseReason: reason,
		})
		if err != nil {
			return err
		}
		// A hook caller (a reservation leaving its waiting state) accepts a
		// lapsed hold expired just above as the release: the allocation
		// left "active" exactly once, in this transaction.
		if n != 1 && (!lapsed || hook == nil) {
			return ErrNotActive
		}
		out.Reassigned = len(res.Moves)
		out.Affected = affectedItems(cs, res.Affected)
		if hook != nil {
			return hook(ctx, tx, model.HookResult{
				AllocationID: ref.ID, AllocationUUID: ref.Uuid, VehicleID: ref.VehicleID, ProductID: ref.RentalProductID,
				ProductVersion: ref.ProductVersion, Kind: ref.Kind, PlateFixed: ref.PlateFixed,
				Moved: w.movedAllocations(ref.ID), Affected: out.Affected,
			})
		}
		return nil
	})
	if err != nil {
		return model.ReleaseResult{}, err
	}
	a, err := s.GetAllocation(ctx, orgID, t.ref.Uuid)
	out.Allocation = a
	return out, err
}

func affectedItems(cs *componentState, ids []string) []model.Affected {
	out := []model.Affected{}
	if cs == nil {
		return out
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] || len(id) < 3 {
			continue
		}
		seen[id] = true
		switch id[0] {
		case 'c':
			n, _ := strconv.ParseInt(id[2:], 10, 64)
			if a, ok := cs.allocs[n]; ok {
				out = append(out, model.Affected{Kind: a.Kind, UUID: a.Uuid})
			}
		case 's':
			if seg, ok := cs.segments[id[2:]]; ok {
				out = append(out, model.Affected{Kind: seg.Kind, UUID: seg.RefUuid})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UUID.String() < out[j].UUID.String() })
	return out
}

// ExpireDue is the hold expiry sweep (worker, every minute): every
// organization's due holds are expired exactly once, component by
// component under the lock, and what depended on them is re-planned.
// Expired holds stop counting at expires_at even before the sweep (the
// engine ignores them and expires them first when it writes).
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	now := s.clock()
	orgs, err := s.q.ListDueHoldOrganizations(ctx, ts(now))
	if err != nil {
		return 0, err
	}
	total := 0
	for _, orgID := range orgs {
		n, err := s.expireOrganization(ctx, orgID, now)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (s *Service) expireOrganization(ctx context.Context, orgID int64, now time.Time) (int, error) {
	due, err := s.q.ListDueHolds(ctx, db.ListDueHoldsParams{OrganizationID: orgID, NowAt: ts(now)})
	if err != nil {
		return 0, err
	}
	snap, err := s.snapshot(ctx, orgID)
	if err != nil {
		return 0, err
	}
	comps := map[int64]bool{}
	orphans := []int64{}
	for _, d := range due {
		if c := snap.component[d.VehicleID]; c != 0 {
			comps[c] = true
		} else {
			orphans = append(orphans, d.ID)
		}
	}
	total := 0
	for comp := range comps {
		n := 0
		err := s.write(ctx, orgID, func(q *db.Queries, tx pgx.Tx) error {
			if err := s.lock(ctx, q, orgID, comp); err != nil {
				return err
			}
			cs, err := s.loadComponent(ctx, q, orgID, snap, comp, now)
			if err != nil {
				return err
			}
			n = len(cs.due)
			w := &writer{ctx: ctx, q: q, tx: tx, orgID: orgID, cs: cs}
			_, err = w.expireDue(now, s.opts)
			return err
		})
		if err != nil {
			return total, err
		}
		total += n
	}
	// Holds on vehicles that left the fleet: nothing to re-plan.
	for _, id := range orphans {
		n, err := s.q.ReleaseAllocation(ctx, db.ReleaseAllocationParams{
			OrganizationID: orgID, ID: id, Status: model.StatusExpired, ReleasedAt: ts(now), ReleaseReason: "expired",
		})
		if err != nil {
			return total, err
		}
		total += int(n)
	}
	return total, nil
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func int8Arg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func itoa(i int) string { return strconv.Itoa(i) }
