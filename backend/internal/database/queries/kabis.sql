-- KABİS rental notifications (TEC-726, docs/design/kabis.md). Every query
-- names the organization; list / count / facets take the branch scope
-- (nil = every branch): a notification is visible when its event branch or
-- its rental's branch is in the member's scope.

-- name: GetKabisRentalFacts :one
-- The actual facts a notification is built from (inside the rental's
-- transaction): the rental, its current vehicle, the actual handover /
-- return places, the reservation's customer and planned pick-up branch.
SELECT r.id, r.uuid, r.reference, r.status, r.vehicle_id, r.actual_pickup_at, r.actual_pickup_branch_id,
    r.actual_pickup_point_id, r.actual_return_at, r.actual_return_branch_id, r.actual_return_point_id,
    res.customer_id, res.pickup_branch_id AS rental_branch_id,
    v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(COALESCE(d.name, '') || ' ' || COALESCE(m.name, '')), ''),
        btrim(COALESCE(v.manual_brand, '') || ' ' || COALESCE(v.manual_model, '')))::text AS vehicle_label
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN vehicles v ON v.organization_id = r.organization_id AND v.id = r.vehicle_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands tb ON tb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id);

-- name: GetKabisCustomer :one
SELECT c.customer_type, c.first_name, c.last_name, c.company_title, c.identity_type, c.identity_enc, c.nationality
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id);

-- name: ListKabisRentalDrivers :many
-- The rental's drivers with their current records (identity and licence
-- numbers sealed; the builder opens them). A driver whose record was
-- removed has no driver row (its data is missing).
SELECT rd.role, (d.id IS NOT NULL)::boolean AS has_record,
    COALESCE(d.first_name, '')::text AS first_name, COALESCE(d.last_name, '')::text AS last_name,
    d.birth_date, d.identity_type, COALESCE(d.identity_enc, '')::text AS identity_enc, d.nationality,
    COALESCE(d.licence_enc, '')::text AS licence_enc, COALESCE(d.licence_class, '')::text AS licence_class,
    d.licence_country
FROM rental_drivers rd
LEFT JOIN drivers d ON d.organization_id = rd.organization_id AND d.id = rd.driver_id AND d.deleted_at IS NULL
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND rd.rental_id = sqlc.arg(rental_id)
ORDER BY CASE rd.role WHEN 'primary' THEN 0 ELSE 1 END, rd.id;

-- name: GetKabisPlace :one
-- A branch (and optionally one of its service points) as a place.
SELECT b.id, b.code, b.name, b.city, b.district, b.address,
    COALESCE((SELECT sp.name FROM service_points sp
              WHERE sp.organization_id = b.organization_id AND sp.id = sqlc.narg(point_id)::bigint), '')::text AS point_name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.id = sqlc.arg(id);

-- name: GetKabisNotificationByRentalEvent :one
SELECT n.id, n.uuid
FROM kabis_notifications n
WHERE n.organization_id = sqlc.arg(organization_id)
  AND n.rental_id = sqlc.arg(rental_id)
  AND n.event = sqlc.arg(event)
  AND n.sequence = sqlc.arg(sequence);

-- name: GetLatestKabisStartChain :one
-- The newest start-side notification of a rental (the start or its last
-- correction) with its job's state.
SELECT n.id, n.uuid, n.event, n.payload_hash, n.integration_job_id,
    j.status AS job_status, j.attempts AS job_attempts
FROM kabis_notifications n
JOIN integration_jobs j ON j.organization_id = n.organization_id AND j.id = n.integration_job_id
WHERE n.organization_id = sqlc.arg(organization_id)
  AND j.organization_id = sqlc.arg(organization_id)
  AND n.rental_id = sqlc.arg(rental_id)
  AND n.event IN ('rental_start', 'correction')
ORDER BY n.id DESC
LIMIT 1;

-- name: NextKabisCorrectionSequence :one
SELECT (COALESCE(MAX(n.sequence), 0) + 1)::int AS next
FROM kabis_notifications n
WHERE n.organization_id = sqlc.arg(organization_id)
  AND n.rental_id = sqlc.arg(rental_id)
  AND n.event = 'correction';

-- name: InsertKabisJob :one
-- The notification's integration job, created in the rental's transaction
-- (outbox: a pending job is due at once; the sweep picks it up if the
-- process stops before the first step). Blocked / waiting jobs have no
-- next step.
INSERT INTO integration_jobs (
    organization_id, integration_account_id, direction, provider, kind, idempotency_key, payload, ref_type, ref_uuid,
    priority, max_attempts, status, next_attempt_at, last_error
) VALUES (
    sqlc.arg(organization_id), sqlc.narg(integration_account_id), 'outbound', sqlc.arg(provider), sqlc.arg(kind),
    sqlc.arg(idempotency_key), sqlc.arg(payload), 'kabis_notification', sqlc.arg(ref_uuid), 'normal',
    sqlc.arg(max_attempts), sqlc.arg(status), sqlc.narg(next_attempt_at), sqlc.arg(last_error)
)
RETURNING id, uuid;

