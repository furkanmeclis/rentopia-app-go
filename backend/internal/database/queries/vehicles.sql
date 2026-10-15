-- Tenant resource: fleet vehicles (TEC-692), their status events and rental
-- group memberships. Every query filters by organization_id (see
-- docs/design/tenant-core.md §8).
--
-- Branch scope: a vehicle is visible when its home branch OR its current
-- branch is in the caller's scope (branch_ids NULL = every branch).
--
-- Kasko value: the latest TSB valuation period of the vehicle's catalog
-- type and model year, computed on read (never copied into the row).

-- name: CreateVehicle :one
INSERT INTO vehicles (
    organization_id, plate, vin, catalog_type_id, manual_entry, manual_brand, manual_model, manual_type,
    model_year, transmission, fuel, body_type, seats, luggage, air_conditioning, attributes_verified,
    color, home_branch_id, current_branch_id, fleet_pool, ownership, supplier_name,
    in_service_date, out_of_service_date, odometer_km, fuel_level, status, is_active
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(plate), sqlc.arg(vin), sqlc.narg(catalog_type_id),
    sqlc.arg(manual_entry), sqlc.arg(manual_brand), sqlc.arg(manual_model), sqlc.arg(manual_type),
    sqlc.arg(model_year), sqlc.narg(transmission), sqlc.narg(fuel), sqlc.narg(body_type),
    sqlc.narg(seats), sqlc.narg(luggage), sqlc.arg(air_conditioning), sqlc.arg(attributes_verified),
    sqlc.arg(color), sqlc.arg(home_branch_id), sqlc.arg(current_branch_id), sqlc.arg(fleet_pool),
    sqlc.arg(ownership), sqlc.arg(supplier_name), sqlc.narg(in_service_date), sqlc.narg(out_of_service_date),
    sqlc.arg(odometer_km), sqlc.narg(fuel_level), sqlc.arg(status), sqlc.arg(is_active)
)
RETURNING *;

-- name: UpdateVehicle :one
UPDATE vehicles
SET plate = sqlc.arg(plate),
    vin = sqlc.arg(vin),
    catalog_type_id = sqlc.narg(catalog_type_id),
    manual_entry = sqlc.arg(manual_entry),
    manual_brand = sqlc.arg(manual_brand),
    manual_model = sqlc.arg(manual_model),
    manual_type = sqlc.arg(manual_type),
    model_year = sqlc.arg(model_year),
    transmission = sqlc.narg(transmission),
    fuel = sqlc.narg(fuel),
    body_type = sqlc.narg(body_type),
    seats = sqlc.narg(seats),
    luggage = sqlc.narg(luggage),
    air_conditioning = sqlc.arg(air_conditioning),
    attributes_verified = sqlc.arg(attributes_verified),
    color = sqlc.arg(color),
    home_branch_id = sqlc.arg(home_branch_id),
    current_branch_id = sqlc.arg(current_branch_id),
    fleet_pool = sqlc.arg(fleet_pool),
    ownership = sqlc.arg(ownership),
    supplier_name = sqlc.arg(supplier_name),
    in_service_date = sqlc.narg(in_service_date),
    out_of_service_date = sqlc.narg(out_of_service_date),
    odometer_km = sqlc.arg(odometer_km),
    fuel_level = sqlc.narg(fuel_level),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetVehicleStatus :one
UPDATE vehicles
SET status = sqlc.arg(status)::text,
    status_changed_at = CASE WHEN status <> sqlc.arg(status)::text THEN NOW() ELSE status_changed_at END,
    odometer_km = sqlc.arg(odometer_km),
    fuel_level = sqlc.narg(fuel_level)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetVehicleCurrentBranch :one
