-- Customer portal (TEC-715, docs/design/customer-portal.md): sign-in
-- lookups, customer sessions, the customer's reservations and customer
-- requests. Every query on a tenant table names organization_id; the
-- session token lookup is global by design (it resolves the organization).

-- name: GetPortalOrganization :one
-- A live organization by its slug (the portal URL names it).
SELECT o.id, o.uuid, o.slug, o.name, o.status, o.access_starts_at, o.access_ends_at
FROM organizations o
WHERE o.slug = sqlc.arg(slug)
  AND o.deleted_at IS NULL;

-- name: GetPortalReservationByReference :one
SELECT r.id, r.uuid, r.reference, r.customer_id, r.status
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.reference = sqlc.arg(reference);

-- name: ListPortalContacts :many
-- Who may sign in for a reservation: the renter (the contract party) and
-- the drivers on it, with their contact values (plain in the database,
-- compared through keyed hashes in the use case). Anonymized / deleted
-- people are left out.
SELECT 'renter'::text AS role, c.id AS customer_id, NULL::bigint AS driver_id, c.phone, c.email,
    c.customer_type, c.first_name, c.last_name, c.company_title
FROM reservations r
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(reservation_id)
  AND c.deleted_at IS NULL
  AND c.anonymized_at IS NULL
UNION ALL
SELECT 'driver'::text AS role, d.customer_id, d.id AS driver_id, d.phone, d.email,
    'individual'::text AS customer_type, d.first_name, d.last_name, ''::text AS company_title
FROM reservation_drivers rd
JOIN drivers d ON d.organization_id = rd.organization_id AND d.id = rd.driver_id
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND d.organization_id = sqlc.arg(organization_id)
  AND rd.reservation_id = sqlc.arg(reservation_id)
  AND d.deleted_at IS NULL;

-- name: GetPortalOTPBinding :one
-- Channel, contact hash and time of a verified challenge (the session binds
-- the code to the contact the customer typed).
SELECT c.channel, c.phone_hash, c.created_at
FROM sms_otp_challenges c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: GetOpenPortalChallenge :one
-- The open code of a sign-in subject: its channel and contact hash (a code
-- typed with another contact of the reservation is refused before it is
-- checked, so it does not use up the open code).
SELECT c.channel, c.phone_hash
FROM sms_otp_challenges c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.purpose = sqlc.arg(purpose)
  AND c.subject_ref = sqlc.arg(subject_ref)
  AND c.status = 'pending';

-- name: InsertPortalSession :one
INSERT INTO customer_portal_sessions (
    organization_id, customer_id, driver_id, reservation_id, token_hash, channel, contact_masked, otp_challenge_uuid,
    ip_address, user_agent, last_seen_at, expires_at, hard_expires_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.narg(driver_id), sqlc.arg(reservation_id), sqlc.arg(token_hash),
    sqlc.arg(channel), sqlc.arg(contact_masked), sqlc.arg(otp_challenge_uuid), sqlc.narg(ip_address), sqlc.arg(user_agent),
    sqlc.arg(now), sqlc.arg(expires_at), sqlc.arg(hard_expires_at)
)
RETURNING id, uuid;

-- name: GetPortalSessionByToken :one
-- The session of a bearer token, with its organization (global lookup by
-- the token hash; the caller compares the organization with the URL).
SELECT s.id, s.uuid, s.organization_id, s.customer_id, s.driver_id, s.reservation_id, s.channel, s.contact_masked,
    s.last_seen_at, s.expires_at, s.hard_expires_at, s.revoked_at, s.created_at,
    o.slug AS organization_slug, o.name AS organization_name, o.status AS organization_status,
    c.uuid AS customer_uuid, c.customer_type, c.first_name, c.last_name, c.company_title,
    d.uuid AS driver_uuid, d.first_name AS driver_first_name, d.last_name AS driver_last_name
FROM customer_portal_sessions s
JOIN organizations o ON o.id = s.organization_id
JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
LEFT JOIN drivers d ON d.organization_id = s.organization_id AND d.id = s.driver_id
WHERE s.token_hash = sqlc.arg(token_hash)
  AND s.organization_id = o.id
  AND o.deleted_at IS NULL;

