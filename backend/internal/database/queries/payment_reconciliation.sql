-- Payment reconciliation after a restore (TEC-962,
-- docs/design/payments.md §3.8): provider notifications that name no
-- payment of ours (payment_webhook_orphans, mixed scope), refunds the
-- provider shows and our ledger does not (payment_reconciliation_items)
-- and the report of our provider payments since a time.

-- name: ListPaymentAccountsForProvider :many
-- An orphan notification names no organization: its signature is tried
-- against the payment accounts of the provider (each row carries its
-- organization_id; the service opens the account in that scope).
SELECT a.organization_id, a.id
FROM integration_accounts a
WHERE a.organization_id IS NOT NULL
  AND a.kind = 'payment'
  AND a.provider = sqlc.arg(provider)
ORDER BY a.status = 'active' DESC, a.organization_id, a.id
LIMIT sqlc.arg(row_limit);

-- name: FindSecurityDepositByConversation :one
-- Deposit pre-authorizations echo the deposit uuid as conversation id;
-- their notifications are not payment orphans.
SELECT d.organization_id, d.id
FROM security_deposits d
WHERE d.uuid = sqlc.arg(uuid)
LIMIT 1;

-- name: UpsertPaymentWebhookOrphan :one
-- One row per notification; a repeated delivery counts deliveries.
INSERT INTO payment_webhook_orphans (
    organization_id, integration_account_id, provider, dedupe_key, event_type, provider_status, provider_payment_id,
    conversation_id, provider_token, reference_code, merchant_id, signature_verified, payload
) VALUES (
    sqlc.narg(organization_id), sqlc.narg(integration_account_id), sqlc.arg(provider), sqlc.arg(dedupe_key),
    sqlc.arg(event_type), sqlc.arg(provider_status), sqlc.arg(provider_payment_id), sqlc.arg(conversation_id),
    sqlc.arg(provider_token), sqlc.arg(reference_code), sqlc.arg(merchant_id), sqlc.arg(signature_verified),
    sqlc.arg(payload)
)
ON CONFLICT (provider, dedupe_key) DO UPDATE
SET deliveries = payment_webhook_orphans.deliveries + 1
RETURNING id, uuid, organization_id, status, (xmax = 0)::boolean AS inserted;

-- name: ListPaymentWebhookOrphans :many
SELECT o.uuid, o.provider, o.event_type, o.provider_status, o.provider_payment_id, o.conversation_id,
    o.reference_code, o.signature_verified, o.deliveries, o.status, o.matched_at, o.match_note, o.created_at,
    o.updated_at, a.uuid AS account_uuid, a.display_name AS account_name,
    p.uuid AS payment_uuid, r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    mu.uuid AS matched_by_uuid, mu.name AS matched_by_name, mu.surname AS matched_by_surname,
    mu.email AS matched_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM payment_webhook_orphans o
