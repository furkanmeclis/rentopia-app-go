-- Tenant resource: campaigns / coupons and their redemptions (TEC-703).
-- Every query filters by organization_id (docs/design/tenant-core.md §8).
--
-- state (computed, for lists and facets), first match wins:
--   inactive   is_active = false
--   expired    end_date < today (Istanbul)
--   scheduled  start_date > today
--   exhausted  used_count >= usage_limit
--   active     otherwise

-- name: CreateCampaign :one
INSERT INTO campaigns (
    organization_id, name, description, campaign_type, code, discount_kind, discount_value, max_discount,
    min_subtotal, start_date, end_date, channels, product_scope, customer_uuids, usage_limit, per_customer_limit,
    min_completed_rentals, release_on_cancel, corporate_eligible, is_active, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(campaign_type), sqlc.narg(code),
    sqlc.arg(discount_kind), sqlc.arg(discount_value), sqlc.narg(max_discount), sqlc.narg(min_subtotal),
    sqlc.narg(start_date), sqlc.narg(end_date), sqlc.narg(channels)::varchar[], sqlc.arg(product_scope),
    sqlc.narg(customer_uuids)::uuid[], sqlc.narg(usage_limit), sqlc.narg(per_customer_limit),
    sqlc.arg(min_completed_rentals), sqlc.arg(release_on_cancel), sqlc.arg(corporate_eligible), sqlc.arg(is_active),
    sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateCampaign :execrows
UPDATE campaigns
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    campaign_type = sqlc.arg(campaign_type),
    code = sqlc.narg(code),
    discount_kind = sqlc.arg(discount_kind),
    discount_value = sqlc.arg(discount_value),
    max_discount = sqlc.narg(max_discount),
    min_subtotal = sqlc.narg(min_subtotal),
    start_date = sqlc.narg(start_date),
    end_date = sqlc.narg(end_date),
    channels = sqlc.narg(channels)::varchar[],
    product_scope = sqlc.arg(product_scope),
    customer_uuids = sqlc.narg(customer_uuids)::uuid[],
    usage_limit = sqlc.narg(usage_limit),
    per_customer_limit = sqlc.narg(per_customer_limit),
    min_completed_rentals = sqlc.arg(min_completed_rentals),
    release_on_cancel = sqlc.arg(release_on_cancel),
    corporate_eligible = sqlc.arg(corporate_eligible),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: SetCampaignActive :execrows
UPDATE campaigns
SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: SoftDeleteCampaign :execrows
UPDATE campaigns
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: GetCampaignByUUID :one
SELECT c.id, c.uuid, c.name, c.description, c.campaign_type, c.code, c.discount_kind, c.discount_value, c.max_discount,
    c.min_subtotal, c.start_date, c.end_date, c.channels::varchar[] AS channels, c.product_scope,
    c.customer_uuids::uuid[] AS customer_uuids, c.usage_limit, c.per_customer_limit, c.used_count,
    c.min_completed_rentals, c.release_on_cancel, c.corporate_eligible, c.is_active, c.created_at, c.updated_at
FROM campaigns c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: GetCampaignByCode :one
SELECT c.id, c.uuid, c.name, c.description, c.campaign_type, c.code, c.discount_kind, c.discount_value, c.max_discount,
    c.min_subtotal, c.start_date, c.end_date, c.channels::varchar[] AS channels, c.product_scope,
    c.customer_uuids::uuid[] AS customer_uuids, c.usage_limit, c.per_customer_limit, c.used_count,
    c.min_completed_rentals, c.release_on_cancel, c.corporate_eligible, c.is_active, c.created_at, c.updated_at
FROM campaigns c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.code IS NOT NULL
  AND lower(c.code) = lower(sqlc.arg(code))
  AND c.deleted_at IS NULL;

-- name: ListRepeatCustomerCampaigns :many
-- Active repeat-customer campaigns (eligibility is evaluated in Go).
SELECT c.id, c.uuid, c.name, c.description, c.campaign_type, c.code, c.discount_kind, c.discount_value, c.max_discount,
    c.min_subtotal, c.start_date, c.end_date, c.channels::varchar[] AS channels, c.product_scope,
    c.customer_uuids::uuid[] AS customer_uuids, c.usage_limit, c.per_customer_limit, c.used_count,
    c.min_completed_rentals, c.release_on_cancel, c.corporate_eligible, c.is_active, c.created_at, c.updated_at
FROM campaigns c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.campaign_type = 'repeat_customer'
  AND c.is_active
  AND c.deleted_at IS NULL
ORDER BY c.id;

-- name: ListCampaigns :many
SELECT c.id, c.uuid, c.name, c.description, c.campaign_type, c.code, c.discount_kind, c.discount_value, c.max_discount,
    c.min_subtotal, c.start_date, c.end_date, c.channels::varchar[] AS channels, c.product_scope,
    c.customer_uuids::uuid[] AS customer_uuids, c.usage_limit, c.per_customer_limit, c.used_count,
    c.min_completed_rentals, c.release_on_cancel, c.corporate_eligible, c.is_active, c.created_at, c.updated_at,
    (CASE
        WHEN NOT c.is_active THEN 'inactive'
        WHEN c.end_date IS NOT NULL AND c.end_date < sqlc.arg(today)::date THEN 'expired'
        WHEN c.start_date IS NOT NULL AND c.start_date > sqlc.arg(today)::date THEN 'scheduled'
        WHEN c.usage_limit IS NOT NULL AND c.used_count >= c.usage_limit THEN 'exhausted'
        ELSE 'active'
    END)::text AS state
FROM campaigns c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(types)::text[] IS NULL OR c.campaign_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.discount_kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(states)::text[] IS NULL OR (CASE
        WHEN NOT c.is_active THEN 'inactive'
        WHEN c.end_date IS NOT NULL AND c.end_date < sqlc.arg(today)::date THEN 'expired'
        WHEN c.start_date IS NOT NULL AND c.start_date > sqlc.arg(today)::date THEN 'scheduled'
        WHEN c.usage_limit IS NOT NULL AND c.used_count >= c.usage_limit THEN 'exhausted'
        ELSE 'active'
    END) = ANY(sqlc.narg(states)::text[]))
  AND (sqlc.narg(channel)::text IS NULL OR c.channels IS NULL OR sqlc.narg(channel)::text = ANY(c.channels))
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.code ILIKE '%' || sqlc.narg(q) || '%'
    OR c.description ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN c.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN c.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN c.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN c.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'start_date' AND NOT sqlc.arg(sort_desc)::boolean THEN c.start_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'start_date' AND sqlc.arg(sort_desc)::boolean THEN c.start_date END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'end_date' AND NOT sqlc.arg(sort_desc)::boolean THEN c.end_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'end_date' AND sqlc.arg(sort_desc)::boolean THEN c.end_date END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'used_count' AND NOT sqlc.arg(sort_desc)::boolean THEN c.used_count END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'used_count' AND sqlc.arg(sort_desc)::boolean THEN c.used_count END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN c.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
  c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCampaigns :one