UPDATE vehicles
SET current_branch_id = sqlc.arg(current_branch_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetVehicleActive :one
UPDATE vehicles
SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteVehicle :execrows
UPDATE vehicles
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: VehiclePlateTaken :one
-- Plates are compared by their key (letters and digits, upper-case) among
-- live, active vehicles.
SELECT EXISTS (
    SELECT 1 FROM vehicles
    WHERE organization_id = sqlc.arg(organization_id)
      AND plate_key = upper(regexp_replace(sqlc.arg(plate)::text, '[^A-Za-z0-9]', '', 'g'))
      AND deleted_at IS NULL
      AND is_active
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: GetVehicleByPlate :one
-- Import natural key: the live vehicle with this plate key (active first).
SELECT * FROM vehicles
WHERE organization_id = sqlc.arg(organization_id)
  AND plate_key = upper(regexp_replace(sqlc.arg(plate)::text, '[^A-Za-z0-9]', '', 'g'))
  AND deleted_at IS NULL
ORDER BY is_active DESC, id DESC
LIMIT 1;

-- name: GetVehicleDetail :one
SELECT sqlc.embed(v),
    t.uuid AS type_uuid, t.raw_name AS type_name, t.tsb_type_code AS type_code, t.years AS type_years,
    t.model_id AS catalog_model_id, m.name AS model_name,
    d.uuid AS brand_uuid, d.name AS brand_name,
    COALESCE(mg.id, 0)::bigint AS model_group_id,
    COALESCE(mg.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS model_group_uuid,
    hb.uuid AS home_branch_uuid, hb.code AS home_branch_code, hb.name AS home_branch_name,
    cb.uuid AS current_branch_uuid, cb.code AS current_branch_code, cb.name AS current_branch_name,
    k.value AS kasko_value, COALESCE(k.period, '')::text AS kasko_period,
    dx.inspection_until::date AS inspection_until, dx.insurance_until::date AS insurance_until,
    dx.kasko_until::date AS kasko_until, dx.document_expires_at::date AS document_expires_at,
    mx.next_maintenance_date::date AS next_maintenance_date,
    -- -1 = no open plan with a km target.
    COALESCE(mx.next_maintenance_km, -1)::int AS next_maintenance_km
FROM vehicles v
JOIN branches hb ON hb.organization_id = v.organization_id AND hb.id = v.home_branch_id
JOIN branches cb ON cb.organization_id = v.organization_id AND cb.id = v.current_branch_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN LATERAL (
    SELECT g.id, g.uuid FROM catalog_models g
    JOIN catalog_brands gb ON gb.id = g.brand_id
    WHERE g.group_key = m.group_key AND gb.display_brand_id = tb.display_brand_id
    ORDER BY g.id
    LIMIT 1
) mg ON TRUE
LEFT JOIN LATERAL (
    SELECT cv.value, cv.period FROM catalog_valuations cv
    WHERE cv.type_id = v.catalog_type_id AND cv.model_year = v.model_year
    ORDER BY cv.period DESC
    LIMIT 1
) k ON TRUE
LEFT JOIN LATERAL (
    -- Expiry tracking (TEC-693): the current document of a type is the live
    -- one with the latest valid_until; document_expires_at is the earliest
    -- of those (the next document to renew).
    SELECT
        MAX(c.until) FILTER (WHERE c.doc_type = 'muayene') AS inspection_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'trafik_sigortasi') AS insurance_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'kasko') AS kasko_until,
        MIN(c.until) AS document_expires_at
    FROM (
        SELECT d.doc_type, MAX(d.valid_until) AS until
        FROM vehicle_documents d
        WHERE d.organization_id = v.organization_id AND d.vehicle_id = v.id
          AND d.deleted_at IS NULL AND d.valid_until IS NOT NULL
        GROUP BY d.doc_type
    ) c
) dx ON TRUE
LEFT JOIN LATERAL (
    -- Next open maintenance plan (by date and by km).
    SELECT MIN(mp.planned_date) AS next_maintenance_date, MIN(mp.planned_km) AS next_maintenance_km
    FROM vehicle_maintenance mp
    WHERE mp.organization_id = v.organization_id AND mp.vehicle_id = v.id
      AND mp.kind = 'planned' AND mp.deleted_at IS NULL
) mx ON TRUE
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: ListVehicles :many
SELECT sqlc.embed(v),
    t.uuid AS type_uuid, t.raw_name AS type_name, t.tsb_type_code AS type_code, t.years AS type_years,
    t.model_id AS catalog_model_id, m.name AS model_name,
    d.uuid AS brand_uuid, d.name AS brand_name,
    COALESCE(mg.id, 0)::bigint AS model_group_id,
    COALESCE(mg.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS model_group_uuid,
    hb.uuid AS home_branch_uuid, hb.code AS home_branch_code, hb.name AS home_branch_name,
    cb.uuid AS current_branch_uuid, cb.code AS current_branch_code, cb.name AS current_branch_name,
    k.value AS kasko_value, COALESCE(k.period, '')::text AS kasko_period,
    dx.inspection_until::date AS inspection_until, dx.insurance_until::date AS insurance_until,
    dx.kasko_until::date AS kasko_until, dx.document_expires_at::date AS document_expires_at,
    mx.next_maintenance_date::date AS next_maintenance_date,
    -- -1 = no open plan with a km target.
    COALESCE(mx.next_maintenance_km, -1)::int AS next_maintenance_km
FROM vehicles v
JOIN branches hb ON hb.organization_id = v.organization_id AND hb.id = v.home_branch_id
JOIN branches cb ON cb.organization_id = v.organization_id AND cb.id = v.current_branch_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN LATERAL (
    SELECT g.id, g.uuid FROM catalog_models g
    JOIN catalog_brands gb ON gb.id = g.brand_id
    WHERE g.group_key = m.group_key AND gb.display_brand_id = tb.display_brand_id
    ORDER BY g.id
    LIMIT 1
) mg ON TRUE
LEFT JOIN LATERAL (
    SELECT cv.value, cv.period FROM catalog_valuations cv
    WHERE cv.type_id = v.catalog_type_id AND cv.model_year = v.model_year
    ORDER BY cv.period DESC
    LIMIT 1
) k ON TRUE
LEFT JOIN LATERAL (
    -- Expiry tracking (TEC-693): the current document of a type is the live
    -- one with the latest valid_until; document_expires_at is the earliest
    -- of those (the next document to renew).
    SELECT
        MAX(c.until) FILTER (WHERE c.doc_type = 'muayene') AS inspection_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'trafik_sigortasi') AS insurance_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'kasko') AS kasko_until,
        MIN(c.until) AS document_expires_at
    FROM (
        SELECT d.doc_type, MAX(d.valid_until) AS until
        FROM vehicle_documents d
        WHERE d.organization_id = v.organization_id AND d.vehicle_id = v.id
          AND d.deleted_at IS NULL AND d.valid_until IS NOT NULL
        GROUP BY d.doc_type
    ) c
) dx ON TRUE
LEFT JOIN LATERAL (
    -- Next open maintenance plan (by date and by km).
    SELECT MIN(mp.planned_date) AS next_maintenance_date, MIN(mp.planned_km) AS next_maintenance_km
    FROM vehicle_maintenance mp
    WHERE mp.organization_id = v.organization_id AND mp.vehicle_id = v.id
      AND mp.kind = 'planned' AND mp.deleted_at IS NULL
) mx ON TRUE
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.deleted_at IS NULL
  -- Branch scope of the caller: home OR current branch; NULL = every branch.
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR v.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(statuses)::text[] IS NULL OR v.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(ownerships)::text[] IS NULL OR v.ownership = ANY(sqlc.narg(ownerships)::text[]))
  AND (sqlc.narg(pools)::text[] IS NULL OR v.fleet_pool = ANY(sqlc.narg(pools)::text[]))
  AND (sqlc.narg(document_statuses)::text[] IS NULL OR (
    CASE
      WHEN dx.document_expires_at IS NULL THEN 'none'
      WHEN dx.document_expires_at < sqlc.arg(today)::date THEN 'expired'
      WHEN dx.document_expires_at <= sqlc.arg(today)::date + sqlc.arg(document_horizon_days)::int THEN 'expiring'
      ELSE 'valid'
    END) = ANY(sqlc.narg(document_statuses)::text[]))
  AND (sqlc.narg(home_branch_uuids)::uuid[] IS NULL OR hb.uuid = ANY(sqlc.narg(home_branch_uuids)::uuid[]))
  AND (sqlc.narg(current_branch_uuids)::uuid[] IS NULL OR cb.uuid = ANY(sqlc.narg(current_branch_uuids)::uuid[]))
  AND (sqlc.narg(group_uuids)::uuid[] IS NULL OR EXISTS (
    SELECT 1 FROM vehicle_rental_groups vg
    JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
    WHERE vg.organization_id = v.organization_id AND vg.vehicle_id = v.id
      AND g.deleted_at IS NULL AND g.uuid = ANY(sqlc.narg(group_uuids)::uuid[])
  ))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR EXISTS (
    SELECT 1 FROM vehicle_rental_groups vg
    JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
    JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
    WHERE vg.organization_id = v.organization_id AND vg.vehicle_id = v.id
      AND g.deleted_at IS NULL AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_key LIKE '%' || upper(regexp_replace(sqlc.narg(q), '[^A-Za-z0-9]', '', 'g')) || '%'
    OR v.vin ILIKE '%' || sqlc.narg(q) || '%'
    OR v.color ILIKE '%' || sqlc.narg(q) || '%'
    OR v.manual_brand ILIKE '%' || sqlc.narg(q) || '%'
    OR v.manual_model ILIKE '%' || sqlc.narg(q) || '%'
    OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%'
    OR m.name ILIKE '%' || sqlc.narg(q) || '%'
    OR d.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'plate' AND NOT sqlc.arg(sort_desc)::boolean THEN v.plate_key END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'plate' AND sqlc.arg(sort_desc)::boolean THEN v.plate_key END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'model_year' AND NOT sqlc.arg(sort_desc)::boolean THEN v.model_year END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'model_year' AND sqlc.arg(sort_desc)::boolean THEN v.model_year END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'odometer_km' AND NOT sqlc.arg(sort_desc)::boolean THEN v.odometer_km END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'odometer_km' AND sqlc.arg(sort_desc)::boolean THEN v.odometer_km END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN v.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN v.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kasko_value' AND NOT sqlc.arg(sort_desc)::boolean THEN k.value END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'kasko_value' AND sqlc.arg(sort_desc)::boolean THEN k.value END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'inspection_until' AND NOT sqlc.arg(sort_desc)::boolean THEN dx.inspection_until END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'inspection_until' AND sqlc.arg(sort_desc)::boolean THEN dx.inspection_until END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'insurance_until' AND NOT sqlc.arg(sort_desc)::boolean THEN dx.insurance_until END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'insurance_until' AND sqlc.arg(sort_desc)::boolean THEN dx.insurance_until END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'kasko_until' AND NOT sqlc.arg(sort_desc)::boolean THEN dx.kasko_until END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'kasko_until' AND sqlc.arg(sort_desc)::boolean THEN dx.kasko_until END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'document_expires_at' AND NOT sqlc.arg(sort_desc)::boolean THEN dx.document_expires_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'document_expires_at' AND sqlc.arg(sort_desc)::boolean THEN dx.document_expires_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'next_maintenance_date' AND NOT sqlc.arg(sort_desc)::boolean THEN mx.next_maintenance_date END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'next_maintenance_date' AND sqlc.arg(sort_desc)::boolean THEN mx.next_maintenance_date END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'next_maintenance_km' AND NOT sqlc.arg(sort_desc)::boolean THEN mx.next_maintenance_km END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'next_maintenance_km' AND sqlc.arg(sort_desc)::boolean THEN mx.next_maintenance_km END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN v.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN v.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN v.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN v.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN v.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN v.created_at END DESC,
  v.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountVehicles :one
SELECT COUNT(*)::bigint
FROM vehicles v
JOIN branches hb ON hb.organization_id = v.organization_id AND hb.id = v.home_branch_id
JOIN branches cb ON cb.organization_id = v.organization_id AND cb.id = v.current_branch_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN LATERAL (
    -- Expiry tracking (TEC-693): the current document of a type is the live
    -- one with the latest valid_until; document_expires_at is the earliest
    -- of those (the next document to renew).
    SELECT
        MAX(c.until) FILTER (WHERE c.doc_type = 'muayene') AS inspection_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'trafik_sigortasi') AS insurance_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'kasko') AS kasko_until,
        MIN(c.until) AS document_expires_at
    FROM (
        SELECT d.doc_type, MAX(d.valid_until) AS until
        FROM vehicle_documents d
        WHERE d.organization_id = v.organization_id AND d.vehicle_id = v.id
          AND d.deleted_at IS NULL AND d.valid_until IS NOT NULL
        GROUP BY d.doc_type
    ) c
) dx ON TRUE
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR v.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(statuses)::text[] IS NULL OR v.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(ownerships)::text[] IS NULL OR v.ownership = ANY(sqlc.narg(ownerships)::text[]))
  AND (sqlc.narg(pools)::text[] IS NULL OR v.fleet_pool = ANY(sqlc.narg(pools)::text[]))
  AND (sqlc.narg(document_statuses)::text[] IS NULL OR (
    CASE
      WHEN dx.document_expires_at IS NULL THEN 'none'
      WHEN dx.document_expires_at < sqlc.arg(today)::date THEN 'expired'
      WHEN dx.document_expires_at <= sqlc.arg(today)::date + sqlc.arg(document_horizon_days)::int THEN 'expiring'
      ELSE 'valid'
    END) = ANY(sqlc.narg(document_statuses)::text[]))
  AND (sqlc.narg(home_branch_uuids)::uuid[] IS NULL OR hb.uuid = ANY(sqlc.narg(home_branch_uuids)::uuid[]))
  AND (sqlc.narg(current_branch_uuids)::uuid[] IS NULL OR cb.uuid = ANY(sqlc.narg(current_branch_uuids)::uuid[]))
  AND (sqlc.narg(group_uuids)::uuid[] IS NULL OR EXISTS (
    SELECT 1 FROM vehicle_rental_groups vg
    JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
    WHERE vg.organization_id = v.organization_id AND vg.vehicle_id = v.id
      AND g.deleted_at IS NULL AND g.uuid = ANY(sqlc.narg(group_uuids)::uuid[])
  ))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR EXISTS (
    SELECT 1 FROM vehicle_rental_groups vg
    JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
    JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
    WHERE vg.organization_id = v.organization_id AND vg.vehicle_id = v.id
      AND g.deleted_at IS NULL AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_key LIKE '%' || upper(regexp_replace(sqlc.narg(q), '[^A-Za-z0-9]', '', 'g')) || '%'
    OR v.vin ILIKE '%' || sqlc.narg(q) || '%'
    OR v.color ILIKE '%' || sqlc.narg(q) || '%'
    OR v.manual_brand ILIKE '%' || sqlc.narg(q) || '%'
    OR v.manual_model ILIKE '%' || sqlc.narg(q) || '%'
    OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%'
    OR m.name ILIKE '%' || sqlc.narg(q) || '%'
    OR d.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListVehicleUUIDs :many
SELECT v.uuid
FROM vehicles v
JOIN branches hb ON hb.organization_id = v.organization_id AND hb.id = v.home_branch_id
JOIN branches cb ON cb.organization_id = v.organization_id AND cb.id = v.current_branch_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN LATERAL (
    -- Expiry tracking (TEC-693): the current document of a type is the live
    -- one with the latest valid_until; document_expires_at is the earliest
    -- of those (the next document to renew).
    SELECT
        MAX(c.until) FILTER (WHERE c.doc_type = 'muayene') AS inspection_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'trafik_sigortasi') AS insurance_until,
        MAX(c.until) FILTER (WHERE c.doc_type = 'kasko') AS kasko_until,
        MIN(c.until) AS document_expires_at
    FROM (
        SELECT d.doc_type, MAX(d.valid_until) AS until
        FROM vehicle_documents d
        WHERE d.organization_id = v.organization_id AND d.vehicle_id = v.id
          AND d.deleted_at IS NULL AND d.valid_until IS NOT NULL
        GROUP BY d.doc_type
    ) c
) dx ON TRUE
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR v.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(statuses)::text[] IS NULL OR v.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(ownerships)::text[] IS NULL OR v.ownership = ANY(sqlc.narg(ownerships)::text[]))
  AND (sqlc.narg(pools)::text[] IS NULL OR v.fleet_pool = ANY(sqlc.narg(pools)::text[]))
  AND (sqlc.narg(document_statuses)::text[] IS NULL OR (
    CASE
      WHEN dx.document_expires_at IS NULL THEN 'none'
      WHEN dx.document_expires_at < sqlc.arg(today)::date THEN 'expired'
      WHEN dx.document_expires_at <= sqlc.arg(today)::date + sqlc.arg(document_horizon_days)::int THEN 'expiring'
      ELSE 'valid'
    END) = ANY(sqlc.narg(document_statuses)::text[]))
  AND (sqlc.narg(home_branch_uuids)::uuid[] IS NULL OR hb.uuid = ANY(sqlc.narg(home_branch_uuids)::uuid[]))
  AND (sqlc.narg(current_branch_uuids)::uuid[] IS NULL OR cb.uuid = ANY(sqlc.narg(current_branch_uuids)::uuid[]))
  AND (sqlc.narg(group_uuids)::uuid[] IS NULL OR EXISTS (
    SELECT 1 FROM vehicle_rental_groups vg
    JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
    WHERE vg.organization_id = v.organization_id AND vg.vehicle_id = v.id
      AND g.deleted_at IS NULL AND g.uuid = ANY(sqlc.narg(group_uuids)::uuid[])
  ))
  AND (sqlc.narg(segment_uuids)::uuid[] IS NULL OR EXISTS (
    SELECT 1 FROM vehicle_rental_groups vg
    JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
    JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
    WHERE vg.organization_id = v.organization_id AND vg.vehicle_id = v.id
      AND g.deleted_at IS NULL AND s.uuid = ANY(sqlc.narg(segment_uuids)::uuid[])
  ))
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_key LIKE '%' || upper(regexp_replace(sqlc.narg(q), '[^A-Za-z0-9]', '', 'g')) || '%'
    OR v.vin ILIKE '%' || sqlc.narg(q) || '%'
    OR v.color ILIKE '%' || sqlc.narg(q) || '%'
    OR v.manual_brand ILIKE '%' || sqlc.narg(q) || '%'
    OR v.manual_model ILIKE '%' || sqlc.narg(q) || '%'
    OR t.raw_name ILIKE '%' || sqlc.narg(q) || '%'
    OR m.name ILIKE '%' || sqlc.narg(q) || '%'
    OR d.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY v.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListVehicleFacets :many
