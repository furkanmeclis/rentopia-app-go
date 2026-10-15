-- Operation tasks (TEC-709). Branch scope: t.branch_id.

-- name: InsertOperationTask :one
-- A live task per rental, type and vehicle at most (the partial unique
-- index): a second open of the same task is a no-op (no row returned).
-- A delivery / collection task the reservation opened before the rental
-- existed (TEC-724) is adopted instead: it gets the rental and vehicle
-- and keeps its assignee, status and place.
WITH res AS (
    SELECT r.reservation_id FROM rentals r
    WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = sqlc.narg(rental_id)::bigint
), adopted AS (
    UPDATE operation_tasks a
    SET rental_id = sqlc.narg(rental_id)::bigint, vehicle_id = COALESCE(sqlc.narg(vehicle_id)::bigint, a.vehicle_id),
        updated_by = sqlc.narg(created_by)::bigint
    WHERE a.organization_id = sqlc.arg(organization_id)
      AND a.reservation_id = (SELECT reservation_id FROM res)
      AND a.rental_id IS NULL
      AND a.task_type = sqlc.arg(task_type)
      AND a.status IN ('planned', 'assigned', 'en_route', 'issue_reported')
    RETURNING a.uuid
), inserted AS (
    INSERT INTO operation_tasks (
        organization_id, task_type, branch_id, rental_id, reservation_id, vehicle_id, due_at, note, created_by, updated_by
    )
    SELECT sqlc.arg(organization_id), sqlc.arg(task_type), sqlc.arg(branch_id), sqlc.narg(rental_id)::bigint,
        (SELECT reservation_id FROM res), sqlc.narg(vehicle_id)::bigint, sqlc.arg(due_at), sqlc.arg(note),
        sqlc.narg(created_by)::bigint, sqlc.narg(created_by)::bigint
    WHERE NOT EXISTS (SELECT 1 FROM adopted)
    ON CONFLICT DO NOTHING
    RETURNING uuid
)
SELECT uuid FROM adopted
UNION ALL
SELECT uuid FROM inserted;

-- name: GetOperationTask :one
SELECT t.id, t.uuid, t.task_type, t.status, t.branch_id, t.due_at, t.note, t.started_at, t.completed_at,
    t.completion_note, t.created_at, t.updated_at, t.vehicle_id,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    au.uuid AS assignee_uuid, au.name AS assignee_name, au.surname AS assignee_surname, au.email AS assignee_email,
    cu.uuid AS completed_by_uuid, cu.name AS completed_by_name, cu.surname AS completed_by_surname, cu.email AS completed_by_email
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
LEFT JOIN rentals r ON r.organization_id = t.organization_id AND r.id = t.rental_id
LEFT JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = t.vehicle_id
LEFT JOIN users au ON au.id = t.assignee_id
LEFT JOIN users cu ON cu.id = t.completed_by
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: ListOperationTasks :many
SELECT t.uuid, t.task_type, t.status, t.due_at, t.note, t.started_at, t.completed_at, t.completion_note,
    t.created_at, t.updated_at,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    au.uuid AS assignee_uuid, au.name AS assignee_name, au.surname AS assignee_surname, au.email AS assignee_email,
    COUNT(*) OVER ()::bigint AS total
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
LEFT JOIN rentals r ON r.organization_id = t.organization_id AND r.id = t.rental_id
LEFT JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = t.vehicle_id
LEFT JOIN users au ON au.id = t.assignee_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(task_types)::text[] IS NULL OR t.task_type = ANY(sqlc.narg(task_types)::text[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(rental_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(rental_uuids)::uuid[]))
  AND (sqlc.narg(vehicle_uuids)::uuid[] IS NULL OR v.uuid = ANY(sqlc.narg(vehicle_uuids)::uuid[]))
  AND (sqlc.narg(assignee_uuids)::uuid[] IS NULL OR au.uuid = ANY(sqlc.narg(assignee_uuids)::uuid[]))
  AND (sqlc.narg(due_from)::timestamptz IS NULL OR t.due_at >= sqlc.narg(due_from)::timestamptz)
  AND (sqlc.narg(due_to)::timestamptz IS NULL OR t.due_at < sqlc.narg(due_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR t.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'due_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.due_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'due_at' AND sqlc.arg(sort_desc)::boolean THEN t.due_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN t.status END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN t.status END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
    t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListOperationTaskFacets :many
SELECT f.facet::text AS facet, f.value::text AS value, f.label::text AS label, COUNT(*)::bigint AS count
FROM (
    SELECT 'status' AS facet, t.status AS value, '' AS label, t.branch_id
    FROM operation_tasks t WHERE t.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'task_type', t.task_type, '', t.branch_id
    FROM operation_tasks t WHERE t.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'branch', b.uuid::text, b.code || ' · ' || b.name, t.branch_id
    FROM operation_tasks t
    JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
    WHERE t.organization_id = sqlc.arg(organization_id) AND b.organization_id = sqlc.arg(organization_id)
) f
WHERE (sqlc.narg(branch_ids)::bigint[] IS NULL OR f.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY f.facet, f.value, f.label
ORDER BY f.facet, COUNT(*) DESC, f.value
LIMIT 2000;

-- name: StartOperationTask :execrows
UPDATE operation_tasks
SET status = 'en_route', started_at = sqlc.arg(started_at), assignee_id = COALESCE(assignee_id, sqlc.narg(actor_id)),
    assigned_at = COALESCE(assigned_at, sqlc.arg(started_at)), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('planned', 'assigned');

-- name: FinishOperationTask :execrows
UPDATE operation_tasks
SET status = sqlc.arg(status), completed_at = sqlc.arg(completed_at), completed_by = sqlc.narg(actor_id),
    completion_note = sqlc.arg(completion_note), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported');

-- name: AssignOperationTask :execrows
-- A new assignee resets the task to assigned (planned without one); the
-- same assignee again changes nothing.
UPDATE operation_tasks
SET status = CASE
        WHEN assignee_id IS NOT DISTINCT FROM sqlc.narg(assignee_id)::bigint THEN status
        WHEN sqlc.narg(assignee_id)::bigint IS NULL THEN 'planned'
        ELSE 'assigned'
    END,
    started_at = CASE WHEN assignee_id IS NOT DISTINCT FROM sqlc.narg(assignee_id)::bigint THEN started_at ELSE NULL END,
    assigned_at = CASE
        WHEN assignee_id IS NOT DISTINCT FROM sqlc.narg(assignee_id)::bigint THEN assigned_at
        WHEN sqlc.narg(assignee_id)::bigint IS NULL THEN NULL
        ELSE NOW()
    END,
    assignee_id = sqlc.narg(assignee_id)::bigint,
    updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported');

-- name: FinishRentalTasks :many
-- Closes the live tasks of a rental of one type (optionally one vehicle):
-- the matching fact was recorded (handover, return) or the rental ended.
UPDATE operation_tasks
SET status = sqlc.arg(status), completed_at = sqlc.arg(completed_at), completed_by = sqlc.narg(actor_id),
    completion_note = sqlc.arg(completion_note), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported')
  AND (sqlc.narg(task_types)::text[] IS NULL OR task_type = ANY(sqlc.narg(task_types)::text[]))
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id)::bigint)
RETURNING uuid;

-- name: RescheduleRentalTasks :exec
-- The planned pick-up / return moved: live tasks of the type follow.
UPDATE operation_tasks
SET due_at = sqlc.arg(due_at), updated_by = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND task_type = sqlc.arg(task_type)
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported')
  AND due_at IS DISTINCT FROM sqlc.arg(due_at);
