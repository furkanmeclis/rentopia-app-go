-- Tenant resource: extras catalog, effective-dated prices, branch stock and
-- equipment allocations (TEC-702). Every query filters by organization_id
-- (docs/design/tenant-core.md §8). docs/design/extras.md.

-- name: CreateExtra :one
INSERT INTO extras (
    organization_id, code, name, description, extra_type, charge_type, min_quantity, max_quantity,
    max_total, channels, status, is_physical, sort_order
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(description), sqlc.arg(extra_type),
    sqlc.arg(charge_type), sqlc.arg(min_quantity), sqlc.arg(max_quantity), sqlc.narg(max_total),
    sqlc.narg(channels)::varchar[], sqlc.arg(status), sqlc.arg(is_physical), sqlc.arg(sort_order)
)
RETURNING id, uuid;

-- name: UpdateExtra :execrows
UPDATE extras
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    extra_type = sqlc.arg(extra_type),
    charge_type = sqlc.arg(charge_type),
    min_quantity = sqlc.arg(min_quantity),
    max_quantity = sqlc.arg(max_quantity),
    max_total = sqlc.narg(max_total),
    channels = sqlc.narg(channels)::varchar[],
    status = sqlc.arg(status),
    is_physical = sqlc.arg(is_physical),
    sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: SetExtraStatus :execrows
UPDATE extras
SET status = sqlc.arg(status)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: SoftDeleteExtra :execrows
UPDATE extras
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: GetExtraByUUID :one
SELECT e.id, e.uuid, e.code, e.name, e.description, e.extra_type, e.charge_type, e.min_quantity, e.max_quantity,
    e.max_total, e.channels::varchar[] AS channels, e.status, e.is_physical, e.sort_order, e.created_at, e.updated_at
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid)
  AND e.deleted_at IS NULL;

-- name: GetExtrasByUUIDs :many
SELECT e.id, e.uuid, e.code, e.name, e.description, e.extra_type, e.charge_type, e.min_quantity, e.max_quantity,
    e.max_total, e.channels::varchar[] AS channels, e.status, e.is_physical, e.sort_order, e.created_at, e.updated_at
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND e.deleted_at IS NULL;

-- name: GetExtraByCode :one
SELECT e.id, e.uuid
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND lower(e.code) = lower(sqlc.arg(code))
  AND e.deleted_at IS NULL;

-- name: ListExtras :many
-- default_price: the current default price row; stock_total: owned units
-- over every branch (physical extras).
SELECT e.id, e.uuid, e.code, e.name, e.description, e.extra_type, e.charge_type, e.min_quantity, e.max_quantity,
    e.max_total, e.channels::varchar[] AS channels, e.status, e.is_physical, e.sort_order, e.created_at, e.updated_at,
    dp.price AS default_price,
    COALESCE((
        SELECT SUM(s.quantity) FROM extra_branch_stock s
        WHERE s.organization_id = e.organization_id AND s.extra_id = e.id
    ), 0)::bigint AS stock_total,
    (
        SELECT COUNT(*) FROM extra_prices pp
        WHERE pp.organization_id = e.organization_id AND pp.extra_id = e.id AND pp.rental_product_id IS NOT NULL
          AND pp.valid_from <= sqlc.arg(now_at)::timestamptz
          AND (pp.valid_to IS NULL OR pp.valid_to > sqlc.arg(now_at)::timestamptz)
    )::bigint AS product_price_count
