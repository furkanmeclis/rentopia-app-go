package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Rentals (TEC-709, docs/design/rentals.md): the allocation of a confirmed
// reservation becomes kind "rental" at the handover (same row: capacity is
// never consumed twice). A rental under way is a started commitment: the
// planner never moves it, and only these calls change it — the actual
// return or an overdue extension (SetRentalEnd) and a mid-rental vehicle
// swap (SwapRentalVehicle). Reservation changes keep working on it as long
// as the start, the pick-up and the vehicle stay (an extension).

var (
	// ErrRentalUnderway: the change would rewrite a rental under way (its
	// start, pick-up, vehicle or product, or a release); rentals use their
	// own calls.
	ErrRentalUnderway = errors.New("the allocation belongs to a rental under way")
	// ErrNotReservation: only a confirmed reservation's allocation can be
	// handed over.
	ErrNotReservation = errors.New("the allocation is not a confirmed reservation")
	// ErrNotRental: the allocation is not a rental under way.
	ErrNotRental = errors.New("the allocation is not a rental under way")
)

// RentalConflictError: a longer rental runs into commitments of its vehicle
// that the planner could not move (nothing was written). The caller re-plans
// them one by one or opens resolution tasks (late return).
type RentalConflictError struct {
	Allocations []model.Affected
}

func (e *RentalConflictError) Error() string {
	return "the rental collides with later commitments of its vehicle"
}

// IsRentalConflict unwraps a RentalConflictError.
func IsRentalConflict(err error) (*RentalConflictError, bool) {
	var ce *RentalConflictError
	ok := errors.As(err, &ce)
	return ce, ok
}

// rentalState is what a rental call works on inside the lock.
type rentalState struct {
	ref db.GetRentalAllocationRefRow
	cs  *componentState
	w   *writer
	now time.Time
	// c is the commitment of the allocation (nil when its period already
	// ended: an overdue rental not extended yet is not in the state).
	c *engine.Commitment
}

// rentalWrite loads the allocation and its component under the lock and
// runs fn in the engine transaction (retried on stale plans).
func (s *Service) rentalWrite(ctx context.Context, orgID int64, id uuid.UUID, fn func(q *db.Queries, tx pgx.Tx, rs *rentalState) error) error {
	if _, err := s.target(ctx, orgID, id); err != nil {
		return err
	}
	return s.write(ctx, orgID, func(q *db.Queries, tx pgx.Tx) error {
		ref, err := q.GetRentalAllocationRef(ctx, db.GetRentalAllocationRefParams{OrganizationID: orgID, Uuid: id})
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
		if err := s.lock(ctx, q, orgID, comp); err != nil {
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
		return fn(q, tx, &rentalState{ref: ref, cs: cs, w: w, now: now, c: cs.st.Commitments[ref.ID]})
	})
}

func (s *Service) prepOf(ctx context.Context, q *db.Queries, orgID, branchID int64) (int, error) {
	rows, err := q.ListAvailabilityBranchesByIDs(ctx, db.ListAvailabilityBranchesByIDsParams{OrganizationID: orgID, Ids: []int64{branchID}})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, validation("return_branch_uuid", "not_found", "branch was not found")
	}
	return int(rows[0].PrepMinutes), nil
}

func versionKeyOf(ref db.GetRentalAllocationRefRow) string {
	return versionKey(ref.RentalProductID, ref.ProductVersion)
}

func hookResult(ref db.GetRentalAllocationRefRow, w *writer, vehicleID int64) model.HookResult {
	return model.HookResult{
		AllocationID: ref.ID, AllocationUUID: ref.Uuid, VehicleID: vehicleID, ProductID: ref.RentalProductID,
		ProductVersion: ref.ProductVersion, Kind: model.KindRental, PlateFixed: true, Moved: w.movedAllocations(ref.ID),
		Affected: []model.Affected{},
	}
}

