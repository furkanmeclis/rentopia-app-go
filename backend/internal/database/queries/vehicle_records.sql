-- Vehicle documents, maintenance and expiry reminders (TEC-693). Vehicle
-- sub-resources: every query filters by organization_id and, for one
-- vehicle, by its internal id (the use case resolved the vehicle in the
-- caller's branch scope first).
--
-- Org-wide lists (alerts, export) apply the vehicle branch scope like the
-- vehicles list: home OR current branch in branch_ids (NULL = every branch).

-- name: CreateVehicleDocument :one
INSERT INTO vehicle_documents (
    organization_id, vehicle_id, doc_type, document_number, issuer, valid_from, valid_until, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(doc_type), sqlc.arg(document_number),
    sqlc.arg(issuer), sqlc.narg(valid_from), sqlc.narg(valid_until), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateVehicleDocument :one
UPDATE vehicle_documents
SET doc_type = sqlc.arg(doc_type),
    document_number = sqlc.arg(document_number),
    issuer = sqlc.arg(issuer),
    valid_from = sqlc.narg(valid_from),
    valid_until = sqlc.narg(valid_until),
    note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetVehicleDocumentFile :one
UPDATE vehicle_documents
SET file_object_key = sqlc.arg(file_object_key),
    file_name = sqlc.arg(file_name),
    file_content_type = sqlc.arg(file_content_type),
    file_size_bytes = sqlc.arg(file_size_bytes)
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteVehicleDocument :execrows
UPDATE vehicle_documents
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetVehicleDocument :one
SELECT d.*, u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname, u.email AS creator_email
FROM vehicle_documents d
LEFT JOIN users u ON u.id = d.created_by
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.vehicle_id = sqlc.arg(vehicle_id)
  AND d.uuid = sqlc.arg(uuid)
  AND d.deleted_at IS NULL;

-- name: ListVehicleDocuments :many
-- Live documents of one vehicle, newest validity first (history included).
SELECT d.*, u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname, u.email AS creator_email
FROM vehicle_documents d
LEFT JOIN users u ON u.id = d.created_by
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.vehicle_id = sqlc.arg(vehicle_id)
  AND d.deleted_at IS NULL
ORDER BY d.doc_type ASC, d.valid_until DESC NULLS LAST, d.id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListVehicleDocumentsForExport :many
-- Live documents of live vehicles in the branch scope (export engine).
SELECT d.*, v.uuid AS vehicle_uuid, v.plate,
    COALESCE(NULLIF(btrim(cd.name || ' ' || COALESCE(cm.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
    (NOT EXISTS (
        SELECT 1 FROM vehicle_documents n
        WHERE n.organization_id = d.organization_id AND n.vehicle_id = d.vehicle_id
          AND n.doc_type = d.doc_type AND n.deleted_at IS NULL AND n.id <> d.id
          AND (
            (n.valid_until IS NOT NULL AND (d.valid_until IS NULL OR n.valid_until > d.valid_until))
            OR (n.valid_until IS NOT DISTINCT FROM d.valid_until AND n.id > d.id)
          )
    ))::boolean AS is_current
FROM vehicle_documents d
JOIN vehicles v ON v.organization_id = d.organization_id AND v.id = d.vehicle_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models cm ON cm.id = ct.model_id
LEFT JOIN catalog_brands cb ON cb.id = ct.brand_id
LEFT JOIN catalog_display_brands cd ON cd.id = cb.display_brand_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.deleted_at IS NULL
  AND v.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(vehicle_uuids)::uuid[] IS NULL OR v.uuid = ANY(sqlc.narg(vehicle_uuids)::uuid[]))
  AND (sqlc.narg(doc_types)::text[] IS NULL OR d.doc_type = ANY(sqlc.narg(doc_types)::text[]))
ORDER BY v.plate_key ASC, d.doc_type ASC, d.valid_until DESC NULLS LAST, d.id DESC
LIMIT sqlc.arg(limit_count);

-- name: CreateVehicleMaintenance :one
INSERT INTO vehicle_maintenance (
    organization_id, vehicle_id, kind, maintenance_type, planned_date, planned_km, done_date, done_km,
    service_provider, cost, note, created_by, completed_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(kind), sqlc.arg(maintenance_type),
    sqlc.narg(planned_date), sqlc.narg(planned_km), sqlc.narg(done_date), sqlc.narg(done_km),
    sqlc.arg(service_provider), sqlc.narg(cost), sqlc.arg(note), sqlc.narg(created_by), sqlc.narg(completed_by)
)
RETURNING *;

-- name: UpdateVehicleMaintenance :one
UPDATE vehicle_maintenance
SET kind = sqlc.arg(kind),
    maintenance_type = sqlc.arg(maintenance_type),
    planned_date = sqlc.narg(planned_date),
    planned_km = sqlc.narg(planned_km),
    done_date = sqlc.narg(done_date),
    done_km = sqlc.narg(done_km),
    service_provider = sqlc.arg(service_provider),
    cost = sqlc.narg(cost),
    note = sqlc.arg(note),
    completed_by = sqlc.narg(completed_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteVehicleMaintenance :execrows
UPDATE vehicle_maintenance
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetVehicleMaintenance :one
SELECT * FROM vehicle_maintenance
WHERE organization_id = sqlc.arg(organization_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: ListVehicleMaintenance :many
-- Open plans first (soonest first), then the done history (newest first).
SELECT m.*, u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname, u.email AS creator_email
FROM vehicle_maintenance m
LEFT JOIN users u ON u.id = m.created_by
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.vehicle_id = sqlc.arg(vehicle_id)
  AND m.deleted_at IS NULL
ORDER BY (m.kind = 'planned') DESC,
    CASE WHEN m.kind = 'planned' THEN m.planned_date END ASC NULLS LAST,
    CASE WHEN m.kind = 'planned' THEN m.planned_km END ASC NULLS LAST,
    m.done_date DESC NULLS LAST, m.id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListVehicleMaintenanceForExport :many
SELECT m.*, v.uuid AS vehicle_uuid, v.plate,
    COALESCE(NULLIF(btrim(cd.name || ' ' || COALESCE(cm.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label
FROM vehicle_maintenance m
JOIN vehicles v ON v.organization_id = m.organization_id AND v.id = m.vehicle_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models cm ON cm.id = ct.model_id
LEFT JOIN catalog_brands cb ON cb.id = ct.brand_id
LEFT JOIN catalog_display_brands cd ON cd.id = cb.display_brand_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.deleted_at IS NULL
  AND v.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(vehicle_uuids)::uuid[] IS NULL OR v.uuid = ANY(sqlc.narg(vehicle_uuids)::uuid[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(maintenance_types)::text[] IS NULL OR m.maintenance_type = ANY(sqlc.narg(maintenance_types)::text[]))
ORDER BY v.plate_key ASC, (m.kind = 'planned') DESC, COALESCE(m.done_date, m.planned_date) DESC NULLS LAST, m.id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListVehicleDocumentAlerts :many
-- Current documents (latest valid_until per vehicle and type) of live,
-- active vehicles that expire on or before due_before (today + horizon).
SELECT cur.* FROM (
    SELECT DISTINCT ON (d.vehicle_id, d.doc_type)
        d.id, d.uuid, d.doc_type, d.document_number, d.valid_until,
        v.id AS vehicle_id, v.uuid AS vehicle_uuid, v.plate, v.odometer_km,
        v.home_branch_id, v.current_branch_id,
        COALESCE(NULLIF(btrim(cd.name || ' ' || COALESCE(cm.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
        b.name AS current_branch_name
    FROM vehicle_documents d
    JOIN vehicles v ON v.organization_id = d.organization_id AND v.id = d.vehicle_id
    JOIN branches b ON b.organization_id = v.organization_id AND b.id = v.current_branch_id
    LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
    LEFT JOIN catalog_models cm ON cm.id = ct.model_id
    LEFT JOIN catalog_brands cb ON cb.id = ct.brand_id
    LEFT JOIN catalog_display_brands cd ON cd.id = cb.display_brand_id
    WHERE d.organization_id = sqlc.arg(organization_id)
      AND d.deleted_at IS NULL
      AND d.valid_until IS NOT NULL
      AND v.deleted_at IS NULL
      AND v.is_active
      AND (
        sqlc.narg(branch_ids)::bigint[] IS NULL
        OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
      )
    ORDER BY d.vehicle_id, d.doc_type, d.valid_until DESC, d.id DESC
) cur
WHERE cur.valid_until <= sqlc.arg(due_before)::date
ORDER BY cur.valid_until ASC, cur.plate ASC, cur.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListVehicleMaintenanceAlerts :many
-- Open plans of live, active vehicles whose planned date is on or before
-- due_before, or whose planned km is within km_horizon of the odometer.
SELECT m.id, m.uuid, m.maintenance_type, m.planned_date, m.planned_km,
    v.id AS vehicle_id, v.uuid AS vehicle_uuid, v.plate, v.odometer_km,
    v.home_branch_id, v.current_branch_id,
    COALESCE(NULLIF(btrim(cd.name || ' ' || COALESCE(cm.name, '')), ''), btrim(v.manual_brand || ' ' || v.manual_model))::text AS vehicle_label,
    b.name AS current_branch_name
FROM vehicle_maintenance m
JOIN vehicles v ON v.organization_id = m.organization_id AND v.id = m.vehicle_id
JOIN branches b ON b.organization_id = v.organization_id AND b.id = v.current_branch_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models cm ON cm.id = ct.model_id
LEFT JOIN catalog_brands cb ON cb.id = ct.brand_id
LEFT JOIN catalog_display_brands cd ON cd.id = cb.display_brand_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.deleted_at IS NULL
  AND m.kind = 'planned'
  AND v.deleted_at IS NULL
  AND v.is_active
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (
    m.planned_date <= sqlc.arg(due_before)::date
    OR m.planned_km <= v.odometer_km + sqlc.arg(km_horizon)::int
  )
ORDER BY COALESCE(m.planned_date, sqlc.arg(due_before)::date) ASC, v.plate ASC, m.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListVehicleReminderOrganizations :many
-- Live, active organizations with at least one document or open plan.
SELECT o.id, o.uuid, o.slug
FROM organizations o
WHERE o.deleted_at IS NULL
  AND o.status = 'active'
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
  AND (
    EXISTS (SELECT 1 FROM vehicle_documents d WHERE d.organization_id = o.id AND d.deleted_at IS NULL AND d.valid_until IS NOT NULL)
    OR EXISTS (SELECT 1 FROM vehicle_maintenance m WHERE m.organization_id = o.id AND m.deleted_at IS NULL AND m.kind = 'planned')
  )
ORDER BY o.id;

-- name: ListVehicleReminderRecipients :many
-- Members holding the permission (through any of their roles) with their
-- branch scope: 'all' or the live branches of 'selected'. Disabled or
-- deleted users get nothing.
SELECT om.id AS member_id, om.user_id, om.branch_scope, u.locale,
    COALESCE(
        array_agg(b.id ORDER BY b.id) FILTER (WHERE b.id IS NOT NULL),
        '{}'
    )::bigint[] AS branch_ids
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL AND u.status <> 'disabled'
LEFT JOIN organization_member_branches omb
    ON omb.organization_id = om.organization_id AND omb.member_id = om.id
LEFT JOIN branches b
    ON b.organization_id = omb.organization_id AND b.id = omb.branch_id AND b.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND EXISTS (
    SELECT 1
    FROM organization_member_roles omr
    JOIN roles r ON r.id = omr.role_id AND r.scope = 'tenant'
    JOIN role_permissions rp ON rp.role_id = omr.role_id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE omr.organization_id = om.organization_id
      AND omr.member_id = om.id
      AND p.slug = sqlc.arg(permission)::text
  )
GROUP BY om.id, om.user_id, om.branch_scope, u.locale
ORDER BY om.id;

-- name: ClaimVehicleReminder :execrows
-- Idempotency: 1 when this subject was not reminded yet today.
INSERT INTO vehicle_reminder_deliveries (
    organization_id, vehicle_id, subject_kind, subject_id, reminder_date, alert_state
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(subject_kind), sqlc.arg(subject_id),
    sqlc.arg(reminder_date), sqlc.arg(alert_state)
)
ON CONFLICT DO NOTHING;

-- name: SetVehicleReminderRecipients :exec
UPDATE vehicle_reminder_deliveries
SET recipients = sqlc.arg(recipients)
WHERE organization_id = sqlc.arg(organization_id)
  AND subject_kind = sqlc.arg(subject_kind)
  AND subject_id = sqlc.arg(subject_id)
  AND reminder_date = sqlc.arg(reminder_date);

-- name: ListLastVehicleReminders :many
-- Latest earlier reminder of each subject (since a cutoff day).
SELECT DISTINCT ON (r.subject_kind, r.subject_id)
    r.subject_kind, r.subject_id, r.reminder_date, r.alert_state
FROM vehicle_reminder_deliveries r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.reminder_date >= sqlc.arg(since)::date
  AND r.reminder_date < sqlc.arg(before)::date
ORDER BY r.subject_kind, r.subject_id, r.reminder_date DESC;
