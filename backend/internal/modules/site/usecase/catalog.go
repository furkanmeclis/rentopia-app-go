package usecase

import (
	"context"
	"errors"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Catalog publish state (TEC-730): which branches, subgroups and extras the
// online channels offer. These switches are live (not part of the draft):
// they change what the public API sells, not how the site looks.
//
//   - branches / subgroups: show_online (public API listings, availability
//     and quotes skip a hidden one);
//   - extras: the web sales channel in extras.channels (the existing channel
//     rule of the public API; NULL = every channel).

func extraOnline(channels []string) bool {
	return channels == nil || slices.Contains(channels, SiteChannel)
}

func catalogExtra(uuid uuid.UUID, code, name, status string, channels []string) model.CatalogExtra {
	return model.CatalogExtra{UUID: uuid, Code: code, Name: name, Status: status, ShowOnline: extraOnline(channels)}
}

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
		out.Extras = append(out.Extras, catalogExtra(e.Uuid, e.Code, e.Name, e.Status, e.Channels))
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

// SetExtraOnline adds or removes the web channel of an extra. An extra sold
// on every channel (NULL) that is switched off keeps every other channel.
func (s *Service) SetExtraOnline(ctx context.Context, orgID int64, id uuid.UUID, in model.VisibilityInput) (model.CatalogExtra, error) {
	e, err := s.q.GetSiteCatalogExtra(ctx, db.GetSiteCatalogExtraParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.CatalogExtra{}, ErrCatalogItemNotFound
	}
	if err != nil {
		return model.CatalogExtra{}, err
	}
	if in.ShowOnline == nil {
		return model.CatalogExtra{}, visibilityRequired()
	}
	if *in.ShowOnline == extraOnline(e.Channels) {
		return catalogExtra(e.Uuid, e.Code, e.Name, e.Status, e.Channels), nil
	}
	var channels []string
	switch {
	case *in.ShowOnline:
		channels = append(slices.Clone(e.Channels), SiteChannel)
	case e.Channels == nil:
		for _, c := range osmodel.SalesChannels {
			if c != SiteChannel {
				channels = append(channels, c)
			}
		}
	default:
		for _, c := range e.Channels {
			if c != SiteChannel {
				channels = append(channels, c)
			}
		}
		if channels == nil {
			channels = []string{}
		}
	}
	u, err := s.q.SetExtraChannels(ctx, db.SetExtraChannelsParams{OrganizationID: orgID, Uuid: id, Channels: channels})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.CatalogExtra{}, ErrCatalogItemNotFound
	}
	if err != nil {
		return model.CatalogExtra{}, err
	}
	return catalogExtra(u.Uuid, u.Code, u.Name, u.Status, u.Channels), nil
}
