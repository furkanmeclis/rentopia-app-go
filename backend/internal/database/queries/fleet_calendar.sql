-- Fleet calendar (TEC-696): one page of vehicles (rows) and what keeps them
-- busy inside a time window (bars). Every query filters by organization_id
-- first (docs/design/tenant-core.md §8); the vehicles page applies the
-- caller's branch scope (home OR current branch), the interval queries only
-- read vehicles of that page.

-- name: ListFleetCalendarVehicles :many
-- One page of live vehicles, plate order. total is the number of matching
-- vehicles (window count, one round trip).
SELECT v.id, v.uuid, v.plate, v.model_year, v.status, v.is_active,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
    hb.uuid AS home_branch_uuid, hb.code AS home_branch_code, hb.name AS home_branch_name,
    cb.uuid AS current_branch_uuid, cb.code AS current_branch_code, cb.name AS current_branch_name,
    COUNT(*) OVER ()::bigint AS total
FROM vehicles v
JOIN branches hb ON hb.organization_id = v.organization_id AND hb.id = v.home_branch_id
JOIN branches cb ON cb.organization_id = v.organization_id AND cb.id = v.current_branch_id
LEFT JOIN catalog_types t ON t.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = t.model_id
LEFT JOIN catalog_brands tb ON tb.id = t.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.deleted_at IS NULL
  -- Branch scope of the caller: home OR current branch; NULL = every branch.
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(vehicle_uuids)::uuid[] IS NULL OR v.uuid = ANY(sqlc.narg(vehicle_uuids)::uuid[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR v.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(statuses)::text[] IS NULL OR v.status = ANY(sqlc.narg(statuses)::text[]))
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
  )
ORDER BY v.plate_key ASC, v.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListFleetCalendarIntervals :many
-- Busy intervals (vehicle_busy_intervals: blocks and non-cancelled
-- transfers) of the given vehicles that overlap [from_at, to_at), with the
-- block / transfer details the calendar bars show.
SELECT bi.vehicle_id, bi.kind, bi.ref_uuid,
    lower(bi.during)::timestamptz AS starts_at,
    upper(bi.during)::timestamptz AS ends_at,
    COALESCE(b.kind, '')::text AS block_kind,
    COALESCE(b.reason, '')::text AS block_reason,
    b.released_at AS block_released_at,
    COALESCE(tr.status, '')::text AS transfer_status,
    lower(tr.planned)::timestamptz AS planned_departure,
    upper(tr.planned)::timestamptz AS planned_arrival,
    tr.actual_departure, tr.actual_arrival,
    COALESCE(tr.from_branch_id, 0)::bigint AS from_branch_id,
    COALESCE(tr.to_branch_id, 0)::bigint AS to_branch_id,
    fb.uuid AS from_branch_uuid, fb.code AS from_branch_code, fb.name AS from_branch_name,
    tb.uuid AS to_branch_uuid, tb.code AS to_branch_code, tb.name AS to_branch_name
FROM vehicle_busy_intervals bi
LEFT JOIN vehicle_blocks b
    ON bi.kind = 'block' AND b.organization_id = bi.organization_id AND b.uuid = bi.ref_uuid
LEFT JOIN vehicle_transfers tr
    ON bi.kind = 'transfer' AND tr.organization_id = bi.organization_id AND tr.uuid = bi.ref_uuid
LEFT JOIN branches fb ON fb.organization_id = tr.organization_id AND fb.id = tr.from_branch_id
LEFT JOIN branches tb ON tb.organization_id = tr.organization_id AND tb.id = tr.to_branch_id
WHERE bi.organization_id = sqlc.arg(organization_id)
  AND bi.vehicle_id = ANY(sqlc.arg(vehicle_ids)::bigint[])
  AND NOT isempty(bi.during)
  AND bi.during && tstzrange(sqlc.arg(from_at)::timestamptz, sqlc.arg(to_at)::timestamptz, '[)')
  AND bi.kind = ANY(sqlc.arg(kinds)::text[])
ORDER BY bi.vehicle_id, lower(bi.during), bi.kind;

-- name: ListFleetCalendarMaintenance :many
-- Maintenance of the given vehicles on a day in [from_day, to_day): open
-- plans by their planned date, done items by their done date. Plans with a
-- km target only have no day and are not on the calendar.
SELECT mt.vehicle_id, mt.uuid, mt.kind, mt.maintenance_type,
    COALESCE(mt.done_date, mt.planned_date)::date AS day,
    mt.planned_km, mt.done_km, mt.service_provider, mt.note
FROM vehicle_maintenance mt
WHERE mt.organization_id = sqlc.arg(organization_id)
  AND mt.vehicle_id = ANY(sqlc.arg(vehicle_ids)::bigint[])
  AND mt.deleted_at IS NULL
  AND COALESCE(mt.done_date, mt.planned_date) >= sqlc.arg(from_day)::date
  AND COALESCE(mt.done_date, mt.planned_date) < sqlc.arg(to_day)::date
ORDER BY mt.vehicle_id, COALESCE(mt.done_date, mt.planned_date), mt.id;
