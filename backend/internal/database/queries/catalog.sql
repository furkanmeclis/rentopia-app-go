-- Platform vehicle catalog (TSB kasko list).

-- name: ListCatalogBrands :many
SELECT b.*,
    (SELECT COUNT(*) FROM catalog_types t WHERE t.brand_id = b.id AND t.is_active)::bigint AS types_count,
    (SELECT COUNT(*) FROM catalog_models m WHERE m.brand_id = b.id)::bigint AS models_count,
    (SELECT COUNT(*) FROM catalog_types t WHERE t.brand_id = b.id AND t.is_active AND t.model_id IS NULL)::bigint AS unresolved_count
FROM catalog_brands b
WHERE (sqlc.narg(category)::text IS NULL OR b.category = sqlc.narg(category))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (
    sqlc.narg(q)::text IS NULL
    OR b.display_name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.tsb_name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.tsb_code = sqlc.narg(q)
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND NOT sqlc.arg(sort_desc)::boolean THEN b.display_name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND sqlc.arg(sort_desc)::boolean THEN b.display_name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'tsb_name' AND NOT sqlc.arg(sort_desc)::boolean THEN b.tsb_name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'tsb_name' AND sqlc.arg(sort_desc)::boolean THEN b.tsb_name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'tsb_code' AND NOT sqlc.arg(sort_desc)::boolean THEN lpad(b.tsb_code, 12, '0') END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'tsb_code' AND sqlc.arg(sort_desc)::boolean THEN lpad(b.tsb_code, 12, '0') END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'category' AND NOT sqlc.arg(sort_desc)::boolean THEN b.category END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'category' AND sqlc.arg(sort_desc)::boolean THEN b.category END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'types_count' AND NOT sqlc.arg(sort_desc)::boolean THEN (SELECT COUNT(*) FROM catalog_types t WHERE t.brand_id = b.id AND t.is_active) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'types_count' AND sqlc.arg(sort_desc)::boolean THEN (SELECT COUNT(*) FROM catalog_types t WHERE t.brand_id = b.id AND t.is_active) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN b.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN b.created_at END DESC,
  b.display_name ASC, b.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCatalogBrands :one
SELECT COUNT(*)::bigint
FROM catalog_brands b
WHERE (sqlc.narg(category)::text IS NULL OR b.category = sqlc.narg(category))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (
    sqlc.narg(q)::text IS NULL
    OR b.display_name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.tsb_name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.tsb_code = sqlc.narg(q)
  );

-- name: GetCatalogBrandByUUID :one
SELECT b.*,
    (SELECT COUNT(*) FROM catalog_types t WHERE t.brand_id = b.id AND t.is_active)::bigint AS types_count,
    (SELECT COUNT(*) FROM catalog_models m WHERE m.brand_id = b.id)::bigint AS models_count,
    (SELECT COUNT(*) FROM catalog_types t WHERE t.brand_id = b.id AND t.is_active AND t.model_id IS NULL)::bigint AS unresolved_count
FROM catalog_brands b
WHERE b.uuid = $1;

-- name: UpdateCatalogBrand :one
UPDATE catalog_brands
SET display_name = COALESCE(sqlc.narg(display_name), display_name),
    category = COALESCE(sqlc.narg(category), category),
    is_active = COALESCE(sqlc.narg(is_active), is_active)
WHERE uuid = sqlc.arg(uuid)
RETURNING *;

-- name: ListCatalogBrandsAll :many
SELECT * FROM catalog_brands ORDER BY id;

-- name: ListCatalogModels :many
SELECT m.*, b.uuid AS brand_uuid, b.display_name AS brand_display_name,
    (SELECT COUNT(*) FROM catalog_types t WHERE t.model_id = m.id AND t.is_active)::bigint AS types_count
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR m.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(source)::text IS NULL OR m.source = sqlc.narg(source))
  AND (sqlc.narg(q)::text IS NULL OR m.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN m.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN m.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'brand' AND NOT sqlc.arg(sort_desc)::boolean THEN b.display_name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'brand' AND sqlc.arg(sort_desc)::boolean THEN b.display_name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'source' AND NOT sqlc.arg(sort_desc)::boolean THEN m.source END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'source' AND sqlc.arg(sort_desc)::boolean THEN m.source END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'types_count' AND NOT sqlc.arg(sort_desc)::boolean THEN (SELECT COUNT(*) FROM catalog_types t WHERE t.model_id = m.id AND t.is_active) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'types_count' AND sqlc.arg(sort_desc)::boolean THEN (SELECT COUNT(*) FROM catalog_types t WHERE t.model_id = m.id AND t.is_active) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN m.created_at END DESC,
  b.display_name ASC, m.name ASC, m.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCatalogModels :one
SELECT COUNT(*)::bigint
FROM catalog_models m
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR m.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(source)::text IS NULL OR m.source = sqlc.narg(source))
  AND (sqlc.narg(q)::text IS NULL OR m.name ILIKE '%' || sqlc.narg(q) || '%');

