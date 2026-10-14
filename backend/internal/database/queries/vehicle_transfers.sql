-- Tenant resource: vehicle transfers between branches (TEC-695). Every
-- query filters by organization_id (docs/design/tenant-core.md §8).
--
-- Branch scope: a transfer is visible when its from OR to branch is in the
-- caller's scope (branch_ids NULL = every branch).

-- name: CreateVehicleTransfer :one
INSERT INTO vehicle_transfers (
    organization_id, vehicle_id, from_branch_id, to_branch_id, responsible_user_id,
    planned, cost, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(from_branch_id), sqlc.arg(to_branch_id),
    sqlc.narg(responsible_user_id),
    tstzrange(sqlc.arg(planned_departure)::timestamptz, sqlc.arg(planned_arrival)::timestamptz, '[)'),
    sqlc.narg(cost), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING uuid;

-- name: UpdateVehicleTransferPlan :execrows
-- Only planned transfers can be re-planned.
UPDATE vehicle_transfers
SET from_branch_id = sqlc.arg(from_branch_id),
    to_branch_id = sqlc.arg(to_branch_id),
    responsible_user_id = sqlc.narg(responsible_user_id),
    planned = tstzrange(sqlc.arg(planned_departure)::timestamptz, sqlc.arg(planned_arrival)::timestamptz, '[)'),
    cost = sqlc.narg(cost),
    note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status = 'planned';

-- name: SetVehicleTransferCost :execrows
-- Cost and note stay editable after departure (fuel, tolls, driver).
UPDATE vehicle_transfers
SET cost = sqlc.narg(cost),
    note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status IN ('in_transit', 'completed');

-- name: StartVehicleTransfer :execrows
UPDATE vehicle_transfers
SET status = 'in_transit',
    actual_departure = sqlc.arg(actual_departure)::timestamptz
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status = 'planned';

-- name: CompleteVehicleTransfer :execrows
UPDATE vehicle_transfers
SET status = 'completed',
    actual_arrival = sqlc.arg(actual_arrival)::timestamptz
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status = 'in_transit';

-- name: CancelVehicleTransfer :execrows
UPDATE vehicle_transfers
SET status = 'cancelled',
    cancelled_at = NOW(),
    note = CASE WHEN sqlc.arg(note)::text = '' THEN note ELSE sqlc.arg(note)::text END
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status IN ('planned', 'in_transit');

-- name: GetVehicleTransfer :one
SELECT t.id, t.uuid, t.vehicle_id, t.from_branch_id, t.to_branch_id, t.status, t.note, t.cost,
    t.actual_departure, t.actual_arrival, t.cancelled_at, t.created_at, t.updated_at,
    lower(t.planned)::timestamptz AS planned_departure,
    upper(t.planned)::timestamptz AS planned_arrival,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
    fb.uuid AS from_branch_uuid, fb.code AS from_branch_code, fb.name AS from_branch_name,
    tb2.uuid AS to_branch_uuid, tb2.code AS to_branch_code, tb2.name AS to_branch_name,
    ru.uuid AS responsible_uuid, ru.name AS responsible_name, ru.surname AS responsible_surname, ru.email AS responsible_email
FROM vehicle_transfers t
JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = t.vehicle_id
JOIN branches fb ON fb.organization_id = t.organization_id AND fb.id = t.from_branch_id
JOIN branches tb2 ON tb2.organization_id = t.organization_id AND tb2.id = t.to_branch_id
LEFT JOIN users ru ON ru.id = t.responsible_user_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands cb ON cb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = cb.display_brand_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: ListVehicleTransfers :many
SELECT t.id, t.uuid, t.vehicle_id, t.from_branch_id, t.to_branch_id, t.status, t.note, t.cost,
    t.actual_departure, t.actual_arrival, t.cancelled_at, t.created_at, t.updated_at,
    lower(t.planned)::timestamptz AS planned_departure,
    upper(t.planned)::timestamptz AS planned_arrival,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
    fb.uuid AS from_branch_uuid, fb.code AS from_branch_code, fb.name AS from_branch_name,
    tb2.uuid AS to_branch_uuid, tb2.code AS to_branch_code, tb2.name AS to_branch_name,
    ru.uuid AS responsible_uuid, ru.name AS responsible_name, ru.surname AS responsible_surname, ru.email AS responsible_email
FROM vehicle_transfers t
JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = t.vehicle_id
JOIN branches fb ON fb.organization_id = t.organization_id AND fb.id = t.from_branch_id
JOIN branches tb2 ON tb2.organization_id = t.organization_id AND tb2.id = t.to_branch_id
LEFT JOIN users ru ON ru.id = t.responsible_user_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands cb ON cb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = cb.display_brand_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR t.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR t.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(from_branch_uuids)::uuid[] IS NULL OR fb.uuid = ANY(sqlc.narg(from_branch_uuids)::uuid[]))
  AND (sqlc.narg(to_branch_uuids)::uuid[] IS NULL OR tb2.uuid = ANY(sqlc.narg(to_branch_uuids)::uuid[]))
  AND (sqlc.narg(vehicle_uuid)::uuid IS NULL OR v.uuid = sqlc.narg(vehicle_uuid)::uuid)
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_key LIKE '%' || upper(regexp_replace(sqlc.narg(q), '[^A-Za-z0-9]', '', 'g')) || '%'
    OR t.note ILIKE '%' || sqlc.narg(q) || '%'
    OR fb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR tb2.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'planned_departure' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(t.planned) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'planned_departure' AND sqlc.arg(sort_desc)::boolean THEN lower(t.planned) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'planned_arrival' AND NOT sqlc.arg(sort_desc)::boolean THEN upper(t.planned) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'planned_arrival' AND sqlc.arg(sort_desc)::boolean THEN upper(t.planned) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN t.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN t.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'cost' AND NOT sqlc.arg(sort_desc)::boolean THEN t.cost END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'cost' AND sqlc.arg(sort_desc)::boolean THEN t.cost END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'plate' AND NOT sqlc.arg(sort_desc)::boolean THEN v.plate_key END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'plate' AND sqlc.arg(sort_desc)::boolean THEN v.plate_key END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN t.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
  t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountVehicleTransfers :one