-- Dynamic filter options of the vehicles list in the caller's branch scope.
-- facet: status | ownership | pool | is_active | document_status | home_branch
-- | current_branch | rental_group | segment; value is the filter value (uuid for references).
WITH scoped AS (
    SELECT v.id, v.organization_id, v.status, v.ownership, v.fleet_pool, v.is_active,
        v.home_branch_id, v.current_branch_id,
        (CASE
          WHEN dx.document_expires_at IS NULL THEN 'none'
          WHEN dx.document_expires_at < sqlc.arg(today)::date THEN 'expired'
          WHEN dx.document_expires_at <= sqlc.arg(today)::date + sqlc.arg(document_horizon_days)::int THEN 'expiring'
          ELSE 'valid'
        END)::text AS document_status
    FROM vehicles v
    LEFT JOIN LATERAL (
        -- Expiry tracking (TEC-693): the current document of a type is the live
        -- one with the latest valid_until; document_expires_at is the earliest
        -- of those (the next document to renew).
        SELECT
            MAX(c.until) FILTER (WHERE c.doc_type = 'muayene') AS inspection_until,
            MAX(c.until) FILTER (WHERE c.doc_type = 'trafik_sigortasi') AS insurance_until,
            MAX(c.until) FILTER (WHERE c.doc_type = 'kasko') AS kasko_until,
            MIN(c.until) AS document_expires_at
        FROM (
            SELECT d.doc_type, MAX(d.valid_until) AS until
            FROM vehicle_documents d
            WHERE d.organization_id = v.organization_id AND d.vehicle_id = v.id
              AND d.deleted_at IS NULL AND d.valid_until IS NOT NULL
            GROUP BY d.doc_type
        ) c
    ) dx ON TRUE
    WHERE v.organization_id = sqlc.arg(organization_id)
      AND v.deleted_at IS NULL
      AND (
        sqlc.narg(branch_ids)::bigint[] IS NULL
        OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
      )
)
SELECT 'status'::text AS facet, s.status::text AS value, ''::text AS label, COUNT(*)::bigint AS count
FROM scoped s GROUP BY s.status
UNION ALL
SELECT 'ownership'::text, s.ownership::text, ''::text, COUNT(*)::bigint
FROM scoped s GROUP BY s.ownership
UNION ALL
SELECT 'pool'::text, s.fleet_pool::text, ''::text, COUNT(*)::bigint
FROM scoped s GROUP BY s.fleet_pool
UNION ALL
SELECT 'is_active'::text, s.is_active::text, ''::text, COUNT(*)::bigint
FROM scoped s GROUP BY s.is_active
UNION ALL
SELECT 'document_status'::text, s.document_status, ''::text, COUNT(*)::bigint
FROM scoped s GROUP BY s.document_status
UNION ALL
SELECT 'home_branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.home_branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'current_branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.current_branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'rental_group'::text, g.uuid::text, (g.code || ' · ' || g.name)::text, COUNT(*)::bigint
FROM scoped s
JOIN vehicle_rental_groups vg ON vg.organization_id = s.organization_id AND vg.vehicle_id = s.id
JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id AND g.deleted_at IS NULL
GROUP BY g.uuid, g.code, g.name
UNION ALL
SELECT 'segment'::text, sg.uuid::text, sg.name::text, COUNT(DISTINCT s.id)::bigint
FROM scoped s
JOIN vehicle_rental_groups vg ON vg.organization_id = s.organization_id AND vg.vehicle_id = s.id
JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id AND g.deleted_at IS NULL
JOIN vehicle_segments sg ON sg.organization_id = g.organization_id AND sg.id = g.segment_id
GROUP BY sg.uuid, sg.name;