-- name: GetCatalogModelByUUID :one
SELECT m.*, b.uuid AS brand_uuid, b.display_name AS brand_display_name,
    (SELECT COUNT(*) FROM catalog_types t WHERE t.model_id = m.id AND t.is_active)::bigint AS types_count
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE m.uuid = $1;

-- name: CreateCatalogModel :one
INSERT INTO catalog_models (brand_id, name, source)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateCatalogModel :one
UPDATE catalog_models
SET name = COALESCE(sqlc.narg(name), name),
    source = COALESCE(sqlc.narg(source), source)
WHERE uuid = sqlc.arg(uuid)
RETURNING *;

-- name: ListCatalogModelsAll :many
SELECT * FROM catalog_models ORDER BY id;

-- name: ReassignCatalogTypesModel :execrows
UPDATE catalog_types SET model_id = sqlc.arg(to_model_id)
WHERE model_id = sqlc.arg(from_model_id);

-- name: ReassignCatalogModelRules :execrows
UPDATE catalog_model_rules SET model_id = sqlc.arg(to_model_id)
WHERE model_id = sqlc.arg(from_model_id);

-- name: DeleteCatalogModel :execrows
DELETE FROM catalog_models WHERE id = $1;

-- name: ListCatalogModelRulesByModel :many
SELECT * FROM catalog_model_rules WHERE model_id = $1 ORDER BY prefix;

-- name: ListCatalogModelRulesAll :many
SELECT * FROM catalog_model_rules ORDER BY brand_id, prefix;

-- name: UpsertCatalogModelRule :one
INSERT INTO catalog_model_rules (brand_id, prefix, model_id)
VALUES ($1, $2, $3)
ON CONFLICT (brand_id, prefix) DO UPDATE SET model_id = EXCLUDED.model_id
RETURNING *;

-- name: ListCatalogTypes :many
SELECT t.id, t.uuid, t.tsb_type_code, t.raw_name, t.years, t.suggested_transmission,
    t.suggested_fuel, t.first_seen_period, t.last_seen_period, t.is_active, t.created_at, t.updated_at,
    b.uuid AS brand_uuid, b.display_name AS brand_display_name, b.tsb_code AS brand_tsb_code,
    m.uuid AS model_uuid, m.name AS model_name, m.source AS model_source
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
LEFT JOIN catalog_models m ON m.id = t.model_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR t.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(category)::text IS NULL OR b.category = sqlc.narg(category))
  AND (sqlc.narg(model_id)::bigint IS NULL OR t.model_id = sqlc.narg(model_id))
  AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  AND (NOT sqlc.arg(unresolved)::boolean OR t.model_id IS NULL)
  AND (sqlc.narg(is_active)::boolean IS NULL OR t.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(fuel)::text IS NULL OR t.suggested_fuel = sqlc.narg(fuel))
  AND (sqlc.narg(transmission)::text IS NULL OR t.suggested_transmission = sqlc.narg(transmission))
  AND (
    sqlc.narg(q)::text IS NULL
    OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%'
    OR t.tsb_type_code = sqlc.narg(q)
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'raw_name' AND NOT sqlc.arg(sort_desc)::boolean THEN t.raw_name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'raw_name' AND sqlc.arg(sort_desc)::boolean THEN t.raw_name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'tsb_type_code' AND NOT sqlc.arg(sort_desc)::boolean THEN lpad(t.tsb_type_code, 12, '0') END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'tsb_type_code' AND sqlc.arg(sort_desc)::boolean THEN lpad(t.tsb_type_code, 12, '0') END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'brand' AND NOT sqlc.arg(sort_desc)::boolean THEN b.display_name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'brand' AND sqlc.arg(sort_desc)::boolean THEN b.display_name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'model' AND NOT sqlc.arg(sort_desc)::boolean THEN m.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'model' AND sqlc.arg(sort_desc)::boolean THEN m.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_seen_period' AND NOT sqlc.arg(sort_desc)::boolean THEN t.last_seen_period END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_seen_period' AND sqlc.arg(sort_desc)::boolean THEN t.last_seen_period END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
  b.display_name ASC, t.raw_name ASC, t.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCatalogTypes :one
SELECT COUNT(*)::bigint
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR t.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(category)::text IS NULL OR b.category = sqlc.narg(category))
  AND (sqlc.narg(model_id)::bigint IS NULL OR t.model_id = sqlc.narg(model_id))
  AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  AND (NOT sqlc.arg(unresolved)::boolean OR t.model_id IS NULL)
  AND (sqlc.narg(is_active)::boolean IS NULL OR t.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(fuel)::text IS NULL OR t.suggested_fuel = sqlc.narg(fuel))
  AND (sqlc.narg(transmission)::text IS NULL OR t.suggested_transmission = sqlc.narg(transmission))
  AND (
    sqlc.narg(q)::text IS NULL
    OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%'
    OR t.tsb_type_code = sqlc.narg(q)
  );