LEFT JOIN integration_accounts a ON a.organization_id = o.organization_id AND a.id = o.integration_account_id
LEFT JOIN payments p ON p.organization_id = o.organization_id AND p.id = o.matched_payment_id
LEFT JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
LEFT JOIN users mu ON mu.id = o.matched_by
WHERE o.organization_id = sqlc.arg(organization_id)
  AND (a.organization_id IS NULL OR a.organization_id = sqlc.arg(organization_id))
  AND (p.organization_id IS NULL OR p.organization_id = sqlc.arg(organization_id))
  AND (r.organization_id IS NULL OR r.organization_id = sqlc.arg(organization_id))
  AND (sqlc.narg(statuses)::text[] IS NULL OR o.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(since)::timestamptz IS NULL OR o.created_at >= sqlc.narg(since)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR o.provider_payment_id ILIKE '%' || sqlc.narg(q) || '%'
    OR o.conversation_id ILIKE '%' || sqlc.narg(q) || '%'
    OR o.reference_code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN o.created_at END ASC,
    o.created_at DESC, o.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetPaymentWebhookOrphan :one
SELECT o.id, o.uuid, o.organization_id, o.integration_account_id, o.provider, o.event_type, o.provider_status,
    o.provider_payment_id, o.conversation_id, o.provider_token, o.reference_code, o.signature_verified, o.status,
    o.matched_payment_id
FROM payment_webhook_orphans o
WHERE o.organization_id = sqlc.arg(organization_id)
  AND o.uuid = sqlc.arg(uuid);

-- name: MarkPaymentWebhookOrphanMatched :execrows
UPDATE payment_webhook_orphans
SET status = 'matched', matched_payment_id = sqlc.arg(payment_id), matched_by = sqlc.narg(matched_by),
    matched_at = NOW(), match_note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'unmatched';

-- name: PaymentWebhookOrphanHealthCounts :one
-- Platform health (every organization and the platform-level rows).
SELECT COUNT(*) FILTER (WHERE o.status = 'unmatched' AND o.organization_id IS NOT NULL)::bigint AS unmatched,
    COUNT(*) FILTER (WHERE o.status = 'unmatched' AND o.organization_id IS NULL)::bigint AS unresolved,
    COUNT(*) FILTER (WHERE o.status = 'matched' AND o.matched_at >= sqlc.arg(since))::bigint AS matched
FROM payment_webhook_orphans o;

-- name: ListUnmatchedPaymentWebhookOrphans :many
-- Platform health issues (newest first, every organization and the
-- platform-level rows).
SELECT o.uuid, o.organization_id, o.provider, o.event_type, o.provider_status, o.provider_payment_id,
    o.conversation_id, o.signature_verified, o.created_at
FROM payment_webhook_orphans o
WHERE o.status = 'unmatched'
ORDER BY o.created_at DESC, o.id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetPaymentIDByConversation :one
SELECT p.id
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.conversation_id = sqlc.arg(conversation_id);

-- name: InsertMatchedPayment :one
-- A checkout paid on the provider whose row was lost (restore): recreated
-- pending under its own order reference, then confirmed by a retrieve.
INSERT INTO payments (
    uuid, organization_id, subject_type, reservation_id, integration_account_id, provider, method, status, currency,
    amount, gross_amount, net_amount, conversation_id, provider_token, note, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), 'reservation', sqlc.arg(reservation_id), sqlc.arg(integration_account_id),
    sqlc.arg(provider), 'checkout_form', 'pending', sqlc.arg(currency), sqlc.arg(amount), sqlc.arg(amount),
    sqlc.arg(amount), sqlc.arg(conversation_id), sqlc.arg(provider_token), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id;

-- name: UpsertRefundGuardItem :one
-- The refund guard's items: refund_mismatch (a refund refused) or
-- unverified (a refund made while the provider could not be read); one
-- open item per payment and kind (a later one refreshes the figures).
INSERT INTO payment_reconciliation_items (
    organization_id, payment_id, kind, currency, our_refunded, provider_refunded, data, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(payment_id), sqlc.arg(kind), sqlc.arg(currency), sqlc.arg(our_refunded),
    sqlc.arg(provider_refunded), sqlc.arg(data), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, payment_id, kind) WHERE status = 'open' DO UPDATE
SET our_refunded = EXCLUDED.our_refunded, provider_refunded = EXCLUDED.provider_refunded, data = EXCLUDED.data
RETURNING uuid;

-- name: ListPaymentReconciliationItems :many
SELECT i.uuid, i.kind, i.status, i.currency, i.our_refunded, i.provider_refunded, i.data, i.resolution,
    i.resolution_note, i.resolved_at, i.created_at,
    p.uuid AS payment_uuid, p.provider_payment_id, r.uuid AS reservation_uuid, r.reference AS reservation_reference
FROM payment_reconciliation_items i
JOIN payments p ON p.organization_id = i.organization_id AND p.id = i.payment_id
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
WHERE i.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (i.status = 'open' OR i.created_at >= sqlc.arg(since))
ORDER BY i.created_at DESC, i.id DESC
LIMIT 500;

-- name: GetPaymentReconciliationItem :one
SELECT i.id, i.uuid, i.kind, i.status, i.currency, i.our_refunded, i.provider_refunded, i.payment_id,
    p.uuid AS payment_uuid
FROM payment_reconciliation_items i
JOIN payments p ON p.organization_id = i.organization_id AND p.id = i.payment_id
WHERE i.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND i.uuid = sqlc.arg(uuid);

-- name: ResolvePaymentReconciliationItem :execrows
UPDATE payment_reconciliation_items
SET status = 'resolved', resolution = sqlc.arg(resolution), resolution_note = sqlc.arg(note),
    resolved_by = sqlc.narg(resolved_by), resolved_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'open';

-- name: ListProviderPaymentsSince :many
-- The reconciliation report: our provider payments created since a time
-- (oldest first, capped by the caller), in the caller's branch scope.
SELECT p.id, p.uuid, p.provider, p.status, p.currency, p.amount, p.gross_amount, p.refunded_amount,
    p.conversation_id, p.provider_token, p.provider_payment_id, p.integration_account_id, p.created_at,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    (SELECT COUNT(*) FROM payment_refunds f
     WHERE f.organization_id = p.organization_id AND f.payment_id = p.id AND f.status = 'pending')::bigint AS pending_refunds
FROM payments p
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND p.provider <> 'manual'
  AND p.created_at >= sqlc.arg(since)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY p.created_at, p.id
LIMIT sqlc.arg(row_limit);