SELECT COUNT(*)::bigint
FROM campaigns c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(types)::text[] IS NULL OR c.campaign_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.discount_kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(states)::text[] IS NULL OR (CASE
        WHEN NOT c.is_active THEN 'inactive'
        WHEN c.end_date IS NOT NULL AND c.end_date < sqlc.arg(today)::date THEN 'expired'
        WHEN c.start_date IS NOT NULL AND c.start_date > sqlc.arg(today)::date THEN 'scheduled'
        WHEN c.usage_limit IS NOT NULL AND c.used_count >= c.usage_limit THEN 'exhausted'
        ELSE 'active'
    END) = ANY(sqlc.narg(states)::text[]))
  AND (sqlc.narg(channel)::text IS NULL OR c.channels IS NULL OR sqlc.narg(channel)::text = ANY(c.channels))
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.code ILIKE '%' || sqlc.narg(q) || '%'
    OR c.description ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListCampaignUUIDs :many
SELECT c.uuid
FROM campaigns c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(types)::text[] IS NULL OR c.campaign_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.discount_kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(states)::text[] IS NULL OR (CASE
        WHEN NOT c.is_active THEN 'inactive'
        WHEN c.end_date IS NOT NULL AND c.end_date < sqlc.arg(today)::date THEN 'expired'
        WHEN c.start_date IS NOT NULL AND c.start_date > sqlc.arg(today)::date THEN 'scheduled'
        WHEN c.usage_limit IS NOT NULL AND c.used_count >= c.usage_limit THEN 'exhausted'
        ELSE 'active'
    END) = ANY(sqlc.narg(states)::text[]))
  AND (sqlc.narg(channel)::text IS NULL OR c.channels IS NULL OR sqlc.narg(channel)::text = ANY(c.channels))
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.code ILIKE '%' || sqlc.narg(q) || '%'
    OR c.description ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY c.id
LIMIT sqlc.arg(limit_count);

-- name: CampaignFacets :many
SELECT 'campaign_type'::text AS facet, c.campaign_type::text AS value, COUNT(*)::bigint AS count
FROM campaigns c WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL GROUP BY c.campaign_type
UNION ALL
SELECT 'discount_kind'::text, c.discount_kind::text, COUNT(*)::bigint
FROM campaigns c WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL GROUP BY c.discount_kind
UNION ALL
SELECT 'state'::text, s.state, COUNT(*)::bigint
FROM (
    SELECT (CASE
        WHEN NOT c.is_active THEN 'inactive'
        WHEN c.end_date IS NOT NULL AND c.end_date < sqlc.arg(today)::date THEN 'expired'
        WHEN c.start_date IS NOT NULL AND c.start_date > sqlc.arg(today)::date THEN 'scheduled'
        WHEN c.usage_limit IS NOT NULL AND c.used_count >= c.usage_limit THEN 'exhausted'
        ELSE 'active'
    END)::text AS state
    FROM campaigns c
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
) s
GROUP BY s.state;

