-- Availability engine (TEC-697, docs/design/availability-engine.md). Every
-- query filters by organization_id (docs/design/tenant-core.md §8). The
-- engine reads vehicles, blocks, transfers and allocations of one
-- eligibility component inside its advisory lock and writes the plan back
-- in the same transaction.

-- name: GetAvailabilityFingerprint :one
-- Changes whenever the inputs of the cached eligibility graph change
-- (vehicles, products, versions, rental groups and their criteria).
SELECT md5(concat_ws('|',
    (SELECT count(*)::text || ':' || COALESCE(max(v.updated_at)::text, '')
        FROM vehicles v WHERE v.organization_id = sqlc.arg(organization_id)),
    (SELECT count(*)::text || ':' || COALESCE(max(p.updated_at)::text, '')
        FROM rental_products p WHERE p.organization_id = sqlc.arg(organization_id)),
    (SELECT count(*)::text
        FROM rental_product_versions pv WHERE pv.organization_id = sqlc.arg(organization_id)),
    (SELECT count(*)::text || ':' || COALESCE(max(g.updated_at)::text, '')
        FROM rental_groups g WHERE g.organization_id = sqlc.arg(organization_id)),
    (SELECT count(*)::text || ':' || COALESCE(max(gm.created_at)::text, '')
        FROM rental_group_catalog_models gm WHERE gm.organization_id = sqlc.arg(organization_id)),
    (SELECT count(*)::text || ':' || COALESCE(max(gt.created_at)::text, '')
        FROM rental_group_catalog_types gt WHERE gt.organization_id = sqlc.arg(organization_id))
))::text AS fingerprint;

-- name: ListAvailabilityProductVersions :many
-- Every version of every product of the organization (live or not: active
-- allocations keep the version they were sold with), with the product.
SELECT pv.id, pv.uuid, pv.organization_id, pv.product_id, pv.version, pv.guarantee_type, pv.vehicle_id,
    pv.variant_level, pv.catalog_model_id, pv.catalog_type_id, pv.rental_group_id, pv.guarantee_label,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title, p.status AS product_status,
    p.is_active AS product_is_active, (p.deleted_at IS NULL)::boolean AS product_live, p.current_version
FROM rental_product_versions pv
JOIN rental_products p ON p.organization_id = pv.organization_id AND p.id = pv.product_id
WHERE pv.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
ORDER BY p.code, pv.version;

-- name: LockAvailabilityComponent :exec
-- Serialises the planners of one eligibility component (transaction-scoped).
SELECT pg_advisory_xact_lock(sqlc.arg(lock_key)::bigint);

-- name: ListAvailabilityVehicleStates :many
-- Fresh location / liveness of the component's vehicles (inside the lock).
SELECT v.id, v.current_branch_id, (v.deleted_at IS NULL AND v.is_active)::boolean AS usable
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.id = ANY(sqlc.arg(vehicle_ids)::bigint[]);

-- name: ListAvailabilitySegments :many
-- Blocks and open transfers of the vehicles that end after now (completed
-- transfers are already reflected in current_branch_id).
SELECT bi.vehicle_id, bi.kind, bi.ref_uuid,
    lower(bi.during)::timestamptz AS starts_at,
    upper(bi.during)::timestamptz AS ends_at,
    COALESCE(t.from_branch_id, 0)::bigint AS from_branch_id,
    COALESCE(bi.branch_after_id, 0)::bigint AS to_branch_id,
    COALESCE(t.status, '')::text AS transfer_status
FROM vehicle_busy_intervals bi
LEFT JOIN vehicle_transfers t
    ON bi.kind = 'transfer' AND t.organization_id = bi.organization_id AND t.uuid = bi.ref_uuid
WHERE bi.organization_id = sqlc.arg(organization_id)
  AND bi.vehicle_id = ANY(sqlc.arg(vehicle_ids)::bigint[])
  AND bi.kind IN ('block', 'transfer')
  AND NOT isempty(bi.during)
  AND (upper_inf(bi.during) OR upper(bi.during) > sqlc.arg(now_at)::timestamptz)
  AND (t.status IS NULL OR t.status IN ('planned', 'in_transit'))
ORDER BY bi.vehicle_id, lower(bi.during);

-- name: ListAvailabilityAllocations :many
-- Active allocations of the vehicles that end after now. Holds past their
-- expiry are returned too (due = true): the caller expires them first.
SELECT a.id, a.uuid, a.vehicle_id, a.rental_product_id, a.product_version, a.kind, a.starts_at, a.ends_at,
    lower(a.period)::timestamptz AS period_start, upper(a.period)::timestamptz AS period_end,
    a.pickup_branch_id, a.return_branch_id, a.fleet_pool, a.channel, a.plate_fixed, a.expires_at,
    (a.kind = 'hold' AND a.expires_at <= sqlc.arg(now_at)::timestamptz)::boolean AS due
FROM vehicle_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.vehicle_id = ANY(sqlc.arg(vehicle_ids)::bigint[])
  AND a.status = 'active'
  AND upper(a.period) > sqlc.arg(now_at)::timestamptz