-- name: ListVehicleGroupsByVehicleIDs :many
-- Live rental group memberships of the given vehicles.
SELECT vg.vehicle_id, g.id AS rental_group_id, g.uuid, g.code, g.name, g.is_active,
    s.uuid AS segment_uuid, s.code AS segment_code, s.name AS segment_name, vg.created_at
FROM vehicle_rental_groups vg
JOIN rental_groups g ON g.organization_id = vg.organization_id AND g.id = vg.rental_group_id
JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
WHERE vg.organization_id = sqlc.arg(organization_id)
  AND vg.vehicle_id = ANY(sqlc.arg(vehicle_ids)::bigint[])
  AND g.deleted_at IS NULL
ORDER BY g.code ASC, g.id ASC;

-- name: AddVehicleRentalGroup :exec
INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(rental_group_id))
ON CONFLICT DO NOTHING;

-- name: RemoveVehicleRentalGroup :execrows
DELETE FROM vehicle_rental_groups
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND rental_group_id = sqlc.arg(rental_group_id);

-- name: ClearVehicleRentalGroups :exec
DELETE FROM vehicle_rental_groups
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id);

-- name: ListRentalGroupMemberVehicles :many
-- Member vehicles (live) of the given rental groups: the explicit side of a
-- GROUP guarantee (fleet/eligibility).
SELECT vg.rental_group_id, v.uuid AS vehicle_uuid
FROM vehicle_rental_groups vg
JOIN vehicles v ON v.organization_id = vg.organization_id AND v.id = vg.vehicle_id
WHERE vg.organization_id = sqlc.arg(organization_id)
  AND vg.rental_group_id = ANY(sqlc.arg(group_ids)::bigint[])
  AND v.deleted_at IS NULL
