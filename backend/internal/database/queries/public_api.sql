-- Public booking API (TEC-728, docs/design/public-api.md): keys,
-- idempotency, the quotes / reservations the API made, the public catalog
-- and signed reservation webhooks. Every query on a tenant table names
-- organization_id; the key lookup by hash is global by design (it resolves
-- the organization), like the customer portal session token.

-- ---------------------------------------------------------------- keys

-- name: CreatePublicAPIKey :one
INSERT INTO public_api_keys (
    organization_id, kind, name, channel, key_prefix, key_last4, key_hash, publishable_key, scopes,
    allowed_origins, rate_limit_per_minute, rotated_from_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(channel), sqlc.arg(key_prefix),
    sqlc.arg(key_last4), sqlc.arg(key_hash), sqlc.narg(publishable_key), sqlc.arg(scopes)::text[],
    sqlc.arg(allowed_origins)::text[], sqlc.arg(rate_limit_per_minute), sqlc.narg(rotated_from_id), sqlc.narg(created_by)
)
RETURNING *;

-- name: GetPublicAPIKeyByHash :one
-- The key of a bearer secret / publishable key with its organization
-- (global lookup by the key hash; the organization comes from the row).
SELECT k.id, k.uuid, k.organization_id, k.kind, k.name, k.channel, k.scopes, k.allowed_origins,
    k.rate_limit_per_minute, k.status, k.expires_at, k.last_used_at,
    o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
    o.status AS organization_status, o.access_starts_at, o.access_ends_at
FROM public_api_keys k
JOIN organizations o ON o.id = k.organization_id
WHERE k.key_hash = sqlc.arg(key_hash)
  AND k.organization_id = o.id
  AND o.deleted_at IS NULL;

-- name: GetPublicAPIKeyByID :one
-- The key a booking session token names (organization + key id from the
-- verified token).
SELECT k.id, k.uuid, k.organization_id, k.kind, k.name, k.channel, k.scopes, k.allowed_origins,
    k.rate_limit_per_minute, k.status, k.expires_at, k.last_used_at,
    o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
    o.status AS organization_status, o.access_starts_at, o.access_ends_at
FROM public_api_keys k
JOIN organizations o ON o.id = k.organization_id
WHERE k.organization_id = sqlc.arg(organization_id)
  AND k.id = sqlc.arg(id)
  AND o.deleted_at IS NULL;

-- name: TouchPublicAPIKey :exec
-- Records use at most once a minute.
UPDATE public_api_keys
SET last_used_at = sqlc.arg(now), last_used_ip = sqlc.narg(ip)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND (last_used_at IS NULL OR last_used_at < sqlc.arg(now)::timestamptz - INTERVAL '1 minute');

-- name: ListPublicAPIKeys :many
SELECT k.*, cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname,
    rf.uuid AS rotated_from_uuid
FROM public_api_keys k
LEFT JOIN users cu ON cu.id = k.created_by
LEFT JOIN public_api_keys rf ON rf.organization_id = k.organization_id AND rf.id = k.rotated_from_id
WHERE k.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR k.name ILIKE '%' || sqlc.narg(q)::text || '%' OR k.key_prefix ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(kinds)::text[] IS NULL OR k.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR k.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR k.status = ANY(sqlc.narg(statuses)::text[]))
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::bool THEN k.name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::bool THEN k.name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'last_used_at' AND NOT sqlc.arg(sort_desc)::bool THEN k.last_used_at END ASC NULLS FIRST,
    CASE WHEN sqlc.arg(sort_field)::text = 'last_used_at' AND sqlc.arg(sort_desc)::bool THEN k.last_used_at END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::bool THEN k.created_at END ASC,
    k.created_at DESC, k.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountPublicAPIKeys :one
SELECT COUNT(*)::bigint
FROM public_api_keys k
WHERE k.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR k.name ILIKE '%' || sqlc.narg(q)::text || '%' OR k.key_prefix ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(kinds)::text[] IS NULL OR k.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR k.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR k.status = ANY(sqlc.narg(statuses)::text[]));

-- name: GetPublicAPIKey :one
SELECT k.*, cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname,
    rf.uuid AS rotated_from_uuid