-- name: GetCatalogTypeByUUID :one
SELECT t.id, t.uuid, t.brand_id, t.model_id, t.tsb_type_code, t.raw_name, t.years, t.suggested_transmission,
    t.suggested_fuel, t.first_seen_period, t.last_seen_period, t.is_active, t.created_at, t.updated_at,
    b.uuid AS brand_uuid, b.display_name AS brand_display_name, b.tsb_code AS brand_tsb_code,
    b.tsb_name AS brand_tsb_name,
    m.uuid AS model_uuid, m.name AS model_name, m.source AS model_source
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
LEFT JOIN catalog_models m ON m.id = t.model_id
WHERE t.uuid = $1;

-- name: UpdateCatalogType :one
UPDATE catalog_types
SET model_id = CASE WHEN sqlc.arg(set_model)::boolean THEN sqlc.narg(model_id)::bigint ELSE model_id END,
    suggested_transmission = CASE WHEN sqlc.arg(set_transmission)::boolean THEN sqlc.narg(suggested_transmission)::text ELSE suggested_transmission END,
    suggested_fuel = CASE WHEN sqlc.arg(set_fuel)::boolean THEN sqlc.narg(suggested_fuel)::text ELSE suggested_fuel END
WHERE uuid = sqlc.arg(uuid)
RETURNING id;

-- name: ListCatalogTypesForRule :many
-- Types of a brand that a new prefix rule may (re)assign: no model yet, or
-- an auto-derived one.
SELECT t.id, t.raw_name
FROM catalog_types t
LEFT JOIN catalog_models m ON m.id = t.model_id
WHERE t.brand_id = $1 AND (t.model_id IS NULL OR m.source = 'auto');

-- name: SetCatalogTypesModel :execrows
UPDATE catalog_types SET model_id = sqlc.arg(model_id)
WHERE id = ANY(sqlc.arg(ids)::bigint[]) AND model_id IS DISTINCT FROM sqlc.arg(model_id);

-- name: ListCatalogTypesState :many
SELECT id, brand_id, model_id, tsb_type_code, raw_name, years, suggested_transmission,
    suggested_fuel, last_seen_period, is_active
FROM catalog_types;

-- name: ListCatalogValuationsByPeriod :many
SELECT type_id, model_year, value FROM catalog_valuations WHERE period = $1;

-- name: ListCatalogTypeValuations :many
SELECT model_year, period, value FROM catalog_valuations
WHERE type_id = $1
ORDER BY period DESC, model_year DESC;

-- name: CountUnresolvedCatalogTypesByCategory :many
SELECT b.category, COUNT(*)::bigint AS unresolved
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
WHERE t.is_active AND t.model_id IS NULL
GROUP BY b.category;

-- name: CreateCatalogImport :one
INSERT INTO catalog_imports (filename, created_by, period)
VALUES ($1, $2, sqlc.narg(period))
RETURNING *;

-- name: SetCatalogImportFileKey :one
UPDATE catalog_imports SET file_key = $2 WHERE id = $1 RETURNING *;

-- name: GetCatalogImportByUUID :one
SELECT * FROM catalog_imports WHERE uuid = $1;

-- name: GetCatalogImportByID :one
SELECT * FROM catalog_imports WHERE id = $1;

-- name: UpdateCatalogImportState :one
UPDATE catalog_imports
SET status = sqlc.arg(status),
    period = COALESCE(sqlc.narg(period), period),
    counts = COALESCE(sqlc.narg(counts), counts),
    diff = COALESCE(sqlc.narg(diff), diff),
    error = sqlc.narg(error),
    applied_at = CASE WHEN sqlc.arg(status)::text = 'applied' THEN NOW() ELSE applied_at END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ClaimCatalogImport :one
