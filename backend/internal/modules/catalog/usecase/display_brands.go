package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// DisplayBrandLogoURL is the API-owned logo stream of a display brand. The
// version query busts caches after a re-upload.
func DisplayBrandLogoURL(id uuid.UUID, key pgtype.Text, updated pgtype.Timestamptz) *string {
	if !key.Valid || strings.TrimSpace(key.String) == "" {
		return nil
	}
	u := fmt.Sprintf("/v1/platform/catalog/display-brands/%s/logo?v=%d", id, updated.Time.Unix())
	return &u
}

func mapDisplayBrand(d db.ListCatalogDisplayBrandsRow) model.DisplayBrand {
	urls := DisplayBrandLogoURLs(d.Uuid, d.LogoObjectKey, d.SyncedLogoVariants, d.UpdatedAt)
	out := model.DisplayBrand{
		UUID: d.Uuid, Name: d.Name, Slug: d.Slug, IsActive: d.IsActive,
		LogoURL: urls.LogoURL, LogoBadgeURL: urls.BadgeURL, LogoWordmarkURL: urls.WordmarkURL,
		LogoUploaded: d.LogoObjectKey.Valid && strings.TrimSpace(d.LogoObjectKey.String) != "",
		BrandColor:   optString(d.BrandColor),
		BrandsCount:  d.BrandsCount, TypesCount: d.TypesCount, ModelsCount: d.ModelsCount,
		TSBNames: d.TsbNames, CreatedAt: d.CreatedAt.Time, UpdatedAt: d.UpdatedAt.Time,
	}
	if out.TSBNames == nil {
		out.TSBNames = []string{}
	}
	mm := &model.DisplayBrandMotomarks{
		ID: optString(d.MotomarksID), MatchSource: d.MotomarksMatchSource,
		SuggestedID: optString(d.MotomarksSuggestedID), SyncedVariants: d.SyncedLogoVariants,
	}
	if mm.SyncedVariants == nil {
		mm.SyncedVariants = []string{}
	}
	if v, ok := repository.NumericToFloat(d.MotomarksMatchScore); ok {
		mm.MatchScore = &v
	}
	if v, ok := repository.NumericToFloat(d.MotomarksSuggestedScore); ok {
		mm.SuggestedScore = &v
	}
	out.Motomarks = mm
	return out
}

func (s *Service) displayBrandID(ctx context.Context, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, *id)
	if err != nil {
		return pgtype.Int8{}, mapErr(repository.MapErr(err))
	}
	return pgtype.Int8{Int64: d.ID, Valid: true}, nil
}

// deleteIfEmpty removes a display brand that no TSB brand points at any
// more and returns its logo key (to delete from storage after commit).
func deleteIfEmpty(ctx context.Context, q *db.Queries, displayBrandID int64) (*string, error) {
	members, err := q.ListCatalogBrandsByDisplayBrand(ctx, displayBrandID)
	if err != nil || len(members) > 0 {
		return nil, err
	}
	all, err := q.ListCatalogDisplayBrandsAll(ctx)
	if err != nil {
		return nil, err
	}
	var key *string
	for _, d := range all {
		if d.ID == displayBrandID && d.LogoObjectKey.Valid {
			k := d.LogoObjectKey.String
			key = &k
		}
	}
	if _, err := q.DeleteCatalogDisplayBrand(ctx, displayBrandID); err != nil {
		return nil, err
	}
	return key, nil
}

// dropLogo deletes a logo object best-effort (the row is already gone).
func (s *Service) dropLogo(ctx context.Context, key *string) {
	if key == nil || s.storage == nil {
		return
	}
	if err := s.storage.Delete(ctx, *key); err != nil {
		s.log.Warn("catalog_display_brand_logo_delete_failed", "key", *key, "error", err)
	}
}

func (s *Service) ListDisplayBrands(ctx context.Context, f model.DisplayBrandFilter) ([]model.DisplayBrand, int64, error) {
	rows, err := s.q.ListCatalogDisplayBrands(ctx, db.ListCatalogDisplayBrandsParams{
		IsActive: boolPtr(f.IsActive), MergedOnly: f.MergedOnly, Q: text(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCatalogDisplayBrands(ctx, db.CountCatalogDisplayBrandsParams{
		IsActive: boolPtr(f.IsActive), MergedOnly: f.MergedOnly, Q: text(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.DisplayBrand, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapDisplayBrand(r))
	}
	return out, total, nil
}

// GetDisplayBrand returns a display brand with its TSB brands.
func (s *Service) GetDisplayBrand(ctx context.Context, id uuid.UUID) (model.DisplayBrand, error) {
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, id)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	out := mapDisplayBrand(db.ListCatalogDisplayBrandsRow(d))
	members, err := s.q.ListCatalogBrandsByDisplayBrand(ctx, d.ID)
	if err != nil {
		return model.DisplayBrand{}, err
	}
	out.Brands = make([]model.DisplayBrandMember, 0, len(members))
	for _, m := range members {
		out.Brands = append(out.Brands, model.DisplayBrandMember{
			UUID: m.Uuid, TSBCode: m.TsbCode, TSBName: m.TsbName, Category: m.Category,
			IsActive: m.IsActive, TypesCount: m.TypesCount,
		})
	}
	return out, nil
}

// PatchDisplayBrand renames (new slug, member display names follow) and/or
// (de)activates a display brand.
func (s *Service) PatchDisplayBrand(ctx context.Context, id uuid.UUID, in model.PatchDisplayBrandInput) (model.DisplayBrand, error) {
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, id)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	params := db.UpdateCatalogDisplayBrandParams{ID: d.ID, IsActive: boolPtr(in.IsActive)}
	err = s.repo.WithTx(ctx, func(q *db.Queries) error {
		if in.Name != nil {
			name := strings.Join(strings.Fields(*in.Name), " ")
			if name == "" {
				return fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
			}
			params.Name = text(name)
			if tsbcatalog.Slug(name) != tsbcatalog.Slug(d.Name) {
				slug, err := repository.UniqueDisplaySlug(ctx, q, name, d.ID)
				if err != nil {
					return err
				}
				params.Slug = text(slug)
			}
		}
		if _, err := q.UpdateCatalogDisplayBrand(ctx, params); err != nil {
			return err
		}
		_, err := q.SyncCatalogBrandsDisplayName(ctx, d.ID)
		return err
	})
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	// Display brand changes re-map TSB brand rows too.
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	return s.GetDisplayBrand(ctx, id)
}

// MergeDisplayBrands moves every TSB brand of src under dst and deletes src
// (FIAT + TOFAS-FIAT -> Fiat). Grouped models follow automatically.
func (s *Service) MergeDisplayBrands(ctx context.Context, src, dst uuid.UUID) (model.DisplayBrand, error) {
	if src == dst {
		return model.DisplayBrand{}, fmt.Errorf("%w: cannot merge a display brand into itself", ErrInvalidRequest)
	}
	from, err := s.q.GetCatalogDisplayBrandByUUID(ctx, src)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	to, err := s.q.GetCatalogDisplayBrandByUUID(ctx, dst)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	var orphan *string
	err = s.repo.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.MoveCatalogBrandsDisplayBrand(ctx, db.MoveCatalogBrandsDisplayBrandParams{FromID: from.ID, ToID: to.ID}); err != nil {
			return err
		}
		var err error
		orphan, err = deleteIfEmpty(ctx, q, from.ID)
		return err
	})
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	s.dropLogo(ctx, orphan)
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	return s.GetDisplayBrand(ctx, dst)
}