FROM public_api_keys k
LEFT JOIN users cu ON cu.id = k.created_by
LEFT JOIN public_api_keys rf ON rf.organization_id = k.organization_id AND rf.id = k.rotated_from_id
WHERE k.organization_id = sqlc.arg(organization_id)
  AND k.uuid = sqlc.arg(uuid);

-- name: UpdatePublicAPIKey :execrows
UPDATE public_api_keys
SET name = sqlc.arg(name), scopes = sqlc.arg(scopes)::text[], allowed_origins = sqlc.arg(allowed_origins)::text[],
    rate_limit_per_minute = sqlc.arg(rate_limit_per_minute)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status = 'active';

-- name: RevokePublicAPIKey :execrows
UPDATE public_api_keys
SET status = 'revoked', revoked_at = sqlc.arg(now), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status = 'active';

-- name: ExpirePublicAPIKey :execrows
-- Rotation: the old key keeps working until expires_at (never extended).
UPDATE public_api_keys
SET expires_at = LEAST(COALESCE(expires_at, sqlc.arg(expires_at)::timestamptz), sqlc.arg(expires_at)::timestamptz)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: PublicAPIKeyLineage :many
-- The key and every key it was rotated from (a successor keeps access to
-- its predecessors' reservations).
WITH RECURSIVE chain AS (
    SELECT k.id, k.rotated_from_id, 1 AS depth
    FROM public_api_keys k
    WHERE k.organization_id = sqlc.arg(organization_id)
      AND k.id = sqlc.arg(id)
    UNION ALL
    SELECT p.id, p.rotated_from_id, c.depth + 1
    FROM public_api_keys p
    JOIN chain c ON p.id = c.rotated_from_id
    WHERE p.organization_id = sqlc.arg(organization_id)
      AND c.depth < 50
)
SELECT chain.id FROM chain;

-- ------------------------------------------------------- idempotency

-- name: DeleteExpiredPublicIdempotencyKey :exec
-- A key may be reused once its record expired.
DELETE FROM public_api_idempotency
WHERE organization_id = sqlc.arg(organization_id)
  AND key_id = sqlc.arg(key_id)
  AND subject = sqlc.arg(subject)
  AND idempotency_key = sqlc.arg(idempotency_key)
  AND expires_at <= sqlc.arg(now);

-- name: ClaimPublicIdempotency :one
-- Starts a write; no row means the key is already taken (see
-- GetPublicIdempotency).
INSERT INTO public_api_idempotency (
    organization_id, key_id, subject, idempotency_key, method, path, request_hash, expires_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(key_id), sqlc.arg(subject), sqlc.arg(idempotency_key), sqlc.arg(method),
    sqlc.arg(path), sqlc.arg(request_hash), sqlc.arg(expires_at)
)
ON CONFLICT ON CONSTRAINT uq_public_api_idempotency_key DO NOTHING
RETURNING id;

-- name: GetPublicIdempotency :one
SELECT id, request_hash, state, response_status, response_enc, created_at
FROM public_api_idempotency
WHERE organization_id = sqlc.arg(organization_id)
  AND key_id = sqlc.arg(key_id)
  AND subject = sqlc.arg(subject)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: TakeOverPublicIdempotency :execrows
-- A write that never finished (crash) may be retried after a while.
UPDATE public_api_idempotency
SET created_at = sqlc.arg(now)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND state = 'processing'
  AND created_at < sqlc.arg(stale_before);

-- name: CompletePublicIdempotency :exec
UPDATE public_api_idempotency
SET state = 'completed', response_status = sqlc.arg(response_status), response_enc = sqlc.arg(response_enc)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: DeletePublicIdempotency :exec
-- A failed write (5xx) frees its key for a retry.
DELETE FROM public_api_idempotency
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: PurgeExpiredPublicIdempotency :execrows
-- Worker sweep over every organization's expired records.
DELETE FROM public_api_idempotency
WHERE organization_id IS NOT NULL
  AND expires_at <= sqlc.arg(now);

-- ------------------------------------------------- quotes, reservations

-- name: InsertPublicQuote :exec
INSERT INTO public_quotes (organization_id, quote_id, key_id, channel)
SELECT q.organization_id, q.id, sqlc.narg(key_id), sqlc.arg(channel)
FROM quotes q
WHERE q.organization_id = sqlc.arg(organization_id)
  AND q.uuid = sqlc.arg(quote_uuid)
ON CONFLICT ON CONSTRAINT uq_public_quotes_organization_quote DO NOTHING;

-- name: GetPublicQuote :one
SELECT pq.quote_id, pq.key_id, pq.channel, q.uuid AS quote_uuid, q.status AS quote_status, q.purpose AS quote_purpose
FROM public_quotes pq
JOIN quotes q ON q.organization_id = pq.organization_id AND q.id = pq.quote_id
WHERE pq.organization_id = sqlc.arg(organization_id)
  AND q.organization_id = sqlc.arg(organization_id)
  AND q.uuid = sqlc.arg(quote_uuid);

-- name: InsertPublicReservation :exec
INSERT INTO public_reservations (organization_id, reservation_id, key_id, channel, access_token_hash)
SELECT r.organization_id, r.id, sqlc.narg(key_id), sqlc.arg(channel), sqlc.arg(access_token_hash)
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(reservation_uuid);

-- name: GetPublicReservationByReference :one
-- A reservation the public API made, by its reference.
SELECT r.id, r.uuid, r.reference, r.status, r.customer_id,
    pr.key_id, pr.channel, pr.access_token_hash
FROM public_reservations pr
JOIN reservations r ON r.organization_id = pr.organization_id AND r.id = pr.reservation_id
WHERE pr.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND r.reference = sqlc.arg(reference);

-- name: GetPublicReservationByUUID :one
SELECT r.id, r.uuid, r.reference, r.status, pr.key_id, pr.channel, o.slug AS organization_slug
FROM public_reservations pr
JOIN reservations r ON r.organization_id = pr.organization_id AND r.id = pr.reservation_id
JOIN organizations o ON o.id = pr.organization_id
WHERE pr.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(reservation_uuid);

-- name: DeleteUnusedPublicCustomer :execrows
-- Undo of a public booking that failed after its customer was created: the
-- customer (and its drivers, cascade) is removed while nothing refers to it.
DELETE FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND NOT EXISTS (
      SELECT 1 FROM reservations r WHERE r.organization_id = c.organization_id AND r.customer_id = c.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM quotes q WHERE q.organization_id = c.organization_id AND q.customer_id = c.id
  );

-- ------------------------------------------------------------ catalog

-- name: ListPublicBranches :many
SELECT b.id, b.uuid, b.code, b.name, b.address, b.city, b.district, b.latitude, b.longitude, b.phone, b.email,
    b.working_hours, b.holiday_exceptions
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND b.is_active
ORDER BY b.city, b.name, b.id;

-- name: ListPublicServicePoints :many
SELECT sp.uuid, sp.branch_id, sp.type, sp.name, sp.description, sp.extra_fee, sp.working_hours, sp.delivery_enabled,
    sp.latitude, sp.longitude, sp.min_notice_minutes, sp.delivery_conditions
FROM service_points sp
JOIN branches b ON b.organization_id = sp.organization_id AND b.id = sp.branch_id
WHERE sp.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND sp.is_active
  AND sp.online_selectable
  AND b.is_active
  AND b.deleted_at IS NULL
ORDER BY sp.branch_id, sp.name, sp.id;

-- name: ListPublicSubgroups :many
-- Products on sale (published, active) with their current conditions.
SELECT p.id, p.uuid, p.code, p.title, p.description, p.current_version,
    s.code AS segment_code, s.name AS segment_name,
    v.guarantee_type, v.guarantee_label, v.deposit_amount, v.min_driver_age, v.min_license_years,
    v.included_km_per_day, v.extra_km_fee, v.fuel_policy,
    COALESCE(NULLIF(btrim(COALESCE(cb.display_name, '') || ' ' || COALESCE(cm.name, '')), ''), '')::text AS representative_model
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
JOIN rental_product_versions v ON v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
LEFT JOIN catalog_models cm ON cm.id = p.representative_model_id
LEFT JOIN catalog_brands cb ON cb.id = cm.brand_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND s.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND p.status = 'published'
  AND p.is_active
  AND p.deleted_at IS NULL
ORDER BY s.sort_order, s.name, p.title, p.id;

-- name: ListPublicSubgroupImages :many
SELECT i.uuid, i.product_id, i.content_type
FROM rental_product_images i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY i.product_id, i.sort_order, i.id;

-- name: ListPublicExtras :many
-- Published extras sold on the channel (channels NULL = every channel).
SELECT e.uuid, e.code, e.name, e.description, e.extra_type, e.charge_type, e.min_quantity, e.max_quantity, e.max_total
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.deleted_at IS NULL
  AND e.status = 'published'
  AND e.extra_type <> 'delivery'
  AND (e.channels IS NULL OR sqlc.arg(channel)::text = ANY(e.channels::text[]))
ORDER BY e.sort_order, e.name, e.id;

-- ------------------------------------------------------------ webhooks

-- name: CreatePublicWebhookEndpoint :one
INSERT INTO public_webhook_endpoints (organization_id, uuid, name, url, secret_enc, secret_last4, events, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(uuid), sqlc.arg(name), sqlc.arg(url), sqlc.arg(secret_enc), sqlc.arg(secret_last4),
    sqlc.arg(events)::text[], sqlc.narg(created_by)
)
RETURNING *;

-- name: ListPublicWebhookEndpoints :many
SELECT w.*
FROM public_webhook_endpoints w
WHERE w.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR w.name ILIKE '%' || sqlc.narg(q)::text || '%' OR w.url ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(statuses)::text[] IS NULL OR w.status = ANY(sqlc.narg(statuses)::text[]))
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::bool THEN w.name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::bool THEN w.name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::bool THEN w.created_at END ASC,
    w.created_at DESC, w.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountPublicWebhookEndpoints :one
