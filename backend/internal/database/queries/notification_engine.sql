-- Notification engine (TEC-733, docs/design/notification-engine.md).

-- Rules -----------------------------------------------------------------

-- name: ListNotificationRules :many
SELECT * FROM notification_rules r
WHERE r.organization_id = sqlc.arg(organization_id)
ORDER BY r.event_code;

-- name: GetNotificationRule :one
SELECT * FROM notification_rules r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.event_code = sqlc.arg(event_code);

-- name: UpsertNotificationRule :one
INSERT INTO notification_rules (organization_id, event_code, enabled, channels, fallback, reminder_offsets, updated_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(event_code), sqlc.arg(enabled), sqlc.arg(channels)::text[],
    sqlc.arg(fallback), sqlc.arg(reminder_offsets)::integer[], sqlc.narg(updated_by)
)
ON CONFLICT (organization_id, event_code) DO UPDATE
SET enabled = EXCLUDED.enabled, channels = EXCLUDED.channels, fallback = EXCLUDED.fallback,
    reminder_offsets = EXCLUDED.reminder_offsets, updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: DeleteNotificationRule :execrows
DELETE FROM notification_rules
WHERE organization_id = sqlc.arg(organization_id)
  AND event_code = sqlc.arg(event_code);

-- Templates -------------------------------------------------------------

-- name: ListNotificationMessageTemplates :many
SELECT * FROM notification_message_templates t
WHERE t.organization_id = sqlc.arg(organization_id)
ORDER BY t.event_code, t.channel, t.language;

-- name: GetNotificationMessageTemplate :one
SELECT * FROM notification_message_templates t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.event_code = sqlc.arg(event_code)
  AND t.channel = sqlc.arg(channel)
  AND t.language = sqlc.arg(language);

-- name: UpsertNotificationMessageTemplate :one
INSERT INTO notification_message_templates (organization_id, event_code, channel, language, subject, body, updated_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(event_code), sqlc.arg(channel), sqlc.arg(language),
    sqlc.arg(subject), sqlc.arg(body), sqlc.narg(updated_by)
)
ON CONFLICT (organization_id, event_code, channel, language) DO UPDATE
SET subject = EXCLUDED.subject, body = EXCLUDED.body, updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: DeleteNotificationMessageTemplate :execrows
DELETE FROM notification_message_templates
WHERE organization_id = sqlc.arg(organization_id)
  AND event_code = sqlc.arg(event_code)
  AND channel = sqlc.arg(channel)
  AND language = sqlc.arg(language);

-- Dispatches ------------------------------------------------------------

-- name: InsertNotificationDispatch :one
-- A second event with the same idempotency key inserts nothing (no rows).
INSERT INTO notification_dispatches (
    organization_id, event_code, audience, customer_id, user_id, ref_type, ref_uuid, idempotency_key,
    language, vars, channels, fallback, status, scheduled_at, last_error, completed_at, test, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(event_code), sqlc.arg(audience), sqlc.narg(customer_id), sqlc.narg(user_id),
    sqlc.arg(ref_type), sqlc.narg(ref_uuid), sqlc.arg(idempotency_key), sqlc.arg(language), sqlc.arg(vars),
    sqlc.arg(channels)::text[], sqlc.arg(fallback), sqlc.arg(status), sqlc.arg(scheduled_at),
    sqlc.arg(last_error), sqlc.narg(completed_at), sqlc.arg(test), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetNotificationDispatchByKey :one
SELECT * FROM notification_dispatches d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.idempotency_key = sqlc.arg(idempotency_key);

-- name: GetNotificationDispatch :one
SELECT * FROM notification_dispatches d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id);

-- name: LockNotificationDispatch :one
SELECT * FROM notification_dispatches d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id)
FOR UPDATE SKIP LOCKED;

-- name: LockNotificationDispatchWait :one
SELECT * FROM notification_dispatches d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id)
FOR UPDATE;

-- name: GetNotificationDispatchView :one
SELECT d.*,
    COALESCE(c.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS customer_uuid,
    COALESCE(NULLIF(c.company_title, ''), TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, '')), '')::text AS customer_name,
    COALESCE(u.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS user_uuid,
    TRIM(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, ''))::text AS user_name
FROM notification_dispatches d
LEFT JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
LEFT JOIN users u ON u.id = d.user_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid);