// UnmergeBrand detaches one TSB brand from a merged display brand into a
// new display brand named name (default: the title-cased TSB name, e.g.
// "Tofas-Fiat").
func (s *Service) UnmergeBrand(ctx context.Context, displayID, brandID uuid.UUID, name string) (model.DisplayBrand, error) {
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, displayID)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	if d.BrandsCount < 2 {
		return model.DisplayBrand{}, fmt.Errorf("%w: display brand has a single TSB brand", ErrInvalidRequest)
	}
	b, err := s.q.GetCatalogBrandByUUID(ctx, brandID)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	if b.DisplayBrandID != d.ID {
		return model.DisplayBrand{}, fmt.Errorf("%w: brand is not part of this display brand", ErrInvalidRequest)
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = tsbcatalog.TitleCase(b.TsbName)
	}
	if _, err := s.q.GetCatalogDisplayBrandByName(ctx, name); err == nil {
		return model.DisplayBrand{}, fmt.Errorf("%w: a display brand named %q already exists", ErrConflict, name)
	} else if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
		return model.DisplayBrand{}, err
	}
	var created db.CatalogDisplayBrand
	err = s.repo.WithTx(ctx, func(q *db.Queries) error {
		var err error
		created, err = repository.EnsureDisplayBrand(ctx, q, name)
		if err != nil {
			return err
		}
		_, err = q.SetCatalogBrandDisplayBrand(ctx, db.SetCatalogBrandDisplayBrandParams{BrandID: b.ID, DisplayBrandID: created.ID})
		return err
	})
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	return s.GetDisplayBrand(ctx, created.Uuid)
}

// SetDisplayBrandLogo stores an uploaded logo (already validated by the
// handler) under catalog/brands/{uuid}/logo.{ext}.
func (s *Service) SetDisplayBrandLogo(ctx context.Context, id uuid.UUID, file storage.File, ext string) (model.DisplayBrand, error) {
	if s.storage == nil {
		return model.DisplayBrand{}, fmt.Errorf("%w: object storage is not configured", ErrInvalidRequest)
	}
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, id)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	key := storage.CatalogBrandLogoObjectKey(d.Uuid.String(), ext)
	if err := s.storage.Upload(ctx, file, key); err != nil {
		return model.DisplayBrand{}, fmt.Errorf("catalog: upload logo: %w", err)
	}
	if _, err := s.q.SetCatalogDisplayBrandLogo(ctx, db.SetCatalogDisplayBrandLogoParams{
		ID: d.ID, LogoObjectKey: pgtype.Text{String: key, Valid: true},
	}); err != nil {
		return model.DisplayBrand{}, err
	}
	if d.LogoObjectKey.Valid && d.LogoObjectKey.String != key {
		old := d.LogoObjectKey.String
		s.dropLogo(ctx, &old)
	}
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	return s.GetDisplayBrand(ctx, id)
}

// ClearDisplayBrandLogo removes the logo.
func (s *Service) ClearDisplayBrandLogo(ctx context.Context, id uuid.UUID) (model.DisplayBrand, error) {
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, id)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	if _, err := s.q.SetCatalogDisplayBrandLogo(ctx, db.SetCatalogDisplayBrandLogoParams{ID: d.ID}); err != nil {
		return model.DisplayBrand{}, err
	}
	if d.LogoObjectKey.Valid {
		old := d.LogoObjectKey.String
		s.dropLogo(ctx, &old)
	}
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	return s.GetDisplayBrand(ctx, id)
}

// OpenDisplayBrandLogo streams a display brand logo.
func (s *Service) OpenDisplayBrandLogo(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, error) {
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, id)
	if err != nil {
		return nil, "", mapErr(repository.MapErr(err))
	}
	if !d.LogoObjectKey.Valid || s.storage == nil {
		return nil, "", ErrNotFound
	}
	rc, _, err := s.storage.Download(ctx, d.LogoObjectKey.String)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, storage.MIMEFromLogoKey(d.LogoObjectKey.String), nil
}
