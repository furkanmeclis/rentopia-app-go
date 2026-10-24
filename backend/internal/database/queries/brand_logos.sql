-- Brand logo sync (motomarks-mirror): settings singleton, mirror brand
-- sources, sync runs and the display brand <-> mirror links.

-- name: GetBrandLogoSyncSettings :one
SELECT * FROM brand_logo_sync_settings WHERE id = 1;

-- name: UpdateBrandLogoSyncSettings :one
UPDATE brand_logo_sync_settings
SET base_url = COALESCE(sqlc.narg(base_url), base_url),
    api_token_enc = CASE
        WHEN sqlc.arg(clear_token)::boolean THEN NULL
        ELSE COALESCE(sqlc.narg(api_token_enc), api_token_enc)
    END,
    enabled = COALESCE(sqlc.narg(enabled), enabled),
    auto_match_min_score = COALESCE(sqlc.narg(auto_match_min_score), auto_match_min_score)
WHERE id = 1
RETURNING *;

-- name: SetBrandLogoSyncState :exec
UPDATE brand_logo_sync_settings
SET last_run_at = NOW(),
    last_etag = COALESCE(sqlc.narg(last_etag), last_etag),
    last_status = sqlc.arg(last_status),
    last_error = sqlc.narg(last_error)
WHERE id = 1;

-- name: GetBrandLogoSourceByMotomarksID :one
SELECT * FROM brand_logo_sources WHERE motomarks_id = $1;

-- name: UpsertBrandLogoSource :one
INSERT INTO brand_logo_sources (motomarks_id, name, color, colors, vehicle_types, logos, source_hash)
VALUES (sqlc.arg(motomarks_id), sqlc.arg(name), sqlc.arg(color), sqlc.arg(colors), sqlc.arg(vehicle_types), sqlc.arg(logos), sqlc.arg(source_hash))
ON CONFLICT (motomarks_id) DO UPDATE SET
    name = EXCLUDED.name,
    color = EXCLUDED.color,
    colors = EXCLUDED.colors,
    vehicle_types = EXCLUDED.vehicle_types,
    logos = EXCLUDED.logos,
    source_hash = EXCLUDED.source_hash
RETURNING *;

-- name: SetBrandLogoSourceObjects :exec
UPDATE brand_logo_sources
SET badge_object_key = sqlc.narg(badge_object_key),
    wordmark_object_key = sqlc.narg(wordmark_object_key),
    full_object_key = sqlc.narg(full_object_key),
    synced_at = NOW()
WHERE id = sqlc.arg(id);

-- name: SearchBrandLogoSources :many
SELECT id, motomarks_id, name, color, vehicle_types, badge_object_key, wordmark_object_key, full_object_key, synced_at
FROM brand_logo_sources
WHERE sqlc.narg(q)::text IS NULL
   OR name ILIKE '%' || sqlc.narg(q) || '%'
   OR motomarks_id ILIKE '%' || sqlc.narg(q) || '%'
ORDER BY name ASC
LIMIT sqlc.arg(limit_count);

-- name: CountBrandLogoSources :one
SELECT COUNT(*) FROM brand_logo_sources;

-- name: CreateBrandLogoSyncRun :one
INSERT INTO brand_logo_sync_runs (triggered_by) VALUES (sqlc.narg(triggered_by))
RETURNING *;

-- name: StartBrandLogoSyncRun :one
UPDATE brand_logo_sync_runs SET status = 'running', started_at = NOW()
WHERE id = $1 AND status = 'queued'
RETURNING *;

-- name: FinishBrandLogoSyncRun :one
UPDATE brand_logo_sync_runs
SET status = sqlc.arg(status), finished_at = NOW(), not_modified = sqlc.arg(not_modified),
    requests = sqlc.arg(requests), brands_seen = sqlc.arg(brands_seen), downloaded = sqlc.arg(downloaded),
    matched = sqlc.arg(matched), unmatched = sqlc.arg(unmatched), error = sqlc.narg(error)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetActiveBrandLogoSyncRun :one
SELECT * FROM brand_logo_sync_runs WHERE status IN ('queued', 'running') ORDER BY id DESC LIMIT 1;

