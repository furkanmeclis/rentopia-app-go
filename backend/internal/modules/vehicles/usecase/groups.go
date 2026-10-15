package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/eligibility"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Catalog model ids are compared as grouped models: a rental group or a
// guarantee that names "Fiat Egea" accepts an Egea listed under any TSB brand
// of the Fiat display brand. Both sides are mapped to the lowest model id of
// the group (the id tenant pickers use).

// Attributes are the eligibility facts of a vehicle row.
func Attributes(row repository.VehicleRow) eligibility.VehicleAttributes {
	v := row.Vehicle
	return eligibility.VehicleAttributes{
		ID: v.Uuid, CatalogModelID: row.ModelGroupID, CatalogTypeID: v.CatalogTypeID.Int64,
		Transmission: v.Transmission.String, Fuel: v.Fuel.String, BodyType: v.BodyType.String,
		Seats: int(v.Seats.Int16), Luggage: int(v.Luggage.Int16), AirConditioning: v.AirConditioning,
	}
}

// groupAttrs are the protected attributes of a rental group.
type groupAttrs struct {
	ID              int64
	Transmission    pgtype.Text
	Fuel            pgtype.Text
	BodyType        pgtype.Text
	MinSeats        pgtype.Int2
	MinLuggage      pgtype.Int2
	AirConditioning bool
}

func attrsOf(g repository.GroupRow) groupAttrs {
	return groupAttrs{
		ID: g.ID, Transmission: g.Transmission, Fuel: g.Fuel, BodyType: g.BodyType,
		MinSeats: g.MinSeats, MinLuggage: g.MinLuggage, AirConditioning: g.AirConditioning,
	}
}

// criteria loads the eligibility criteria of groups (catalog models mapped
// to grouped models).
func (s *Service) criteria(ctx context.Context, orgID int64, groups []groupAttrs) (map[int64]eligibility.GroupCriteria, error) {
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	models, types, err := s.repo.GroupCriteria(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	all := []int64{}
	for _, list := range models {
		all = append(all, list...)
	}
	canonical, err := s.repo.CanonicalModelIDs(ctx, all)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]eligibility.GroupCriteria, len(groups))
	for _, g := range groups {
		mapped := make([]int64, 0, len(models[g.ID]))
		for _, id := range models[g.ID] {
			if c, ok := canonical[id]; ok {
				mapped = append(mapped, c)
			}
		}
		out[g.ID] = eligibility.GroupCriteria{
			Transmission: g.Transmission.String, Fuel: g.Fuel.String, BodyType: g.BodyType.String,
			MinSeats: int(g.MinSeats.Int16), MinLuggage: int(g.MinLuggage.Int16),
			AirConditioning: g.AirConditioning, CatalogModelIDs: mapped, CatalogTypeIDs: types[g.ID],
		}
	}
	return out, nil
}

// Groups returns the rental group memberships of a vehicle with their
// current eligibility, and the live active groups it is eligible for but not
// a member of (suggestions).
func (s *Service) Groups(ctx context.Context, orgID int64, id uuid.UUID) (model.GroupsView, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return model.GroupsView{}, err
	}
	groups, err := s.repo.LiveGroups(ctx, orgID)
	if err != nil {
		return model.GroupsView{}, err
	}
	attrs := make([]groupAttrs, 0, len(groups))
	for _, g := range groups {
		attrs = append(attrs, attrsOf(g))
	}
	crit, err := s.criteria(ctx, orgID, attrs)
	if err != nil {
		return model.GroupsView{}, err
	}
	members, err := s.repo.MembershipsByVehicle(ctx, orgID, []int64{row.Vehicle.ID})
	if err != nil {
		return model.GroupsView{}, err
	}
	vehicle := Attributes(row)
	out := model.GroupsView{Items: []model.Membership{}, Suggestions: []model.Suggestion{}}
	isMember := map[int64]bool{}
	for _, m := range members[row.Vehicle.ID] {
		isMember[m.RentalGroupID] = true
		ok, reasons := eligibility.EligibleForGroup(vehicle, crit[m.RentalGroupID])
		out.Items = append(out.Items, model.Membership{
			Group: membershipRef(m), Eligible: ok, Reasons: reasons, AddedAt: m.CreatedAt.Time.UTC(),
		})
	}
	for _, g := range groups {
		if isMember[g.ID] || !g.IsActive {
			continue
		}
		if ok, reasons := eligibility.EligibleForGroup(vehicle, crit[g.ID]); ok {
			out.Suggestions = append(out.Suggestions, model.Suggestion{Group: groupRef(g), Eligible: true, Reasons: reasons})
		}
	}
	return out, nil
}

