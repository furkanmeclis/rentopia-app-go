-- Field tasks (TEC-724): the field view of operation_tasks (place,
-- contact, meeting details, assignee, timeline). Branch scope: the task's
-- branch or, for a transfer, its destination. The vehicle of a delivery
-- task opened before the rental is the reservation's planned vehicle.

-- name: GetFieldTask :one
SELECT t.id, t.uuid, t.task_type, t.status, t.origin, t.branch_id, t.due_at, t.planned_minutes, t.note,
    t.started_at, t.completed_at, t.completion_note, t.assigned_at, t.issue_note, t.issue_reported_at,
    t.location_kind, t.address, t.district, t.city, t.latitude, t.longitude,
    t.contact_name, t.contact_phone, t.flight_number, t.meeting_note, t.created_at, t.updated_at,
    t.rental_id, t.reservation_id, t.assignee_id, t.to_branch_id, t.transfer_id,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    tb.uuid AS to_branch_uuid, tb.code AS to_branch_code, tb.name AS to_branch_name,
    sp.uuid AS point_uuid, sp.name AS point_name, sp.type AS point_type,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference, res.status AS reservation_status,
    res.starts_at AS reservation_starts_at, res.ends_at AS reservation_ends_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate, v.id AS resolved_vehicle_id,
    au.uuid AS assignee_uuid, au.name AS assignee_name, au.surname AS assignee_surname, au.email AS assignee_email,
    cu.uuid AS completed_by_uuid, cu.name AS completed_by_name, cu.surname AS completed_by_surname, cu.email AS completed_by_email,
    vt.uuid AS transfer_uuid, vt.status AS transfer_status,
    COALESCE(NULLIF(c.company_title, ''), TRIM(c.first_name || ' ' || c.last_name), '')::text AS customer_name,
    COALESCE(c.phone, '')::text AS customer_phone
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
LEFT JOIN branches tb ON tb.organization_id = t.organization_id AND tb.id = t.to_branch_id
LEFT JOIN service_points sp ON sp.organization_id = t.organization_id AND sp.id = t.point_id
LEFT JOIN rentals r ON r.organization_id = t.organization_id AND r.id = t.rental_id
LEFT JOIN reservations res ON res.organization_id = t.organization_id AND res.id = t.reservation_id
LEFT JOIN vehicle_allocations va ON va.organization_id = res.organization_id AND va.id = res.allocation_id
LEFT JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = COALESCE(t.vehicle_id, va.vehicle_id)
LEFT JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id AND c.anonymized_at IS NULL
LEFT JOIN users au ON au.id = t.assignee_id
LEFT JOIN users cu ON cu.id = t.completed_by
LEFT JOIN vehicle_transfers vt ON vt.organization_id = t.organization_id AND vt.id = t.transfer_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: ListFieldTasks :many
SELECT t.id, t.uuid, t.task_type, t.status, t.origin, t.branch_id, t.due_at, t.planned_minutes, t.note,
    t.started_at, t.completed_at, t.completion_note, t.assigned_at, t.issue_note, t.issue_reported_at,
    t.location_kind, t.address, t.district, t.city, t.latitude, t.longitude,
    t.contact_name, t.contact_phone, t.flight_number, t.meeting_note, t.created_at, t.updated_at,
    t.rental_id, t.reservation_id, t.assignee_id, t.to_branch_id, t.transfer_id,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    tb.uuid AS to_branch_uuid, tb.code AS to_branch_code, tb.name AS to_branch_name,
    sp.uuid AS point_uuid, sp.name AS point_name, sp.type AS point_type,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference, res.status AS reservation_status,
    res.starts_at AS reservation_starts_at, res.ends_at AS reservation_ends_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate, v.id AS resolved_vehicle_id,
    au.uuid AS assignee_uuid, au.name AS assignee_name, au.surname AS assignee_surname, au.email AS assignee_email,
    cu.uuid AS completed_by_uuid, cu.name AS completed_by_name, cu.surname AS completed_by_surname, cu.email AS completed_by_email,
    vt.uuid AS transfer_uuid, vt.status AS transfer_status,
    COALESCE(NULLIF(c.company_title, ''), TRIM(c.first_name || ' ' || c.last_name), '')::text AS customer_name,
    COALESCE(c.phone, '')::text AS customer_phone,
    COUNT(*) OVER ()::bigint AS total
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
LEFT JOIN branches tb ON tb.organization_id = t.organization_id AND tb.id = t.to_branch_id
LEFT JOIN service_points sp ON sp.organization_id = t.organization_id AND sp.id = t.point_id
LEFT JOIN rentals r ON r.organization_id = t.organization_id AND r.id = t.rental_id
LEFT JOIN reservations res ON res.organization_id = t.organization_id AND res.id = t.reservation_id
LEFT JOIN vehicle_allocations va ON va.organization_id = res.organization_id AND va.id = res.allocation_id
LEFT JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = COALESCE(t.vehicle_id, va.vehicle_id)
LEFT JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id AND c.anonymized_at IS NULL
LEFT JOIN users au ON au.id = t.assignee_id
LEFT JOIN users cu ON cu.id = t.completed_by
LEFT JOIN vehicle_transfers vt ON vt.organization_id = t.organization_id AND vt.id = t.transfer_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR t.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(task_types)::text[] IS NULL OR t.task_type = ANY(sqlc.narg(task_types)::text[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(assignee_uuids)::uuid[] IS NULL OR au.uuid = ANY(sqlc.narg(assignee_uuids)::uuid[])
       OR (sqlc.narg(include_unassigned)::boolean IS TRUE AND t.assignee_id IS NULL))
  AND (sqlc.narg(unassigned)::boolean IS NULL OR (t.assignee_id IS NULL) = sqlc.narg(unassigned)::boolean)
  AND (sqlc.narg(assignee_id)::bigint IS NULL OR t.assignee_id = sqlc.narg(assignee_id)::bigint)
  AND (sqlc.narg(location_kinds)::text[] IS NULL OR t.location_kind = ANY(sqlc.narg(location_kinds)::text[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR res.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(due_from)::timestamptz IS NULL OR t.due_at >= sqlc.narg(due_from)::timestamptz)
  AND (sqlc.narg(due_to)::timestamptz IS NULL OR t.due_at < sqlc.narg(due_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR res.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR t.address ILIKE '%' || sqlc.narg(q) || '%'
    OR t.flight_number ILIKE '%' || sqlc.narg(q) || '%'
    OR t.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'due_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.due_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'due_at' AND sqlc.arg(sort_desc)::boolean THEN t.due_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN t.status END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN t.status END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'task_type' AND NOT sqlc.arg(sort_desc)::boolean THEN t.task_type END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'task_type' AND sqlc.arg(sort_desc)::boolean THEN t.task_type END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
    t.due_at ASC, t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListFieldTaskUUIDs :many
-- Bulk "all matching": the uuids of the filtered list (capped).
SELECT t.uuid
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
LEFT JOIN branches tb ON tb.organization_id = t.organization_id AND tb.id = t.to_branch_id
LEFT JOIN service_points sp ON sp.organization_id = t.organization_id AND sp.id = t.point_id
LEFT JOIN rentals r ON r.organization_id = t.organization_id AND r.id = t.rental_id
LEFT JOIN reservations res ON res.organization_id = t.organization_id AND res.id = t.reservation_id
LEFT JOIN vehicle_allocations va ON va.organization_id = res.organization_id AND va.id = res.allocation_id
LEFT JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = COALESCE(t.vehicle_id, va.vehicle_id)
LEFT JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id AND c.anonymized_at IS NULL
LEFT JOIN users au ON au.id = t.assignee_id
LEFT JOIN users cu ON cu.id = t.completed_by
LEFT JOIN vehicle_transfers vt ON vt.organization_id = t.organization_id AND vt.id = t.transfer_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR t.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(task_types)::text[] IS NULL OR t.task_type = ANY(sqlc.narg(task_types)::text[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(assignee_uuids)::uuid[] IS NULL OR au.uuid = ANY(sqlc.narg(assignee_uuids)::uuid[])
       OR (sqlc.narg(include_unassigned)::boolean IS TRUE AND t.assignee_id IS NULL))
  AND (sqlc.narg(unassigned)::boolean IS NULL OR (t.assignee_id IS NULL) = sqlc.narg(unassigned)::boolean)
  AND (sqlc.narg(assignee_id)::bigint IS NULL OR t.assignee_id = sqlc.narg(assignee_id)::bigint)
  AND (sqlc.narg(location_kinds)::text[] IS NULL OR t.location_kind = ANY(sqlc.narg(location_kinds)::text[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR res.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(due_from)::timestamptz IS NULL OR t.due_at >= sqlc.narg(due_from)::timestamptz)
  AND (sqlc.narg(due_to)::timestamptz IS NULL OR t.due_at < sqlc.narg(due_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR res.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR t.address ILIKE '%' || sqlc.narg(q) || '%'
    OR t.flight_number ILIKE '%' || sqlc.narg(q) || '%'
    OR t.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY t.due_at ASC, t.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListFieldTaskFacets :many
SELECT f.facet::text AS facet, f.value::text AS value, f.label::text AS label, COUNT(*)::bigint AS count
FROM (
    SELECT 'status' AS facet, t.status AS value, '' AS label, t.branch_id, t.to_branch_id
    FROM operation_tasks t WHERE t.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'task_type', t.task_type, '', t.branch_id, t.to_branch_id
    FROM operation_tasks t WHERE t.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'location_kind', t.location_kind, '', t.branch_id, t.to_branch_id
    FROM operation_tasks t WHERE t.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'branch', b.uuid::text, b.code || ' · ' || b.name, t.branch_id, t.to_branch_id
    FROM operation_tasks t
    JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
    WHERE t.organization_id = sqlc.arg(organization_id) AND b.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'assignee', COALESCE(u.uuid::text, ''), COALESCE(TRIM(u.name || ' ' || u.surname), ''), t.branch_id, t.to_branch_id
    FROM operation_tasks t
    LEFT JOIN users u ON u.id = t.assignee_id
    WHERE t.organization_id = sqlc.arg(organization_id)
) f
WHERE (sqlc.narg(branch_ids)::bigint[] IS NULL OR f.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR f.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY f.facet, f.value, f.label
ORDER BY f.facet, COUNT(*) DESC, f.value
LIMIT 2000;

-- name: ListFieldTaskEvents :many
SELECT e.uuid, e.action, e.from_status, e.to_status, e.note, e.data, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM operation_task_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.task_id = sqlc.arg(task_id)
ORDER BY e.created_at ASC, e.id ASC;

-- name: InsertFieldTask :one
-- A task staff open by hand or a reservation opens for a delivery /
-- collection (one live task per reservation and type before the rental:
-- uq_operation_tasks_reservation_live; a second open returns no row).
INSERT INTO operation_tasks (
    organization_id, task_type, status, origin, branch_id, rental_id, reservation_id, vehicle_id, due_at,
    planned_minutes, note, location_kind, point_id, address, district, city, latitude, longitude,
    contact_name, contact_phone, flight_number, meeting_note, to_branch_id, transfer_id, assignee_id, assigned_at,
    created_by, updated_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(task_type), sqlc.arg(status), sqlc.arg(origin), sqlc.arg(branch_id),
    sqlc.narg(rental_id), sqlc.narg(reservation_id), sqlc.narg(vehicle_id), sqlc.arg(due_at),
    sqlc.arg(planned_minutes), sqlc.arg(note), sqlc.arg(location_kind), sqlc.narg(point_id), sqlc.arg(address),
    sqlc.arg(district), sqlc.arg(city), sqlc.narg(latitude), sqlc.narg(longitude), sqlc.arg(contact_name),
    sqlc.arg(contact_phone), sqlc.arg(flight_number), sqlc.arg(meeting_note), sqlc.narg(to_branch_id),
    sqlc.narg(transfer_id), sqlc.narg(assignee_id), sqlc.narg(assigned_at), sqlc.narg(created_by), sqlc.narg(created_by)
)
ON CONFLICT DO NOTHING
RETURNING id, uuid;

-- name: UpdateFieldTaskPlan :execrows
-- Re-plans a live task: time, duration, place, contact and meeting details.
UPDATE operation_tasks
SET due_at = sqlc.arg(due_at), planned_minutes = sqlc.arg(planned_minutes), note = sqlc.arg(note),
    branch_id = sqlc.arg(branch_id), location_kind = sqlc.arg(location_kind), point_id = sqlc.narg(point_id),
    address = sqlc.arg(address), district = sqlc.arg(district), city = sqlc.arg(city),
    latitude = sqlc.narg(latitude), longitude = sqlc.narg(longitude), contact_name = sqlc.arg(contact_name),
    contact_phone = sqlc.arg(contact_phone), flight_number = sqlc.arg(flight_number),
    meeting_note = sqlc.arg(meeting_note), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported');

-- name: TransitionFieldTask :execrows
-- Moves a task from one status to the next (the use case validated the
-- transition; the from status guards a concurrent change).
UPDATE operation_tasks
SET status = sqlc.arg(to_status),
    started_at = CASE WHEN sqlc.arg(to_status)::text = 'en_route' THEN COALESCE(started_at, sqlc.arg(at)) ELSE started_at END,
    issue_note = CASE WHEN sqlc.arg(to_status)::text = 'issue_reported' THEN sqlc.arg(note) ELSE issue_note END,
    issue_reported_at = CASE WHEN sqlc.arg(to_status)::text = 'issue_reported' THEN sqlc.arg(at) ELSE issue_reported_at END,
    completed_at = CASE WHEN sqlc.arg(to_status)::text IN ('completed', 'cancelled') THEN sqlc.arg(at) ELSE NULL END,
    completed_by = CASE WHEN sqlc.arg(to_status)::text IN ('completed', 'cancelled') THEN sqlc.narg(actor_id) ELSE NULL END,
    completion_note = CASE WHEN sqlc.arg(to_status)::text IN ('completed', 'cancelled') THEN sqlc.arg(note) ELSE completion_note END,
    updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = sqlc.arg(from_status);

-- name: SetFieldTaskTransfer :exec
UPDATE operation_tasks
SET transfer_id = sqlc.narg(transfer_id), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetFieldTaskTransferByID :one
SELECT vt.uuid, vt.status
FROM vehicle_transfers vt
WHERE vt.organization_id = sqlc.arg(organization_id)
  AND vt.id = sqlc.arg(id);

-- name: GetFieldTransferIDByUUID :one
SELECT vt.id
FROM vehicle_transfers vt
WHERE vt.organization_id = sqlc.arg(organization_id)
  AND vt.uuid = sqlc.arg(uuid);

-- name: GetFieldMember :one
-- A member of the organization as a task assignee: branch scope and
-- whether a role lets them work field tasks.
SELECT u.id, u.uuid, u.name, u.surname, u.email, om.branch_scope,
    COALESCE((
        SELECT array_agg(mb.branch_id ORDER BY mb.branch_id)
        FROM organization_member_branches mb
        WHERE mb.organization_id = om.organization_id AND mb.member_id = om.id
    ), '{}')::bigint[] AS branch_ids,
    EXISTS (
        SELECT 1
        FROM organization_member_roles mr
        JOIN role_permissions rp ON rp.role_id = mr.role_id
        JOIN permissions p ON p.id = rp.permission_id
        WHERE mr.organization_id = om.organization_id AND mr.member_id = om.id
          AND p.slug IN ('tenant.field_tasks.execute', 'tenant.field_tasks.assign')
    )::boolean AS can_work
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.uuid = sqlc.arg(uuid)
  AND u.deleted_at IS NULL;

-- name: ListFieldAssignees :many
-- Members whose roles let them work field tasks (pickers, the per-staff
-- day timeline), with their branch scope.
SELECT u.id, u.uuid, u.name, u.surname, u.email, om.branch_scope,
    COALESCE((
        SELECT array_agg(mb.branch_id ORDER BY mb.branch_id)
        FROM organization_member_branches mb
        WHERE mb.organization_id = om.organization_id AND mb.member_id = om.id
    ), '{}')::bigint[] AS branch_ids
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.deleted_at IS NULL
  AND EXISTS (
        SELECT 1
        FROM organization_member_roles mr
        JOIN role_permissions rp ON rp.role_id = mr.role_id
        JOIN permissions p ON p.id = rp.permission_id
        WHERE mr.organization_id = om.organization_id AND mr.member_id = om.id
          AND p.slug IN ('tenant.field_tasks.execute', 'tenant.field_tasks.assign')
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY u.name, u.surname, u.id
LIMIT 200;

-- name: GetFieldBranch :one
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: GetFieldVehicle :one
SELECT v.id, v.uuid, v.plate, v.status, v.home_branch_id, v.current_branch_id
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: GetFieldPoint :one
SELECT sp.id, sp.uuid, sp.name, sp.type, sp.branch_id, sp.is_active
FROM service_points sp
WHERE sp.organization_id = sqlc.arg(organization_id)
  AND sp.uuid = sqlc.arg(uuid);

-- name: GetFieldReservation :one
-- What the delivery automation needs of a reservation: status, plan,
-- places with their delivery settings, delivery details, the live rental.
SELECT res.id, res.uuid, res.reference, res.status, res.starts_at, res.ends_at,
    res.pickup_branch_id, res.return_branch_id, res.pickup_point_id, res.return_point_id,
    res.pickup_delivery, res.return_delivery,
    COALESCE(pp.delivery_enabled, FALSE)::boolean AS pickup_point_delivery,
    COALESCE(pp.branch_id, 0)::bigint AS pickup_point_branch_id,
    COALESCE(rp.delivery_enabled, FALSE)::boolean AS return_point_delivery,
    COALESCE(rp.branch_id, 0)::bigint AS return_point_branch_id,
    rt.id AS rental_id, rt.status AS rental_status
FROM reservations res
LEFT JOIN service_points pp ON pp.organization_id = res.organization_id AND pp.id = res.pickup_point_id
LEFT JOIN service_points rp ON rp.organization_id = res.organization_id AND rp.id = res.return_point_id
LEFT JOIN rentals rt ON rt.organization_id = res.organization_id AND rt.reservation_id = res.id AND rt.status <> 'cancelled'
WHERE res.organization_id = sqlc.arg(organization_id)
  AND res.uuid = sqlc.arg(uuid);

-- name: ListReservationFieldTasks :many
-- The live handover / return tasks of a reservation (with or without the
-- rental).
SELECT t.id, t.uuid, t.task_type, t.status, t.rental_id, t.location_kind, t.origin
FROM operation_tasks t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.reservation_id = sqlc.arg(reservation_id)
  AND t.task_type IN ('handover', 'return')
  AND t.status IN ('planned', 'assigned', 'en_route', 'issue_reported')
ORDER BY t.id;

-- name: GetDeliveryPoint :one
-- A service point with what delivery eligibility and the fee need.
SELECT sp.id, sp.uuid, sp.name, sp.type, sp.branch_id, sp.is_active, sp.online_selectable,
    sp.delivery_enabled, sp.delivery_extra_id, sp.extra_fee, sp.working_hours, sp.service_districts,
    sp.latitude, sp.longitude, sp.service_radius_km, sp.min_notice_minutes, sp.delivery_conditions,
    b.working_hours AS branch_working_hours, b.holiday_exceptions AS branch_holiday_exceptions,
    b.city AS branch_city
FROM service_points sp
JOIN branches b ON b.organization_id = sp.organization_id AND b.id = sp.branch_id
WHERE sp.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND sp.uuid = sqlc.arg(uuid);

-- name: GetDeliveryExtraPrice :one
-- The price of a delivery extra effective at a time: the product's row,
-- else the default row.
SELECT e.uuid AS extra_uuid, e.code, e.name, e.charge_type, e.status, e.max_total,
    ep.uuid AS price_uuid, ep.price
FROM extras e
JOIN extra_prices ep ON ep.organization_id = e.organization_id AND ep.extra_id = e.id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND ep.organization_id = sqlc.arg(organization_id)
  AND e.id = sqlc.arg(extra_id)
  AND e.deleted_at IS NULL
  AND (ep.rental_product_id IS NULL OR ep.rental_product_id = (
        SELECT rp.id FROM rental_products rp
        WHERE rp.organization_id = sqlc.arg(organization_id) AND rp.uuid = sqlc.narg(product_uuid)::uuid))
  AND ep.valid_from <= sqlc.arg(at)
  AND (ep.valid_to IS NULL OR ep.valid_to > sqlc.arg(at))
ORDER BY (ep.rental_product_id IS NULL) ASC
LIMIT 1;

-- name: ListDeliveryPoints :many
-- The delivery points of the organization (wizard picker), optionally of
-- one branch.
SELECT sp.uuid, sp.name, sp.type, sp.description, sp.extra_fee, sp.delivery_extra_id, sp.working_hours,
    sp.service_districts, sp.min_notice_minutes, sp.delivery_conditions, sp.online_selectable,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name, b.id AS branch_id,
    e.uuid AS delivery_extra_uuid, e.name AS delivery_extra_name
FROM service_points sp
JOIN branches b ON b.organization_id = sp.organization_id AND b.id = sp.branch_id
LEFT JOIN extras e ON e.organization_id = sp.organization_id AND e.id = sp.delivery_extra_id
WHERE sp.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND sp.is_active
  AND sp.delivery_enabled
  AND b.deleted_at IS NULL
  AND b.is_active
  AND (sqlc.narg(branch_uuid)::uuid IS NULL OR b.uuid = sqlc.narg(branch_uuid)::uuid)
ORDER BY b.code, sp.name
LIMIT 500;

-- name: GetTaskPlacementFacts :one
-- A rental's handover / return task and the delivery facts of its
-- reservation (placement.Locate).
SELECT t.id, t.task_type, t.origin, t.location_kind,
    res.pickup_point_id, res.return_point_id, res.pickup_delivery, res.return_delivery,
    COALESCE(pp.delivery_enabled, FALSE)::boolean AS pickup_point_delivery,
    COALESCE(rp.delivery_enabled, FALSE)::boolean AS return_point_delivery
FROM operation_tasks t
JOIN reservations res ON res.organization_id = t.organization_id AND res.id = t.reservation_id
LEFT JOIN service_points pp ON pp.organization_id = res.organization_id AND pp.id = res.pickup_point_id
LEFT JOIN service_points rp ON rp.organization_id = res.organization_id AND rp.id = res.return_point_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: SetOperationTaskPlace :exec
-- The place, contact and meeting details of a task (delivery automation;
-- no change = no write, so the timeline stays quiet).
UPDATE operation_tasks
SET location_kind = sqlc.arg(location_kind), point_id = sqlc.narg(point_id), address = sqlc.arg(address),
    district = sqlc.arg(district), city = sqlc.arg(city), latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude), contact_name = sqlc.arg(contact_name), contact_phone = sqlc.arg(contact_phone),
    flight_number = sqlc.arg(flight_number), meeting_note = sqlc.arg(meeting_note), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported')
  AND (location_kind, point_id, address, district, city, latitude, longitude, contact_name, contact_phone,
       flight_number, meeting_note)
      IS DISTINCT FROM (sqlc.arg(location_kind)::text, sqlc.narg(point_id)::bigint, sqlc.arg(address)::text,
       sqlc.arg(district)::text, sqlc.arg(city)::text, sqlc.narg(latitude)::double precision,
       sqlc.narg(longitude)::double precision, sqlc.arg(contact_name)::text, sqlc.arg(contact_phone)::text,
       sqlc.arg(flight_number)::text, sqlc.arg(meeting_note)::text);

-- name: RescheduleFieldTask :exec
UPDATE operation_tasks
SET due_at = sqlc.arg(due_at), branch_id = sqlc.arg(branch_id), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported')
  AND (due_at, branch_id) IS DISTINCT FROM (sqlc.arg(due_at)::timestamptz, sqlc.arg(branch_id)::bigint);

-- name: CancelReservationFieldTasks :many
-- The reservation ended (cancelled, expired, no-show) or no longer asks for
-- a delivery: its live tasks without a rental are cancelled.
UPDATE operation_tasks
SET status = 'cancelled', completed_at = sqlc.arg(at), completed_by = sqlc.narg(actor_id),
    completion_note = sqlc.arg(note), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND reservation_id = sqlc.arg(reservation_id)
  AND rental_id IS NULL
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported')
  AND (sqlc.narg(task_types)::text[] IS NULL OR task_type = ANY(sqlc.narg(task_types)::text[]))
RETURNING uuid;

-- name: GetBranchUUIDByID :one
SELECT b.uuid
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.id = sqlc.arg(id);

-- name: GetFieldUserByID :one
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM users u
WHERE u.id = sqlc.arg(id);
