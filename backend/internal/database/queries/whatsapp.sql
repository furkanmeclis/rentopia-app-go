-- WhatsApp channel (TEC-735, docs/design/whatsapp.md).

-- Account ---------------------------------------------------------------

-- name: SetWhatsAppAccountCredentials :execrows
-- The generated session token sealed into the WhatsApp integration
-- account (the account form has no fields: nothing is typed in).
UPDATE integration_accounts
SET credentials_ciphertext = sqlc.arg(credentials_ciphertext),
    credentials_hint = sqlc.arg(credentials_hint),
    updated_by = sqlc.narg(updated_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND kind = 'whatsapp';

-- Sessions --------------------------------------------------------------

-- name: GetWhatsAppSessionByAccount :one
SELECT * FROM whatsapp_sessions s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.integration_account_id = sqlc.arg(integration_account_id);

-- name: GetWhatsAppSessionByWebhook :one
-- Webhook deliveries name their session by the secret URL segment; the
-- row carries its organization_id for every later query.
SELECT * FROM whatsapp_sessions s
WHERE s.webhook_token_hash = sqlc.arg(webhook_token_hash)
  AND s.organization_id > 0;

-- name: InsertWhatsAppSession :one
INSERT INTO whatsapp_sessions (
    organization_id, integration_account_id, provider, status, webhook_token_hash,
    risk_acknowledged_at, risk_acknowledged_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(provider), 'created',
    sqlc.arg(webhook_token_hash), sqlc.arg(risk_acknowledged_at), sqlc.narg(risk_acknowledged_by)
)
RETURNING *;

-- name: ResetWhatsAppSession :one
-- A new provider session for the account (after a removal on the server).
UPDATE whatsapp_sessions
SET provider = sqlc.arg(provider), status = 'created', webhook_token_hash = sqlc.arg(webhook_token_hash),
    phone_masked = '', risk_acknowledged_at = sqlc.arg(risk_acknowledged_at),
    risk_acknowledged_by = sqlc.narg(risk_acknowledged_by), connected_at = NULL, disconnected_at = NULL,
    last_status_at = NULL, last_error = '', alerted_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: AcknowledgeWhatsAppRisk :exec
UPDATE whatsapp_sessions
SET risk_acknowledged_at = sqlc.arg(risk_acknowledged_at), risk_acknowledged_by = sqlc.narg(risk_acknowledged_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetWhatsAppSessionState :one
UPDATE whatsapp_sessions
SET status = sqlc.arg(status),
    phone_masked = sqlc.arg(phone_masked),
    connected_at = sqlc.narg(connected_at),
    disconnected_at = sqlc.narg(disconnected_at),
    last_status_at = sqlc.arg(last_status_at),
    last_error = sqlc.arg(last_error),
    alerted_at = sqlc.narg(alerted_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: SetWhatsAppSessionSettings :one
UPDATE whatsapp_sessions
SET rate_limit_per_minute = sqlc.arg(rate_limit_per_minute)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: ListWhatsAppSessionsToWatch :many
-- Sessions of active WhatsApp accounts that were linked once: the health
-- sweep reads their state at the provider.
SELECT s.id, s.organization_id, s.integration_account_id, s.status
FROM whatsapp_sessions s
JOIN integration_accounts a ON a.organization_id = s.organization_id AND a.id = s.integration_account_id
WHERE s.organization_id > 0
  AND a.organization_id = s.organization_id
  AND a.status = 'active'
  AND a.kind = 'whatsapp'
  AND s.status IN ('connected', 'disconnected')
ORDER BY s.id;

-- name: ListWhatsAppAlertRecipients :many
-- Members who manage integrations (they can reconnect the session).
SELECT DISTINCT om.user_id, COALESCE(u.locale, '')::text AS locale
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL AND u.status <> 'disabled'
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
ORDER BY om.user_id;

-- Messages --------------------------------------------------------------

-- name: CountWhatsAppSessionSince :one
-- Sends of the session since a time (rate limit).
SELECT COUNT(*)::bigint
FROM whatsapp_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(since)
  AND m.status <> 'failed';

-- name: CountWhatsAppRecipientSince :one
SELECT COUNT(*)::bigint
FROM whatsapp_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.session_id = sqlc.arg(session_id)
  AND m.recipient_hash = sqlc.arg(recipient_hash)
  AND m.created_at >= sqlc.arg(since)
  AND m.status <> 'failed';

-- name: InsertWhatsAppMessage :one
INSERT INTO whatsapp_messages (
    organization_id, session_id, kind, purpose, delivery_uuid, customer_id, recipient_masked, recipient_hash,
    provider_message_id, status, test
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(session_id), sqlc.arg(kind), sqlc.arg(purpose), sqlc.narg(delivery_uuid),
    sqlc.narg(customer_id), sqlc.arg(recipient_masked), sqlc.arg(recipient_hash), sqlc.arg(provider_message_id),
    'queued', sqlc.arg(test)
)
RETURNING *;

-- name: MarkWhatsAppMessageSent :exec
UPDATE whatsapp_messages
SET status = 'sent', sent_at = sqlc.arg(sent_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'queued';

-- name: MarkWhatsAppMessageFailed :execrows
UPDATE whatsapp_messages
SET status = 'failed', error = sqlc.arg(error), failed_at = sqlc.arg(failed_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('queued', 'sent');

-- name: GetWhatsAppMessageByProviderID :one
SELECT * FROM whatsapp_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.provider_message_id = sqlc.arg(provider_message_id);

-- name: MarkWhatsAppMessageDelivered :execrows
-- A delivery receipt; a late one after the read receipt keeps "read".
UPDATE whatsapp_messages
SET status = CASE WHEN status = 'read' THEN 'read' ELSE 'delivered' END,
    delivered_at = COALESCE(delivered_at, sqlc.arg(at)),
    error = '', failed_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('queued', 'sent', 'delivered', 'failed')
  AND delivered_at IS NULL;

-- name: MarkWhatsAppMessageRead :execrows
UPDATE whatsapp_messages
SET status = 'read', read_at = sqlc.arg(at), delivered_at = COALESCE(delivered_at, sqlc.arg(at)),
    error = '', failed_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND read_at IS NULL;

-- name: ListWhatsAppMessagesAwaitingReceipt :many
-- Sent messages without a delivery receipt since before a time.
SELECT * FROM whatsapp_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.status = 'sent'
  AND m.sent_at < sqlc.arg(before)
ORDER BY m.sent_at
LIMIT 200;

-- name: ListWhatsAppOrganizationsAwaitingReceipt :many
SELECT DISTINCT m.organization_id
FROM whatsapp_messages m
WHERE m.organization_id > 0
  AND m.status = 'sent'
  AND m.sent_at < sqlc.arg(before)
ORDER BY m.organization_id;

-- name: ListWhatsAppMessages :many
SELECT m.uuid, m.kind, m.purpose, m.delivery_uuid, m.recipient_masked, m.provider_message_id, m.status, m.error,
    m.test, m.sent_at, m.delivered_at, m.read_at, m.failed_at, m.created_at,
    COALESCE(c.uuid, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS customer_uuid,
    COALESCE(NULLIF(c.company_title, ''), TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, '')), '')::text AS customer_name,
    COUNT(*) OVER ()::bigint AS total_count
FROM whatsapp_messages m
LEFT JOIN customers c ON c.organization_id = m.organization_id AND c.id = m.customer_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR m.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR m.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR m.created_at < sqlc.narg(created_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.recipient_masked ILIKE '%' || sqlc.narg(q) || '%'
    OR m.purpose ILIKE '%' || sqlc.narg(q) || '%'
    OR m.provider_message_id ILIKE '%' || sqlc.narg(q) || '%'
    OR c.first_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.last_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN m.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN m.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN m.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'purpose' AND NOT sqlc.arg(sort_desc)::boolean THEN m.purpose END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'purpose' AND sqlc.arg(sort_desc)::boolean THEN m.purpose END DESC,
  m.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
