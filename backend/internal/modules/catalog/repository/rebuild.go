package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
)

type rebuildType struct {
	id      int64
	name    string
	modelID int64
	auto    bool // current model is auto-derived (or none)
}

type rebuildBrand struct {
	name     string
	category string
	types    []rebuildType
	manual   []tsbcatalog.ModelRef
	rules    []tsbcatalog.RuleRef
	byName   map[string]int64 // lower(name) -> id, every model of the brand
}

// RebuildAutoModels re-derives the model of every type that has no model or
// an auto-derived one, with the current heuristics (brand patterns, glued
// names). Manual models and prefix rules are kept and win; auto models left
// without types or rules are deleted. Runs in one transaction under the
// import lock.
func (r *Postgres) RebuildAutoModels(ctx context.Context) (model.RebuildResult, error) {
	var res model.RebuildResult
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("catalog rebuild: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, importLockKey); err != nil {
		return res, fmt.Errorf("catalog rebuild: lock: %w", err)
	}
	counts := func() (int, int, error) {
		var unresolved, auto int
		err := tx.QueryRow(ctx, `
			SELECT (SELECT COUNT(*) FROM catalog_types WHERE is_active AND model_id IS NULL),
			       (SELECT COUNT(*) FROM catalog_models WHERE source = 'auto')`).Scan(&unresolved, &auto)
		return unresolved, auto, err
	}
	if res.UnresolvedBefore, res.AutoModelsBefore, err = counts(); err != nil {
		return res, err
	}

	brands := map[int64]*rebuildBrand{}
	rows, err := tx.Query(ctx, `SELECT id, tsb_name, category FROM catalog_brands`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id int64
		b := &rebuildBrand{byName: map[string]int64{}}
		if err := rows.Scan(&id, &b.name, &b.category); err != nil {
			rows.Close()
			return res, err
		}
		brands[id] = b
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	rows, err = tx.Query(ctx, `SELECT id, brand_id, name, source FROM catalog_models`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id, brandID int64
		var name, source string
		if err := rows.Scan(&id, &brandID, &name, &source); err != nil {
			rows.Close()
			return res, err
		}
		b := brands[brandID]
		if b == nil {
			continue
		}
		b.byName[strings.ToLower(name)] = id
		if source == model.SourceManual {
			b.manual = append(b.manual, tsbcatalog.ModelRef{ID: id, Name: name})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	rows, err = tx.Query(ctx, `SELECT brand_id, prefix, model_id FROM catalog_model_rules`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var brandID, modelID int64
		var prefix string
		if err := rows.Scan(&brandID, &prefix, &modelID); err != nil {
			rows.Close()
			return res, err
		}
		if b := brands[brandID]; b != nil {
			b.rules = append(b.rules, tsbcatalog.RuleRef{Prefix: prefix, ModelID: modelID})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	rows, err = tx.Query(ctx, `
		SELECT t.id, t.brand_id, t.raw_name, COALESCE(t.model_id, 0), (t.model_id IS NULL OR m.source = 'auto')
		FROM catalog_types t
		LEFT JOIN catalog_models m ON m.id = t.model_id
		ORDER BY t.id`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var t rebuildType
		var brandID int64
		if err := rows.Scan(&t.id, &brandID, &t.name, &t.modelID, &t.auto); err != nil {
			rows.Close()
			return res, err
		}
		if b := brands[brandID]; b != nil {
			b.types = append(b.types, t)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	var setIDs, setModels []int64
	var clearIDs []int64
	for brandID, b := range brands {
		if len(b.types) == 0 {
			continue
		}
		names := make([]string, 0, len(b.types))
		for _, t := range b.types {
			names = append(names, t.name)
		}
		resolver := tsbcatalog.NewResolver(b.rules, b.manual, b.category != tsbcatalog.CategoryOther).WithBrand(b.name)
		resolver.Prime(names)
		for _, t := range b.types {
			if !t.auto {
				continue
			}
			m := resolver.Resolve(t.name)
			var target int64
			switch m.Source {
			case "rule", "model":
				target = m.ModelID
			case "derived":
				key := strings.ToLower(m.Name)
				id, ok := b.byName[key]
				if !ok {
					if err := tx.QueryRow(ctx, `
						INSERT INTO catalog_models (brand_id, name, source) VALUES ($1, $2, 'auto')
						RETURNING id`, brandID, m.Name).Scan(&id); err != nil {
						return res, fmt.Errorf("catalog rebuild: create model %q: %w", m.Name, err)
					}
					b.byName[key] = id
					res.ModelsCreated++
				}
				target = id
			}
			if target == t.modelID {
				continue
			}
			if target == 0 {
				clearIDs = append(clearIDs, t.id)
			} else {
				setIDs = append(setIDs, t.id)
				setModels = append(setModels, target)
			}
		}
	}
	if len(setIDs) > 0 {
		tag, err := tx.Exec(ctx, `
			UPDATE catalog_types t SET model_id = x.model_id
			FROM unnest($1::bigint[], $2::bigint[]) AS x(id, model_id)
			WHERE t.id = x.id`, setIDs, setModels)
		if err != nil {
			return res, fmt.Errorf("catalog rebuild: assign: %w", err)
		}
		res.TypesChanged += int(tag.RowsAffected())
	}
	if len(clearIDs) > 0 {
		tag, err := tx.Exec(ctx, `UPDATE catalog_types SET model_id = NULL WHERE id = ANY($1::bigint[])`, clearIDs)
		if err != nil {
			return res, fmt.Errorf("catalog rebuild: clear: %w", err)
		}
		res.TypesChanged += int(tag.RowsAffected())
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM catalog_models m
		WHERE m.source = 'auto'
		  AND NOT EXISTS (SELECT 1 FROM catalog_types t WHERE t.model_id = m.id)
		  AND NOT EXISTS (SELECT 1 FROM catalog_model_rules r WHERE r.model_id = m.id)`)
	if err != nil {
		return res, fmt.Errorf("catalog rebuild: prune: %w", err)
	}
	res.ModelsDeleted = int(tag.RowsAffected())
	if res.UnresolvedAfter, res.AutoModelsAfter, err = counts(); err != nil {
		return res, err
	}
	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("catalog rebuild: commit: %w", err)
	}
	return res, nil
}