-- name: GetBrandLogoSyncRunByID :one
SELECT * FROM brand_logo_sync_runs WHERE id = $1;

-- name: ListBrandLogoSyncRuns :many
SELECT r.*, u.uuid AS triggered_by_uuid, u.name AS triggered_by_name
FROM brand_logo_sync_runs r
LEFT JOIN users u ON u.id = r.triggered_by
ORDER BY r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountBrandLogoSyncRuns :one
SELECT COUNT(*) FROM brand_logo_sync_runs;

-- name: ListCatalogDisplayBrandsToMatch :many
SELECT id, uuid, name
FROM catalog_display_brands
WHERE motomarks_match_source = 'none' AND motomarks_id IS NULL AND is_active
ORDER BY id;

-- name: SetCatalogDisplayBrandMotomarks :one
UPDATE catalog_display_brands
SET motomarks_id = sqlc.narg(motomarks_id),
    motomarks_match_score = sqlc.narg(motomarks_match_score),
    motomarks_match_source = sqlc.arg(motomarks_match_source),
    motomarks_suggested_id = sqlc.narg(motomarks_suggested_id),
    motomarks_suggested_score = sqlc.narg(motomarks_suggested_score)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: RefreshCatalogDisplayBrandsSyncedLogos :execrows
-- Denormalizes the synced variants and brand color of linked display brands.
UPDATE catalog_display_brands d
SET synced_logo_variants = x.variants, brand_color = NULLIF(x.color, '')
FROM (
    SELECT s.motomarks_id, s.color,
        ARRAY_REMOVE(ARRAY[
            CASE WHEN s.badge_object_key IS NOT NULL THEN 'badge' END,
            CASE WHEN s.wordmark_object_key IS NOT NULL THEN 'wordmark' END,
            CASE WHEN s.full_object_key IS NOT NULL THEN 'full' END
        ], NULL)::text[] AS variants
    FROM brand_logo_sources s
) x
WHERE d.motomarks_id = x.motomarks_id
  AND (d.synced_logo_variants IS DISTINCT FROM x.variants OR d.brand_color IS DISTINCT FROM NULLIF(x.color, ''));

-- name: ClearCatalogDisplayBrandsSyncedLogos :execrows
UPDATE catalog_display_brands
SET synced_logo_variants = '{}', brand_color = NULL
WHERE motomarks_id IS NULL AND (cardinality(synced_logo_variants) > 0 OR brand_color IS NOT NULL);

-- name: ListUnmatchedCatalogDisplayBrands :many
SELECT d.id, d.uuid, d.name, d.slug, d.is_active, d.logo_object_key, d.updated_at,
    d.motomarks_suggested_id, d.motomarks_suggested_score,
    s.name AS suggested_name,
    (SELECT COUNT(*) FROM catalog_types t JOIN catalog_brands b ON b.id = t.brand_id
        WHERE b.display_brand_id = d.id AND t.is_active)::bigint AS types_count
FROM catalog_display_brands d
LEFT JOIN brand_logo_sources s ON s.motomarks_id = d.motomarks_suggested_id
WHERE d.motomarks_match_source = 'none' AND d.motomarks_id IS NULL
  AND (sqlc.narg(q)::text IS NULL OR d.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY d.name ASC, d.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountUnmatchedCatalogDisplayBrands :one
SELECT COUNT(*)
FROM catalog_display_brands d
WHERE d.motomarks_match_source = 'none' AND d.motomarks_id IS NULL
  AND (sqlc.narg(q)::text IS NULL OR d.name ILIKE '%' || sqlc.narg(q) || '%');

-- name: CountCatalogDisplayBrandsByMatchSource :many
SELECT motomarks_match_source, COUNT(*)::bigint AS count
FROM catalog_display_brands
GROUP BY motomarks_match_source;

-- name: GetCatalogDisplayBrandLogoKeys :one
-- Every logo object of a display brand: the admin upload and the synced variants.
SELECT d.id, d.uuid, d.logo_object_key, d.updated_at,
    s.badge_object_key, s.wordmark_object_key, s.full_object_key
FROM catalog_display_brands d
LEFT JOIN brand_logo_sources s ON s.motomarks_id = d.motomarks_id
WHERE d.uuid = $1;