SELECT COUNT(*)::bigint
FROM vehicle_transfers t
JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = t.vehicle_id
JOIN branches fb ON fb.organization_id = t.organization_id AND fb.id = t.from_branch_id
JOIN branches tb2 ON tb2.organization_id = t.organization_id AND tb2.id = t.to_branch_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR t.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR t.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(from_branch_uuids)::uuid[] IS NULL OR fb.uuid = ANY(sqlc.narg(from_branch_uuids)::uuid[]))
  AND (sqlc.narg(to_branch_uuids)::uuid[] IS NULL OR tb2.uuid = ANY(sqlc.narg(to_branch_uuids)::uuid[]))
  AND (sqlc.narg(vehicle_uuid)::uuid IS NULL OR v.uuid = sqlc.narg(vehicle_uuid)::uuid)
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_key LIKE '%' || upper(regexp_replace(sqlc.narg(q), '[^A-Za-z0-9]', '', 'g')) || '%'
    OR t.note ILIKE '%' || sqlc.narg(q) || '%'
    OR fb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR tb2.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListVehicleTransferFacets :many
-- facet: status | from_branch | to_branch, in the caller's branch scope.
WITH scoped AS (
    SELECT t.organization_id, t.status, t.from_branch_id, t.to_branch_id
    FROM vehicle_transfers t
    WHERE t.organization_id = sqlc.arg(organization_id)
      AND (
        sqlc.narg(branch_ids)::bigint[] IS NULL
        OR t.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR t.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
      )
)
SELECT 'status'::text AS facet, s.status::text AS value, ''::text AS label, COUNT(*)::bigint AS count
FROM scoped s GROUP BY s.status
UNION ALL
SELECT 'from_branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.from_branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'to_branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.to_branch_id
GROUP BY b.uuid, b.name;

-- name: ListOverlappingVehicleTransfers :many
-- Open (planned / in transit) transfers of a vehicle overlapping
-- [starts_at, ends_at) (NULL end = unbounded), except exclude_id.
SELECT t.uuid, t.status, lower(t.planned)::timestamptz AS starts_at, upper(t.planned)::timestamptz AS ends_at
FROM vehicle_transfers t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.vehicle_id = sqlc.arg(vehicle_id)
  AND t.status IN ('planned', 'in_transit')
  AND t.planned && tstzrange(sqlc.arg(starts_at)::timestamptz, sqlc.narg(ends_at)::timestamptz, '[)')
  AND t.id <> sqlc.arg(exclude_id)::bigint
ORDER BY lower(t.planned);

-- name: ListVehicleMovements :many
-- Every non-cancelled transfer of a vehicle, oldest first: the input of
-- LocationAt (where is the vehicle at time t).
SELECT t.uuid, t.status, t.from_branch_id, t.to_branch_id,
    lower(t.planned)::timestamptz AS planned_departure,
    upper(t.planned)::timestamptz AS planned_arrival,
    t.actual_departure, t.actual_arrival
FROM vehicle_transfers t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.vehicle_id = sqlc.arg(vehicle_id)
  AND t.status <> 'cancelled'
ORDER BY COALESCE(t.actual_departure, lower(t.planned)), t.id;