FROM extras e
LEFT JOIN extra_prices dp
    ON dp.organization_id = e.organization_id AND dp.extra_id = e.id AND dp.rental_product_id IS NULL
    AND dp.valid_from <= sqlc.arg(now_at)::timestamptz
    AND (dp.valid_to IS NULL OR dp.valid_to > sqlc.arg(now_at)::timestamptz)
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.deleted_at IS NULL
  AND (sqlc.narg(types)::text[] IS NULL OR e.extra_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(charge_types)::text[] IS NULL OR e.charge_type = ANY(sqlc.narg(charge_types)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR e.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(is_physical)::boolean IS NULL OR e.is_physical = sqlc.narg(is_physical))
  AND (sqlc.narg(channel)::text IS NULL OR e.channels IS NULL OR sqlc.narg(channel)::text = ANY(e.channels))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.code ILIKE '%' || sqlc.narg(q) || '%'
    OR e.name ILIKE '%' || sqlc.narg(q) || '%'
    OR e.description ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN e.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN e.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN e.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN e.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'extra_type' AND NOT sqlc.arg(sort_desc)::boolean THEN e.extra_type END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'extra_type' AND sqlc.arg(sort_desc)::boolean THEN e.extra_type END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN e.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN e.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'default_price' AND NOT sqlc.arg(sort_desc)::boolean THEN dp.price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'default_price' AND sqlc.arg(sort_desc)::boolean THEN dp.price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN e.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN e.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN e.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN e.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN e.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN e.created_at END DESC,
  e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountExtras :one
SELECT COUNT(*)::bigint
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.deleted_at IS NULL
  AND (sqlc.narg(types)::text[] IS NULL OR e.extra_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(charge_types)::text[] IS NULL OR e.charge_type = ANY(sqlc.narg(charge_types)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR e.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(is_physical)::boolean IS NULL OR e.is_physical = sqlc.narg(is_physical))
  AND (sqlc.narg(channel)::text IS NULL OR e.channels IS NULL OR sqlc.narg(channel)::text = ANY(e.channels))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.code ILIKE '%' || sqlc.narg(q) || '%'
    OR e.name ILIKE '%' || sqlc.narg(q) || '%'
    OR e.description ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListExtraUUIDs :many
-- Bulk "all matching" targets.
SELECT e.uuid
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.deleted_at IS NULL
  AND (sqlc.narg(types)::text[] IS NULL OR e.extra_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(charge_types)::text[] IS NULL OR e.charge_type = ANY(sqlc.narg(charge_types)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR e.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(is_physical)::boolean IS NULL OR e.is_physical = sqlc.narg(is_physical))
  AND (sqlc.narg(channel)::text IS NULL OR e.channels IS NULL OR sqlc.narg(channel)::text = ANY(e.channels))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.code ILIKE '%' || sqlc.narg(q) || '%'
    OR e.name ILIKE '%' || sqlc.narg(q) || '%'
    OR e.description ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY e.id
LIMIT sqlc.arg(limit_count);

-- name: ExtraFacets :many
SELECT 'extra_type'::text AS facet, e.extra_type::text AS value, COUNT(*)::bigint AS count
FROM extras e WHERE e.organization_id = sqlc.arg(organization_id) AND e.deleted_at IS NULL GROUP BY e.extra_type
UNION ALL
SELECT 'charge_type'::text, e.charge_type::text, COUNT(*)::bigint
FROM extras e WHERE e.organization_id = sqlc.arg(organization_id) AND e.deleted_at IS NULL GROUP BY e.charge_type
UNION ALL
SELECT 'status'::text, e.status::text, COUNT(*)::bigint
FROM extras e WHERE e.organization_id = sqlc.arg(organization_id) AND e.deleted_at IS NULL GROUP BY e.status
UNION ALL
SELECT 'is_physical'::text, e.is_physical::text, COUNT(*)::bigint
FROM extras e WHERE e.organization_id = sqlc.arg(organization_id) AND e.deleted_at IS NULL GROUP BY e.is_physical;

-- ---------------------------------------------------------------- prices

-- name: ListExtraPriceMatrix :many
-- Every live rental product with the extra's price effective at now_at
-- (NULL columns = the product falls back to the default price).
SELECT p.id AS product_id, p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    p.status AS product_status, s.uuid AS segment_uuid, s.name AS segment_name,
    ep.uuid AS price_uuid, ep.price, ep.valid_from, ep.valid_to
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
LEFT JOIN extra_prices ep
    ON ep.organization_id = p.organization_id AND ep.extra_id = sqlc.arg(extra_id) AND ep.rental_product_id = p.id
    AND ep.valid_from <= sqlc.arg(now_at)::timestamptz
    AND (ep.valid_to IS NULL OR ep.valid_to > sqlc.arg(now_at)::timestamptz)
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
ORDER BY p.code, p.id;

-- name: GetEffectiveExtraPrice :one
-- The price row of one extra and product (NULL product = default)
-- effective at now_at.
SELECT ep.id, ep.uuid, ep.price, ep.valid_from, ep.valid_to
FROM extra_prices ep
WHERE ep.organization_id = sqlc.arg(organization_id)
  AND ep.extra_id = sqlc.arg(extra_id)
  AND COALESCE(ep.rental_product_id, 0) = COALESCE(sqlc.narg(rental_product_id)::bigint, 0)
  AND ep.valid_from <= sqlc.arg(now_at)::timestamptz
  AND (ep.valid_to IS NULL OR ep.valid_to > sqlc.arg(now_at)::timestamptz);

-- name: ListScheduledExtraPrices :many
-- Price rows that start after now_at (scheduled changes).
SELECT ep.uuid, ep.rental_product_id, p.uuid AS product_uuid, ep.price, ep.valid_from, ep.valid_to
FROM extra_prices ep
LEFT JOIN rental_products p ON p.organization_id = ep.organization_id AND p.id = ep.rental_product_id
WHERE ep.organization_id = sqlc.arg(organization_id)
  AND ep.extra_id = sqlc.arg(extra_id)
  AND ep.valid_from > sqlc.arg(now_at)::timestamptz
ORDER BY ep.valid_from, ep.id;

-- name: ResolveExtraPrices :many
-- For each extra: the price of the product effective at now_at, else the
-- default price (product rows sort first).
SELECT DISTINCT ON (ep.extra_id) ep.extra_id, ep.uuid, ep.price, ep.rental_product_id
FROM extra_prices ep
WHERE ep.organization_id = sqlc.arg(organization_id)
  AND ep.extra_id = ANY(sqlc.arg(extra_ids)::bigint[])
  AND (ep.rental_product_id IS NULL OR ep.rental_product_id = sqlc.arg(rental_product_id))
  AND ep.valid_from <= sqlc.arg(now_at)::timestamptz
  AND (ep.valid_to IS NULL OR ep.valid_to > sqlc.arg(now_at)::timestamptz)
ORDER BY ep.extra_id, (ep.rental_product_id IS NULL), ep.valid_from DESC;

-- name: CloseExtraPrice :execrows
-- Ends the row that is effective at from_at (a price change from from_at).
UPDATE extra_prices
SET valid_to = sqlc.arg(from_at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id)
  AND extra_id = sqlc.arg(extra_id)
  AND COALESCE(rental_product_id, 0) = COALESCE(sqlc.narg(rental_product_id)::bigint, 0)
  AND valid_from < sqlc.arg(from_at)::timestamptz
  AND (valid_to IS NULL OR valid_to > sqlc.arg(from_at)::timestamptz);

-- name: DeleteFutureExtraPrices :execrows
-- Drops rows that would only start at or after from_at (never quoted:
-- quotes use the row effective at the quote time).
DELETE FROM extra_prices
WHERE organization_id = sqlc.arg(organization_id)
  AND extra_id = sqlc.arg(extra_id)
  AND COALESCE(rental_product_id, 0) = COALESCE(sqlc.narg(rental_product_id)::bigint, 0)
  AND valid_from >= sqlc.arg(from_at)::timestamptz;

-- name: InsertExtraPrice :one
INSERT INTO extra_prices (organization_id, extra_id, rental_product_id, price, valid_from, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(extra_id), sqlc.narg(rental_product_id), sqlc.arg(price),
    sqlc.arg(valid_from), sqlc.narg(created_by)
)
RETURNING uuid;

-- name: ListExtraPriceHistory :many
SELECT ep.uuid, ep.price, ep.valid_from, ep.valid_to, ep.created_at,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM extra_prices ep
LEFT JOIN rental_products p ON p.organization_id = ep.organization_id AND p.id = ep.rental_product_id
LEFT JOIN users u ON u.id = ep.created_by
WHERE ep.organization_id = sqlc.arg(organization_id)
  AND ep.extra_id = sqlc.arg(extra_id)
ORDER BY ep.created_at DESC, ep.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountExtraPriceHistory :one
SELECT COUNT(*)::bigint
FROM extra_prices ep
WHERE ep.organization_id = sqlc.arg(organization_id)
  AND ep.extra_id = sqlc.arg(extra_id);

-- name: ListExtraPriceExport :many
-- Price list export: current default and product prices of live extras.
SELECT e.uuid AS extra_uuid, e.code AS extra_code, e.name AS extra_name, e.charge_type,
    p.code AS product_code, p.title AS product_title, ep.price, ep.valid_from
FROM extra_prices ep
JOIN extras e ON e.organization_id = ep.organization_id AND e.id = ep.extra_id AND e.deleted_at IS NULL
LEFT JOIN rental_products p ON p.organization_id = ep.organization_id AND p.id = ep.rental_product_id
WHERE ep.organization_id = sqlc.arg(organization_id)
  AND ep.valid_from <= sqlc.arg(now_at)::timestamptz
  AND (ep.valid_to IS NULL OR ep.valid_to > sqlc.arg(now_at)::timestamptz)
ORDER BY e.code, p.code NULLS FIRST;

-- name: GetExtraProductByCode :one
SELECT p.id, p.uuid
FROM rental_products p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND lower(p.code) = lower(sqlc.arg(code))
  AND p.deleted_at IS NULL;

-- ----------------------------------------------------------------- stock

-- name: ListExtraStock :many
-- Every live branch (inside the branch scope) with the extra's stock.
SELECT b.id AS branch_id, b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name, b.is_active AS branch_active,
    COALESCE(s.quantity, 0)::int AS quantity, s.updated_at
FROM branches b
LEFT JOIN extra_branch_stock s ON s.organization_id = b.organization_id AND s.branch_id = b.id AND s.extra_id = sqlc.arg(extra_id)
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY b.code, b.id;

-- name: UpsertExtraStock :exec
INSERT INTO extra_branch_stock (organization_id, extra_id, branch_id, quantity)
VALUES (sqlc.arg(organization_id), sqlc.arg(extra_id), sqlc.arg(branch_id), sqlc.arg(quantity))
ON CONFLICT (organization_id, extra_id, branch_id) DO UPDATE SET quantity = EXCLUDED.quantity;

-- name: EnsureExtraStockRow :exec
-- Creates a zero stock row so the reserve path always has a row to lock.
INSERT INTO extra_branch_stock (organization_id, extra_id, branch_id, quantity)
VALUES (sqlc.arg(organization_id), sqlc.arg(extra_id), sqlc.arg(branch_id), 0)
ON CONFLICT (organization_id, extra_id, branch_id) DO NOTHING;

-- name: LockExtraStock :one
-- The serialisation point of every writer of one extra at one branch.
SELECT s.quantity
FROM extra_branch_stock s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.extra_id = sqlc.arg(extra_id)
  AND s.branch_id = sqlc.arg(branch_id)
FOR UPDATE;

-- name: GetExtraStock :one
-- Unlocked read (tests prove the lock is what prevents overselling).
SELECT s.quantity
FROM extra_branch_stock s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.extra_id = sqlc.arg(extra_id)
  AND s.branch_id = sqlc.arg(branch_id);

-- name: ListBranchStockForExtras :many
SELECT s.extra_id, s.quantity
FROM extra_branch_stock s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.branch_id = sqlc.arg(branch_id)
  AND s.extra_id = ANY(sqlc.arg(extra_ids)::bigint[]);

-- ----------------------------------------------------------- allocations

-- name: ListOverlappingEquipment :many
-- Active allocations of extras at a branch overlapping [from_at, to_at);
-- lapsed holds no longer count (lazy expiry).
SELECT a.extra_id, a.kind, a.starts_at, a.ends_at, a.quantity
FROM equipment_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.extra_id = ANY(sqlc.arg(extra_ids)::bigint[])
  AND a.branch_id = sqlc.arg(branch_id)
  AND a.status = 'active'
  AND (a.kind <> 'hold' OR a.expires_at > sqlc.arg(now_at)::timestamptz)
  AND a.period && tstzrange(sqlc.arg(from_at)::timestamptz, sqlc.arg(to_at)::timestamptz, '[)')
  AND (sqlc.narg(exclude_id)::bigint IS NULL OR a.id <> sqlc.narg(exclude_id)::bigint);

-- name: InsertEquipmentAllocation :one
INSERT INTO equipment_allocations (
    organization_id, extra_id, branch_id, quantity, kind, starts_at, ends_at, expires_at,
    vehicle_allocation_id, ref_type, ref_uuid, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(extra_id), sqlc.arg(branch_id), sqlc.arg(quantity), sqlc.arg(kind),
    sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.narg(expires_at), sqlc.narg(vehicle_allocation_id),
    sqlc.narg(ref_type), sqlc.narg(ref_uuid), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetVehicleAllocationForEquipment :one
-- The availability allocation an equipment hold is tied to, locked FOR
-- SHARE so a concurrent release waits for the link (trigger in 000069).
SELECT va.id, va.uuid, va.kind, va.status, va.starts_at, va.ends_at, va.expires_at, va.pickup_branch_id,
    va.return_branch_id, va.ref_type, va.ref_uuid
FROM vehicle_allocations va
WHERE va.organization_id = sqlc.arg(organization_id)
  AND va.uuid = sqlc.arg(uuid)
FOR SHARE;

-- name: GetEquipmentAllocation :one
SELECT a.id, a.uuid, a.quantity, a.kind, a.status, a.starts_at, a.ends_at, a.expires_at, a.ref_type, a.ref_uuid,
    a.note, a.released_at, a.release_reason, a.created_at, a.updated_at, a.branch_id,
    e.uuid AS extra_uuid, e.code AS extra_code, e.name AS extra_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    va.uuid AS vehicle_allocation_uuid,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM equipment_allocations a
JOIN extras e ON e.organization_id = a.organization_id AND e.id = a.extra_id
JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
LEFT JOIN vehicle_allocations va ON va.organization_id = a.organization_id AND va.id = a.vehicle_allocation_id
LEFT JOIN users u ON u.id = a.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: ListEquipmentAllocations :many
SELECT a.id, a.uuid, a.quantity, a.kind, a.status, a.starts_at, a.ends_at, a.expires_at, a.ref_type, a.ref_uuid,
    a.note, a.released_at, a.release_reason, a.created_at, a.updated_at, a.branch_id,
    e.uuid AS extra_uuid, e.code AS extra_code, e.name AS extra_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    va.uuid AS vehicle_allocation_uuid,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM equipment_allocations a
JOIN extras e ON e.organization_id = a.organization_id AND e.id = a.extra_id
JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
LEFT JOIN vehicle_allocations va ON va.organization_id = a.organization_id AND va.id = a.vehicle_allocation_id
LEFT JOIN users u ON u.id = a.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(extra_id)::bigint IS NULL OR a.extra_id = sqlc.narg(extra_id)::bigint)
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.code ILIKE '%' || sqlc.narg(q) || '%'
    OR e.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.starts_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND sqlc.arg(sort_desc)::boolean THEN a.starts_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'ends_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.ends_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'ends_at' AND sqlc.arg(sort_desc)::boolean THEN a.ends_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'quantity' AND NOT sqlc.arg(sort_desc)::boolean THEN a.quantity END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'quantity' AND sqlc.arg(sort_desc)::boolean THEN a.quantity END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN a.created_at END DESC,
  a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountEquipmentAllocations :one
SELECT COUNT(*)::bigint
FROM equipment_allocations a
JOIN extras e ON e.organization_id = a.organization_id AND e.id = a.extra_id
JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(extra_id)::bigint IS NULL OR a.extra_id = sqlc.narg(extra_id)::bigint)
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.code ILIKE '%' || sqlc.narg(q) || '%'
    OR e.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.note ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ReleaseEquipmentAllocation :execrows
-- Leaves "active" exactly once.
UPDATE equipment_allocations
SET status = sqlc.arg(status), released_at = sqlc.arg(released_at)::timestamptz,
    released_by = sqlc.narg(released_by), release_reason = sqlc.arg(release_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: ConfirmEquipmentAllocation :execrows
-- Hold -> reservation on the same row (stock is not consumed twice).
UPDATE equipment_allocations
SET kind = 'reservation', expires_at = NULL,
    ref_type = COALESCE(sqlc.narg(ref_type), ref_type), ref_uuid = COALESCE(sqlc.narg(ref_uuid), ref_uuid)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND kind = 'hold'
  AND status = 'active'
  AND expires_at > sqlc.arg(now_at)::timestamptz;

-- name: ListDueEquipmentOrganizations :many
SELECT DISTINCT a.organization_id
FROM equipment_allocations a
WHERE a.status = 'active' AND a.kind = 'hold' AND a.expires_at <= sqlc.arg(now_at)::timestamptz;

-- name: ExpireDueEquipment :execrows
UPDATE equipment_allocations
SET status = 'expired', released_at = sqlc.arg(now_at)::timestamptz, release_reason = 'expired'
WHERE organization_id = sqlc.arg(organization_id)
  AND status = 'active'
  AND kind = 'hold'
  AND expires_at <= sqlc.arg(now_at)::timestamptz;
