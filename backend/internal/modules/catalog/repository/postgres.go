// Package repository is the vehicle catalog persistence layer: sqlc queries
// for CRUD and lists, pgx COPY for the monthly bulk import.
package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// importLockKey serializes catalog applies (pg_advisory_xact_lock).
const importLockKey int64 = 0x7a5b_ca7a_1061

// Postgres implements the catalog repository.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// Queries exposes the sqlc queries for simple reads and writes.
func (r *Postgres) Queries() *db.Queries { return r.q }

// WithTx runs fn in a transaction.
func (r *Postgres) WithTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MapErr converts driver errors into repository errors.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
	}
	return err
}

// NumericToFloat converts a NUMERIC to float64 (0 when NULL).
func NumericToFloat(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0, false
	}
	return f.Float64, true
}

// LoadState reads the stored catalog needed to plan an import of period.
func (r *Postgres) LoadState(ctx context.Context, period string) (model.State, error) {
	st := model.State{
		Brands:     map[string]model.StateBrand{},
		Models:     map[int64][]tsbcatalog.ModelRef{},
		Rules:      map[int64][]tsbcatalog.RuleRef{},
		Types:      map[model.TypeKey]model.StateType{},
		Valuations: map[model.ValuationKey]int64{},
	}
	brands, err := r.q.ListCatalogBrandsAll(ctx)
	if err != nil {
		return st, err
	}
	codeByID := make(map[int64]string, len(brands))
	for _, b := range brands {
		st.Brands[b.TsbCode] = model.StateBrand{ID: b.ID, Name: b.TsbName, Category: b.Category}
		codeByID[b.ID] = b.TsbCode
	}
	models, err := r.q.ListCatalogModelsAll(ctx)
	if err != nil {
		return st, err
	}
	for _, m := range models {
		st.Models[m.BrandID] = append(st.Models[m.BrandID], tsbcatalog.ModelRef{ID: m.ID, Name: m.Name})
	}
	rules, err := r.q.ListCatalogModelRulesAll(ctx)
	if err != nil {
		return st, err
	}
	for _, rl := range rules {
		st.Rules[rl.BrandID] = append(st.Rules[rl.BrandID], tsbcatalog.RuleRef{Prefix: rl.Prefix, ModelID: rl.ModelID})
	}
	types, err := r.q.ListCatalogTypesState(ctx)
	if err != nil {
		return st, err
	}
	for _, t := range types {
		code := codeByID[t.BrandID]
		st.Types[model.TypeKey{BrandCode: code, TypeCode: t.TsbTypeCode}] = model.StateType{
			ID: t.ID, BrandCode: code, ModelID: t.ModelID.Int64, RawName: t.RawName,
			Years: t.Years, IsActive: t.IsActive, LastSeen: t.LastSeenPeriod,
		}
	}
	vals, err := r.q.ListCatalogValuationsByPeriod(ctx, period)
	if err != nil {
		return st, err
	}
	for _, v := range vals {
		f, _ := NumericToFloat(v.Value)
		st.Valuations[model.ValuationKey{TypeID: v.TypeID, Year: v.ModelYear}] = int64(math.Round(f))
	}
	return st, nil
}