-- name: UpdateNotificationDispatchState :exec
UPDATE notification_dispatches
SET status = sqlc.arg(status), next_channel_index = sqlc.arg(next_channel_index),
    last_error = sqlc.arg(last_error), completed_at = sqlc.narg(completed_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListDueNotificationDispatchOrganizations :many
-- Organizations with dispatches due (sweep).
SELECT DISTINCT d.organization_id
FROM notification_dispatches d
WHERE d.status = 'pending'
  AND d.scheduled_at <= sqlc.arg(now_at)::timestamptz
LIMIT 1000;

-- name: ListDueNotificationDispatches :many
SELECT d.id FROM notification_dispatches d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.status = 'pending'
  AND d.scheduled_at <= sqlc.arg(now_at)::timestamptz
ORDER BY d.scheduled_at, d.id
LIMIT sqlc.arg(limit_count);

-- Deliveries ------------------------------------------------------------

-- name: InsertNotificationDelivery :one
-- One attempt per dispatch and channel: a conflicting insert returns no rows.
INSERT INTO notification_deliveries (
    organization_id, dispatch_id, channel, attempt_no, status, subject, body
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(dispatch_id), sqlc.arg(channel), sqlc.arg(attempt_no),
    sqlc.arg(status), sqlc.arg(subject), sqlc.arg(body)
)
ON CONFLICT (organization_id, dispatch_id, channel) DO NOTHING
RETURNING *;

-- name: UpdateNotificationDelivery :execrows
-- The send's result; a delivery report that arrived first is kept.
UPDATE notification_deliveries
SET status = sqlc.arg(status), provider = sqlc.arg(provider), provider_ref = sqlc.arg(provider_ref),
    external_uuid = sqlc.narg(external_uuid), recipient_masked = sqlc.arg(recipient_masked),
    error = sqlc.arg(error), sent_at = sqlc.narg(sent_at), finished_at = sqlc.narg(finished_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'sending';

-- name: GetNotificationDeliveryByUUID :one
SELECT * FROM notification_deliveries dl
WHERE dl.organization_id = sqlc.arg(organization_id)
  AND dl.uuid = sqlc.arg(uuid);

-- name: GetNotifySMSRef :one
-- The notification delivery an SMS was sent for (delivery reports).
SELECT m.ref_type, m.ref_uuid FROM sms_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.uuid = sqlc.arg(uuid);

-- name: FinishNotificationDelivery :execrows
-- A delivery report: only a delivery still waiting for one changes.
UPDATE notification_deliveries
SET status = sqlc.arg(status), error = sqlc.arg(error),
    provider_ref = CASE WHEN sqlc.arg(provider_ref)::text = '' THEN provider_ref ELSE sqlc.arg(provider_ref)::text END,
    finished_at = sqlc.arg(finished_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('sending', 'sent');

-- name: GetNotificationDeliveryByExternal :one
SELECT * FROM notification_deliveries dl
WHERE dl.organization_id = sqlc.arg(organization_id)
  AND dl.channel = sqlc.arg(channel)
  AND dl.external_uuid = sqlc.arg(external_uuid);

-- name: ListNotificationDeliveriesForDispatch :many
SELECT * FROM notification_deliveries dl
WHERE dl.organization_id = sqlc.arg(organization_id)
  AND dl.dispatch_id = sqlc.arg(dispatch_id)
ORDER BY dl.attempt_no, dl.id;

-- name: ListNotificationDeliveries :many
SELECT dl.id, dl.uuid, dl.channel, dl.attempt_no, dl.provider, dl.provider_ref, dl.recipient_masked,
    dl.subject, dl.status, dl.error, dl.sent_at, dl.finished_at, dl.created_at,
    d.uuid AS dispatch_uuid, d.event_code, d.audience, d.ref_type, d.ref_uuid, d.status AS dispatch_status, d.test,
    COALESCE(c.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS customer_uuid,
    COALESCE(NULLIF(c.company_title, ''), TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, '')), '')::text AS customer_name,
    COALESCE(u.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS user_uuid,
    TRIM(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, ''))::text AS user_name,
    COUNT(*) OVER ()::bigint AS total_count
FROM notification_deliveries dl
JOIN notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
LEFT JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
LEFT JOIN users u ON u.id = d.user_id
WHERE dl.organization_id = sqlc.arg(organization_id)
  AND d.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(channels)::text[] IS NULL OR dl.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR dl.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(events)::text[] IS NULL OR d.event_code = ANY(sqlc.narg(events)::text[]))
  AND (sqlc.narg(audiences)::text[] IS NULL OR d.audience = ANY(sqlc.narg(audiences)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR dl.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR dl.created_at < sqlc.narg(created_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR dl.recipient_masked ILIKE '%' || sqlc.narg(q) || '%'
    OR dl.subject ILIKE '%' || sqlc.narg(q) || '%'
    OR dl.provider_ref ILIKE '%' || sqlc.narg(q) || '%'
    OR c.first_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.last_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN dl.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN dl.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'channel' AND NOT sqlc.arg(sort_desc)::boolean THEN dl.channel END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'channel' AND sqlc.arg(sort_desc)::boolean THEN dl.channel END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN dl.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN dl.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'event_code' AND NOT sqlc.arg(sort_desc)::boolean THEN d.event_code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'event_code' AND sqlc.arg(sort_desc)::boolean THEN d.event_code END DESC,
  dl.created_at DESC, dl.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: NotificationDeliveryFacets :many
SELECT 'channel'::text AS facet, dl.channel::text AS value, COUNT(*)::bigint AS count
FROM notification_deliveries dl WHERE dl.organization_id = sqlc.arg(organization_id) GROUP BY dl.channel
UNION ALL
SELECT 'status'::text, dl.status::text, COUNT(*)::bigint
FROM notification_deliveries dl WHERE dl.organization_id = sqlc.arg(organization_id) GROUP BY dl.status
UNION ALL
SELECT 'event_code'::text, d.event_code::text, COUNT(*)::bigint
FROM notification_deliveries dl
JOIN notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
WHERE dl.organization_id = sqlc.arg(organization_id) AND d.organization_id = sqlc.arg(organization_id)
GROUP BY d.event_code;

-- Recipients ------------------------------------------------------------

-- name: GetNotificationCustomer :one
SELECT c.id, c.uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.phone, c.email,
    COALESCE(c.nationality, '')::text AS nationality,
    (c.anonymized_at IS NOT NULL OR c.deleted_at IS NOT NULL)::boolean AS inactive
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id);

-- name: GetNotificationCustomerByUUID :one
SELECT c.id FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: GetNotificationStaff :one
-- A staff recipient: an active member of the organization.
SELECT u.id, u.uuid, u.name, u.surname, u.email, u.locale
FROM users u
JOIN organization_members m ON m.user_id = u.id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND u.id = sqlc.arg(user_id)
  AND u.deleted_at IS NULL
LIMIT 1;

-- name: CountStaffPushTargets :one
SELECT ((SELECT COUNT(*) FROM push_subscriptions ps WHERE ps.user_id = sqlc.arg(user_id))
      + (SELECT COUNT(*) FROM push_devices pd WHERE pd.user_id = sqlc.arg(user_id)))::bigint;

-- name: GetNotificationOrganization :one
SELECT o.id, o.uuid, o.slug, o.name FROM organizations o
WHERE o.id = sqlc.arg(organization_id) AND o.deleted_at IS NULL;

-- name: ListNotificationOrganizations :many
-- Live, active organizations (reminder sweep).
SELECT o.id AS organization_id FROM organizations o
WHERE o.deleted_at IS NULL AND o.status = 'active'
ORDER BY o.id;

-- Customer web push -----------------------------------------------------

-- name: UpsertCustomerPushSubscription :one
INSERT INTO customer_push_subscriptions (organization_id, customer_id, endpoint, key_p256dh, key_auth)
VALUES (sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.arg(endpoint), sqlc.arg(key_p256dh), sqlc.arg(key_auth))
ON CONFLICT (organization_id, customer_id, endpoint) DO UPDATE
SET key_p256dh = EXCLUDED.key_p256dh, key_auth = EXCLUDED.key_auth
RETURNING *;

-- name: DeleteCustomerPushSubscription :execrows
DELETE FROM customer_push_subscriptions
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND endpoint = sqlc.arg(endpoint);

-- name: ListCustomerPushSubscriptions :many
SELECT * FROM customer_push_subscriptions s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.customer_id = sqlc.arg(customer_id)
ORDER BY s.id;

-- name: DeleteCustomerPushSubscriptionByID :exec
DELETE FROM customer_push_subscriptions
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- Event sources ---------------------------------------------------------

-- name: GetNotifyReservation :one
SELECT r.id, r.uuid, r.reference, r.status, r.customer_id, r.starts_at, r.ends_at, r.payment_deadline,
    pb.name AS pickup_branch_name, rb.name AS return_branch_name
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id);

-- name: ListNotifyPickupCandidates :many
-- Confirmed reservations whose pickup is within the horizon and not handed over.
SELECT r.id, r.uuid, r.reference, r.customer_id, r.starts_at AS at, pb.name AS branch_name
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND r.status = 'confirmed'
  AND r.starts_at > sqlc.arg(now_at)::timestamptz
  AND r.starts_at <= sqlc.arg(until_at)::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM rentals rn
    WHERE rn.organization_id = r.organization_id AND rn.reservation_id = r.id
      AND rn.status IN ('active', 'return_check', 'returned')
  )
ORDER BY r.starts_at
LIMIT 2000;

-- name: ListNotifyReturnCandidates :many
-- Active rentals whose planned return is within the horizon.
SELECT rn.id, rn.uuid, rn.reference, r.customer_id, r.ends_at AS at, rb.name AS branch_name
FROM rentals rn
JOIN reservations r ON r.organization_id = rn.organization_id AND r.id = rn.reservation_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
WHERE rn.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND rn.status = 'active'
  AND r.ends_at > sqlc.arg(now_at)::timestamptz
  AND r.ends_at <= sqlc.arg(until_at)::timestamptz
ORDER BY r.ends_at
LIMIT 2000;

-- name: ListNotifyPaymentCandidates :many
-- Reservations waiting for payment whose deadline is within the horizon.
SELECT r.id, r.uuid, r.reference, r.customer_id, r.payment_deadline::timestamptz AS at, pb.name AS branch_name
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND r.status = 'awaiting_payment'
  AND r.payment_deadline IS NOT NULL
  AND r.payment_deadline > sqlc.arg(now_at)::timestamptz
  AND r.payment_deadline <= sqlc.arg(until_at)::timestamptz
ORDER BY r.payment_deadline
LIMIT 2000;

-- name: ListNotifyOverdueRentals :many
-- Active rentals that became overdue recently.
SELECT rn.id, rn.uuid, rn.reference, r.customer_id, rn.overdue_at::timestamptz AS at, rb.name AS branch_name
FROM rentals rn
JOIN reservations r ON r.organization_id = rn.organization_id AND r.id = rn.reservation_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
WHERE rn.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND rn.status = 'active'
  AND rn.overdue_at IS NOT NULL
  AND rn.overdue_at > sqlc.arg(since_at)::timestamptz
ORDER BY rn.overdue_at
LIMIT 2000;

-- name: ListNotifyIssuedInvoices :many
-- Invoices issued (accepted by the integrator) recently, for a customer.
SELECT i.id, i.uuid, i.number::text AS reference, i.customer_id::bigint AS customer_id, i.updated_at AS at
FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.status = 'succeeded'
  AND i.customer_id IS NOT NULL
  AND i.number IS NOT NULL
  AND i.updated_at > sqlc.arg(since_at)::timestamptz
ORDER BY i.updated_at
LIMIT 2000;

-- name: ListNotifyAccountDue :many
-- Customer accounts with an open (positive) balance and a debit falling
-- due within the window: the earliest such due date per account.
SELECT a.id, a.uuid, a.code AS reference, a.customer_id::bigint AS customer_id, MIN(e.due_date)::date AS due_date
FROM accounts a
JOIN ledger_entries e ON e.organization_id = a.organization_id AND e.account_id = a.id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND e.organization_id = sqlc.arg(organization_id)
  AND a.party_kind = 'customer'
  AND a.customer_id IS NOT NULL
  AND a.status = 'active'
  AND e.direction = 'debit'
  AND e.purpose <> 'deposit'
  AND e.due_date >= sqlc.arg(from_date)::date
  AND e.due_date <= sqlc.arg(to_date)::date
  AND (
    SELECT COALESCE(SUM(CASE WHEN b.direction = 'debit' THEN b.amount ELSE -b.amount END), 0)
    FROM ledger_entries b
    WHERE b.organization_id = a.organization_id AND b.account_id = a.id AND b.purpose <> 'deposit'
  ) > 0
GROUP BY a.id, a.uuid, a.code, a.customer_id
ORDER BY a.id
LIMIT 2000;

-- name: ListNotifyAssignedTasks :many
-- Field tasks assigned recently that are still open.
SELECT t.id, t.uuid, t.task_type, t.assignee_id::bigint AS assignee_id, t.due_at, t.assigned_at::timestamptz AS assigned_at,
    b.name AS branch_name
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND t.assignee_id IS NOT NULL
  AND t.assigned_at IS NOT NULL
  AND t.assigned_at > sqlc.arg(since_at)::timestamptz
  AND t.status IN ('planned', 'assigned', 'en_route')
ORDER BY t.assigned_at
LIMIT 2000;

-- name: ListNotifyOverdueTasks :many
-- Open field tasks past their due time (recently), with an assignee.
SELECT t.id, t.uuid, t.task_type, t.assignee_id::bigint AS assignee_id, t.due_at, b.name AS branch_name
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND t.assignee_id IS NOT NULL
  AND t.status IN ('planned', 'assigned', 'en_route')
  AND t.due_at < sqlc.arg(now_at)::timestamptz
  AND t.due_at > sqlc.arg(since_at)::timestamptz
ORDER BY t.due_at
LIMIT 2000;
