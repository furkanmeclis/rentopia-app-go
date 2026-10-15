-- Payments, refunds and payment links (TEC-707). Every query filters by
-- organization_id; lists apply the reservation's branch scope (pick-up OR
-- return branch). The two public resolvers (pay-link token, provider
-- conversation id) return the organization of the row so the caller can
-- scope everything after them.

-- name: InsertPaymentLink :one
INSERT INTO payment_links (
    uuid, organization_id, token, subject_type, reservation_id, integration_account_id, purpose, amount, currency,
    expires_at, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(token), 'reservation', sqlc.arg(reservation_id),
    sqlc.arg(integration_account_id), sqlc.arg(purpose), sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(expires_at),
    sqlc.narg(created_by)
)
RETURNING id;

-- name: CancelActivePaymentLinks :execrows
-- A new link replaces the reservation's active one.
UPDATE payment_links
SET status = 'cancelled', cancelled_at = sqlc.arg(cancelled_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND reservation_id = sqlc.arg(reservation_id)
  AND status = 'active';

-- name: CancelPaymentLink :execrows
UPDATE payment_links
SET status = 'cancelled', cancelled_at = sqlc.arg(cancelled_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: ExpirePaymentLink :execrows
UPDATE payment_links
SET status = 'expired'
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active'
  AND expires_at <= sqlc.arg(now_at);

-- name: MarkPaymentLinkPaid :execrows
-- A verified payment closes its link (also an expired one: a late payment
-- is still a payment of that link).
UPDATE payment_links
SET status = 'paid', paid_at = sqlc.arg(paid_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('active', 'expired', 'cancelled');

-- name: ResolvePaymentLinkToken :one
-- Public pay page: the token names the link and its organization.
SELECT l.organization_id, l.id, l.uuid
FROM payment_links l
WHERE l.token = sqlc.arg(token);

-- name: GetPaymentLink :one
SELECT l.id, l.uuid, l.token, l.purpose, l.amount, l.currency, l.status, l.expires_at, l.paid_at, l.cancelled_at,
    l.created_at, l.integration_account_id,
    r.id AS reservation_id, r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    r.status AS reservation_status, r.pickup_branch_id, r.return_branch_id, r.starts_at AS reservation_starts_at,
    r.ends_at AS reservation_ends_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM payment_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
LEFT JOIN users cu ON cu.id = l.created_by
WHERE l.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND l.uuid = sqlc.arg(uuid);

-- name: GetPaymentLinkPublic :one
-- What the public pay page shows: organization name, reservation
-- reference, dates, product title and the amount.
SELECT l.id, l.uuid, l.token, l.purpose, l.amount, l.currency, l.status, l.expires_at, l.paid_at,
    l.integration_account_id,
    r.id AS reservation_id, r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    r.starts_at AS reservation_starts_at, r.ends_at AS reservation_ends_at,
    p.title AS product_title, o.name AS organization_name, o.uuid AS organization_uuid
FROM payment_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
JOIN organizations o ON o.id = l.organization_id
WHERE l.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND l.id = sqlc.arg(id);

-- name: ListPaymentLinks :many
SELECT l.uuid, l.token, l.purpose, l.amount, l.currency, l.status, l.expires_at, l.paid_at, l.cancelled_at, l.created_at,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM payment_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
LEFT JOIN users cu ON cu.id = l.created_by
WHERE l.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR l.status = ANY(sqlc.narg(statuses)::text[]))
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: InsertPayment :one
INSERT INTO payments (
    uuid, organization_id, subject_type, reservation_id, payment_link_id, integration_account_id, provider, method,
    status, currency, amount, gross_amount, net_amount, conversation_id, verified_via, verified_at, note, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), 'reservation', sqlc.arg(reservation_id), sqlc.narg(payment_link_id),
    sqlc.narg(integration_account_id), sqlc.arg(provider), sqlc.arg(method), sqlc.arg(status), sqlc.arg(currency),
    sqlc.arg(amount), sqlc.arg(gross_amount), sqlc.arg(net_amount), sqlc.arg(conversation_id), sqlc.arg(verified_via),
    sqlc.narg(verified_at), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id;

-- name: SetPaymentCheckout :execrows
UPDATE payments
SET provider_token = sqlc.arg(provider_token)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: ResolvePaymentConversation :many
-- Public webhook: the provider echoes our conversation id; it names the
-- payment and its organization (at most one row; two = refused).
SELECT p.organization_id, p.id, p.uuid
FROM payments p
WHERE p.conversation_id = sqlc.arg(conversation_id)
LIMIT 2;

-- name: GetPayment :one
SELECT p.id, p.uuid, p.provider, p.method, p.status, p.currency, p.amount, p.gross_amount, p.commission_amount,
    p.net_amount, p.refunded_amount, p.conversation_id, p.provider_token, p.provider_payment_id, p.provider_status,
    p.provider_data, p.failure_reason, p.verified_via, p.verified_at, p.applied_at, p.outcome, p.note,
    p.integration_account_id, p.payment_link_id, p.created_at, p.updated_at,
    r.id AS reservation_id, r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    r.pickup_branch_id, r.return_branch_id,
    l.uuid AS link_uuid,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM payments p
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
LEFT JOIN payment_links l ON l.organization_id = p.organization_id AND l.id = p.payment_link_id
LEFT JOIN users cu ON cu.id = p.created_by
WHERE p.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid);

-- name: GetPaymentByID :one
SELECT p.id, p.uuid, p.provider, p.method, p.status, p.currency, p.amount, p.refunded_amount, p.conversation_id,
    p.provider_token, p.provider_payment_id, p.integration_account_id, p.payment_link_id, p.applied_at,
    p.reservation_id, p.created_at
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.id = sqlc.arg(id);

-- name: GetLatestPendingLinkPayment :one
-- The newest checkout of a link still waiting for confirmation.
SELECT p.id, p.uuid
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.payment_link_id = sqlc.arg(payment_link_id)
  AND p.status = 'pending'
  AND p.provider_token IS NOT NULL
ORDER BY p.created_at DESC, p.id DESC
LIMIT 1;

-- name: GetLatestLinkPayment :one
SELECT p.id, p.uuid, p.status, p.failure_reason, p.outcome
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.payment_link_id = sqlc.arg(payment_link_id)
ORDER BY p.created_at DESC, p.id DESC
LIMIT 1;

-- name: ConfirmPayment :execrows
-- pending → succeeded exactly once (concurrent deliveries: one wins).
UPDATE payments
SET status = 'succeeded',
    provider_payment_id = sqlc.arg(provider_payment_id),
    provider_status = sqlc.arg(provider_status),
    gross_amount = sqlc.arg(gross_amount),
    commission_amount = sqlc.arg(commission_amount),
    net_amount = sqlc.arg(net_amount),
    provider_data = sqlc.arg(provider_data),
    verified_via = sqlc.arg(verified_via),
    verified_at = sqlc.arg(verified_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: FailPayment :execrows
UPDATE payments
SET status = 'failed', provider_status = sqlc.arg(provider_status), failure_reason = sqlc.arg(failure_reason),
    verified_via = sqlc.arg(verified_via), verified_at = sqlc.arg(verified_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: ClaimPaymentApply :execrows
-- One caller tells the reservation (concurrent deliveries / polls): the
-- claim lapses after two minutes so a crashed apply is retried.
UPDATE payments
SET outcome = 'applying'
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND applied_at IS NULL
  AND status IN ('succeeded', 'partially_refunded', 'refunded')
  AND (outcome = '' OR (outcome = 'applying' AND updated_at < sqlc.arg(stale_before)));

-- name: MarkPaymentApplied :exec
UPDATE payments
SET applied_at = sqlc.arg(applied_at), outcome = sqlc.arg(outcome)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SumReservationPayments :one
-- What the reservation has received: amounts of confirmed payments minus
-- their refunds, and the refunded total.
SELECT COALESCE(SUM(p.amount - p.refunded_amount), 0)::numeric(14, 2) AS paid,
    COALESCE(SUM(p.refunded_amount), 0)::numeric(14, 2) AS refunded
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.reservation_id = sqlc.arg(reservation_id)
  AND p.status IN ('succeeded', 'partially_refunded', 'refunded');

-- name: ListPayments :many
SELECT p.uuid, p.provider, p.method, p.status, p.currency, p.amount, p.gross_amount, p.commission_amount,
    p.net_amount, p.refunded_amount, p.provider_payment_id, p.verified_via, p.verified_at, p.outcome,
    p.failure_reason, p.note, p.created_at,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    l.uuid AS link_uuid,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM payments p
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
LEFT JOIN payment_links l ON l.organization_id = p.organization_id AND l.id = p.payment_link_id
LEFT JOIN users cu ON cu.id = p.created_by
WHERE p.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(providers)::text[] IS NULL OR p.provider = ANY(sqlc.narg(providers)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR p.provider_payment_id ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN p.amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN p.amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
    p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListPendingPaymentOrganizations :many
-- Reconciliation sweep: organizations with checkouts waiting longer than
-- the grace period (a lost webhook is recovered by a provider retrieve).
SELECT DISTINCT p.organization_id
FROM payments p
WHERE p.status = 'pending'
  AND p.provider_token IS NOT NULL
  AND p.created_at <= sqlc.arg(older_than)
  AND p.created_at > sqlc.arg(newer_than);

-- name: ListPendingPayments :many
SELECT p.uuid
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.status = 'pending'
  AND p.provider_token IS NOT NULL
  AND p.created_at <= sqlc.arg(older_than)
  AND p.created_at > sqlc.arg(newer_than)
ORDER BY p.id
LIMIT 200;

-- name: GetReservationRefByID :one
SELECT r.uuid, r.reference
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id);

-- name: InsertPaymentRefund :one
INSERT INTO payment_refunds (organization_id, payment_id, amount, currency, reason, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(payment_id), sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(reason),
    sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: FinishPaymentRefund :exec
UPDATE payment_refunds
SET status = sqlc.arg(status), provider_refund_id = sqlc.narg(provider_refund_id),
    failure_reason = sqlc.arg(failure_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: AddPaymentRefunded :execrows
-- Accumulates a refund (never above the amount).
UPDATE payments
SET refunded_amount = refunded_amount + sqlc.arg(amount)::numeric,
    status = CASE WHEN refunded_amount + sqlc.arg(amount)::numeric >= amount THEN 'refunded' ELSE 'partially_refunded' END
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('succeeded', 'partially_refunded')
  AND refunded_amount + sqlc.arg(amount)::numeric <= amount;

-- name: ListPaymentRefunds :many
SELECT f.uuid, f.amount, f.currency, f.status, f.provider_refund_id, f.reason, f.failure_reason, f.created_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM payment_refunds f
LEFT JOIN users cu ON cu.id = f.created_by
WHERE f.organization_id = sqlc.arg(organization_id)
  AND f.payment_id = sqlc.arg(payment_id)
ORDER BY f.created_at, f.id;