SELECT COUNT(*)::bigint
FROM public_webhook_endpoints w
WHERE w.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR w.name ILIKE '%' || sqlc.narg(q)::text || '%' OR w.url ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(statuses)::text[] IS NULL OR w.status = ANY(sqlc.narg(statuses)::text[]));

-- name: GetPublicWebhookEndpoint :one
SELECT w.*
FROM public_webhook_endpoints w
WHERE w.organization_id = sqlc.arg(organization_id)
  AND w.uuid = sqlc.arg(uuid);

-- name: GetPublicWebhookEndpointByID :one
SELECT w.*
FROM public_webhook_endpoints w
WHERE w.organization_id = sqlc.arg(organization_id)
  AND w.id = sqlc.arg(id);

-- name: UpdatePublicWebhookEndpoint :execrows
UPDATE public_webhook_endpoints
SET name = sqlc.arg(name), url = sqlc.arg(url), events = sqlc.arg(events)::text[], status = sqlc.arg(status)::text,
    consecutive_failures = CASE WHEN sqlc.arg(status)::text = 'active' AND status <> 'active' THEN 0 ELSE consecutive_failures END
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid);

-- name: RotatePublicWebhookSecret :execrows
UPDATE public_webhook_endpoints
SET secret_enc = sqlc.arg(secret_enc), secret_last4 = sqlc.arg(secret_last4)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid);