// StartRental hands the allocation of a confirmed reservation over: the
// same row becomes kind rental with a promised plate. A handover earlier
// than the planned pick-up moves the start to it when the planner can keep
// every other commitment (otherwise the planned start stays; the vehicle
// is out either way). hook runs in the same transaction.
func (s *Service) StartRental(ctx context.Context, orgID int64, id uuid.UUID, at time.Time, hook model.TxHook) (model.Allocation, error) {
	at = at.UTC().Truncate(time.Microsecond)
	err := s.rentalWrite(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, rs *rentalState) error {
		ref := rs.ref
		switch ref.Kind {
		case model.KindRental:
			return ErrRentalUnderway
		case model.KindReservation:
		default:
			return ErrNotReservation
		}
		if at.Before(ref.StartsAt.Time) && rs.c != nil {
			req := engine.Request{
				Self: ref.ID, Key: versionKeyOf(ref), Period: engine.Interval{Start: at, End: rs.c.Period.End}, End: rs.c.End,
				Pickup: ref.PickupBranchID, Return: ref.ReturnBranchID, Vehicles: []int64{ref.VehicleID}, Fixed: true,
			}
			if plan, ok := rs.cs.st.Place(req, s.opts); ok && plan.Vehicle == ref.VehicleID {
				if err := rs.w.applyMoves(plan.Moves); err != nil {
					return err
				}
				n, err := q.ReplanAllocation(ctx, db.ReplanAllocationParams{
					OrganizationID: orgID, ID: ref.ID, FromVehicleID: ref.VehicleID, ToVehicleID: ref.VehicleID,
					RentalProductID: ref.RentalProductID, ProductVersion: ref.ProductVersion,
					StartsAt: ts(at), EndsAt: ref.EndsAt, PrepMinutes: int32(rs.c.Period.End.Sub(rs.c.End) / time.Minute),
					PeriodEnd: ts(rs.c.Period.End), PickupBranchID: ref.PickupBranchID, ReturnBranchID: ref.ReturnBranchID,
					PickupPointID: ref.PickupPointID, ReturnPointID: ref.ReturnPointID, PlateFixed: true,
				})
				if err != nil {
					return err
				}
				if n != 1 {
					return errStale
				}
			}
		}
		n, err := q.MarkAllocationRental(ctx, db.MarkAllocationRentalParams{OrganizationID: orgID, ID: ref.ID})
		if err != nil {
			return err
		}
		if n != 1 {
			return errStale
		}
		if hook != nil {
			return hook(ctx, tx, hookResult(ref, rs.w, ref.VehicleID))
		}
		return nil
	})
	if err != nil {
		return model.Allocation{}, err
	}
	return s.GetAllocation(ctx, orgID, id)
}