ORDER BY vg.rental_group_id, v.id;

-- name: ListLiveRentalGroupsForVehicles :many
-- Every live rental group of the organization (eligibility candidates).
SELECT g.*, s.uuid AS segment_uuid, s.code AS segment_code, s.name AS segment_name
FROM rental_groups g
JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.deleted_at IS NULL
ORDER BY g.code ASC, g.id ASC;

-- name: GetLiveRentalGroupForVehicles :one
SELECT g.*, s.uuid AS segment_uuid, s.code AS segment_code, s.name AS segment_name
FROM rental_groups g
JOIN vehicle_segments s ON s.organization_id = g.organization_id AND s.id = g.segment_id
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.uuid = sqlc.arg(uuid)
  AND g.deleted_at IS NULL;

-- name: VehicleCanonicalModelIDs :many
-- Platform catalog (not organization scoped): the grouped model of each
-- catalog model, the lowest id among the models with the same group key
-- under the TSB brands of one display brand (the id tenant pickers use).
SELECT m.id, (
    SELECT g.id FROM catalog_models g
    JOIN catalog_brands gb ON gb.id = g.brand_id
    WHERE g.group_key = m.group_key AND gb.display_brand_id = b.display_brand_id
    ORDER BY g.id
    LIMIT 1
)::bigint AS canonical_id
FROM catalog_models m
JOIN catalog_brands b ON b.id = m.brand_id
WHERE m.id = ANY(sqlc.arg(ids)::bigint[]);