-- ---------------------------------------------------------------- targets

-- name: DeleteCampaignTargets :exec
DELETE FROM campaign_targets
WHERE organization_id = sqlc.arg(organization_id)
  AND campaign_id = sqlc.arg(campaign_id);

-- name: InsertCampaignTarget :exec
INSERT INTO campaign_targets (organization_id, campaign_id, rental_product_id, segment_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(campaign_id), sqlc.narg(rental_product_id), sqlc.narg(segment_id));

-- name: ListCampaignTargets :many
SELECT t.campaign_id, t.rental_product_id, t.segment_id,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    s.uuid AS segment_uuid, s.code AS segment_code, s.name AS segment_name
FROM campaign_targets t
LEFT JOIN rental_products p ON p.organization_id = t.organization_id AND p.id = t.rental_product_id
LEFT JOIN vehicle_segments s ON s.organization_id = t.organization_id AND s.id = t.segment_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.campaign_id = ANY(sqlc.arg(campaign_ids)::bigint[])
ORDER BY t.id;

-- name: GetCampaignSegmentByUUID :one
SELECT s.id, s.uuid
FROM vehicle_segments s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid)
  AND s.deleted_at IS NULL;

-- name: GetCampaignProductByUUID :one
SELECT p.id, p.uuid, s.id AS segment_id, s.uuid AS segment_uuid
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- ------------------------------------------------------------ redemptions

-- name: IncrementCampaignUsage :one
-- The atomic usage counter (RT-A12): never passes usage_limit; no row =
-- the limit is reached (or the campaign is gone / inactive).
UPDATE campaigns
SET used_count = used_count + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND is_active
  AND (usage_limit IS NULL OR used_count < usage_limit)
RETURNING used_count;

-- name: DecrementCampaignUsage :execrows
UPDATE campaigns
SET used_count = used_count - 1
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND used_count > 0;

-- name: CountCustomerRedemptions :one
-- Usages of a campaign by a customer that still count (reserved /
-- consumed, or released without giving the usage back).
SELECT COUNT(*)::bigint
FROM campaign_redemptions r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.campaign_id = sqlc.arg(campaign_id)
  AND r.customer_uuid = sqlc.arg(customer_uuid)
  AND (r.status <> 'released' OR NOT r.usage_returned);

-- name: InsertCampaignRedemption :one
INSERT INTO campaign_redemptions (
    organization_id, campaign_id, reservation_uuid, customer_uuid, code, subtotal, discount_amount, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(campaign_id), sqlc.arg(reservation_uuid), sqlc.narg(customer_uuid),
    sqlc.narg(code), sqlc.arg(subtotal), sqlc.arg(discount_amount), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetLiveRedemptionForReservation :one
-- The reserved / consumed redemption of a reservation, locked.
SELECT r.id, r.uuid, r.campaign_id, r.status, c.release_on_cancel
FROM campaign_redemptions r
JOIN campaigns c ON c.organization_id = r.organization_id AND c.id = r.campaign_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.reservation_uuid = sqlc.arg(reservation_uuid)
  AND r.status <> 'released'
FOR UPDATE OF r;

-- name: ConsumeCampaignRedemption :execrows
UPDATE campaign_redemptions
SET status = 'consumed', consumed_at = sqlc.arg(now_at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'reserved';

-- name: ReleaseCampaignRedemption :execrows
UPDATE campaign_redemptions
SET status = 'released', released_at = sqlc.arg(now_at)::timestamptz,
    release_reason = sqlc.arg(release_reason), usage_returned = sqlc.arg(usage_returned)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'released';

-- name: GetCampaignRedemption :one
SELECT r.id, r.uuid, r.reservation_uuid, r.customer_uuid, r.code, r.subtotal, r.discount_amount, r.status,
    r.release_reason, r.usage_returned, r.created_at, r.consumed_at, r.released_at,
    c.uuid AS campaign_uuid, c.name AS campaign_name
FROM campaign_redemptions r
JOIN campaigns c ON c.organization_id = r.organization_id AND c.id = r.campaign_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: ListCampaignRedemptions :many
SELECT r.id, r.uuid, r.reservation_uuid, r.customer_uuid, r.code, r.subtotal, r.discount_amount, r.status,
    r.release_reason, r.usage_returned, r.created_at, r.consumed_at, r.released_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM campaign_redemptions r
LEFT JOIN users u ON u.id = r.created_by
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.campaign_id = sqlc.arg(campaign_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCampaignRedemptions :one
SELECT COUNT(*)::bigint
FROM campaign_redemptions r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.campaign_id = sqlc.arg(campaign_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]));
