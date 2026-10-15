package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/eligibility"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// Rental group membership is explicit (fleet/eligibility): a vehicle serves
// a GROUP guarantee only while it is a member of the group and still meets
// its criteria. Adding a member only widens what can be sold; removing one
// may take a vehicle away from commitments already planned on it, so a
// removal goes through the availability engine (MembershipGuard): inside
// the component lock it re-plans every future commitment that relied on
// the membership onto another eligible vehicle and removes the membership
// in the same transaction, or refuses the whole change with
// MembershipInUseError (handlers answer 409) when one of them cannot move
// (a promised plate, a rental that already started, no other vehicle).

// Reasons a commitment blocks a membership removal.
const (
	BlockPlateFixed = "plate_fixed"
	BlockStarted    = "started"
	BlockNoVehicle  = "no_vehicle"
)

// MembershipGuard removes rental group memberships of a vehicle after
// re-planning the commitments that depend on them (availability engine).
// apply deletes the rows in the guard's transaction; cause identifies the
// change on the reservations it moves.
type MembershipGuard interface {
	RemoveMemberships(ctx context.Context, orgID, vehicleID int64, groupIDs []int64, cause uuid.UUID, apply func(q *db.Queries) error) error
}

// SetMembershipGuard wires the availability engine (optional: without one
// memberships are removed directly, e.g. in tests or tools without the
// engine).
func (s *Service) SetMembershipGuard(g MembershipGuard) { s.guard = g }

// BlockingReservation is the reservation of a blocking commitment.
type BlockingReservation struct {
	UUID      uuid.UUID
	Reference string
}

// BlockingAllocation is a commitment that relies on a membership being
// removed and cannot be re-planned.
type BlockingAllocation struct {
	UUID        uuid.UUID
	Kind        string // hold | reservation
	StartsAt    time.Time
	EndsAt      time.Time
	Reason      string // BlockPlateFixed | BlockStarted | BlockNoVehicle
	Reservation *BlockingReservation
}

// MembershipInUseError refuses a membership removal: the listed future
// commitments sit on the vehicle for a group it would leave and no re-plan
// can move them.
type MembershipInUseError struct {
	Allocations []BlockingAllocation
}

func (e *MembershipInUseError) Error() string {
	refs := make([]string, 0, len(e.Allocations))
	for _, a := range e.Allocations {
		if a.Reservation != nil {
			refs = append(refs, a.Reservation.Reference)
		} else {
			refs = append(refs, a.UUID.String())
		}
	}
	return "the vehicle has commitments that rely on this rental group membership: " + strings.Join(refs, ", ")
}

// changeMemberships runs apply (membership writes of one vehicle) in one
// transaction; when it removes groups and a guard is wired, the guard
// re-plans the dependent commitments first and runs apply in its own
// transaction.
func (s *Service) changeMemberships(ctx context.Context, orgID int64, vehicle db.Vehicle, removed []int64, apply func(tx *repository.Postgres) error) error {
	if s.guard == nil || len(removed) == 0 {
		return s.repo.InTx(ctx, apply)
	}
	return s.guard.RemoveMemberships(ctx, orgID, vehicle.ID, removed, vehicle.Uuid, func(q *db.Queries) error {
		return apply(repository.NewPostgres(nil, q))
	})
}