// Apply writes a plan in one transaction. Every statement only touches rows
// that actually differ, so applying the same plan twice changes nothing.
func (r *Postgres) Apply(ctx context.Context, plan model.Plan) (model.ApplyResult, error) {
	var res model.ApplyResult
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("catalog apply: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, importLockKey); err != nil {
		return res, fmt.Errorf("catalog apply: lock: %w", err)
	}

	// 1. Brands: insert new, follow TSB renames. Admin fields stay.
	codes := make([]string, 0, len(plan.Brands))
	names := make([]string, 0, len(plan.Brands))
	displays := make([]string, 0, len(plan.Brands))
	categories := make([]string, 0, len(plan.Brands))
	for _, b := range plan.Brands {
		display := b.DisplayName
		if display == "" {
			display = tsbcatalog.DefaultsForBrand(b.Name).DisplayName
		}
		category := b.Category
		if category == "" {
			category = tsbcatalog.CategoryPassenger
		}
		codes = append(codes, b.Code)
		names = append(names, b.Name)
		displays = append(displays, display)
		categories = append(categories, category)
	}
	brows, err := tx.Query(ctx, `
		INSERT INTO catalog_brands AS b (tsb_code, tsb_name, display_name, category)
		SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[])
		ON CONFLICT (tsb_code) DO UPDATE SET tsb_name = EXCLUDED.tsb_name
		WHERE b.tsb_name IS DISTINCT FROM EXCLUDED.tsb_name
		RETURNING (xmax = 0)`, codes, names, displays, categories)
	if err != nil {
		return res, fmt.Errorf("catalog apply: brands: %w", err)
	}
	for brows.Next() {
		var inserted bool
		if err := brows.Scan(&inserted); err != nil {
			brows.Close()
			return res, err
		}
		if inserted {
			res.BrandsInserted++
		} else {
			res.BrandsUpdated++
		}
	}
	brows.Close()
	if err := brows.Err(); err != nil {
		return res, fmt.Errorf("catalog apply: brands: %w", err)
	}
	brandIDs := map[string]int64{}
	rows, err := tx.Query(ctx, `SELECT tsb_code, id FROM catalog_brands`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var code string
		var id int64
		if err := rows.Scan(&code, &id); err != nil {
			rows.Close()
			return res, err
		}
		brandIDs[code] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	// 2. Auto models derived by the plan.
	if len(plan.NewModels) > 0 {
		brandCol := make([]int64, 0, len(plan.NewModels))
		nameCol := make([]string, 0, len(plan.NewModels))
		for _, m := range plan.NewModels {
			brandCol = append(brandCol, brandIDs[m.BrandCode])
			nameCol = append(nameCol, m.Name)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO catalog_models (brand_id, name, source)
			SELECT brand_id, name, 'auto' FROM unnest($1::bigint[], $2::text[]) AS x(brand_id, name)
			ON CONFLICT (brand_id, lower(name)) DO NOTHING`, brandCol, nameCol)
		if err != nil {
			return res, fmt.Errorf("catalog apply: models: %w", err)
		}
		res.ModelsInserted = int(tag.RowsAffected())
	}
	modelIDs := map[[2]string]int64{}
	if len(plan.NewModels) > 0 {
		rows, err := tx.Query(ctx, `SELECT b.tsb_code, lower(m.name), m.id FROM catalog_models m JOIN catalog_brands b ON b.id = m.brand_id`)
		if err != nil {
			return res, err
		}
		for rows.Next() {
			var code, name string
			var id int64
			if err := rows.Scan(&code, &name, &id); err != nil {
				rows.Close()
				return res, err
			}
			modelIDs[[2]string{code, name}] = id
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
	}

	// 3. Types via COPY into a temp table, then one upsert.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_catalog_types (
			brand_id BIGINT NOT NULL,
			tsb_type_code TEXT NOT NULL,
			raw_name TEXT NOT NULL,
			years INTEGER[] NOT NULL,
			suggested_transmission TEXT NULL,
			suggested_fuel TEXT NULL,
			model_id BIGINT NULL
		) ON COMMIT DROP`); err != nil {
		return res, err
	}
	typeRows := make([][]any, 0, len(plan.Types))
	for _, t := range plan.Types {
		var modelID any
		switch {
		case t.ModelID > 0:
			modelID = t.ModelID
		case t.ModelName != "":
			if id, ok := modelIDs[[2]string{t.BrandCode, strings.ToLower(t.ModelName)}]; ok {
				modelID = id
			}
		}
		typeRows = append(typeRows, []any{
			brandIDs[t.BrandCode], t.Code, t.RawName, t.Years,
			nullable(t.Transmission), nullable(t.Fuel), modelID,
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_catalog_types"},
		[]string{"brand_id", "tsb_type_code", "raw_name", "years", "suggested_transmission", "suggested_fuel", "model_id"},
		pgx.CopyFromRows(typeRows)); err != nil {
		return res, fmt.Errorf("catalog apply: copy types: %w", err)
	}
	trows, err := tx.Query(ctx, `
		INSERT INTO catalog_types AS t (
			brand_id, tsb_type_code, raw_name, years, suggested_transmission, suggested_fuel,
			model_id, first_seen_period, last_seen_period, is_active
		)
		SELECT brand_id, tsb_type_code, raw_name, years, suggested_transmission, suggested_fuel,
			model_id, $1, $1, TRUE
		FROM tmp_catalog_types
		ON CONFLICT (brand_id, tsb_type_code) DO UPDATE SET
			raw_name = EXCLUDED.raw_name,
			years = EXCLUDED.years,
			suggested_transmission = COALESCE(t.suggested_transmission, EXCLUDED.suggested_transmission),
			suggested_fuel = COALESCE(t.suggested_fuel, EXCLUDED.suggested_fuel),
			model_id = COALESCE(t.model_id, EXCLUDED.model_id),
			last_seen_period = EXCLUDED.last_seen_period,
			is_active = TRUE
		WHERE EXCLUDED.last_seen_period >= t.last_seen_period AND (
			t.raw_name IS DISTINCT FROM EXCLUDED.raw_name
			OR t.years IS DISTINCT FROM EXCLUDED.years
			OR (t.suggested_transmission IS NULL AND EXCLUDED.suggested_transmission IS NOT NULL)
			OR (t.suggested_fuel IS NULL AND EXCLUDED.suggested_fuel IS NOT NULL)
			OR (t.model_id IS NULL AND EXCLUDED.model_id IS NOT NULL)
			OR t.last_seen_period <> EXCLUDED.last_seen_period
			OR NOT t.is_active
		)
		RETURNING (xmax = 0)`, plan.Period)
	if err != nil {
		return res, fmt.Errorf("catalog apply: upsert types: %w", err)
	}
	for trows.Next() {
		var inserted bool
		if err := trows.Scan(&inserted); err != nil {
			trows.Close()
			return res, err
		}
		if inserted {
			res.TypesInserted++
		} else {
			res.TypesUpdated++
		}
	}
	trows.Close()
	if err := trows.Err(); err != nil {
		return res, fmt.Errorf("catalog apply: upsert types: %w", err)
	}

	// 4. Deactivate active types missing from the file.
	tag, err := tx.Exec(ctx, `
		UPDATE catalog_types t SET is_active = FALSE
		WHERE t.is_active AND t.last_seen_period < $1
		  AND NOT EXISTS (
			SELECT 1 FROM tmp_catalog_types x
			WHERE x.brand_id = t.brand_id AND x.tsb_type_code = t.tsb_type_code
		  )`, plan.Period)
	if err != nil {
		return res, fmt.Errorf("catalog apply: deactivate: %w", err)
	}
	res.TypesDeactivated = int(tag.RowsAffected())

	// 5. Valuations of the period (history: other periods are never touched).
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_catalog_valuations (
			brand_id BIGINT NOT NULL,
			tsb_type_code TEXT NOT NULL,
			model_year INTEGER NOT NULL,
			value NUMERIC(14, 2) NOT NULL
		) ON COMMIT DROP`); err != nil {
		return res, err
	}
	valRows := make([][]any, 0, len(plan.Valuations))
	for _, v := range plan.Valuations {
		var n pgtype.Numeric
		if err := n.Scan(fmt.Sprintf("%d", v.Value)); err != nil {
			return res, err
		}
		valRows = append(valRows, []any{brandIDs[v.BrandCode], v.TypeCode, v.Year, n})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_catalog_valuations"},
		[]string{"brand_id", "tsb_type_code", "model_year", "value"},
		pgx.CopyFromRows(valRows)); err != nil {
		return res, fmt.Errorf("catalog apply: copy valuations: %w", err)
	}
	tag, err = tx.Exec(ctx, `
		INSERT INTO catalog_valuations AS cv (type_id, model_year, period, value)
		SELECT t.id, v.model_year, $1, v.value
		FROM tmp_catalog_valuations v
		JOIN catalog_types t ON t.brand_id = v.brand_id AND t.tsb_type_code = v.tsb_type_code
		ON CONFLICT (type_id, model_year, period) DO UPDATE SET value = EXCLUDED.value
		WHERE cv.value IS DISTINCT FROM EXCLUDED.value`, plan.Period)
	if err != nil {
		return res, fmt.Errorf("catalog apply: valuations: %w", err)
	}
	res.ValuationsUpserted = int(tag.RowsAffected())

	unresolved, err := r.q.WithTx(tx).CountUnresolvedCatalogTypesByCategory(ctx)
	if err != nil {
		return res, err
	}
	res.UnresolvedByCategory = map[string]int{}
	for _, u := range unresolved {
		res.UnresolvedByCategory[u.Category] = int(u.Unresolved)
	}
	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("catalog apply: commit: %w", err)
	}
	return res, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