// SetRentalEnd moves the end (and the return branch / point) of a rental
// under way: the actual return, an early return or the extension of an
// overdue vehicle. The planner keeps the vehicle (promised plate) and moves
// flexible later commitments when the range grows or the return branch
// changes. Without a plan: RentalConflictError (nothing written), or with
// Force the facts are written anyway, clipped before the next commitment,
// and what broke is reported as affected.
func (s *Service) SetRentalEnd(ctx context.Context, orgID int64, id uuid.UUID, in model.RentalEnd) (model.RentalEndResult, error) {
	end := in.EndsAt.UTC().Truncate(time.Microsecond)
	out := model.RentalEndResult{Affected: []model.Affected{}}
	err := s.rentalWrite(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, rs *rentalState) error {
		out = model.RentalEndResult{Affected: []model.Affected{}}
		ref := rs.ref
		if ref.Kind != model.KindRental {
			return ErrNotRental
		}
		if !end.After(ref.StartsAt.Time) {
			return validation("ends_at", "before_start", "the return must be after the pick-up")
		}
		ret := ref.ReturnBranchID
		if in.ReturnBranchID != 0 {
			ret = in.ReturnBranchID
		}
		point := ref.ReturnPointID
		if in.SetPoint {
			point = pgtype.Int8{Int64: in.ReturnPointID, Valid: in.ReturnPointID != 0}
		}
		prep, err := s.prepOf(ctx, q, orgID, ret)
		if err != nil {
			return err
		}
		periodEnd := end.Add(time.Duration(prep) * time.Minute)
		req := engine.Request{
			Key: versionKeyOf(ref), Period: engine.Interval{Start: ref.StartsAt.Time, End: periodEnd}, End: end,
			Pickup: ref.PickupBranchID, Return: ret, Vehicles: []int64{ref.VehicleID}, Fixed: true,
		}
		if rs.c != nil {
			req.Self = ref.ID
		}
		plan, ok := rs.cs.st.Place(req, s.opts)
		if ok && plan.Vehicle == ref.VehicleID {
			if err := rs.w.applyMoves(plan.Moves); err != nil {
				return err
			}
		} else {
			later, err := q.ListVehicleCommitmentsAfter(ctx, db.ListVehicleCommitmentsAfterParams{
				OrganizationID: orgID, VehicleID: ref.VehicleID, ExcludeID: ref.ID, FromAt: ref.StartsAt,
			})
			if err != nil {
				return err
			}
			broken := []model.Affected{}
			nextStart := time.Time{}
			for i, l := range later {
				if !l.PeriodStart.Time.After(ref.StartsAt.Time) {
					continue // an earlier commitment (never the case for a started rental)
				}
				if nextStart.IsZero() {
					nextStart = l.PeriodStart.Time
				}
				overlaps := l.PeriodStart.Time.Before(periodEnd)
				stranded := i == firstAfter(later, ref.StartsAt.Time) && l.PickupBranchID != ret
				if overlaps || stranded {
					broken = append(broken, model.Affected{Kind: l.Kind, UUID: l.Uuid})
				}
			}
			// Blocks and open transfers win too (the guard trigger refuses
			// an allocation over them).
			if veh := rs.cs.st.Vehicles[ref.VehicleID]; veh != nil {
				for _, seg := range veh.Segments {
					if seg.Period.Start.After(ref.StartsAt.Time) && (nextStart.IsZero() || seg.Period.Start.Before(nextStart)) {
						nextStart = seg.Period.Start
					}
				}
			}
			if !in.Force {
				if len(broken) == 0 {
					return &CapacityError{Reason: model.ReasonNoVehicle}
				}
				return &RentalConflictError{Allocations: broken}
			}
			// Forced facts: the period never overlaps the next commitment
			// (EXCLUDE); the preparation buffer, then the end itself, are
			// clipped before it.
			if !nextStart.IsZero() && periodEnd.After(nextStart) {
				out.Clipped = true
				periodEnd = nextStart
				if end.After(periodEnd) {
					end = periodEnd
				}
				prep = int(periodEnd.Sub(end) / time.Minute)
				periodEnd = end.Add(time.Duration(prep) * time.Minute)
			}
			out.Affected = broken
		}
		n, err := q.ReplanAllocation(ctx, db.ReplanAllocationParams{
			OrganizationID: orgID, ID: ref.ID, FromVehicleID: ref.VehicleID, ToVehicleID: ref.VehicleID,
			RentalProductID: ref.RentalProductID, ProductVersion: ref.ProductVersion,
			StartsAt: ref.StartsAt, EndsAt: ts(end), PrepMinutes: int32(prep), PeriodEnd: ts(periodEnd),
			PickupBranchID: ref.PickupBranchID, ReturnBranchID: ret, PickupPointID: ref.PickupPointID, ReturnPointID: point,
			PlateFixed: true,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return errStale
		}
		if in.Hook != nil {
			r := hookResult(ref, rs.w, ref.VehicleID)
			r.Affected = out.Affected
			return in.Hook(ctx, tx, r)
		}
		return nil
	})
	if err != nil {
		return model.RentalEndResult{}, err
	}
	a, err := s.GetAllocation(ctx, orgID, id)
	out.Allocation = a
	return out, err
}

// firstAfter is the index of the first commitment starting after t (-1 =
// none).
func firstAfter(rows []db.ListVehicleCommitmentsAfterRow, t time.Time) int {
	for i, r := range rows {
		if r.PeriodStart.Time.After(t) {
			return i
		}
	}
	return -1
}