-- name: GetVehicleCatalogType :one
-- Platform catalog type picked on a vehicle form (not organization scoped).
SELECT t.id, t.uuid, t.raw_name, t.tsb_type_code, t.years, t.model_id, t.suggested_transmission,
    t.suggested_fuel, t.is_active
FROM catalog_types t
WHERE t.uuid = sqlc.arg(uuid);

-- name: FindVehicleCatalogTypeByCode :many
-- Import: catalog types with a TSB type code (codes are unique per TSB
-- brand only, so several may match).
SELECT t.id, t.uuid, t.raw_name, t.tsb_type_code, t.years, t.model_id, t.suggested_transmission,
    t.suggested_fuel, t.is_active, d.name AS brand_name
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
JOIN catalog_display_brands d ON d.id = b.display_brand_id
WHERE t.tsb_type_code = sqlc.arg(code)
  AND t.is_active
ORDER BY t.id;

-- name: FindVehicleCatalogTypesByNames :many
-- Import: active catalog types of a display brand + model name, optionally
-- narrowed by a type name fragment.
SELECT t.id, t.uuid, t.raw_name, t.tsb_type_code, t.years, t.model_id, t.suggested_transmission,
    t.suggested_fuel, t.is_active
FROM catalog_types t
JOIN catalog_brands b ON b.id = t.brand_id
JOIN catalog_display_brands d ON d.id = b.display_brand_id
JOIN catalog_models m ON m.id = t.model_id
WHERE t.is_active AND b.is_active
  AND lower(d.name) = lower(sqlc.arg(brand)::text)
  AND m.group_key = lower(regexp_replace(btrim(sqlc.arg(model)::text), '\s+', ' ', 'g'))
  AND (sqlc.narg(type_name)::text IS NULL OR t.raw_name ILIKE '%' || sqlc.narg(type_name) || '%')
