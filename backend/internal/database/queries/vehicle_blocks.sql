-- Tenant resource: vehicle usage blocks (TEC-694). Every query filters by
-- organization_id (docs/design/tenant-core.md §8). during is [start, end);
-- an unbounded end is an open-ended block. The vehicle (and with it the
-- caller's branch scope) is resolved by the use case before these run.

-- name: GetFleetOpsVehicle :one
-- A live vehicle with the fields the block / transfer rules need.
SELECT v.id, v.uuid, v.plate, v.status, v.home_branch_id, v.current_branch_id, v.is_active,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label
FROM vehicles v
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: CreateVehicleBlock :one
INSERT INTO vehicle_blocks (organization_id, vehicle_id, kind, during, reason, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(kind),
    tstzrange(sqlc.arg(starts_at)::timestamptz, sqlc.narg(ends_at)::timestamptz, '[)'),
    sqlc.arg(reason), sqlc.narg(created_by)
)
RETURNING uuid;

-- name: UpdateVehicleBlock :execrows
UPDATE vehicle_blocks
SET kind = sqlc.arg(kind),
    during = tstzrange(sqlc.arg(starts_at)::timestamptz, sqlc.narg(ends_at)::timestamptz, '[)'),
    reason = sqlc.arg(reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: ReleaseVehicleBlock :execrows
-- "Engeli kapat": the block ends at released_at.
UPDATE vehicle_blocks
SET during = tstzrange(lower(during), sqlc.arg(released_at)::timestamptz, '[)'),
    released_at = sqlc.arg(released_at)::timestamptz,
    released_by = sqlc.narg(released_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: SoftDeleteVehicleBlock :execrows
UPDATE vehicle_blocks
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetVehicleBlock :one
SELECT b.id, b.uuid, b.vehicle_id, b.kind, b.reason, b.released_at, b.created_at, b.updated_at,
    lower(b.during)::timestamptz AS starts_at,
    upper(b.during)::timestamptz AS ends_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    ru.uuid AS released_by_uuid, ru.name AS released_by_name, ru.surname AS released_by_surname, ru.email AS released_by_email
FROM vehicle_blocks b
LEFT JOIN users cu ON cu.id = b.created_by
LEFT JOIN users ru ON ru.id = b.released_by
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.vehicle_id = sqlc.arg(vehicle_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: ListVehicleBlocks :many
-- Blocks of one vehicle, newest start first. states: scheduled (starts
-- after now) | active (covers now) | ended (ended before now).
SELECT b.id, b.uuid, b.vehicle_id, b.kind, b.reason, b.released_at, b.created_at, b.updated_at,
    lower(b.during)::timestamptz AS starts_at,
    upper(b.during)::timestamptz AS ends_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    ru.uuid AS released_by_uuid, ru.name AS released_by_name, ru.surname AS released_by_surname, ru.email AS released_by_email
FROM vehicle_blocks b
LEFT JOIN users cu ON cu.id = b.created_by
LEFT JOIN users ru ON ru.id = b.released_by
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.vehicle_id = sqlc.arg(vehicle_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(kinds)::text[] IS NULL OR b.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (
    sqlc.narg(states)::text[] IS NULL
    OR ('scheduled' = ANY(sqlc.narg(states)::text[]) AND lower(b.during) > sqlc.arg(now)::timestamptz)
    OR ('active' = ANY(sqlc.narg(states)::text[]) AND b.during @> sqlc.arg(now)::timestamptz)
    OR ('ended' = ANY(sqlc.narg(states)::text[]) AND upper(b.during) <= sqlc.arg(now)::timestamptz)
  )
ORDER BY lower(b.during) DESC, b.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountVehicleBlocks :one
SELECT COUNT(*)::bigint
FROM vehicle_blocks b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.vehicle_id = sqlc.arg(vehicle_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(kinds)::text[] IS NULL OR b.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (
    sqlc.narg(states)::text[] IS NULL
    OR ('scheduled' = ANY(sqlc.narg(states)::text[]) AND lower(b.during) > sqlc.arg(now)::timestamptz)
    OR ('active' = ANY(sqlc.narg(states)::text[]) AND b.during @> sqlc.arg(now)::timestamptz)
    OR ('ended' = ANY(sqlc.narg(states)::text[]) AND upper(b.during) <= sqlc.arg(now)::timestamptz)
  );

-- name: ListOverlappingVehicleBlocks :many
-- Live blocks of a vehicle that overlap [starts_at, ends_at) (NULL end =
-- unbounded), except exclude_id (the block being changed).
SELECT b.uuid, b.kind, lower(b.during)::timestamptz AS starts_at, upper(b.during)::timestamptz AS ends_at
FROM vehicle_blocks b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.vehicle_id = sqlc.arg(vehicle_id)
  AND b.deleted_at IS NULL
  AND b.during && tstzrange(sqlc.arg(starts_at)::timestamptz, sqlc.narg(ends_at)::timestamptz, '[)')
  AND b.id <> sqlc.arg(exclude_id)::bigint
ORDER BY lower(b.during);

-- name: ListVehicleBusyIntervals :many
-- Busy intervals of one vehicle that overlap [from_at, to_at) (NULL end =
-- unbounded), from the vehicle_busy_intervals view; kinds NULL = all.
SELECT bi.kind, bi.ref_uuid,
    lower(bi.during)::timestamptz AS starts_at,
    upper(bi.during)::timestamptz AS ends_at,
    bi.branch_after_id
FROM vehicle_busy_intervals bi
WHERE bi.organization_id = sqlc.arg(organization_id)
  AND bi.vehicle_id = sqlc.arg(vehicle_id)
  AND NOT isempty(bi.during)
  AND bi.during && tstzrange(sqlc.arg(from_at)::timestamptz, sqlc.narg(to_at)::timestamptz, '[)')
  AND (sqlc.narg(kinds)::text[] IS NULL OR bi.kind = ANY(sqlc.narg(kinds)::text[]))
ORDER BY lower(bi.during), bi.kind;
