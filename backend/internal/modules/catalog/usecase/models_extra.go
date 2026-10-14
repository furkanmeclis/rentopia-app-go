package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxBulkModels caps one bulk rule request.
const MaxBulkModels = 500

// sameNameModel returns the model named name of a TSB brand, creating a
// manual one when missing (used to assign a sibling brand's model).
func (s *Service) sameNameModel(ctx context.Context, brandID int64, name string) (db.CatalogModel, error) {
	m, err := s.q.GetCatalogModelByBrandName(ctx, db.GetCatalogModelByBrandNameParams{BrandID: brandID, Name: name})
	if err == nil {
		return m, nil
	}
	if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
		return m, err
	}
	m, err = s.q.CreateCatalogModel(ctx, db.CreateCatalogModelParams{BrandID: brandID, Name: name, Source: model.SourceManual})
	if err != nil {
		return m, mapErr(repository.MapErr(err))
	}
	return m, nil
}

// BulkModelRules saves "type name starts with <model name>" as a prefix rule
// for each model and applies it to the brand's types without a model or
// with an auto-derived one, so later imports follow it too.
func (s *Service) BulkModelRules(ctx context.Context, ids []uuid.UUID) (model.BulkRulesResult, error) {
	var res model.BulkRulesResult
	if len(ids) == 0 || len(ids) > MaxBulkModels {
		return res, fmt.Errorf("%w: send 1-%d model uuids", ErrInvalidRequest, MaxBulkModels)
	}
	models, err := s.q.ListCatalogModelsByUUIDs(ctx, ids)
	if err != nil {
		return res, err
	}
	if len(models) != len(uniqueUUIDs(ids)) {
		return res, ErrNotFound
	}
	err = s.repo.WithTx(ctx, func(q *db.Queries) error {
		candidates := map[int64][]db.ListCatalogTypesForRuleRow{}
		for _, m := range models {
			if _, err := q.UpsertCatalogModelRule(ctx, db.UpsertCatalogModelRuleParams{
				BrandID: m.BrandID, Prefix: m.Name, ModelID: m.ID,
			}); err != nil {
				return err
			}
			res.RulesSaved++
			cands, ok := candidates[m.BrandID]
			if !ok {
				var err error
				if cands, err = q.ListCatalogTypesForRule(ctx, m.BrandID); err != nil {
					return err
				}
				candidates[m.BrandID] = cands
			}
			typeIDs := make([]int64, 0)
			for _, c := range cands {
				if tsbcatalog.HasPrefixToken(c.RawName, m.Name) {
					typeIDs = append(typeIDs, c.ID)
				}
			}
			if len(typeIDs) == 0 {
				continue
			}
			n, err := q.SetCatalogTypesModel(ctx, db.SetCatalogTypesModelParams{
				ModelID: pgtype.Int8{Int64: m.ID, Valid: true}, Ids: typeIDs,
			})
			if err != nil {
				return err
			}
			res.TypesAssigned += int(n)
		}
		return nil
	})
	if err != nil {
		return model.BulkRulesResult{}, mapErr(repository.MapErr(err))
	}
	s.publish(ctx, LiveModels, resourceevents.ActionBulk)
	s.publish(ctx, LiveTypes, resourceevents.ActionBulk)
	return res, nil
}

// RebuildAutoModels re-derives auto models with the current heuristics.
func (s *Service) RebuildAutoModels(ctx context.Context) (model.RebuildResult, error) {
	return s.repo.RebuildAutoModels(ctx)
}

func uniqueUUIDs(ids []uuid.UUID) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}
