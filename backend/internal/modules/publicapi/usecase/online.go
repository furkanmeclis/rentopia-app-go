package usecase

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

// offline is the part of the catalog the panel took off the online
// channels (branches.show_online / rental_products.show_online = false,
// TEC-730). Listings already skip it; availability and quotes refuse it
// even when a caller names it directly.
type offline struct {
	branches, products []uuid.UUID
}

func (s *Service) offlineCatalog(ctx context.Context, orgID int64) (offline, error) {
	row, err := s.q.ListPublicHiddenCatalog(ctx, orgID)
	if err != nil {
		return offline{}, err
	}
	return offline{branches: row.BranchUuids, products: row.ProductUuids}, nil
}

func (o offline) branch(id *uuid.UUID) bool  { return id != nil && slices.Contains(o.branches, *id) }
func (o offline) product(id *uuid.UUID) bool { return id != nil && slices.Contains(o.products, *id) }

// checkWhere refuses hidden branches and subgroups named in a request.
func (o offline) checkWhere(ve *ValidationError, pickup, ret, subgroup *uuid.UUID) {
	if o.branch(pickup) {
		ve.add("pickup_branch_uuid", "not_found", "this branch is not offered online")
	}
	if o.branch(ret) {
		ve.add("return_branch_uuid", "not_found", "this branch is not offered online")
	}
	if o.product(subgroup) {
		ve.add("subgroup_uuid", "not_found", "this subgroup is not offered online")
	}
}