ORDER BY a.vehicle_id, lower(a.period);

-- name: ListProductCommitments :many
-- Active, unexpired allocations of a product whose customer range overlaps
-- [from_at, to_at): channel rule accounting.
SELECT a.id, a.channel, a.pickup_branch_id, a.starts_at, a.ends_at
FROM vehicle_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.rental_product_id = sqlc.arg(rental_product_id)
  AND a.status = 'active'
  AND (a.kind <> 'hold' OR a.expires_at > sqlc.arg(now_at)::timestamptz)
  AND a.starts_at < sqlc.arg(to_at)::timestamptz
  AND a.ends_at > sqlc.arg(from_at)::timestamptz;

-- name: InsertAllocation :one
INSERT INTO vehicle_allocations (
    organization_id, vehicle_id, rental_product_id, product_version, kind, starts_at, ends_at, prep_minutes, period,
    pickup_branch_id, return_branch_id, pickup_point_id, return_point_id, fleet_pool, channel, plate_fixed,
    expires_at, ref_type, ref_uuid, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(rental_product_id), sqlc.arg(product_version),
    sqlc.arg(kind), sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.arg(prep_minutes),
    tstzrange(sqlc.arg(starts_at)::timestamptz, sqlc.arg(period_end)::timestamptz, '[)'),
    sqlc.arg(pickup_branch_id), sqlc.arg(return_branch_id), sqlc.narg(pickup_point_id), sqlc.narg(return_point_id),
    sqlc.arg(fleet_pool), sqlc.arg(channel), sqlc.arg(plate_fixed), sqlc.narg(expires_at),
    sqlc.narg(ref_type), sqlc.narg(ref_uuid), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: MoveAllocation :execrows
-- A reassignment step; matches only when the row is still where the plan
-- found it (optimistic check: 0 rows = the plan is stale, retry).
UPDATE vehicle_allocations
SET vehicle_id = sqlc.arg(to_vehicle_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND vehicle_id = sqlc.arg(from_vehicle_id)
  AND status = 'active';

-- name: ReplanAllocation :execrows
-- Date change / plate assignment of one allocation (same optimistic check).
UPDATE vehicle_allocations
SET vehicle_id = sqlc.arg(to_vehicle_id),
    starts_at = sqlc.arg(starts_at),
    ends_at = sqlc.arg(ends_at),
    prep_minutes = sqlc.arg(prep_minutes),
    period = tstzrange(sqlc.arg(starts_at)::timestamptz, sqlc.arg(period_end)::timestamptz, '[)'),
    pickup_branch_id = sqlc.arg(pickup_branch_id),
    return_branch_id = sqlc.arg(return_branch_id),
    pickup_point_id = sqlc.narg(pickup_point_id),
    return_point_id = sqlc.narg(return_point_id),
    plate_fixed = sqlc.arg(plate_fixed)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND vehicle_id = sqlc.arg(from_vehicle_id)
  AND status = 'active';

-- name: ReleaseAllocation :execrows
-- Cancel / release: leaves "active" exactly once.
UPDATE vehicle_allocations
SET status = sqlc.arg(status), released_at = sqlc.arg(released_at)::timestamptz,
    released_by = sqlc.narg(released_by), release_reason = sqlc.arg(release_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: ExtendHold :execrows
UPDATE vehicle_allocations
SET expires_at = sqlc.arg(expires_at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND kind = 'hold'
  AND status = 'active'
  AND expires_at > sqlc.arg(now_at)::timestamptz;

-- name: ConfirmAllocation :execrows
-- Hold -> confirmed reservation (TEC-706): same row, so capacity is never
-- consumed twice.
UPDATE vehicle_allocations
SET kind = 'reservation', expires_at = NULL, ref_type = sqlc.narg(ref_type), ref_uuid = sqlc.narg(ref_uuid)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND kind = 'hold'
  AND status = 'active'
  AND expires_at > sqlc.arg(now_at)::timestamptz;

-- name: GetAllocationRef :one
-- The planning facts of one allocation by uuid.
SELECT a.id, a.uuid, a.vehicle_id, a.rental_product_id, a.product_version, a.kind, a.status, a.starts_at, a.ends_at,
    a.pickup_branch_id, a.return_branch_id, a.pickup_point_id, a.return_point_id, a.plate_fixed, a.expires_at,
    a.channel, a.fleet_pool
FROM vehicle_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: GetAllocation :one
SELECT a.id, a.uuid, a.kind, a.status, a.starts_at, a.ends_at, a.prep_minutes, upper(a.period)::timestamptz AS ready_at,
    a.fleet_pool, a.channel, a.plate_fixed, a.expires_at, a.ref_type, a.ref_uuid, a.note,
    a.released_at, a.release_reason, a.created_at, a.updated_at, a.product_version,
    a.pickup_branch_id, a.return_branch_id,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    pp.uuid AS pickup_point_uuid, pp.name AS pickup_point_name,
    rp.uuid AS return_point_uuid, rp.name AS return_point_name,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM vehicle_allocations a
JOIN rental_products p ON p.organization_id = a.organization_id AND p.id = a.rental_product_id
JOIN vehicles v ON v.organization_id = a.organization_id AND v.id = a.vehicle_id
JOIN branches pb ON pb.organization_id = a.organization_id AND pb.id = a.pickup_branch_id
JOIN branches rb ON rb.organization_id = a.organization_id AND rb.id = a.return_branch_id
LEFT JOIN service_points pp ON pp.organization_id = a.organization_id AND pp.id = a.pickup_point_id
LEFT JOIN service_points rp ON rp.organization_id = a.organization_id AND rp.id = a.return_point_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN users cu ON cu.id = a.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: ListAllocationUUIDs :many
-- One page of allocations (newest first) in the caller's branch scope
-- (pick-up OR return branch); the caller hydrates each with GetAllocation.
SELECT a.uuid, COUNT(*) OVER ()::bigint AS total
FROM vehicle_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR a.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR a.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  -- Unexpired only when listing active holds.
  AND (NOT sqlc.arg(live_only)::boolean OR a.kind <> 'hold' OR a.expires_at > sqlc.arg(now_at)::timestamptz)
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListDueHoldOrganizations :many
-- Organizations with active holds past their expiry (expiry sweep).
SELECT DISTINCT a.organization_id
FROM vehicle_allocations a
WHERE a.kind = 'hold' AND a.status = 'active' AND a.expires_at <= sqlc.arg(now_at)::timestamptz
LIMIT 1000;

-- name: ListDueHolds :many
SELECT a.id, a.uuid, a.vehicle_id
FROM vehicle_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.kind = 'hold' AND a.status = 'active' AND a.expires_at <= sqlc.arg(now_at)::timestamptz
ORDER BY a.expires_at
LIMIT 500;

-- name: GetAvailabilityBranch :one
SELECT b.id, b.uuid, b.code, b.name, b.prep_minutes, b.is_active
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: ListAvailabilityBranchesByIDs :many
SELECT b.id, b.uuid, b.code, b.name, b.prep_minutes, b.is_active
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.id = ANY(sqlc.arg(ids)::bigint[]);

-- name: GetAvailabilityServicePoint :one
SELECT sp.id, sp.uuid, sp.name, sp.is_active, sp.branch_id
FROM service_points sp
WHERE sp.organization_id = sqlc.arg(organization_id)
  AND sp.uuid = sqlc.arg(uuid);

-- name: GetAvailabilityProduct :one
SELECT p.id, p.uuid, p.code, p.title, p.status, p.is_active, p.current_version
FROM rental_products p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: GetAvailabilityVehicle :one
SELECT v.id, v.uuid, v.plate, v.home_branch_id, v.current_branch_id
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- ------------------------------------------------- channel capacity rules

-- name: ListActiveCapacityRules :many
SELECT r.id, r.uuid, r.rule_type, r.channel, r.rental_product_id, r.branch_id, r.quantity
FROM channel_capacity_rules r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND r.is_active;

-- name: ListCapacityRules :many
SELECT r.id, r.uuid, r.rule_type, r.channel, r.quantity, r.note, r.is_active, r.created_at, r.updated_at,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    COUNT(*) OVER ()::bigint AS total
FROM channel_capacity_rules r
LEFT JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
LEFT JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (sqlc.narg(rule_types)::text[] IS NULL OR r.rule_type = ANY(sqlc.narg(rule_types)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR r.channel = ANY(sqlc.narg(channels)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
    OR r.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'quantity' AND NOT sqlc.arg(sort_desc)::boolean THEN r.quantity END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'quantity' AND sqlc.arg(sort_desc)::boolean THEN r.quantity END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'rule_type' AND NOT sqlc.arg(sort_desc)::boolean THEN r.rule_type END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'rule_type' AND sqlc.arg(sort_desc)::boolean THEN r.rule_type END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
    r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetCapacityRule :one
SELECT r.id, r.uuid, r.rule_type, r.channel, r.rental_product_id, r.branch_id, r.quantity, r.note, r.is_active,
    r.created_at, r.updated_at,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name
FROM channel_capacity_rules r
LEFT JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
LEFT JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid)
  AND r.deleted_at IS NULL;

-- name: CreateCapacityRule :one
INSERT INTO channel_capacity_rules (
    organization_id, rule_type, channel, rental_product_id, branch_id, quantity, note, is_active, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rule_type), sqlc.narg(channel), sqlc.narg(rental_product_id),
    sqlc.narg(branch_id), sqlc.arg(quantity), sqlc.arg(note), sqlc.arg(is_active), sqlc.narg(created_by)
)
RETURNING uuid;

-- name: UpdateCapacityRule :execrows
UPDATE channel_capacity_rules
SET rule_type = sqlc.arg(rule_type), channel = sqlc.narg(channel), rental_product_id = sqlc.narg(rental_product_id),
    branch_id = sqlc.narg(branch_id), quantity = sqlc.arg(quantity), note = sqlc.arg(note), is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: SoftDeleteCapacityRule :execrows
UPDATE channel_capacity_rules
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;
