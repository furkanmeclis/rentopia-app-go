-- Channel manager (TEC-736, docs/design/channel-manager.md): channel
-- accounts (integration accounts of kind channel + their sync state),
-- mappings, offers, channel bookings, inbound messages and the sync log.
-- Every table is organization-owned; every query filters by
-- organization_id (the worker sweep selects it to re-scope).

-- ---------------------------------------------------------------- accounts

-- name: ListChannelAccounts :many
SELECT a.id, a.uuid, a.provider, a.display_name, a.mode, a.status, a.last_check_status, a.last_checked_at,
    c.channel, c.push_enabled, c.pull_enabled, c.stop_sale, c.horizon_days, c.push_interval_minutes,
    c.pull_interval_minutes, c.validity_minutes, c.offline_policy, c.offline_quota, c.collected_by, c.commission_rate,
    c.push_requested_at, c.last_push_at, c.last_push_ok_at, c.last_push_status, c.last_push_error, c.pushed_valid_until,
    c.last_pull_at, c.last_pull_ok_at, c.last_pull_status, c.last_pull_error, c.consecutive_failures,
    (SELECT COUNT(*) FROM channel_messages m
     WHERE m.organization_id = a.organization_id AND m.integration_account_id = a.id AND m.status = 'failed')::bigint AS open_errors,
    (SELECT COUNT(*) FROM channel_mappings mp
     WHERE mp.organization_id = a.organization_id AND mp.integration_account_id = a.id AND mp.is_active)::bigint AS active_mappings
FROM integration_accounts a
JOIN channel_accounts c ON c.organization_id = a.organization_id AND c.integration_account_id = a.id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.kind = 'channel'
ORDER BY (a.status = 'active') DESC, a.display_name, a.id;

-- name: GetChannelAccount :one
SELECT a.id, a.uuid, a.provider, a.display_name, a.mode, a.status, a.last_check_status, a.last_checked_at,
    c.channel, c.push_enabled, c.pull_enabled, c.stop_sale, c.horizon_days, c.push_interval_minutes,
    c.pull_interval_minutes, c.validity_minutes, c.offline_policy, c.offline_quota, c.collected_by, c.commission_rate,
    c.push_requested_at, c.last_push_at, c.last_push_ok_at, c.last_push_status, c.last_push_error, c.pushed_valid_until,
    c.last_pull_at, c.last_pull_ok_at, c.last_pull_status, c.last_pull_error, c.consecutive_failures,
    (SELECT COUNT(*) FROM channel_messages m
     WHERE m.organization_id = a.organization_id AND m.integration_account_id = a.id AND m.status = 'failed')::bigint AS open_errors,
    (SELECT COUNT(*) FROM channel_mappings mp
     WHERE mp.organization_id = a.organization_id AND mp.integration_account_id = a.id AND mp.is_active)::bigint AS active_mappings
FROM integration_accounts a
JOIN channel_accounts c ON c.organization_id = a.organization_id AND c.integration_account_id = a.id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.kind = 'channel'
  AND a.uuid = sqlc.arg(uuid);

-- name: GetChannelAccountState :one
SELECT c.id, c.organization_id, c.integration_account_id, c.channel, c.push_enabled, c.pull_enabled, c.stop_sale,
    c.horizon_days, c.push_interval_minutes, c.pull_interval_minutes, c.validity_minutes, c.offline_policy,
    c.offline_quota, c.collected_by, c.commission_rate, c.push_requested_at, c.last_push_at, c.pull_cursor,
    c.last_pull_at, c.consecutive_failures
FROM channel_accounts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.integration_account_id = sqlc.arg(integration_account_id);

-- name: EnsureChannelAccount :exec
INSERT INTO channel_accounts (organization_id, integration_account_id, channel, push_requested_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(channel), NOW())
ON CONFLICT (organization_id, integration_account_id) DO NOTHING;

-- name: EnsureChannelAccountsForOrganization :exec
-- Sync state rows for every channel account of the organization (the
-- channel code is the provider without its "_fake" suffix).
INSERT INTO channel_accounts (organization_id, integration_account_id, channel, push_requested_at)
SELECT a.organization_id, a.id, regexp_replace(a.provider, '_fake$', ''), NOW()
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.kind = 'channel'
ON CONFLICT (organization_id, integration_account_id) DO NOTHING;

