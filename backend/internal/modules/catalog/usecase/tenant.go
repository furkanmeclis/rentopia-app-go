package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/google/uuid"
)

// Tenant read-only search. Tenants see display brands (one "Fiat" for FIAT
// and TOFAS-FIAT) and grouped models (one "Egea" whatever TSB brand lists
// it); a grouped model is addressed by the uuid of any of its models.

func (s *Service) TenantBrands(ctx context.Context, f model.TenantFilter) ([]model.TenantBrand, int64, error) {
	if f.Category != "" && !validCategory(f.Category) {
		return nil, 0, fmt.Errorf("%w: unknown category", ErrInvalidRequest)
	}
	rows, err := s.q.TenantListCatalogBrands(ctx, db.TenantListCatalogBrandsParams{
		Category: text(f.Category), Q: text(f.Q), Year: int4Ptr(f.Year), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.TenantCountCatalogBrands(ctx, db.TenantCountCatalogBrandsParams{
		Category: text(f.Category), Q: text(f.Q), Year: int4Ptr(f.Year),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.TenantBrand, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.TenantBrand{
			UUID: r.Uuid, DisplayName: r.Name, Slug: r.Slug, Category: r.Category,
			LogoURL: DisplayBrandLogoURL(r.Uuid, r.LogoObjectKey, r.UpdatedAt),
		})
	}
	return out, total, nil
}

// tenantDisplayBrand resolves a brand reference (display brand uuid, or a
// TSB brand uuid from older clients) to an active display brand.
func (s *Service) tenantDisplayBrand(ctx context.Context, ref uuid.UUID) (db.GetActiveCatalogDisplayBrandForTenantRow, error) {
	d, err := s.q.GetActiveCatalogDisplayBrandForTenant(ctx, ref)
	if err != nil {
		return d, mapErr(repository.MapErr(err))
	}
	return d, nil
}

func (s *Service) TenantModels(ctx context.Context, f model.TenantFilter) ([]model.TenantModel, int64, error) {
	d, err := s.tenantDisplayBrand(ctx, f.BrandUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.TenantListCatalogModels(ctx, db.TenantListCatalogModelsParams{
		DisplayBrandID: d.ID, Q: text(f.Q), Year: int4Ptr(f.Year), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.TenantCountCatalogModels(ctx, db.TenantCountCatalogModelsParams{
		DisplayBrandID: d.ID, Q: text(f.Q), Year: int4Ptr(f.Year),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.TenantModel, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.TenantModel{
			UUID: r.Uuid, Name: r.Name, Brand: model.BrandRef{UUID: d.Uuid, DisplayName: d.Name},
		})
	}
	return out, total, nil
}

// TenantTypes lists the types of a grouped model across every TSB brand of
// its display brand.
func (s *Service) TenantTypes(ctx context.Context, f model.TenantFilter) ([]model.TenantType, int64, error) {
	g, err := s.q.GetCatalogModelGroup(ctx, f.ModelUUID)
	if err != nil {
		return nil, 0, mapErr(repository.MapErr(err))
	}
	if !g.DisplayBrandActive {
		return nil, 0, ErrNotFound
	}
	ref := model.BrandRef{UUID: g.DisplayBrandUuid, DisplayName: g.DisplayBrandName}
	rows, err := s.q.TenantListCatalogTypes(ctx, db.TenantListCatalogTypesParams{
		DisplayBrandID: g.DisplayBrandID, GroupKey: g.GroupKey,
		Q: text(f.Q), Year: int4Ptr(f.Year), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.TenantCountCatalogTypes(ctx, db.TenantCountCatalogTypesParams{
		DisplayBrandID: g.DisplayBrandID, GroupKey: g.GroupKey, Q: text(f.Q), Year: int4Ptr(f.Year),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.TenantType, 0, len(rows))
	for _, r := range rows {
		tt := model.TenantType{
			UUID: r.Uuid, TSBTypeCode: r.TsbTypeCode, Name: r.RawName, Years: r.Years,
			SuggestedTransmission: optString(r.SuggestedTransmission), SuggestedFuel: optString(r.SuggestedFuel),
			Brand: ref,
			Model: model.NamedRef{UUID: f.ModelUUID, Name: g.GroupName},
		}
		if tt.Years == nil {
			tt.Years = []int32{}
		}
		if v, ok := repository.NumericToFloat(r.KaskoValue); ok {
			tt.KaskoValue = &v
			p := r.KaskoPeriod
			tt.KaskoPeriod = &p
		}
		out = append(out, tt)
	}
	return out, total, nil
}
