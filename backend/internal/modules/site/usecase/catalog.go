package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Storefront publish state of the catalog (TEC-730): which branches,
// subgroups and extras the branded site and the widget offer
// (show_online). These switches are live (not part of the draft). They
// apply to publishable-key callers only (public API storefront listings,
// availability and quotes skip a hidden item); integrations with secret
// keys keep seeing everything their channel sells.

// Catalog lists the organization's branches, subgroups and extras with
// their online state.
func (s *Service) Catalog(ctx context.Context, orgID int64) (model.Catalog, error) {
	out := model.Catalog{Branches: []model.CatalogBranch{}, Subgroups: []model.CatalogSubgroup{}, Extras: []model.CatalogExtra{}}
	branches, err := s.q.ListSiteCatalogBranches(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, b := range branches {
		out.Branches = append(out.Branches, model.CatalogBranch{
			UUID: b.Uuid, Code: b.Code, Name: b.Name, City: b.City, Active: b.IsActive, ShowOnline: b.ShowOnline,
		})
	}
	products, err := s.q.ListSiteCatalogProducts(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, p := range products {
		out.Subgroups = append(out.Subgroups, model.CatalogSubgroup{
			UUID: p.Uuid, Code: p.Code, Title: p.Title, Segment: p.SegmentName, Status: p.Status, Active: p.IsActive,
			ShowOnline: p.ShowOnline,
		})
	}
	extras, err := s.q.ListSiteCatalogExtras(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, e := range extras {
		out.Extras = append(out.Extras, model.CatalogExtra{
			UUID: e.Uuid, Code: e.Code, Name: e.Name, Status: e.Status, ShowOnline: e.ShowOnline,
		})
	}
	return out, nil
}

func visibilityRequired() error {
	ve := &ValidationError{}
	ve.add("show_online", "required", "show_online is required")
	return ve
}

// SetBranchOnline switches a branch on or off the online channels. The
// branch is looked up first (another organization's branch is
// ErrCatalogItemNotFound whatever the body says).
func (s *Service) SetBranchOnline(ctx context.Context, orgID int64, id uuid.UUID, in model.VisibilityInput) (model.CatalogBranch, error) {
	arg := db.SetBranchShowOnlineParams{OrganizationID: orgID, Uuid: id}
	if in.ShowOnline != nil {
		arg.ShowOnline = pgtype.Bool{Bool: *in.ShowOnline, Valid: true}
	}
	b, err := s.q.SetBranchShowOnline(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.CatalogBranch{}, ErrCatalogItemNotFound
	}
	if err != nil {
		return model.CatalogBranch{}, err
	}
	if in.ShowOnline == nil {
		return model.CatalogBranch{}, visibilityRequired()
	}
	return model.CatalogBranch{UUID: b.Uuid, Code: b.Code, Name: b.Name, City: b.City, Active: b.IsActive, ShowOnline: b.ShowOnline}, nil
}

// SetSubgroupOnline switches a subgroup (rental product) on or off the
// online channels.
func (s *Service) SetSubgroupOnline(ctx context.Context, orgID int64, id uuid.UUID, in model.VisibilityInput) (model.CatalogSubgroup, error) {
	arg := db.SetProductShowOnlineParams{OrganizationID: orgID, Uuid: id}
	if in.ShowOnline != nil {
		arg.ShowOnline = pgtype.Bool{Bool: *in.ShowOnline, Valid: true}
	}
	p, err := s.q.SetProductShowOnline(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.CatalogSubgroup{}, ErrCatalogItemNotFound
	}
	if err != nil {
		return model.CatalogSubgroup{}, err
	}
	if in.ShowOnline == nil {
		return model.CatalogSubgroup{}, visibilityRequired()
	}
	return model.CatalogSubgroup{
		UUID: p.Uuid, Code: p.Code, Title: p.Title, Status: p.Status, Active: p.IsActive, ShowOnline: p.ShowOnline,
	}, nil
}

// SetExtraOnline shows or hides an extra on the storefront. Its sales
// channels are not touched: integrations keep selling it.
func (s *Service) SetExtraOnline(ctx context.Context, orgID int64, id uuid.UUID, in model.VisibilityInput) (model.CatalogExtra, error) {
	arg := db.SetExtraShowOnlineParams{OrganizationID: orgID, Uuid: id}
	if in.ShowOnline != nil {
		arg.ShowOnline = pgtype.Bool{Bool: *in.ShowOnline, Valid: true}
	}
	e, err := s.q.SetExtraShowOnline(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.CatalogExtra{}, ErrCatalogItemNotFound
	}
	if err != nil {
		return model.CatalogExtra{}, err
	}
	if in.ShowOnline == nil {
		return model.CatalogExtra{}, visibilityRequired()
	}
	return model.CatalogExtra{UUID: e.Uuid, Code: e.Code, Name: e.Name, Status: e.Status, ShowOnline: e.ShowOnline}, nil
}
