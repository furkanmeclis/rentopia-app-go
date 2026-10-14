-- Tenant resource: fleet classification (TEC-691) — vehicle segments,
-- rental groups (+ catalog criteria) and rental products (+ versions, images).
-- Every query on these tables filters by organization_id
-- (see docs/design/tenant-core.md §8). List queries return the table rows;
-- the repository hydrates references with the batch lookups below.

-- ---------------------------------------------------------------- segments

-- name: CreateVehicleSegment :one
INSERT INTO vehicle_segments (organization_id, code, name, description, sort_order, is_active)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(sort_order), sqlc.arg(is_active)
)
RETURNING *;

-- name: GetVehicleSegmentByUUID :one
SELECT * FROM vehicle_segments
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: VehicleSegmentCodeTaken :one
SELECT EXISTS (
    SELECT 1 FROM vehicle_segments
    WHERE organization_id = sqlc.arg(organization_id)
      AND code = sqlc.arg(code)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: ListVehicleSegments :many
SELECT * FROM vehicle_segments s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.code ILIKE '%' || sqlc.narg(q) || '%'
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN s.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN s.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN s.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN s.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN s.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN s.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN s.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN s.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN s.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN s.created_at END DESC,
  s.sort_order ASC, s.code ASC, s.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountVehicleSegments :one
SELECT COUNT(*)::bigint FROM vehicle_segments s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.code ILIKE '%' || sqlc.narg(q) || '%'
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListVehicleSegmentsByIDs :many
SELECT * FROM vehicle_segments
WHERE organization_id = sqlc.arg(organization_id)
  AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: UpdateVehicleSegment :one
UPDATE vehicle_segments
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    sort_order = sqlc.arg(sort_order),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteVehicleSegment :execrows
UPDATE vehicle_segments
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: CountVehicleSegmentUsage :many
-- Live groups and products per segment.
SELECT s.id AS segment_id,
    (SELECT COUNT(*) FROM rental_groups g
     WHERE g.organization_id = s.organization_id AND g.segment_id = s.id AND g.deleted_at IS NULL)::bigint AS groups_count,
    (SELECT COUNT(*) FROM rental_products p
     WHERE p.organization_id = s.organization_id AND p.segment_id = s.id AND p.deleted_at IS NULL)::bigint AS products_count
FROM vehicle_segments s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListVehicleSegmentActiveFacets :many
SELECT is_active, COUNT(*)::bigint AS count
FROM vehicle_segments
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
GROUP BY is_active
ORDER BY is_active DESC;

-- ----------------------------------------------------------- rental groups

-- name: CreateRentalGroup :one
INSERT INTO rental_groups (
    organization_id, code, name, description, segment_id, transmission, fuel, body_type,
    min_seats, min_luggage, air_conditioning, acriss_code, is_active
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(segment_id), sqlc.narg(transmission), sqlc.narg(fuel), sqlc.narg(body_type),
    sqlc.narg(min_seats), sqlc.narg(min_luggage), sqlc.arg(air_conditioning),
    sqlc.narg(acriss_code), sqlc.arg(is_active)
)
RETURNING *;

-- name: GetRentalGroupByUUID :one
SELECT * FROM rental_groups
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: RentalGroupCodeTaken :one
SELECT EXISTS (
    SELECT 1 FROM rental_groups
    WHERE organization_id = sqlc.arg(organization_id)
      AND code = sqlc.arg(code)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: ListRentalGroups :many
SELECT * FROM rental_groups g
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR g.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR g.segment_id IN (
    SELECT s.id FROM vehicle_segments s
    WHERE s.organization_id = g.organization_id AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (sqlc.narg(transmissions)::text[] IS NULL OR g.transmission = ANY(sqlc.narg(transmissions)::text[]))
  AND (sqlc.narg(fuels)::text[] IS NULL OR g.fuel = ANY(sqlc.narg(fuels)::text[]))
  AND (sqlc.narg(body_types)::text[] IS NULL OR g.body_type = ANY(sqlc.narg(body_types)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR g.code ILIKE '%' || sqlc.narg(q) || '%'
    OR g.name ILIKE '%' || sqlc.narg(q) || '%'
    OR g.acriss_code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN g.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN g.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN g.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN g.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'acriss_code' AND NOT sqlc.arg(sort_desc)::boolean THEN g.acriss_code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'acriss_code' AND sqlc.arg(sort_desc)::boolean THEN g.acriss_code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'min_seats' AND NOT sqlc.arg(sort_desc)::boolean THEN g.min_seats END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'min_seats' AND sqlc.arg(sort_desc)::boolean THEN g.min_seats END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN g.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN g.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN g.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN g.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN g.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN g.created_at END DESC,
  g.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRentalGroups :one
SELECT COUNT(*)::bigint FROM rental_groups g
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR g.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR g.segment_id IN (
    SELECT s.id FROM vehicle_segments s
    WHERE s.organization_id = g.organization_id AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (sqlc.narg(transmissions)::text[] IS NULL OR g.transmission = ANY(sqlc.narg(transmissions)::text[]))
  AND (sqlc.narg(fuels)::text[] IS NULL OR g.fuel = ANY(sqlc.narg(fuels)::text[]))
  AND (sqlc.narg(body_types)::text[] IS NULL OR g.body_type = ANY(sqlc.narg(body_types)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR g.code ILIKE '%' || sqlc.narg(q) || '%'
    OR g.name ILIKE '%' || sqlc.narg(q) || '%'
    OR g.acriss_code ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListRentalGroupsByIDs :many
SELECT * FROM rental_groups
WHERE organization_id = sqlc.arg(organization_id)
  AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: UpdateRentalGroup :one
UPDATE rental_groups
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    segment_id = sqlc.arg(segment_id),
    transmission = sqlc.narg(transmission),
    fuel = sqlc.narg(fuel),
    body_type = sqlc.narg(body_type),
    min_seats = sqlc.narg(min_seats),
    min_luggage = sqlc.narg(min_luggage),
    air_conditioning = sqlc.arg(air_conditioning),
    acriss_code = sqlc.narg(acriss_code),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetRentalGroupActive :one
UPDATE rental_groups
SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteRentalGroup :execrows
UPDATE rental_groups
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: CountRentalGroupProducts :many
-- Live products whose current version guarantees each group.
SELECT v.rental_group_id::bigint AS rental_group_id, COUNT(*)::bigint AS products_count
FROM rental_products p
JOIN rental_product_versions v
  ON v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND v.rental_group_id = ANY(sqlc.arg(ids)::bigint[])
GROUP BY v.rental_group_id;

-- name: ListRentalGroupSegmentFacets :many
SELECT s.uuid, s.code, s.name, COUNT(*)::bigint AS count
FROM rental_groups g
JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.deleted_at IS NULL
GROUP BY s.uuid, s.code, s.name, s.sort_order
ORDER BY s.sort_order ASC, s.code ASC;

-- name: ListRentalGroupAttributeFacets :many
-- One row per (attribute, value) in use: transmission, fuel, body_type.
SELECT a.attribute::text AS attribute, a.value::text AS value, COUNT(*)::bigint AS count
FROM rental_groups g
CROSS JOIN LATERAL (
    VALUES ('transmission', g.transmission), ('fuel', g.fuel), ('body_type', g.body_type)
) AS a (attribute, value)
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.deleted_at IS NULL
  AND a.value IS NOT NULL
GROUP BY a.attribute, a.value
ORDER BY a.attribute, a.value;

-- name: DeleteRentalGroupCatalogModels :exec
DELETE FROM rental_group_catalog_models
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_group_id = sqlc.arg(rental_group_id);

-- name: InsertRentalGroupCatalogModels :exec
INSERT INTO rental_group_catalog_models (organization_id, rental_group_id, catalog_model_id)
SELECT sqlc.arg(organization_id), sqlc.arg(rental_group_id), unnest(sqlc.arg(catalog_model_ids)::bigint[])
ON CONFLICT DO NOTHING;

-- name: DeleteRentalGroupCatalogTypes :exec
DELETE FROM rental_group_catalog_types
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_group_id = sqlc.arg(rental_group_id);

-- name: InsertRentalGroupCatalogTypes :exec
INSERT INTO rental_group_catalog_types (organization_id, rental_group_id, catalog_type_id)
SELECT sqlc.arg(organization_id), sqlc.arg(rental_group_id), unnest(sqlc.arg(catalog_type_ids)::bigint[])
ON CONFLICT DO NOTHING;

-- name: ListRentalGroupCatalogModelIDs :many
SELECT rental_group_id, catalog_model_id
FROM rental_group_catalog_models
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_group_id = ANY(sqlc.arg(group_ids)::bigint[])
ORDER BY rental_group_id, catalog_model_id;

-- name: ListRentalGroupCatalogTypeIDs :many
SELECT rental_group_id, catalog_type_id
FROM rental_group_catalog_types
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_group_id = ANY(sqlc.arg(group_ids)::bigint[])
ORDER BY rental_group_id, catalog_type_id;

-- ------------------------------------------- platform catalog references
-- (catalog tables are platform data, not organization scoped)

-- name: FleetCatalogModelRefsByUUIDs :many
SELECT m.id, m.uuid, m.name, b.uuid AS brand_uuid, b.display_name AS brand_display_name
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE m.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: FleetCatalogModelRefsByIDs :many
SELECT m.id, m.uuid, m.name, b.uuid AS brand_uuid, b.display_name AS brand_display_name
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE m.id = ANY(sqlc.arg(ids)::bigint[]);

-- name: FleetCatalogTypeRefsByUUIDs :many
SELECT t.id, t.uuid, t.raw_name, t.model_id, m.uuid AS model_uuid, m.name AS model_name,
    b.uuid AS brand_uuid, b.display_name AS brand_display_name
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
LEFT JOIN catalog_models m ON m.id = t.model_id
WHERE t.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: FleetCatalogTypeRefsByIDs :many
SELECT t.id, t.uuid, t.raw_name, t.model_id, m.uuid AS model_uuid, m.name AS model_name,
    b.uuid AS brand_uuid, b.display_name AS brand_display_name
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
LEFT JOIN catalog_models m ON m.id = t.model_id
WHERE t.id = ANY(sqlc.arg(ids)::bigint[]);

-- -------------------------------------------------------- rental products

-- name: CreateRentalProduct :one
INSERT INTO rental_products (
    organization_id, code, title, description, segment_id, representative_model_id,
    representative_type_id, status, is_active, current_version
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(title), sqlc.arg(description),
    sqlc.arg(segment_id), sqlc.narg(representative_model_id), sqlc.narg(representative_type_id),
    sqlc.arg(status), sqlc.arg(is_active), 1
)
RETURNING *;

-- name: GetRentalProductByUUID :one
SELECT * FROM rental_products
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetRentalProductByUUIDForUpdate :one
SELECT * FROM rental_products
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
FOR UPDATE;

-- name: RentalProductCodeTaken :one
SELECT EXISTS (
    SELECT 1 FROM rental_products
    WHERE organization_id = sqlc.arg(organization_id)
      AND code = sqlc.arg(code)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: ListRentalProducts :many
SELECT * FROM rental_products p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR p.segment_id IN (
    SELECT s.id FROM vehicle_segments s
    WHERE s.organization_id = p.organization_id AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (sqlc.narg(guarantee_types)::text[] IS NULL OR EXISTS (
    SELECT 1 FROM rental_product_versions v
    WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
      AND v.guarantee_type = ANY(sqlc.narg(guarantee_types)::text[])
  ))
  AND (sqlc.narg(rental_group_uuid)::uuid IS NULL OR EXISTS (
    SELECT 1 FROM rental_product_versions v
    JOIN rental_groups g ON g.organization_id = v.organization_id AND g.id = v.rental_group_id
    WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
      AND g.uuid = sqlc.narg(rental_group_uuid)
  ))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN p.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN p.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'title' AND NOT sqlc.arg(sort_desc)::boolean THEN p.title END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'title' AND sqlc.arg(sort_desc)::boolean THEN p.title END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN p.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN p.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'current_version' AND NOT sqlc.arg(sort_desc)::boolean THEN p.current_version END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'current_version' AND sqlc.arg(sort_desc)::boolean THEN p.current_version END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN p.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN p.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN p.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
  p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRentalProducts :one
SELECT COUNT(*)::bigint FROM rental_products p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR p.segment_id IN (
    SELECT s.id FROM vehicle_segments s
    WHERE s.organization_id = p.organization_id AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (sqlc.narg(guarantee_types)::text[] IS NULL OR EXISTS (
    SELECT 1 FROM rental_product_versions v
    WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
      AND v.guarantee_type = ANY(sqlc.narg(guarantee_types)::text[])
  ))
  AND (sqlc.narg(rental_group_uuid)::uuid IS NULL OR EXISTS (
    SELECT 1 FROM rental_product_versions v
    JOIN rental_groups g ON g.organization_id = v.organization_id AND g.id = v.rental_group_id
    WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
      AND g.uuid = sqlc.narg(rental_group_uuid)
  ))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: UpdateRentalProduct :one
UPDATE rental_products
SET code = sqlc.arg(code),
    title = sqlc.arg(title),
    description = sqlc.arg(description),
    segment_id = sqlc.arg(segment_id),
    representative_model_id = sqlc.narg(representative_model_id),
    representative_type_id = sqlc.narg(representative_type_id),
    status = sqlc.arg(status),
    is_active = sqlc.arg(is_active),
    current_version = sqlc.arg(current_version)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetRentalProductActive :one
UPDATE rental_products
SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: TouchRentalProduct :exec
UPDATE rental_products
SET updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SoftDeleteRentalProduct :execrows
UPDATE rental_products
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: ListRentalProductStatusFacets :many
SELECT status, COUNT(*)::bigint AS count
FROM rental_products
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
GROUP BY status
ORDER BY status;

-- name: ListRentalProductGuaranteeFacets :many
SELECT v.guarantee_type, COUNT(*)::bigint AS count
FROM rental_products p
JOIN rental_product_versions v
  ON v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
GROUP BY v.guarantee_type
ORDER BY v.guarantee_type;

-- name: ListRentalProductSegmentFacets :many
SELECT s.uuid, s.code, s.name, COUNT(*)::bigint AS count
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
GROUP BY s.uuid, s.code, s.name, s.sort_order
ORDER BY s.sort_order ASC, s.code ASC;

-- name: GetRentalProductSearchDocument :one
SELECT p.uuid, p.code, p.title, p.status, p.is_active, p.organization_id, o.slug AS organization_slug,
    s.name AS segment_name
FROM rental_products p
JOIN organizations o ON o.id = p.organization_id
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: ListRentalProductSearchDocuments :many
SELECT p.uuid, p.code, p.title, p.status, p.is_active, p.organization_id, o.slug AS organization_slug,
    s.name AS segment_name
FROM rental_products p
JOIN organizations o ON o.id = p.organization_id
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
ORDER BY p.id ASC;

-- ------------------------------------------------- rental product versions

-- name: CreateRentalProductVersion :one
INSERT INTO rental_product_versions (
    organization_id, product_id, version, guarantee_type, vehicle_id, variant_level,
    catalog_model_id, catalog_type_id, rental_group_id, guarantee_label, deposit_amount,
    min_driver_age, min_license_years, included_km_per_day, extra_km_fee, fuel_policy, notes,
    change_note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(product_id), sqlc.arg(version), sqlc.arg(guarantee_type),
    sqlc.narg(vehicle_id), sqlc.narg(variant_level), sqlc.narg(catalog_model_id),
    sqlc.narg(catalog_type_id), sqlc.narg(rental_group_id), sqlc.arg(guarantee_label),
    sqlc.arg(deposit_amount), sqlc.arg(min_driver_age), sqlc.arg(min_license_years),
    sqlc.narg(included_km_per_day), sqlc.arg(extra_km_fee), sqlc.arg(fuel_policy), sqlc.arg(notes),
    sqlc.arg(change_note), sqlc.narg(created_by)
)
RETURNING *;

-- name: ListRentalProductVersions :many
SELECT * FROM rental_product_versions
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id)
ORDER BY version DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRentalProductVersions :one
SELECT COUNT(*)::bigint FROM rental_product_versions
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id);

-- name: GetRentalProductVersionByUUID :one
SELECT * FROM rental_product_versions
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListCurrentRentalProductVersions :many
SELECT v.* FROM rental_product_versions v
JOIN rental_products p ON p.organization_id = v.organization_id AND p.id = v.product_id
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.product_id = ANY(sqlc.arg(product_ids)::bigint[])
  AND v.version = p.current_version;

-- name: ListUsersByIDsForFleet :many
SELECT id, uuid, name, surname, email FROM users
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- --------------------------------------------------- rental product images

-- name: CreateRentalProductImage :one
INSERT INTO rental_product_images (
    uuid, organization_id, product_id, object_key, filename, content_type, size_bytes, sort_order
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(product_id), sqlc.arg(object_key),
    sqlc.arg(filename), sqlc.arg(content_type), sqlc.arg(size_bytes),
    COALESCE((
        SELECT MAX(sort_order) + 1 FROM rental_product_images
        WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id)
    ), 0)
)
RETURNING *;

-- name: ListRentalProductImages :many
SELECT * FROM rental_product_images
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY product_id, sort_order, id;

-- name: CountRentalProductImages :one
SELECT COUNT(*)::bigint FROM rental_product_images
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id);

-- name: GetRentalProductImage :one
SELECT * FROM rental_product_images
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id)
  AND uuid = sqlc.arg(uuid);

-- name: DeleteRentalProductImage :execrows
DELETE FROM rental_product_images
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id)
  AND uuid = sqlc.arg(uuid);

-- name: SetRentalProductImageSort :execrows
UPDATE rental_product_images
SET sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = sqlc.arg(product_id)
  AND uuid = sqlc.arg(uuid);

-- name: GetPublicRentalProductImage :one
-- Public media stream: the organization is named by its uuid in the URL.
SELECT i.object_key, i.content_type, i.size_bytes
FROM rental_product_images i
JOIN rental_products p ON p.organization_id = i.organization_id AND p.id = i.product_id
WHERE i.organization_id = (SELECT o.id FROM organizations o WHERE o.uuid = sqlc.arg(organization_uuid))
  AND i.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: FleetVehicleRef :one
-- EXACT_VEHICLE guarantees: the vehicle must be a live vehicle of the
-- organization; its plate becomes the guarantee label.
SELECT plate
FROM vehicles
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;
