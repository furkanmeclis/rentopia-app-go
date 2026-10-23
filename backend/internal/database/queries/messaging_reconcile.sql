-- Outbound messages waiting in restored data (TEC-962,
-- cmd/messaging-reconcile, docs/operations/backup-restore.md RT-A32 (c)).
-- The operator lists what was still unsent at backup time (created before
-- the backup, due by the restore) and marks the ones the provider logs show
-- as sent after the backup, so the worker never sends them again. Every
-- query runs across organizations unless organization_id is given; list
-- and mark use the same filter, so mark changes exactly what list showed.

-- name: ListReconcileDispatches :many
SELECT d.id, d.uuid, d.organization_id, o.slug AS organization_slug, d.event_code, d.audience, d.status,
    d.channels, d.next_channel_index, d.test, d.created_at, d.scheduled_at
FROM notification_dispatches d
JOIN organizations o ON o.id = d.organization_id
WHERE d.status IN ('pending', 'sending')
  AND d.created_at < sqlc.arg(before_at)::timestamptz
  AND d.scheduled_at <= sqlc.arg(due_until)::timestamptz
  AND (sqlc.narg(organization_id)::bigint IS NULL OR d.organization_id = sqlc.narg(organization_id)::bigint)
ORDER BY d.created_at, d.id
LIMIT sqlc.arg(limit_count);

-- name: ReconcileDispatches :many
UPDATE notification_dispatches AS d
SET status = 'reconciled', last_error = sqlc.arg(note)::text, completed_at = NOW()
WHERE d.status IN ('pending', 'sending')
  AND d.created_at < sqlc.arg(before_at)::timestamptz
  AND d.scheduled_at <= sqlc.arg(due_until)::timestamptz
  AND (sqlc.narg(organization_id)::bigint IS NULL OR d.organization_id = sqlc.narg(organization_id)::bigint)
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR d.uuid = ANY(sqlc.narg(uuids)::uuid[]))
RETURNING d.organization_id, d.uuid;

-- name: ListReconcileSMSMessages :many
-- Queued SMS never accepted by the provider (no provider message id): the
-- runner would submit them again. SMS with a provider id are checked by
-- their delivery report, never resent.
SELECT m.id, m.uuid, m.organization_id, o.slug AS organization_slug, m.class, m.purpose, m.provider, m.phone_masked,
    m.status, m.ref_type, m.created_at
FROM sms_messages m
JOIN organizations o ON o.id = m.organization_id
WHERE m.status = 'queued'
  AND m.provider_message_id = ''
  AND m.created_at < sqlc.arg(before_at)::timestamptz
  AND (sqlc.narg(organization_id)::bigint IS NULL OR m.organization_id = sqlc.narg(organization_id)::bigint)
ORDER BY m.created_at, m.id
LIMIT sqlc.arg(limit_count);

-- name: ReconcileSMSMessages :many
-- The send job then finishes without calling the provider (the SMS send
-- operation checks the message status first).
UPDATE sms_messages AS m
SET status = 'reconciled', error = sqlc.arg(note)::text, payload_ciphertext = ''
WHERE m.status = 'queued'
  AND m.provider_message_id = ''
  AND m.created_at < sqlc.arg(before_at)::timestamptz
  AND (sqlc.narg(organization_id)::bigint IS NULL OR m.organization_id = sqlc.narg(organization_id)::bigint)
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR m.uuid = ANY(sqlc.narg(uuids)::uuid[]))
RETURNING m.organization_id, m.uuid;

-- name: ListReconcileNotifications :many
-- Platform and organization notifications (e-mail, in-app, push) still
-- queued or processing.
SELECT n.id, n.uuid, n.organization_id, n.channel, n.status, COALESCE(n.template_code, '')::text AS template_code,
    COALESCE(n.source_event, '')::text AS source_event, COALESCE(n.recipient, '')::text AS recipient, n.created_at
FROM notifications n
WHERE n.status IN ('queued', 'processing')
  AND n.created_at < sqlc.arg(before_at)::timestamptz
  AND (n.scheduled_at IS NULL OR n.scheduled_at <= sqlc.arg(due_until)::timestamptz)
  AND (sqlc.narg(organization_id)::bigint IS NULL OR n.organization_id = sqlc.narg(organization_id)::bigint)
ORDER BY n.created_at, n.id
LIMIT sqlc.arg(limit_count);

-- name: ReconcileNotifications :many
UPDATE notifications AS n
SET status = 'cancelled', cancelled_at = NOW(), last_error = sqlc.arg(note)::text
WHERE n.status IN ('queued', 'processing')
  AND n.created_at < sqlc.arg(before_at)::timestamptz
  AND (n.scheduled_at IS NULL OR n.scheduled_at <= sqlc.arg(due_until)::timestamptz)
  AND (sqlc.narg(organization_id)::bigint IS NULL OR n.organization_id = sqlc.narg(organization_id)::bigint)
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR n.uuid = ANY(sqlc.narg(uuids)::uuid[]))
RETURNING n.id, n.uuid;

-- name: GetReconcileOrganizationBySlug :one
SELECT o.id FROM organizations o WHERE o.slug = sqlc.arg(slug);