-- name: ListActiveChannelIntegrationAccounts :many
-- Worker sweep: every active channel account of every organization.
SELECT a.id, a.organization_id, a.uuid, a.provider
FROM integration_accounts a
WHERE a.kind = 'channel'
  AND a.status = 'active'
ORDER BY a.organization_id, a.id;

-- name: UpdateChannelAccountSettings :execrows
UPDATE channel_accounts
SET push_enabled = sqlc.arg(push_enabled),
    pull_enabled = sqlc.arg(pull_enabled),
    stop_sale = sqlc.arg(stop_sale),
    horizon_days = sqlc.arg(horizon_days),
    push_interval_minutes = sqlc.arg(push_interval_minutes),
    pull_interval_minutes = sqlc.arg(pull_interval_minutes),
    validity_minutes = sqlc.arg(validity_minutes),
    offline_policy = sqlc.arg(offline_policy),
    offline_quota = sqlc.arg(offline_quota),
    collected_by = sqlc.arg(collected_by),
    commission_rate = sqlc.arg(commission_rate),
    push_requested_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND integration_account_id = sqlc.arg(integration_account_id);

-- name: RequestChannelPush :exec
-- An event changed sellable capacity: every channel account of the
-- organization pushes at the next sweep.
UPDATE channel_accounts
SET push_requested_at = sqlc.arg(at)
WHERE organization_id = sqlc.arg(organization_id)
  AND push_enabled;

-- name: LockChannelAccount :execrows
UPDATE channel_accounts
SET locked_until = sqlc.arg(locked_until)
WHERE organization_id = sqlc.arg(organization_id)
  AND integration_account_id = sqlc.arg(integration_account_id)
  AND (locked_until IS NULL OR locked_until < sqlc.arg(now));

-- name: UnlockChannelAccount :exec
UPDATE channel_accounts
SET locked_until = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND integration_account_id = sqlc.arg(integration_account_id);

-- name: SaveChannelPushState :exec
-- ok: the push was accepted (pushed_valid_until moves, failures reset); a
-- request made after the push started stays pending.
UPDATE channel_accounts
SET last_push_at = sqlc.arg(at),
    last_push_status = sqlc.arg(status),
    last_push_error = sqlc.arg(error),
    last_push_ok_at = CASE WHEN sqlc.arg(ok)::boolean THEN sqlc.arg(at) ELSE last_push_ok_at END,
    pushed_valid_until = CASE WHEN sqlc.arg(ok)::boolean THEN sqlc.narg(valid_until)::timestamptz ELSE pushed_valid_until END,
    consecutive_failures = CASE WHEN sqlc.arg(ok)::boolean THEN 0 ELSE consecutive_failures + 1 END,
    push_requested_at = CASE WHEN push_requested_at IS NOT NULL AND push_requested_at <= sqlc.arg(started_at)
        THEN NULL ELSE push_requested_at END
WHERE organization_id = sqlc.arg(organization_id)
  AND integration_account_id = sqlc.arg(integration_account_id);

-- name: SaveChannelPullState :exec
UPDATE channel_accounts
SET last_pull_at = sqlc.arg(at),
    last_pull_status = sqlc.arg(status),
    last_pull_error = sqlc.arg(error),
    last_pull_ok_at = CASE WHEN sqlc.arg(ok)::boolean THEN sqlc.arg(at) ELSE last_pull_ok_at END,
    pull_cursor = sqlc.arg(pull_cursor)
WHERE organization_id = sqlc.arg(organization_id)
  AND integration_account_id = sqlc.arg(integration_account_id);

-- ---------------------------------------------------------------- mappings

-- name: ListChannelMappings :many
SELECT mp.id, mp.uuid, mp.entity_type, mp.external_code, mp.external_name, mp.priority, mp.stop_sale, mp.max_quota,
    mp.is_active, mp.note, mp.created_at, mp.updated_at,
    a.uuid AS account_uuid, a.display_name AS account_name, a.provider AS account_provider,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    e.uuid AS extra_uuid, e.code AS extra_code, e.name AS extra_name,
    COUNT(*) OVER ()::bigint AS total
