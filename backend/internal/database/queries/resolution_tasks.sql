-- Resolution tasks (TEC-698): manager tasks for reservations whose plate
-- plan broke and could not be re-planned. Every query filters by
-- organization_id first; lists apply the reservation's branch scope.

-- name: InsertResolutionTask :one
-- At most one open task per reservation, type and source (ON CONFLICT on
-- the partial unique index returns nothing).
INSERT INTO resolution_tasks (organization_id, reservation_id, task_type, reason, source_type, source_uuid, data)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(task_type), sqlc.arg(reason),
    sqlc.arg(source_type), sqlc.narg(source_uuid), sqlc.arg(data)
)
ON CONFLICT (organization_id, reservation_id, task_type, COALESCE(source_uuid, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status = 'open'
DO NOTHING
RETURNING uuid;

-- name: GetResolutionTask :one
SELECT t.id, t.uuid, t.task_type, t.status, t.reason, t.source_type, t.source_uuid, t.data, t.resolved_at,
    t.resolution_note, t.created_at, t.updated_at,
    r.id AS reservation_id, r.uuid AS reservation_uuid, r.reference AS reservation_reference, r.status AS reservation_status,
    r.starts_at AS reservation_starts_at, r.ends_at AS reservation_ends_at,
    r.pickup_branch_id, r.return_branch_id,
    au.uuid AS assignee_uuid, au.name AS assignee_name, au.surname AS assignee_surname, au.email AS assignee_email,
    ru.uuid AS resolved_by_uuid, ru.name AS resolved_by_name, ru.surname AS resolved_by_surname, ru.email AS resolved_by_email
FROM resolution_tasks t
JOIN reservations r ON r.organization_id = t.organization_id AND r.id = t.reservation_id
LEFT JOIN users au ON au.id = t.assignee_id
LEFT JOIN users ru ON ru.id = t.resolved_by
WHERE t.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: ListResolutionTasks :many
SELECT t.uuid, t.task_type, t.status, t.reason, t.source_type, t.source_uuid, t.data, t.resolved_at,
    t.resolution_note, t.created_at, t.updated_at,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference, r.status AS reservation_status,
    r.starts_at AS reservation_starts_at, r.ends_at AS reservation_ends_at,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    au.uuid AS assignee_uuid, au.name AS assignee_name, au.surname AS assignee_surname, au.email AS assignee_email,
    COUNT(*) OVER ()::bigint AS total
FROM resolution_tasks t
JOIN reservations r ON r.organization_id = t.organization_id AND r.id = t.reservation_id
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
LEFT JOIN users au ON au.id = t.assignee_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(task_types)::text[] IS NULL OR t.task_type = ANY(sqlc.narg(task_types)::text[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(assignee_uuids)::uuid[] IS NULL OR au.uuid = ANY(sqlc.narg(assignee_uuids)::uuid[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR t.reason ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.starts_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND sqlc.arg(sort_desc)::boolean THEN r.starts_at END DESC,
    t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ResolveResolutionTask :execrows
UPDATE resolution_tasks
SET status = 'resolved', resolved_at = sqlc.arg(resolved_at)::timestamptz, resolved_by = sqlc.narg(resolved_by),
    resolution_note = sqlc.arg(resolution_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'open';

-- name: AssignResolutionTask :execrows
UPDATE resolution_tasks
SET assignee_id = sqlc.narg(assignee_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ResolveReservationTasks :execrows
-- Closes the open tasks of a reservation (cancelled / expired / re-planned).
UPDATE resolution_tasks
SET status = 'resolved', resolved_at = sqlc.arg(resolved_at)::timestamptz, resolved_by = sqlc.narg(resolved_by),
    resolution_note = sqlc.arg(resolution_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND reservation_id = sqlc.arg(reservation_id)
  AND status = 'open'
  AND (sqlc.narg(task_types)::text[] IS NULL OR task_type = ANY(sqlc.narg(task_types)::text[]));