-- name: TouchPortalSession :execrows
-- Slides the idle expiry (never beyond the hard cap).
UPDATE customer_portal_sessions
SET last_seen_at = sqlc.arg(now),
    expires_at = LEAST(sqlc.arg(expires_at)::timestamptz, hard_expires_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- name: RevokePortalSession :execrows
UPDATE customer_portal_sessions
SET revoked_at = sqlc.arg(now),
    revoked_reason = sqlc.arg(reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- name: ListPortalReservations :many
-- The reservations a session may see: the customer's (renter session) or
-- the ones the driver is on (driver session), newest pick-up first.
SELECT r.id, r.uuid, r.reference, r.status, r.payment_status, r.starts_at, r.ends_at, r.currency,
    r.total_amount, r.prepayment_amount, r.paid_amount,
    pb.name AS pickup_branch_name, rb.name AS return_branch_name, sp.title AS product_title
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
JOIN rental_products sp ON sp.organization_id = r.organization_id AND sp.id = r.sold_rental_product_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND sp.organization_id = sqlc.arg(organization_id)
  AND (
    (sqlc.narg(driver_id)::bigint IS NULL AND r.customer_id = sqlc.arg(customer_id))
    OR (sqlc.narg(driver_id)::bigint IS NOT NULL AND EXISTS (
        SELECT 1 FROM reservation_drivers rd
        WHERE rd.organization_id = r.organization_id
          AND rd.reservation_id = r.id
          AND rd.driver_id = sqlc.narg(driver_id)::bigint))
  )
ORDER BY r.starts_at DESC, r.id DESC
LIMIT 100;

-- name: GetPortalReservation :one
-- One reservation of the session's scope (same rule as the list).
SELECT r.id, r.uuid, r.reference, r.status, r.customer_id, r.policy_snapshot, r.currency,
    r.total_amount, r.prepayment_amount, r.paid_amount
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid)
  AND (
    (sqlc.narg(driver_id)::bigint IS NULL AND r.customer_id = sqlc.arg(customer_id))
    OR (sqlc.narg(driver_id)::bigint IS NOT NULL AND EXISTS (
        SELECT 1 FROM reservation_drivers rd
        WHERE rd.organization_id = r.organization_id
          AND rd.reservation_id = r.id
          AND rd.driver_id = sqlc.narg(driver_id)::bigint))
  );

-- name: GetActivePaymentLinkForReservation :one
-- The newest active, unexpired pay-by-link link of a reservation.
SELECT l.uuid, l.token, l.amount, l.currency, l.purpose, l.expires_at
FROM payment_links l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.reservation_id = sqlc.arg(reservation_id)
  AND l.status = 'active'
  AND l.expires_at > sqlc.arg(now)
ORDER BY l.created_at DESC, l.id DESC
LIMIT 1;

-- name: InsertCustomerRequest :one
INSERT INTO customer_requests (
    organization_id, reservation_id, customer_id, driver_id, session_id, request_type, status, message,
    requested_starts_at, requested_ends_at, policy_text, cancellation_fee, currency, outcome, contact_masked,
    resolution_note, resolved_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(customer_id), sqlc.narg(driver_id), sqlc.narg(session_id),
    sqlc.arg(request_type), sqlc.arg(status), sqlc.arg(message), sqlc.narg(requested_starts_at), sqlc.narg(requested_ends_at),
    sqlc.arg(policy_text), sqlc.narg(cancellation_fee), sqlc.arg(currency), sqlc.arg(outcome), sqlc.arg(contact_masked),
    sqlc.arg(resolution_note), sqlc.narg(resolved_at)
)
RETURNING id, uuid;

-- name: ListReservationCustomerRequests :many
-- The requests of one reservation (portal view; newest first).
SELECT q.uuid, q.request_type, q.status, q.message, q.requested_starts_at, q.requested_ends_at, q.policy_text,
    q.cancellation_fee, q.currency, q.outcome, q.resolution_note, q.resolved_at, q.created_at
FROM customer_requests q
WHERE q.organization_id = sqlc.arg(organization_id)
  AND q.reservation_id = sqlc.arg(reservation_id)
ORDER BY q.created_at DESC, q.id DESC
LIMIT 50;

-- name: ListCustomerRequests :many
-- One page of customer requests in the caller's branch scope (pick-up OR
-- return branch of the reservation).
SELECT q.id, q.uuid, q.request_type, q.status, q.message, q.requested_starts_at, q.requested_ends_at, q.policy_text,
    q.cancellation_fee, q.currency, q.outcome, q.contact_masked, q.resolution_note, q.resolved_at, q.created_at, q.updated_at,
    r.id AS reservation_id, r.uuid AS reservation_uuid, r.reference AS reservation_reference, r.status AS reservation_status,
    r.starts_at AS reservation_starts_at, r.ends_at AS reservation_ends_at,
    c.uuid AS customer_uuid, c.customer_type, c.first_name, c.last_name, c.company_title,
    d.first_name AS driver_first_name, d.last_name AS driver_last_name,
    u.uuid AS resolved_by_uuid, u.name AS resolved_by_name, u.surname AS resolved_by_surname, u.email AS resolved_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM customer_requests q
JOIN reservations r ON r.organization_id = q.organization_id AND r.id = q.reservation_id
JOIN customers c ON c.organization_id = q.organization_id AND c.id = q.customer_id
LEFT JOIN drivers d ON d.organization_id = q.organization_id AND d.id = q.driver_id
LEFT JOIN users u ON u.id = q.resolved_by
WHERE q.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL
       OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR q.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(request_types)::text[] IS NULL OR q.request_type = ANY(sqlc.narg(request_types)::text[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(uuid)::uuid IS NULL OR q.uuid = sqlc.narg(uuid)::uuid)
  AND (sqlc.narg(q)::text IS NULL OR r.reference ILIKE '%' || sqlc.narg(q)::text || '%' OR q.message ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_desc)::bool THEN q.created_at END DESC,
    CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN q.created_at END ASC,
    q.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOpenCustomerRequests :one
SELECT COUNT(*)::bigint
FROM customer_requests q
JOIN reservations r ON r.organization_id = q.organization_id AND r.id = q.reservation_id
WHERE q.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND q.status = 'open'
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL
       OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
       OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[]));

-- name: ResolveCustomerRequest :execrows
UPDATE customer_requests
SET status = sqlc.arg(status),
    resolution_note = sqlc.arg(resolution_note),
    resolved_by = sqlc.narg(resolved_by),
    resolved_at = sqlc.arg(resolved_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'open';