-- Moves an import into a working state only from an allowed state, so two
-- apply requests cannot run at once.
UPDATE catalog_imports
SET status = sqlc.arg(to_status), error = NULL
WHERE id = sqlc.arg(id) AND status = ANY(sqlc.arg(from_statuses)::text[])
RETURNING *;

-- name: ListCatalogImports :many
SELECT i.*, u.email AS created_by_email
FROM catalog_imports i
LEFT JOIN users u ON u.id = i.created_by
WHERE (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR i.filename ILIKE '%' || sqlc.narg(q) || '%' OR i.period = sqlc.narg(q))
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'period' AND NOT sqlc.arg(sort_desc)::boolean THEN i.period END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'period' AND sqlc.arg(sort_desc)::boolean THEN i.period END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN i.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN i.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN i.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN i.created_at END DESC,
  i.created_at DESC, i.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCatalogImports :one
SELECT COUNT(*)::bigint
FROM catalog_imports i
WHERE (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL OR i.filename ILIKE '%' || sqlc.narg(q) || '%' OR i.period = sqlc.narg(q));

-- name: GetCatalogLatestPeriod :one
SELECT COALESCE(MAX(period), '')::text FROM catalog_valuations;

-- Tenant read-only search (vehicle forms).

-- name: TenantListCatalogBrands :many
SELECT b.uuid, b.display_name, b.category
FROM catalog_brands b
WHERE b.is_active
  AND (sqlc.narg(category)::text IS NULL OR b.category = sqlc.narg(category))
  AND (sqlc.narg(q)::text IS NULL OR b.display_name ILIKE '%' || sqlc.narg(q) || '%')
  AND EXISTS (
    SELECT 1 FROM catalog_types t
    WHERE t.brand_id = b.id AND t.is_active
      AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  )
ORDER BY b.display_name, b.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: TenantCountCatalogBrands :one
SELECT COUNT(*)::bigint
FROM catalog_brands b
WHERE b.is_active
  AND (sqlc.narg(category)::text IS NULL OR b.category = sqlc.narg(category))
  AND (sqlc.narg(q)::text IS NULL OR b.display_name ILIKE '%' || sqlc.narg(q) || '%')
  AND EXISTS (
    SELECT 1 FROM catalog_types t
    WHERE t.brand_id = b.id AND t.is_active
      AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  );

-- name: TenantListCatalogModels :many
SELECT m.uuid, m.name, b.uuid AS brand_uuid, b.display_name AS brand_display_name
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE b.is_active AND b.uuid = sqlc.arg(brand_uuid)
  AND (sqlc.narg(q)::text IS NULL OR m.name ILIKE '%' || sqlc.narg(q) || '%')
  AND EXISTS (
    SELECT 1 FROM catalog_types t
    WHERE t.model_id = m.id AND t.is_active
      AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  )
ORDER BY m.name, m.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: TenantCountCatalogModels :one
SELECT COUNT(*)::bigint
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE b.is_active AND b.uuid = sqlc.arg(brand_uuid)
  AND (sqlc.narg(q)::text IS NULL OR m.name ILIKE '%' || sqlc.narg(q) || '%')
  AND EXISTS (
    SELECT 1 FROM catalog_types t
    WHERE t.model_id = m.id AND t.is_active
      AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  );

-- name: TenantListCatalogTypes :many
SELECT t.uuid, t.tsb_type_code, t.raw_name, t.years, t.suggested_transmission, t.suggested_fuel,
    m.uuid AS model_uuid, m.name AS model_name,
    b.uuid AS brand_uuid, b.display_name AS brand_display_name,
    v.value AS kasko_value, COALESCE(v.period, '')::text AS kasko_period
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN LATERAL (
    SELECT cv.value, cv.period FROM catalog_valuations cv
    WHERE cv.type_id = t.id AND cv.model_year = sqlc.narg(year)::int
    ORDER BY cv.period DESC
    LIMIT 1
) v ON TRUE
WHERE t.is_active AND b.is_active AND m.uuid = sqlc.arg(model_uuid)
  AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  AND (sqlc.narg(q)::text IS NULL OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY t.raw_name, t.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: TenantCountCatalogTypes :one
SELECT COUNT(*)::bigint
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
JOIN catalog_models m ON m.id = t.model_id
WHERE t.is_active AND b.is_active AND m.uuid = sqlc.arg(model_uuid)
  AND (sqlc.narg(year)::int IS NULL OR t.years @> ARRAY[sqlc.narg(year)::int])
  AND (sqlc.narg(q)::text IS NULL OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%');