-- name: InsertKabisNotification :one
INSERT INTO kabis_notifications (
    uuid, organization_id, rental_id, branch_id, rental_branch_id, event, sequence, cause, corrects_id,
    integration_job_id, occurred_at, plate, payload_enc, payload_hash, summary, missing_fields, hold_reason
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(branch_id), sqlc.arg(rental_branch_id),
    sqlc.arg(event), sqlc.arg(sequence), sqlc.arg(cause), sqlc.narg(corrects_id), sqlc.arg(integration_job_id),
    sqlc.arg(occurred_at), sqlc.arg(plate), sqlc.arg(payload_enc), sqlc.arg(payload_hash), sqlc.arg(summary),
    sqlc.arg(missing_fields), sqlc.arg(hold_reason)
)
RETURNING id;

-- name: RefreshKabisNotification :execrows
-- A notification that was not sent yet takes the current data.
UPDATE kabis_notifications
SET plate = sqlc.arg(plate), payload_enc = sqlc.arg(payload_enc), payload_hash = sqlc.arg(payload_hash),
    summary = sqlc.arg(summary), missing_fields = sqlc.arg(missing_fields), hold_reason = sqlc.arg(hold_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: RouteKabisJob :execrows
-- Moves a job that has sent nothing yet (blocked, waiting, or pending with
-- no attempt) to its new state and route. A job a worker holds, or one
-- that already made a call, is left alone (0 rows).
UPDATE integration_jobs
SET status = sqlc.arg(status)::text, provider = sqlc.arg(provider), integration_account_id = sqlc.narg(integration_account_id),
    next_attempt_at = sqlc.narg(next_attempt_at), last_error = sqlc.arg(last_error)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND (status IN ('blocked', 'waiting_dependency') OR (status = 'pending' AND attempts = 0))
  AND (locked_until IS NULL OR locked_until < NOW());

-- name: GetKabisNotification :one
SELECT n.id, n.uuid, n.organization_id, n.rental_id, n.branch_id, n.rental_branch_id, n.event, n.sequence, n.cause,
    n.integration_job_id, n.occurred_at, n.plate, n.payload_enc, n.payload_hash, n.summary, n.missing_fields,
    n.hold_reason, n.last_verified_at, n.resolution, n.resolution_reason, n.resolution_reference, n.resolved_at,
    n.attachment_key, n.attachment_name, n.attachment_content_type, n.attachment_size, n.created_at, n.updated_at,
    cn.uuid AS corrects_uuid,
    j.uuid AS job_uuid, j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
    j.last_error AS job_last_error, j.provider AS job_provider, j.provider_ref AS job_provider_ref,
    j.next_attempt_at AS job_next_attempt_at, j.submitted_at AS job_submitted_at, j.completed_at AS job_completed_at,
    j.locked_until AS job_locked_until, j.kind AS job_kind,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    ru.uuid AS resolved_by_uuid, ru.name AS resolved_by_name, ru.surname AS resolved_by_surname,
    ru.email AS resolved_by_email,
    a.uuid AS account_uuid, a.display_name AS account_name
FROM kabis_notifications n
JOIN integration_jobs j ON j.organization_id = n.organization_id AND j.id = n.integration_job_id
JOIN rentals r ON r.organization_id = n.organization_id AND r.id = n.rental_id
JOIN branches b ON b.organization_id = n.organization_id AND b.id = n.branch_id
LEFT JOIN kabis_notifications cn ON cn.organization_id = n.organization_id AND cn.id = n.corrects_id
LEFT JOIN users ru ON ru.id = n.resolved_by
LEFT JOIN integration_accounts a ON a.organization_id = j.organization_id AND a.id = j.integration_account_id
WHERE n.organization_id = sqlc.arg(organization_id)
  AND j.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (cn.id IS NULL OR cn.organization_id = sqlc.arg(organization_id))
  AND (a.id IS NULL OR a.organization_id = sqlc.arg(organization_id))
  AND n.uuid = sqlc.arg(uuid);

-- name: GetKabisNotificationForJob :one
-- The notification of a runner job (the operation opens its payload).
SELECT n.id, n.uuid, n.event, n.payload_enc, n.missing_fields
FROM kabis_notifications n
WHERE n.organization_id = sqlc.arg(organization_id)
  AND n.integration_job_id = sqlc.arg(integration_job_id);

-- name: ListKabisNotifications :many
SELECT n.uuid, n.event, n.sequence, n.cause, n.occurred_at, n.plate, n.summary, n.missing_fields, n.hold_reason,
    n.last_verified_at, n.resolution, n.resolution_reference, n.resolved_at, n.created_at, n.updated_at,
    j.uuid AS job_uuid, j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
    j.last_error AS job_last_error, j.provider AS job_provider, j.provider_ref AS job_provider_ref,
    j.next_attempt_at AS job_next_attempt_at, j.submitted_at AS job_submitted_at, j.completed_at AS job_completed_at,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    COUNT(*) OVER ()::bigint AS total
FROM kabis_notifications n
JOIN integration_jobs j ON j.organization_id = n.organization_id AND j.id = n.integration_job_id
JOIN rentals r ON r.organization_id = n.organization_id AND r.id = n.rental_id
JOIN branches b ON b.organization_id = n.organization_id AND b.id = n.branch_id
WHERE n.organization_id = sqlc.arg(organization_id)
  AND j.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL
       OR n.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR n.rental_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR j.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(events)::text[] IS NULL OR n.event = ANY(sqlc.narg(events)::text[]))
  AND (sqlc.narg(rental_uuid)::uuid IS NULL OR r.uuid = sqlc.narg(rental_uuid)::uuid)
  AND (sqlc.narg(occurred_from)::timestamptz IS NULL OR n.occurred_at >= sqlc.narg(occurred_from)::timestamptz)
  AND (sqlc.narg(occurred_to)::timestamptz IS NULL OR n.occurred_at < sqlc.narg(occurred_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR n.plate ILIKE '%' || sqlc.narg(q)::text || '%'
       OR r.reference ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'occurred_at' AND NOT sqlc.arg(sort_desc)::boolean THEN n.occurred_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'occurred_at' AND sqlc.arg(sort_desc)::boolean THEN n.occurred_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN j.updated_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN j.updated_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN n.created_at END ASC,
    n.created_at DESC, n.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: KabisStatusFacets :many
SELECT j.status, COUNT(*)::bigint AS total
FROM kabis_notifications n
JOIN integration_jobs j ON j.organization_id = n.organization_id AND j.id = n.integration_job_id
WHERE n.organization_id = sqlc.arg(organization_id)
  AND j.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL
       OR n.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR n.rental_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY j.status;

-- name: KabisEventFacets :many
SELECT n.event, COUNT(*)::bigint AS total
FROM kabis_notifications n
WHERE n.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL
       OR n.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR n.rental_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY n.event;

-- name: ListWaitingKabisJobs :many
-- The release sweep: notifications waiting on a dependency, across
-- organizations (each row carries its organization_id; the service
-- re-scopes from it).
SELECT n.organization_id, n.id, n.uuid, j.id AS job_id
FROM kabis_notifications n
JOIN integration_jobs j ON j.organization_id = n.organization_id AND j.id = n.integration_job_id
WHERE j.status = 'waiting_dependency'
  AND j.kind LIKE 'kabis.%'
ORDER BY n.organization_id, n.id
LIMIT sqlc.arg(limit_count);

-- name: ResolveKabisJob :execrows
-- Staff resolution: reported_manually / reconciled from an allowed state,
-- unless a worker holds the job.
UPDATE integration_jobs
SET status = sqlc.arg(status)::text, next_attempt_at = NULL, completed_at = NOW(), locked_until = NULL, last_error = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = ANY(sqlc.arg(from_statuses)::text[])
  AND (locked_until IS NULL OR locked_until < NOW());

-- name: SetKabisResolution :exec
UPDATE kabis_notifications
SET resolution = sqlc.arg(resolution), resolution_reason = sqlc.arg(reason), resolution_reference = sqlc.arg(reference),
    resolved_by = sqlc.narg(resolved_by), resolved_at = NOW(), attachment_key = sqlc.narg(attachment_key),
    attachment_name = sqlc.arg(attachment_name), attachment_content_type = sqlc.arg(attachment_content_type),
    attachment_size = sqlc.arg(attachment_size)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: TouchKabisVerified :exec
-- A status check with the provider happened (CheckStatus attempt).
UPDATE kabis_notifications
SET last_verified_at = sqlc.arg(verified_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND integration_job_id = sqlc.arg(integration_job_id);

-- name: GetKabisOrganizationUUID :one
SELECT o.uuid FROM organizations o WHERE o.id = sqlc.arg(organization_id);