// groupVehicles loads a live group with its criteria, its member vehicles
// in the caller's branch scope (plate order) and the active vehicles in
// scope that meet the criteria but are not members (candidates).
func (s *Service) groupVehicles(ctx context.Context, orgID int64, groupUUID uuid.UUID) (repository.GroupRow, eligibility.GroupCriteria, []repository.VehicleRow, []repository.VehicleRow, error) {
	g, err := s.repo.GetGroup(ctx, orgID, groupUUID)
	if err != nil {
		return g, eligibility.GroupCriteria{}, nil, nil, mapNotFound(err, ErrGroupNotFound)
	}
	crit, err := s.criteria(ctx, orgID, []groupAttrs{attrsOf(g)})
	if err != nil {
		return g, eligibility.GroupCriteria{}, nil, nil, err
	}
	scope := orgctx.BranchScope(ctx)
	members, _, err := s.repo.ListVehicles(ctx, orgID, model.ListFilter{
		GroupUUIDs: []uuid.UUID{g.Uuid}, BranchIDs: scope, Limit: MaxListTargets, SortField: "plate",
	})
	if err != nil {
		return g, eligibility.GroupCriteria{}, nil, nil, err
	}
	isMember := make(map[int64]bool, len(members))
	for _, row := range members {
		isMember[row.Vehicle.ID] = true
	}
	active := true
	fleet, _, err := s.repo.ListVehicles(ctx, orgID, model.ListFilter{
		IsActive: &active, BranchIDs: scope, Limit: MaxListTargets, SortField: "plate",
	})
	if err != nil {
		return g, eligibility.GroupCriteria{}, nil, nil, err
	}
	candidates := []repository.VehicleRow{}
	for _, row := range fleet {
		if isMember[row.Vehicle.ID] {
			continue
		}
		if ok, _ := eligibility.EligibleForGroup(Attributes(row), crit[g.ID]); ok {
			candidates = append(candidates, row)
		}
	}
	return g, crit[g.ID], members, candidates, nil
}

// GroupMembers lists the member vehicles of a rental group (caller's branch
// scope, plate order) with their current eligibility, and counts the
// candidates "add all eligible vehicles" would add.
func (s *Service) GroupMembers(ctx context.Context, orgID int64, groupUUID uuid.UUID, limit, offset int32) (model.GroupMembers, error) {
	g, crit, members, candidates, err := s.groupVehicles(ctx, orgID, groupUUID)
	if err != nil {
		return model.GroupMembers{}, err
	}
	out := model.GroupMembers{
		Items: []model.GroupMember{}, Total: int64(len(members)), CandidatesTotal: int64(len(candidates)),
		Limit: limit, Offset: offset,
	}
	type check struct {
		ok      bool
		reasons []string
	}
	checks := make([]check, len(members))
	for i, row := range members {
		ok, reasons := eligibility.EligibleForGroup(Attributes(row), crit)
		checks[i] = check{ok, reasons}
		if ok {
			out.EligibleTotal++
		}
	}
	if int(offset) >= len(members) {
		return out, nil
	}
	end := min(int(offset)+int(limit), len(members))
	page := members[offset:end]
	ids := make([]int64, 0, len(page))
	for _, row := range page {
		ids = append(ids, row.Vehicle.ID)
	}
	added, err := s.repo.MembershipsByVehicle(ctx, orgID, ids)
	if err != nil {
		return model.GroupMembers{}, err
	}
	items, err := s.hydrate(ctx, orgID, page)
	if err != nil {
		return model.GroupMembers{}, err
	}
	for i, v := range items {
		c := checks[int(offset)+i]
		m := model.GroupMember{Vehicle: v, Eligible: c.ok, Reasons: c.reasons}
		for _, row := range added[page[i].Vehicle.ID] {
			if row.RentalGroupID == g.ID {
				m.AddedAt = row.CreatedAt.Time.UTC()
			}
		}
		out.Items = append(out.Items, m)
	}
	return out, nil
}

// AddEligibleMembers is "add all eligible vehicles": every active vehicle
// in the caller's branch scope that meets the group's criteria and is not a
// member yet becomes a member, in one transaction. Membership stays
// explicit; this is the deliberate bulk action that creates it.
func (s *Service) AddEligibleMembers(ctx context.Context, orgID int64, groupUUID uuid.UUID) (model.GroupAddEligibleResult, error) {
	g, _, _, candidates, err := s.groupVehicles(ctx, orgID, groupUUID)
	if err != nil {
		return model.GroupAddEligibleResult{}, err
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		for _, row := range candidates {
			if err := tx.AddMembership(ctx, orgID, row.Vehicle.ID, g.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.GroupAddEligibleResult{}, err
	}
	out := model.GroupAddEligibleResult{VehicleUUIDs: make([]uuid.UUID, 0, len(candidates))}
	for _, row := range candidates {
		out.VehicleUUIDs = append(out.VehicleUUIDs, row.Vehicle.Uuid)
	}
	out.Added = int64(len(out.VehicleUUIDs))
	return out, nil
}