// AddGroup adds a vehicle to a rental group after checking eligibility
// (NotEligibleError lists every failed criterion).
func (s *Service) AddGroup(ctx context.Context, orgID int64, id, groupUUID uuid.UUID) (model.Membership, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return model.Membership{}, err
	}
	g, err := s.eligibleGroup(ctx, orgID, row, groupUUID)
	if err != nil {
		return model.Membership{}, err
	}
	if err := s.repo.AddMembership(ctx, orgID, row.Vehicle.ID, g.ID); err != nil {
		return model.Membership{}, err
	}
	return model.Membership{Group: groupRef(g), Eligible: true, Reasons: []string{}, AddedAt: s.now().UTC()}, nil
}

// RemoveGroup removes a vehicle from a rental group. Future commitments
// that relied on the membership are re-planned onto other vehicles in the
// same transaction; when one cannot move the removal is refused
// (MembershipInUseError).
func (s *Service) RemoveGroup(ctx context.Context, orgID int64, id, groupUUID uuid.UUID) error {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return err
	}
	g, err := s.repo.GetGroup(ctx, orgID, groupUUID)
	if err != nil {
		return mapNotFound(err, ErrMembershipNotFound)
	}
	members, err := s.repo.MembershipsByVehicle(ctx, orgID, []int64{row.Vehicle.ID})
	if err != nil {
		return err
	}
	isMember := false
	for _, m := range members[row.Vehicle.ID] {
		isMember = isMember || m.RentalGroupID == g.ID
	}
	if !isMember {
		return ErrMembershipNotFound
	}
	return s.changeMemberships(ctx, orgID, row.Vehicle, []int64{g.ID}, func(tx *repository.Postgres) error {
		removed, err := tx.RemoveMembership(ctx, orgID, row.Vehicle.ID, g.ID)
		if err != nil {
			return err
		}
		if !removed {
			return ErrMembershipNotFound
		}
		return nil
	})
}

// SetGroups makes groupUUIDs the exact memberships of a vehicle (bulk
// replace) or adds them (replace=false). Every new group is checked for
// eligibility unless check is false (rollback restores the previous set as
// it was). Groups a replace drops are removed like RemoveGroup (dependent
// commitments re-planned, or MembershipInUseError). It returns the previous
// memberships.
func (s *Service) SetGroups(ctx context.Context, orgID int64, id uuid.UUID, groupUUIDs []uuid.UUID, replace, check bool) ([]uuid.UUID, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.MembershipsByVehicle(ctx, orgID, []int64{row.Vehicle.ID})
	if err != nil {
		return nil, err
	}
	prev := make([]uuid.UUID, 0, len(members[row.Vehicle.ID]))
	current := map[uuid.UUID]bool{}
	for _, m := range members[row.Vehicle.ID] {
		prev = append(prev, m.Uuid)
		current[m.Uuid] = true
	}
	ids := make([]int64, 0, len(groupUUIDs))
	for _, gid := range groupUUIDs {
		var g repository.GroupRow
		if check && !current[gid] {
			g, err = s.eligibleGroup(ctx, orgID, row, gid)
		} else {
			g, err = s.repo.GetGroup(ctx, orgID, gid)
			err = mapNotFound(err, ErrGroupNotFound)
		}
		if err != nil {
			return prev, err
		}
		ids = append(ids, g.ID)
	}
	removed := []int64{}
	if replace {
		kept := map[int64]bool{}
		for _, gid := range ids {
			kept[gid] = true
		}
		for _, m := range members[row.Vehicle.ID] {
			if !kept[m.RentalGroupID] {
				removed = append(removed, m.RentalGroupID)
			}
		}
	}
	err = s.changeMemberships(ctx, orgID, row.Vehicle, removed, func(tx *repository.Postgres) error {
		if replace {
			if err := tx.ClearMemberships(ctx, orgID, row.Vehicle.ID); err != nil {
				return err
			}
		}
		for _, gid := range ids {
			if err := tx.AddMembership(ctx, orgID, row.Vehicle.ID, gid); err != nil {
				return err
			}
		}
		return nil
	})
	return prev, err
}