FROM channel_mappings mp
JOIN integration_accounts a ON a.organization_id = mp.organization_id AND a.id = mp.integration_account_id
LEFT JOIN branches b ON b.organization_id = mp.organization_id AND b.id = mp.branch_id
LEFT JOIN rental_products p ON p.organization_id = mp.organization_id AND p.id = mp.rental_product_id
LEFT JOIN extras e ON e.organization_id = mp.organization_id AND e.id = mp.extra_id
WHERE mp.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_uuid)::uuid IS NULL OR a.uuid = sqlc.narg(account_uuid)::uuid)
  AND (sqlc.narg(entity_types)::text[] IS NULL OR mp.entity_type = ANY(sqlc.narg(entity_types)::text[]))
  AND (sqlc.narg(active)::boolean IS NULL OR mp.is_active = sqlc.narg(active)::boolean)
  AND (sqlc.narg(stop_sale)::boolean IS NULL OR mp.stop_sale = sqlc.narg(stop_sale)::boolean)
  AND (sqlc.narg(q)::text IS NULL
       OR mp.external_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR mp.external_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR b.code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR b.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR e.code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR e.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'external_code' AND NOT sqlc.arg(sort_desc)::boolean THEN mp.external_code END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'external_code' AND sqlc.arg(sort_desc)::boolean THEN mp.external_code END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN mp.updated_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN mp.updated_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN mp.created_at END ASC,
    mp.created_at DESC, mp.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ChannelMappingFacets :many
SELECT mp.entity_type, COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE mp.stop_sale)::bigint AS stop_sale,
    COUNT(*) FILTER (WHERE NOT mp.is_active)::bigint AS inactive
FROM channel_mappings mp
WHERE mp.organization_id = sqlc.arg(organization_id)
GROUP BY mp.entity_type;

-- name: GetChannelMapping :one
SELECT mp.id, mp.uuid, mp.integration_account_id, mp.entity_type, mp.branch_id, mp.rental_product_id, mp.extra_id,
    mp.external_code, mp.external_name, mp.priority, mp.stop_sale, mp.max_quota, mp.is_active, mp.note,
    mp.created_at, mp.updated_at,
    a.uuid AS account_uuid, a.display_name AS account_name, a.provider AS account_provider,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    e.uuid AS extra_uuid, e.code AS extra_code, e.name AS extra_name
FROM channel_mappings mp
JOIN integration_accounts a ON a.organization_id = mp.organization_id AND a.id = mp.integration_account_id
LEFT JOIN branches b ON b.organization_id = mp.organization_id AND b.id = mp.branch_id
LEFT JOIN rental_products p ON p.organization_id = mp.organization_id AND p.id = mp.rental_product_id
LEFT JOIN extras e ON e.organization_id = mp.organization_id AND e.id = mp.extra_id
WHERE mp.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND mp.uuid = sqlc.arg(uuid);

-- name: InsertChannelMapping :one
INSERT INTO channel_mappings (
    organization_id, integration_account_id, entity_type, branch_id, rental_product_id, extra_id, external_code,
    external_name, priority, stop_sale, max_quota, is_active, note, created_by, updated_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(entity_type), sqlc.narg(branch_id),
    sqlc.narg(rental_product_id), sqlc.narg(extra_id), sqlc.arg(external_code), sqlc.arg(external_name),
    sqlc.arg(priority), sqlc.arg(stop_sale), sqlc.narg(max_quota), sqlc.arg(is_active), sqlc.arg(note),
    sqlc.narg(actor_id), sqlc.narg(actor_id)
)
RETURNING uuid;

-- name: UpdateChannelMapping :execrows
UPDATE channel_mappings
SET external_code = sqlc.arg(external_code),
    external_name = sqlc.arg(external_name),
    branch_id = sqlc.narg(branch_id),
    priority = sqlc.arg(priority),
    stop_sale = sqlc.arg(stop_sale),
    max_quota = sqlc.narg(max_quota),
    is_active = sqlc.arg(is_active),
    note = sqlc.arg(note),
    updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: DeleteChannelMapping :execrows
DELETE FROM channel_mappings
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListActiveChannelMappings :many
-- Push and booking resolution: the account's active mappings with the
-- internal uuids and codes.
SELECT mp.id, mp.uuid, mp.entity_type, mp.external_code, mp.priority, mp.stop_sale, mp.max_quota,
    mp.branch_id, mp.rental_product_id, mp.extra_id,
    b.uuid AS branch_uuid, b.code AS branch_code, b.is_active AS branch_active,
    p.uuid AS product_uuid, p.code AS product_code, p.status AS product_status,
    e.uuid AS extra_uuid, e.code AS extra_code
