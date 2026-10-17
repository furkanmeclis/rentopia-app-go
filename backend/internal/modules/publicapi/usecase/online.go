package usecase

import (
	"context"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/google/uuid"
)

// offline is the part of the catalog the panel hid from the storefront
// (branded site and widget: show_online = false on branches, rental
// products and extras, TEC-730). Storefront listings already skip it;
// storefront availability and quotes refuse it even when a caller names it
// directly. Secret server keys (integrations) ignore it: for them offline
// is empty.
type offline struct {
	branches, products, extras []uuid.UUID
}

func (s *Service) offlineCatalog(ctx context.Context, c model.Credential) (offline, error) {
	if !c.Storefront() {
		return offline{}, nil
	}
	row, err := s.q.ListPublicHiddenCatalog(ctx, c.OrganizationID)
	if err != nil {
		return offline{}, err
	}
	return offline{branches: row.BranchUuids, products: row.ProductUuids, extras: row.ExtraUuids}, nil
}

func (o offline) branch(id *uuid.UUID) bool  { return id != nil && slices.Contains(o.branches, *id) }
func (o offline) product(id *uuid.UUID) bool { return id != nil && slices.Contains(o.products, *id) }
func (o offline) extra(id uuid.UUID) bool    { return slices.Contains(o.extras, id) }

// checkWhere refuses hidden branches and subgroups named in a request.
func (o offline) checkWhere(ve *ValidationError, pickup, ret, subgroup *uuid.UUID) {
	if o.branch(pickup) {
		ve.add("pickup_branch_uuid", "not_found", "this branch is not offered here")
	}
	if o.branch(ret) {
		ve.add("return_branch_uuid", "not_found", "this branch is not offered here")
	}
	if o.product(subgroup) {
		ve.add("subgroup_uuid", "not_found", "this subgroup is not offered here")
	}
}