// eligibleGroup loads a live group and checks the vehicle against it.
func (s *Service) eligibleGroup(ctx context.Context, orgID int64, row repository.VehicleRow, groupUUID uuid.UUID) (repository.GroupRow, error) {
	g, err := s.repo.GetGroup(ctx, orgID, groupUUID)
	if err != nil {
		return g, mapNotFound(err, ErrGroupNotFound)
	}
	crit, err := s.criteria(ctx, orgID, []groupAttrs{attrsOf(g)})
	if err != nil {
		return g, err
	}
	if ok, reasons := eligibility.EligibleForGroup(Attributes(row), crit[g.ID]); !ok {
		return g, &NotEligibleError{GroupUUID: g.Uuid, GroupCode: g.Code, Reasons: reasons}
	}
	return g, nil
}

// ProductVehicles lists the active vehicles (in the caller's branch scope)
// that keep the promise of a product's current version (SatisfiesGuarantee),
// plate order.
func (s *Service) ProductVehicles(ctx context.Context, orgID int64, productUUID uuid.UUID, limit, offset int32) (model.ProductVehicles, error) {
	version, err := s.repo.ProductCurrentVersion(ctx, orgID, productUUID)
	if err != nil {
		return model.ProductVehicles{}, mapNotFound(err, ErrProductNotFound)
	}
	g, err := s.guarantee(ctx, orgID, version)
	if err != nil {
		return model.ProductVehicles{}, err
	}
	active := true
	rows, _, err := s.repo.ListVehicles(ctx, orgID, model.ListFilter{
		IsActive: &active, BranchIDs: orgctx.BranchScope(ctx),
		Limit: MaxListTargets, SortField: "plate",
	})
	if err != nil {
		return model.ProductVehicles{}, err
	}
	matched := make([]repository.VehicleRow, 0)
	for _, row := range rows {
		if ok, _ := eligibility.SatisfiesGuarantee(Attributes(row), g); ok {
			matched = append(matched, row)
		}
	}
	out := model.ProductVehicles{
		GuaranteeType: version.GuaranteeType, Total: int64(len(matched)), Limit: limit, Offset: offset,
		Items: []model.Vehicle{},
	}
	if int(offset) >= len(matched) {
		return out, nil
	}
	end := min(int(offset)+int(limit), len(matched))
	items, err := s.hydrate(ctx, orgID, matched[offset:end])
	if err != nil {
		return model.ProductVehicles{}, err
	}
	out.Items = items
	return out, nil
}