FROM channel_mappings mp
LEFT JOIN branches b ON b.organization_id = mp.organization_id AND b.id = mp.branch_id
LEFT JOIN rental_products p ON p.organization_id = mp.organization_id AND p.id = mp.rental_product_id
LEFT JOIN extras e ON e.organization_id = mp.organization_id AND e.id = mp.extra_id
WHERE mp.organization_id = sqlc.arg(organization_id)
  AND mp.integration_account_id = sqlc.arg(integration_account_id)
  AND mp.is_active
ORDER BY mp.entity_type, mp.external_code, mp.priority, mp.id;

-- name: ChannelBranchByUUID :one
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: ChannelProductByUUID :one
SELECT p.id, p.uuid, p.code, p.title
FROM rental_products p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: ChannelExtraByUUID :one
SELECT e.id, e.uuid, e.code, e.name
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid)
  AND e.deleted_at IS NULL;

-- ---------------------------------------------------------------- offers

-- name: UpsertChannelOffer :exec
INSERT INTO channel_offers (
    organization_id, integration_account_id, offer_ref, mapping_id, rental_product_id, product_version, branch_id,
    external_code, location_code, stay_date, daily_price, currency, available
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(offer_ref), sqlc.arg(mapping_id),
    sqlc.arg(rental_product_id), sqlc.arg(product_version), sqlc.arg(branch_id), sqlc.arg(external_code),
    sqlc.arg(location_code), sqlc.arg(stay_date), sqlc.narg(daily_price), sqlc.arg(currency), sqlc.arg(available)
)
ON CONFLICT (organization_id, integration_account_id, offer_ref) DO UPDATE SET available = EXCLUDED.available;

-- name: GetChannelOffer :one
SELECT o.offer_ref, o.mapping_id, o.rental_product_id, o.product_version, o.branch_id, o.external_code,
    o.location_code, o.stay_date, p.uuid AS product_uuid
FROM channel_offers o
JOIN rental_products p ON p.organization_id = o.organization_id AND p.id = o.rental_product_id
WHERE o.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND o.integration_account_id = sqlc.arg(integration_account_id)
  AND o.offer_ref = sqlc.arg(offer_ref);

-- name: LatestChannelOffer :one
-- The newest offer pushed for an external code at a location on a day.
SELECT o.offer_ref, o.mapping_id, o.rental_product_id, o.product_version, o.branch_id, p.uuid AS product_uuid
FROM channel_offers o
JOIN rental_products p ON p.organization_id = o.organization_id AND p.id = o.rental_product_id
WHERE o.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND o.integration_account_id = sqlc.arg(integration_account_id)
  AND o.external_code = sqlc.arg(external_code)
  AND o.location_code = sqlc.arg(location_code)
  AND o.stay_date = sqlc.arg(stay_date)
ORDER BY o.created_at DESC, o.id DESC
LIMIT 1;

-- name: DeleteExpiredChannelOffers :execrows
DELETE FROM channel_offers
WHERE organization_id = sqlc.arg(organization_id)
  AND stay_date < sqlc.arg(before);

-- ---------------------------------------------------------------- bookings

-- name: ChannelBookingLock :exec
-- Serialises the messages of one external booking (transaction-scoped).
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: GetChannelReservationByExternal :one
SELECT cr.id, cr.uuid, cr.integration_account_id, cr.channel, cr.external_ref, cr.reservation_id, cr.customer_id,
    cr.status, cr.channel_status, cr.last_version, cr.applied_version, cr.rental_product_id,
    r.uuid AS reservation_uuid, r.status AS reservation_status, r.reference AS reservation_reference,
    cu.uuid AS customer_uuid
FROM channel_reservations cr
LEFT JOIN reservations r ON r.organization_id = cr.organization_id AND r.id = cr.reservation_id
LEFT JOIN customers cu ON cu.organization_id = cr.organization_id AND cu.id = cr.customer_id
WHERE cr.organization_id = sqlc.arg(organization_id)
  AND cr.channel = sqlc.arg(channel)
  AND cr.external_ref = sqlc.arg(external_ref);

-- name: InsertChannelReservation :exec
INSERT INTO channel_reservations (organization_id, integration_account_id, channel, external_ref)
VALUES (sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(channel), sqlc.arg(external_ref))
ON CONFLICT (organization_id, channel, external_ref) DO NOTHING;

