package usecase

import (
	"context"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CauseMembership is the cause recorded on reservations a membership
// removal re-planned.
const CauseMembership = "rental_group_membership"

// PlanMovedHandler records reassignments made by the engine outside a
// reservation flow on the reservations they moved, in the same transaction
// (reservations: plan_moved events).
type PlanMovedHandler interface {
	PlanMoved(ctx context.Context, q *db.Queries, orgID int64, moved []model.MovedAllocation, cause string, causeRef uuid.UUID) error
}

// SetPlanMovedHandler wires the reservations hook (optional).
func (s *Service) SetPlanMovedHandler(h PlanMovedHandler) { s.planMoved = h }

var _ vehicleusecase.MembershipGuard = (*Service)(nil)

// RemoveMemberships takes a vehicle out of rental groups (the vehicles
// module's MembershipGuard). Inside the component lock the vehicle leaves
// the eligible set of every version guaranteeing one of the groups; each
// future commitment on it that relied on such a version (the vehicle was
// eligible for it) is re-planned onto another eligible vehicle, possibly
// moving flexible commitments. When every one moves, the moves, the
// membership rows (apply) and the plan_moved records are written in one
// transaction. Otherwise nothing changes and the removal is refused with
// vehicleusecase.MembershipInUseError listing the commitments that cannot
// move: a promised plate, a rental that already started, no other vehicle.
func (s *Service) RemoveMemberships(ctx context.Context, orgID, vehicleID int64, groupIDs []int64, cause uuid.UUID, apply func(q *db.Queries) error) error {
	err := s.write(ctx, orgID, func(q *db.Queries, tx pgx.Tx) error {
		snap, err := s.snapshot(ctx, orgID)
		if err != nil {
			return err
		}
		comp := snap.component[vehicleID]
		if comp == 0 {
			// Not in the planned fleet (inactive / deleted): nothing to move.
			return apply(q)
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
		leaving := map[int64]bool{}
		for _, id := range groupIDs {
			leaving[id] = true
		}
		// The vehicle leaves the guarantees of those groups.
		lost := map[string]bool{}
		for key, v := range snap.versions {
			set := cs.st.Eligible[key]
			if v.RentalGroupID == 0 || !leaving[v.RentalGroupID] || !set[vehicleID] {
				continue
			}
			lost[key] = true
			next := make(map[int64]bool, len(set))
			for id, ok := range set {
				if id != vehicleID {
					next[id] = ok
				}
			}
			cs.st.Eligible[key] = next
		}
		cs.st.Invalidate()
		affected := []*engine.Commitment{}
		for _, c := range cs.st.Commitments {
			if c.Vehicle == vehicleID && lost[c.Key] {
				affected = append(affected, c)
			}
		}
		sort.Slice(affected, func(i, j int) bool {
			if !affected[i].Period.Start.Equal(affected[j].Period.Start) {
				return affected[i].Period.Start.Before(affected[j].Period.Start)
			}
			return affected[i].ID < affected[j].ID
		})
		blocked := map[int64]string{}
		for _, c := range affected {
			if c.Vehicle != vehicleID {
				continue // moved by an earlier plan
			}
			switch {
			case c.Fixed:
				blocked[c.ID] = vehicleusecase.BlockPlateFixed
				continue
			case !c.Period.Start.After(now):
				blocked[c.ID] = vehicleusecase.BlockStarted
				continue
			}
			req := engine.Request{Self: c.ID, Key: c.Key, Period: c.Period, End: c.End, Pickup: c.Pickup, Return: c.Return}
			plan, ok := cs.st.Place(req, s.opts)
			if !ok {
				blocked[c.ID] = vehicleusecase.BlockNoVehicle
				continue
			}
			cs.st.Apply(req, plan, 0)
			moves := append([]engine.Move{}, plan.Moves...)
			moves = append(moves, engine.Move{Commitment: c.ID, From: vehicleID, To: plan.Vehicle})
			if err := w.applyMoves(moves); err != nil {
				return err
			}
		}
		if len(blocked) > 0 {
			return s.membershipInUse(ctx, q, orgID, cs, blocked)
		}
		if err := apply(q); err != nil {
			return err
		}
		if moved := w.movedAllocations(0); len(moved) > 0 && s.planMoved != nil {
			return s.planMoved.PlanMoved(ctx, q, orgID, moved, CauseMembership, cause)
		}
		return nil
	})
	if err == nil {
		s.cache.invalidate(orgID)
	}
	return err
}

// membershipInUse builds the refusal of a membership removal.
func (s *Service) membershipInUse(ctx context.Context, q *db.Queries, orgID int64, cs *componentState, blocked map[int64]string) error {
	ids := make([]int64, 0, len(blocked))
	for id := range blocked {
		ids = append(ids, id)
	}
	refs, err := q.ListAllocationReservations(ctx, db.ListAllocationReservationsParams{OrganizationID: orgID, AllocationIds: ids})
	if err != nil {
		return err
	}
	byAlloc := map[int64]*vehicleusecase.BlockingReservation{}
	for _, r := range refs {
		byAlloc[r.AllocationID.Int64] = &vehicleusecase.BlockingReservation{UUID: r.Uuid, Reference: r.Reference}
	}
	out := &vehicleusecase.MembershipInUseError{Allocations: make([]vehicleusecase.BlockingAllocation, 0, len(ids))}
	for _, id := range ids {
		a := cs.allocs[id]
		out.Allocations = append(out.Allocations, vehicleusecase.BlockingAllocation{
			UUID: a.Uuid, Kind: a.Kind, StartsAt: a.StartsAt.Time.UTC(), EndsAt: a.EndsAt.Time.UTC(),
			Reason: blocked[id], Reservation: byAlloc[id],
		})
	}
	sort.Slice(out.Allocations, func(i, j int) bool {
		a, b := out.Allocations[i], out.Allocations[j]
		if !a.StartsAt.Equal(b.StartsAt) {
			return a.StartsAt.Before(b.StartsAt)
		}
		return a.UUID.String() < b.UUID.String()
	})
	return out
}