// guarantee maps a product version to eligibility terms.
func (s *Service) guarantee(ctx context.Context, orgID int64, v db.RentalProductVersion) (eligibility.Guarantee, error) {
	g := eligibility.Guarantee{Type: v.GuaranteeType, VariantLevel: v.VariantLevel.String, CatalogTypeID: v.CatalogTypeID.Int64}
	if v.VehicleID.Valid {
		id := uuid.UUID(v.VehicleID.Bytes)
		g.VehicleID = &id
	}
	if v.CatalogModelID.Valid {
		canonical, err := s.repo.CanonicalModelIDs(ctx, []int64{v.CatalogModelID.Int64})
		if err != nil {
			return g, err
		}
		g.CatalogModelID = canonical[v.CatalogModelID.Int64]
	}
	if v.RentalGroupID.Valid {
		groups, err := s.repo.GroupsByIDs(ctx, orgID, []int64{v.RentalGroupID.Int64})
		if err != nil {
			return g, err
		}
		if row, ok := groups[v.RentalGroupID.Int64]; ok {
			crit, err := s.criteria(ctx, orgID, []groupAttrs{{
				ID: row.ID, Transmission: row.Transmission, Fuel: row.Fuel, BodyType: row.BodyType,
				MinSeats: row.MinSeats, MinLuggage: row.MinLuggage, AirConditioning: row.AirConditioning,
			}})
			if err != nil {
				return g, err
			}
			c := crit[row.ID]
			g.Group = &c
			members, err := s.repo.GroupMembers(ctx, orgID, []int64{row.ID})
			if err != nil {
				return g, err
			}
			g.Members = members[row.ID]
		}
	}
	return g, nil
}

// AvailabilityFleet lists every live, active vehicle of the organization
// (no branch scope: capacity is organization-wide; the availability engine
// applies location itself), plate order.
func (s *Service) AvailabilityFleet(ctx context.Context, orgID int64) ([]repository.VehicleRow, error) {
	active := true
	rows, _, err := s.repo.ListVehicles(ctx, orgID, model.ListFilter{
		IsActive: &active, Limit: MaxListTargets, SortField: "plate",
	})
	return rows, err
}

// Guarantees maps product versions to eligibility terms in one pass (rental
// group criteria and canonical catalog models loaded in batches), keyed by
// version id. The availability engine builds its eligibility graph with it.
func (s *Service) Guarantees(ctx context.Context, orgID int64, versions []db.RentalProductVersion) (map[int64]eligibility.Guarantee, error) {
	groupIDs := []int64{}
	modelIDs := []int64{}
	for _, v := range versions {
		if v.RentalGroupID.Valid {
			groupIDs = append(groupIDs, v.RentalGroupID.Int64)
		}
		if v.CatalogModelID.Valid {
			modelIDs = append(modelIDs, v.CatalogModelID.Int64)
		}
	}
	canonical, err := s.repo.CanonicalModelIDs(ctx, modelIDs)
	if err != nil {
		return nil, err
	}
	groups, err := s.repo.GroupsByIDs(ctx, orgID, groupIDs)
	if err != nil {
		return nil, err
	}
	attrs := make([]groupAttrs, 0, len(groups))
	for _, row := range groups {
		attrs = append(attrs, groupAttrs{
			ID: row.ID, Transmission: row.Transmission, Fuel: row.Fuel, BodyType: row.BodyType,
			MinSeats: row.MinSeats, MinLuggage: row.MinLuggage, AirConditioning: row.AirConditioning,
		})
	}
	crit, err := s.criteria(ctx, orgID, attrs)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.GroupMembers(ctx, orgID, groupIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]eligibility.Guarantee, len(versions))
	for _, v := range versions {
		g := eligibility.Guarantee{Type: v.GuaranteeType, VariantLevel: v.VariantLevel.String, CatalogTypeID: v.CatalogTypeID.Int64}
		if v.VehicleID.Valid {
			id := uuid.UUID(v.VehicleID.Bytes)
			g.VehicleID = &id
		}
		if v.CatalogModelID.Valid {
			g.CatalogModelID = canonical[v.CatalogModelID.Int64]
		}
		if v.RentalGroupID.Valid {
			if c, ok := crit[v.RentalGroupID.Int64]; ok {
				g.Group = &c
				g.Members = members[v.RentalGroupID.Int64]
			}
		}
		out[v.ID] = g
	}
	return out, nil
}

// IsNotEligible reports whether err is a NotEligibleError.
func IsNotEligible(err error) (*NotEligibleError, bool) {
	var ne *NotEligibleError
	ok := errors.As(err, &ne)
	return ne, ok
}