-- name: UpdateChannelReservationReceived :exec
-- The newest message seen (not necessarily applied).
UPDATE channel_reservations
SET last_version = GREATEST(last_version, sqlc.arg(version)),
    last_message_at = sqlc.arg(at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ApplyChannelReservation :exec
UPDATE channel_reservations
SET reservation_id = sqlc.narg(reservation_id),
    customer_id = sqlc.narg(customer_id),
    status = sqlc.arg(status),
    channel_status = sqlc.arg(channel_status),
    applied_version = sqlc.arg(applied_version),
    product_code = sqlc.arg(product_code),
    pickup_code = sqlc.arg(pickup_code),
    return_code = sqlc.arg(return_code),
    offer_ref = sqlc.arg(offer_ref),
    rental_product_id = sqlc.narg(rental_product_id),
    starts_at = sqlc.narg(starts_at),
    ends_at = sqlc.narg(ends_at),
    collected_by = sqlc.arg(collected_by),
    currency = sqlc.arg(currency),
    gross_amount = sqlc.arg(gross_amount),
    commission_amount = sqlc.arg(commission_amount),
    net_amount = sqlc.arg(net_amount),
    prepaid_amount = sqlc.arg(prepaid_amount),
    local_total = sqlc.narg(local_total),
    price_difference = sqlc.narg(price_difference),
    last_error_code = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: FailChannelReservation :exec
UPDATE channel_reservations
SET status = CASE WHEN reservation_id IS NULL AND status = 'pending' THEN 'failed' ELSE status END,
    last_error_code = sqlc.arg(error_code)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ChannelReservationByExternalReference :one
-- A reservation created for the booking whose link was not stored (the
-- process stopped between the two writes).
SELECT r.id, r.uuid, r.status, r.reference
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.channel = sqlc.arg(channel)
  AND r.external_reference = sqlc.arg(external_reference);

-- name: ChannelCustomerIDByUUID :one
SELECT c.id
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: ListChannelReservationsForReconciliation :many
SELECT cr.uuid, cr.external_ref, cr.status, cr.channel_status, cr.applied_version, cr.starts_at, cr.collected_by,
    cr.currency, cr.gross_amount, cr.commission_amount, cr.net_amount, cr.local_total, cr.price_difference,
    cr.last_error_code,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference, r.status AS reservation_status,
    r.total_amount AS reservation_total, r.commission_amount AS reservation_commission,
    r.collecting_party AS reservation_collecting_party
FROM channel_reservations cr
LEFT JOIN reservations r ON r.organization_id = cr.organization_id AND r.id = cr.reservation_id
WHERE cr.organization_id = sqlc.arg(organization_id)
  AND cr.integration_account_id = sqlc.arg(integration_account_id)
  AND cr.starts_at >= sqlc.arg(from_at)
  AND cr.starts_at < sqlc.arg(to_at)
ORDER BY cr.starts_at, cr.external_ref
LIMIT 5000;

-- ---------------------------------------------------------------- messages

-- name: InsertChannelMessage :one
-- Stored once per account and message id: a repeat only counts a delivery
-- (inserted = false).
INSERT INTO channel_messages (
    organization_id, integration_account_id, channel, message_key, external_ref, message_type, version,
    occurred_at, source, payload_enc, summary
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(channel), sqlc.arg(message_key),
    sqlc.arg(external_ref), sqlc.arg(message_type), sqlc.arg(version), sqlc.narg(occurred_at), sqlc.arg(source),
    sqlc.arg(payload_enc), sqlc.arg(summary)
)
ON CONFLICT (organization_id, integration_account_id, message_key)
DO UPDATE SET deliveries = channel_messages.deliveries + 1
RETURNING id, uuid, status, (xmax = 0)::boolean AS inserted;

-- name: GetChannelMessageForProcessing :one
SELECT m.id, m.uuid, m.integration_account_id, m.channel, m.message_key, m.external_ref, m.message_type,
    m.version, m.status, m.payload_enc, m.attempts, m.acked_at
FROM channel_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.id = sqlc.arg(id);

-- name: GetChannelMessageIDByUUID :one
SELECT m.id, m.integration_account_id
FROM channel_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.uuid = sqlc.arg(uuid);

-- name: SaveChannelMessageResult :exec
UPDATE channel_messages
SET status = sqlc.arg(status),
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    attempts = attempts + 1,
    processed_at = sqlc.arg(at),
    channel_reservation_id = COALESCE(sqlc.narg(channel_reservation_id), channel_reservation_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SaveChannelMessageAck :exec
UPDATE channel_messages
SET acked_at = sqlc.narg(acked_at),
    ack_error = sqlc.arg(ack_error)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: IgnoreChannelMessage :execrows
UPDATE channel_messages
SET status = 'ignored',
    resolved_by = sqlc.narg(resolved_by),
    resolved_at = sqlc.arg(at),
    resolution_note = sqlc.arg(note),
    acked_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'failed';

-- name: ListChannelMessagesToAck :many
-- Final results the channel has not been told yet (failed messages wait
-- for a retry or a dismissal).
SELECT m.id, m.message_key, m.external_ref, m.status, m.error_code, m.error_message, m.resolution_note,
    r.reference AS reservation_reference
FROM channel_messages m
LEFT JOIN channel_reservations cr ON cr.organization_id = m.organization_id AND cr.id = m.channel_reservation_id
LEFT JOIN reservations r ON r.organization_id = cr.organization_id AND r.id = cr.reservation_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.integration_account_id = sqlc.arg(integration_account_id)
  AND m.acked_at IS NULL
  AND m.status IN ('applied', 'duplicate', 'stale', 'ignored')
ORDER BY m.id
LIMIT 200;

-- name: ListChannelMessages :many
SELECT m.uuid, m.channel, m.message_key, m.external_ref, m.message_type, m.version, m.occurred_at, m.source,
    m.status, m.error_code, m.error_message, m.attempts, m.deliveries, m.processed_at, m.acked_at, m.ack_error,
    m.summary, m.resolution_note, m.resolved_at, m.created_at, m.updated_at,
    a.uuid AS account_uuid, a.display_name AS account_name, a.provider AS account_provider,
    cr.uuid AS booking_uuid, r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    COUNT(*) OVER ()::bigint AS total
FROM channel_messages m
JOIN integration_accounts a ON a.organization_id = m.organization_id AND a.id = m.integration_account_id
LEFT JOIN channel_reservations cr ON cr.organization_id = m.organization_id AND cr.id = m.channel_reservation_id
LEFT JOIN reservations r ON r.organization_id = cr.organization_id AND r.id = cr.reservation_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_uuid)::uuid IS NULL OR a.uuid = sqlc.narg(account_uuid)::uuid)
  AND (sqlc.narg(statuses)::text[] IS NULL OR m.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(types)::text[] IS NULL OR m.message_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(external_ref)::text IS NULL OR m.external_ref = sqlc.narg(external_ref)::text)
  AND (sqlc.narg(q)::text IS NULL
       OR m.external_ref ILIKE '%' || sqlc.narg(q)::text || '%'
       OR m.message_key ILIKE '%' || sqlc.narg(q)::text || '%'
       OR r.reference ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'occurred_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.occurred_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'occurred_at' AND sqlc.arg(sort_desc)::boolean THEN m.occurred_at END DESC,
    m.created_at DESC, m.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetChannelMessage :one
SELECT m.uuid, m.channel, m.message_key, m.external_ref, m.message_type, m.version, m.occurred_at, m.source,
    m.status, m.error_code, m.error_message, m.attempts, m.deliveries, m.processed_at, m.acked_at, m.ack_error,
    m.summary, m.resolution_note, m.resolved_at, m.created_at, m.updated_at,
    a.uuid AS account_uuid, a.display_name AS account_name, a.provider AS account_provider,
    cr.uuid AS booking_uuid, cr.status AS booking_status, cr.applied_version AS booking_applied_version,
    cr.last_version AS booking_last_version, cr.collected_by AS booking_collected_by,
    cr.gross_amount AS booking_gross, cr.commission_amount AS booking_commission, cr.net_amount AS booking_net,
    cr.price_difference AS booking_price_difference, cr.currency AS booking_currency,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference, r.status AS reservation_status,
    ru.name AS resolved_by_name, ru.surname AS resolved_by_surname
FROM channel_messages m
JOIN integration_accounts a ON a.organization_id = m.organization_id AND a.id = m.integration_account_id
LEFT JOIN channel_reservations cr ON cr.organization_id = m.organization_id AND cr.id = m.channel_reservation_id
LEFT JOIN reservations r ON r.organization_id = cr.organization_id AND r.id = cr.reservation_id
LEFT JOIN users ru ON ru.id = m.resolved_by
WHERE m.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND m.uuid = sqlc.arg(uuid);

-- name: ChannelMessageFacets :many
SELECT m.status, m.message_type, COUNT(*)::bigint AS total
FROM channel_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
GROUP BY m.status, m.message_type;

-- ---------------------------------------------------------------- sync log

-- name: InsertChannelSyncRun :one
INSERT INTO channel_sync_runs (organization_id, integration_account_id, direction, trigger, actor_id, started_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(integration_account_id), sqlc.arg(direction), sqlc.arg(trigger),
    sqlc.narg(actor_id), sqlc.arg(started_at))
RETURNING id, uuid;

-- name: FinishChannelSyncRun :exec
UPDATE channel_sync_runs
SET status = sqlc.arg(status),
    items_total = sqlc.arg(items_total),
    items_ok = sqlc.arg(items_ok),
    items_failed = sqlc.arg(items_failed),
    error = sqlc.arg(error),
    details = sqlc.arg(details),
    finished_at = sqlc.arg(finished_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListChannelSyncRuns :many
SELECT s.uuid, s.direction, s.trigger, s.status, s.items_total, s.items_ok, s.items_failed, s.error, s.details,
    s.started_at, s.finished_at,
    a.uuid AS account_uuid, a.display_name AS account_name, a.provider AS account_provider,
    u.name AS actor_name, u.surname AS actor_surname,
    COUNT(*) OVER ()::bigint AS total
FROM channel_sync_runs s
JOIN integration_accounts a ON a.organization_id = s.organization_id AND a.id = s.integration_account_id
LEFT JOIN users u ON u.id = s.actor_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_uuid)::uuid IS NULL OR a.uuid = sqlc.narg(account_uuid)::uuid)
  AND (sqlc.narg(run_uuid)::uuid IS NULL OR s.uuid = sqlc.narg(run_uuid)::uuid)
  AND (sqlc.narg(directions)::text[] IS NULL OR s.direction = ANY(sqlc.narg(directions)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR s.status = ANY(sqlc.narg(statuses)::text[]))
ORDER BY
    CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN s.started_at END ASC,
    s.started_at DESC, s.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- ---------------------------------------------------------------- platform health

-- name: ChannelHealthCounts :one
-- Platform health (TEC-739): channel accounts of every organization.
SELECT COUNT(*) FILTER (WHERE a.status = 'active')::bigint AS active,
    COUNT(*) FILTER (WHERE a.status = 'active'
        AND (c.last_push_status = 'failed' OR c.last_pull_status = 'failed'))::bigint AS failing,
    COUNT(*) FILTER (WHERE a.status = 'active' AND c.push_enabled
        AND c.pushed_valid_until < sqlc.arg(now)::timestamptz)::bigint AS expired,
    COUNT(*) FILTER (WHERE a.status <> 'active')::bigint AS disabled,
    (SELECT COUNT(*) FROM channel_messages m
     WHERE m.organization_id IS NOT NULL AND m.status = 'failed')::bigint AS error_queue
FROM integration_accounts a
JOIN channel_accounts c ON c.organization_id = a.organization_id AND c.integration_account_id = a.id
WHERE a.kind = 'channel'
  AND a.organization_id IS NOT NULL;

-- name: ListChannelHealthIssues :many
-- Active channel accounts whose last sync failed, whose pushed capacity
-- expired or that hold failed messages (newest first).
SELECT a.organization_id, a.uuid, a.provider, a.display_name,
    c.last_push_status, c.last_push_error, c.last_pull_status, c.last_pull_error,
    c.last_push_at, c.last_pull_at, c.pushed_valid_until, c.push_enabled, c.updated_at,
    (SELECT COUNT(*) FROM channel_messages m
     WHERE m.organization_id = a.organization_id AND m.integration_account_id = a.id AND m.status = 'failed')::bigint AS open_errors
FROM integration_accounts a
JOIN channel_accounts c ON c.organization_id = a.organization_id AND c.integration_account_id = a.id
WHERE a.kind = 'channel'
  AND a.status = 'active'
  AND a.organization_id IS NOT NULL
  AND (c.last_push_status = 'failed' OR c.last_pull_status = 'failed'
       OR (c.push_enabled AND c.pushed_valid_until < sqlc.arg(now)::timestamptz)
       OR EXISTS (SELECT 1 FROM channel_messages m
                  WHERE m.organization_id = a.organization_id AND m.integration_account_id = a.id AND m.status = 'failed'))
ORDER BY c.updated_at DESC, a.id DESC
LIMIT sqlc.arg(row_limit);