ORDER BY t.raw_name, t.id;

-- name: ListCatalogValuationsForVehicle :many
-- Platform catalog: kasko value history of one type and model year.
SELECT period, value
FROM catalog_valuations
WHERE type_id = sqlc.arg(type_id) AND model_year = sqlc.arg(model_year)
ORDER BY period DESC;

-- name: FindVehicleImportBranch :one
-- Import: a live branch of the organization by code or name (code first).
SELECT * FROM branches
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (upper(code) = upper(sqlc.arg(ref)::text) OR lower(name) = lower(sqlc.arg(ref)::text))
ORDER BY (upper(code) = upper(sqlc.arg(ref)::text)) DESC, id ASC
LIMIT 1;

-- name: InsertVehicleEvent :one
INSERT INTO vehicle_events (
    organization_id, vehicle_id, event_type, from_status, to_status, odometer_km, fuel_level, note, actor_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(event_type), sqlc.narg(from_status),
    sqlc.arg(to_status), sqlc.narg(odometer_km), sqlc.narg(fuel_level), sqlc.arg(note), sqlc.narg(actor_id)
)
RETURNING *;

-- name: ListVehicleEvents :many
SELECT e.*, u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM vehicle_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.vehicle_id = sqlc.arg(vehicle_id)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountVehicleEvents :one
SELECT COUNT(*)::bigint
FROM vehicle_events
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id);

-- name: GetVehicleSearchDocument :one
SELECT v.id, v.uuid, v.plate, v.vin, v.model_year, v.status, v.home_branch_id, v.current_branch_id,
    v.organization_id, o.slug AS organization_slug,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label
FROM vehicles v
JOIN organizations o ON o.id = v.organization_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: ListVehicleSearchDocuments :many
SELECT v.id, v.uuid, v.plate, v.vin, v.model_year, v.status, v.home_branch_id, v.current_branch_id,
    v.organization_id, o.slug AS organization_slug,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label
FROM vehicles v
JOIN organizations o ON o.id = v.organization_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.deleted_at IS NULL
ORDER BY v.id ASC;