// SwapRentalVehicle moves a rental under way to another vehicle (the old
// one is returned at in.At at in.BranchID, its preparation applies; the new
// one is handed over at the same time and place and carries the rest of the
// range up to the planned return, plate promised). Both allocations change
// in one transaction; the new allocation is returned. The new vehicle must
// keep the sold guarantee (another class is an upgrade of the reservation).
func (s *Service) SwapRentalVehicle(ctx context.Context, orgID int64, id uuid.UUID, in model.RentalSwap) (model.Allocation, error) {
	v, err := s.q.GetAvailabilityVehicle(ctx, db.GetAvailabilityVehicleParams{OrganizationID: orgID, Uuid: in.VehicleUUID})
	if err != nil {
		return model.Allocation{}, validation("vehicle_uuid", "not_found", "vehicle was not found")
	}
	at := in.At.UTC().Truncate(time.Microsecond)
	var created uuid.UUID
	err = s.rentalWrite(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, rs *rentalState) error {
		ref := rs.ref
		if ref.Kind != model.KindRental {
			return ErrNotRental
		}
		if v.ID == ref.VehicleID {
			return validation("vehicle_uuid", "same_vehicle", "the rental already uses this vehicle")
		}
		if !at.After(ref.StartsAt.Time) {
			return validation("at", "before_start", "the swap must be after the pick-up")
		}
		if !at.Before(ref.EndsAt.Time) {
			return validation("at", "after_return", "the swap must be before the planned return (extend first)")
		}
		key := versionKeyOf(ref)
		if !rs.cs.st.Eligible[key][v.ID] {
			return validation("vehicle_uuid", "not_eligible", "the vehicle does not keep the product's guarantee")
		}
		swapPrep, err := s.prepOf(ctx, q, orgID, in.BranchID)
		if err != nil {
			return err
		}
		retPrep, err := s.prepOf(ctx, q, orgID, ref.ReturnBranchID)
		if err != nil {
			return err
		}
		// 1. The old leg ends at the swap (shorter: never overlaps).
		oldEnd := at.Add(time.Duration(swapPrep) * time.Minute)
		if rs.c != nil {
			rs.c.Period.End, rs.c.End, rs.c.Return = oldEnd, at, in.BranchID
			rs.cs.st.Invalidate()
		}
		n, err := q.ReplanAllocation(ctx, db.ReplanAllocationParams{
			OrganizationID: orgID, ID: ref.ID, FromVehicleID: ref.VehicleID, ToVehicleID: ref.VehicleID,
			RentalProductID: ref.RentalProductID, ProductVersion: ref.ProductVersion,
			StartsAt: ref.StartsAt, EndsAt: ts(at), PrepMinutes: int32(swapPrep), PeriodEnd: ts(oldEnd),
			PickupBranchID: ref.PickupBranchID, ReturnBranchID: in.BranchID, PickupPointID: ref.PickupPointID,
			PlateFixed: true,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return errStale
		}
		// 2. The new leg: the rest of the range on the new vehicle.
		periodEnd := ref.EndsAt.Time.Add(time.Duration(retPrep) * time.Minute)
		req := engine.Request{
			Key: key, Period: engine.Interval{Start: at, End: periodEnd}, End: ref.EndsAt.Time,
			Pickup: in.BranchID, Return: ref.ReturnBranchID, Vehicles: []int64{v.ID}, Fixed: true,
		}
		plan, ok := rs.cs.st.Place(req, s.opts)
		if !ok || plan.Vehicle != v.ID {
			return &CapacityError{Reason: model.ReasonNoVehicle}
		}
		if err := rs.w.applyMoves(plan.Moves); err != nil {
			return err
		}
		pool := ref.FleetPool
		if vi := rs.cs.st.Vehicles[v.ID]; vi != nil {
			pool = vi.Pool
		}
		row, err := q.InsertAllocation(ctx, db.InsertAllocationParams{
			OrganizationID: orgID, VehicleID: v.ID, RentalProductID: ref.RentalProductID, ProductVersion: ref.ProductVersion,
			Kind: model.KindRental, StartsAt: ts(at), EndsAt: ref.EndsAt, PrepMinutes: int32(retPrep), PeriodEnd: ts(periodEnd),
			PickupBranchID: in.BranchID, ReturnBranchID: ref.ReturnBranchID, ReturnPointID: ref.ReturnPointID,
			FleetPool: pool, Channel: ref.Channel, PlateFixed: true, RefType: ref.RefType, RefUuid: ref.RefUuid,
			Note: in.Note, CreatedBy: int8Arg(in.ActorID),
		})
		if err != nil {
			return err
		}
		created = row.Uuid
		if in.Hook != nil {
			return in.Hook(ctx, tx, model.HookResult{
				AllocationID: row.ID, AllocationUUID: row.Uuid, VehicleID: v.ID, ProductID: ref.RentalProductID,
				ProductVersion: ref.ProductVersion, Kind: model.KindRental, PlateFixed: true,
				Moved: rs.w.movedAllocations(ref.ID), Affected: []model.Affected{},
			})
		}
		return nil
	})
	if err != nil {
		return model.Allocation{}, err
	}
	return s.GetAllocation(ctx, orgID, created)
}

// rentalGuard refuses changes that would rewrite a rental under way: a
// moved start or pick-up, another vehicle or product. An extension (the
// same start and places, a later end) stays allowed.
func rentalGuard(ref db.GetAllocationRefRow, r resolved, vehicle int64) error {
	if ref.Kind != model.KindRental {
		return nil
	}
	if !r.start.Equal(ref.StartsAt.Time) || r.pickup.ID != ref.PickupBranchID || (vehicle != 0 && vehicle != ref.VehicleID) {
		return ErrRentalUnderway
	}
	return nil
}