-- name: DeletePublicWebhookEndpoint :execrows
DELETE FROM public_webhook_endpoints
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListWebhookEndpointsForEvent :many
SELECT w.id
FROM public_webhook_endpoints w
WHERE w.organization_id = sqlc.arg(organization_id)
  AND w.status = 'active'
  AND sqlc.arg(event_type)::text = ANY(w.events);

-- name: InsertPublicWebhookDelivery :one
INSERT INTO public_webhook_deliveries (organization_id, endpoint_id, event_type, event_uuid, payload, next_attempt_at)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(endpoint_id), sqlc.arg(event_type), sqlc.arg(event_uuid), sqlc.arg(payload),
    sqlc.arg(next_attempt_at)
)
RETURNING *;

-- name: ClaimDueWebhookDeliveries :many
-- Worker sweep over every organization: takes due deliveries for a lease
-- (a crashed attempt is retried when the lease ends).
UPDATE public_webhook_deliveries d
SET next_attempt_at = sqlc.arg(lease_until)
WHERE d.id IN (
    SELECT x.id
    FROM public_webhook_deliveries x
    WHERE x.organization_id IS NOT NULL
      AND x.status = 'pending'
      AND x.next_attempt_at <= sqlc.arg(now)
    ORDER BY x.next_attempt_at
    LIMIT sqlc.arg(row_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING d.id, d.uuid, d.organization_id, d.endpoint_id, d.event_type, d.event_uuid, d.payload, d.attempts;

-- name: ClaimWebhookDelivery :one
-- One delivery now (panel test / retry), unless another attempt holds it.
UPDATE public_webhook_deliveries d
SET next_attempt_at = sqlc.arg(lease_until)
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id)
  AND d.status = 'pending'
  AND d.next_attempt_at <= sqlc.arg(now)
RETURNING d.id, d.uuid, d.organization_id, d.endpoint_id, d.event_type, d.event_uuid, d.payload, d.attempts;

-- name: FinishWebhookDelivery :exec
UPDATE public_webhook_deliveries
SET status = sqlc.arg(status), attempts = attempts + 1, next_attempt_at = sqlc.arg(next_attempt_at),
    last_status_code = sqlc.narg(last_status_code), last_error = sqlc.arg(last_error), delivered_at = sqlc.narg(delivered_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: RecordWebhookEndpointResult :exec
UPDATE public_webhook_endpoints
SET last_success_at = CASE WHEN sqlc.arg(ok)::bool THEN sqlc.arg(now)::timestamptz ELSE last_success_at END,
    last_failure_at = CASE WHEN sqlc.arg(ok)::bool THEN last_failure_at ELSE sqlc.arg(now)::timestamptz END,
    consecutive_failures = CASE WHEN sqlc.arg(ok)::bool THEN 0 ELSE consecutive_failures + 1 END
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListPublicWebhookDeliveries :many
SELECT d.id, d.uuid, d.event_type, d.event_uuid, d.status, d.attempts, d.next_attempt_at, d.last_status_code,
    d.last_error, d.delivered_at, d.created_at, d.updated_at,
    w.uuid AS endpoint_uuid, w.name AS endpoint_name, w.url AS endpoint_url
FROM public_webhook_deliveries d
JOIN public_webhook_endpoints w ON w.organization_id = d.organization_id AND w.id = d.endpoint_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND w.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(endpoint_uuids)::uuid[] IS NULL OR w.uuid = ANY(sqlc.narg(endpoint_uuids)::uuid[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR d.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(event_types)::text[] IS NULL OR d.event_type = ANY(sqlc.narg(event_types)::text[]))
ORDER BY
    CASE WHEN sqlc.arg(sort_desc)::bool THEN d.created_at END DESC,
    CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN d.created_at END ASC,
    d.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountPublicWebhookDeliveries :one
SELECT COUNT(*)::bigint
FROM public_webhook_deliveries d
JOIN public_webhook_endpoints w ON w.organization_id = d.organization_id AND w.id = d.endpoint_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND w.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(endpoint_uuids)::uuid[] IS NULL OR w.uuid = ANY(sqlc.narg(endpoint_uuids)::uuid[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR d.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(event_types)::text[] IS NULL OR d.event_type = ANY(sqlc.narg(event_types)::text[]));

-- name: GetPublicWebhookDelivery :one
SELECT d.id, d.uuid, d.event_type, d.event_uuid, d.payload, d.status, d.attempts, d.next_attempt_at, d.last_status_code,
    d.last_error, d.delivered_at, d.created_at, d.updated_at,
    w.uuid AS endpoint_uuid, w.name AS endpoint_name, w.url AS endpoint_url
FROM public_webhook_deliveries d
JOIN public_webhook_endpoints w ON w.organization_id = d.organization_id AND w.id = d.endpoint_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND w.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid);

-- name: RequeuePublicWebhookDelivery :execrows
-- Panel retry of a failed delivery.
UPDATE public_webhook_deliveries
SET status = 'pending', next_attempt_at = sqlc.arg(now), last_error = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND status = 'failed';

-- name: CountPublicWebhookDeliveryStats :one
-- Panel header: failed deliveries of the last 7 days.
SELECT COUNT(*) FILTER (WHERE d.status = 'failed')::bigint AS failed,
    COUNT(*) FILTER (WHERE d.status = 'pending')::bigint AS pending
FROM public_webhook_deliveries d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.created_at >= sqlc.arg(since);
