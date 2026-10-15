-- Tenant resources: rate plans with duration bands (TEC-700) and named date
-- rules (TEC-701). Every query filters by organization_id
-- (docs/design/tenant-core.md §8); joined tenant tables are constrained by
-- their own alias.organization_id.
--
-- Versions: at most one 'draft' and one 'active' version per plan / rule
-- (partial unique indexes), so the LEFT JOINs on status below match at most
-- one row each.
--
-- Branch scope (rate plans): a plan without a branch (common / channel) is
-- visible to everyone; a branch plan only inside the member's scope.

-- ------------------------------------------------------------------ lookups

-- name: GetPricingProductByCode :one
SELECT * FROM rental_products
WHERE organization_id = sqlc.arg(organization_id)
  AND upper(code) = upper(sqlc.arg(code))
  AND deleted_at IS NULL;

-- name: ListPricingProducts :many
-- Live products with their segment (matrix rows, overlap checks).
SELECT p.id, p.uuid, p.code, p.title, p.status, p.is_active,
    s.id AS segment_id, s.uuid AS segment_uuid, s.code AS segment_code, s.name AS segment_name
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
ORDER BY p.code ASC, p.id ASC;

-- name: GetPricingProductByUUID :one
SELECT p.id, p.uuid, p.code, p.title, s.id AS segment_id, s.uuid AS segment_uuid
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: ListPricingSegmentsByUUIDs :many
SELECT id, uuid, code, name FROM vehicle_segments
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND deleted_at IS NULL;

-- name: ListPricingProductsByUUIDs :many
SELECT id, uuid, code, title FROM rental_products
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND deleted_at IS NULL;

-- name: ListPricingBranchesByUUIDs :many
SELECT id, uuid, code, name FROM branches
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND deleted_at IS NULL;

-- --------------------------------------------------------------- rate plans

-- name: CreateRatePlan :one
INSERT INTO rate_plans (organization_id, rental_product_id, scope, branch_id, channel, name)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_product_id), sqlc.arg(scope), sqlc.narg(branch_id),
    sqlc.narg(channel), sqlc.arg(name)
)
RETURNING id, uuid;

-- name: GetRatePlan :one
SELECT p.id, p.uuid, p.scope, p.branch_id, p.channel, p.name, p.status, p.current_version, p.active_version,
    p.activated_at, p.created_at, p.updated_at, p.rental_product_id,
    rp.uuid AS product_uuid, rp.code AS product_code, rp.title AS product_title, rp.segment_id AS product_segment_id,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name
FROM rate_plans p
JOIN rental_products rp ON rp.organization_id = p.organization_id AND rp.id = p.rental_product_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: LockRatePlan :one
-- Serialises draft edits and activation of one plan.
SELECT id FROM rate_plans
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateRatePlanName :execrows
UPDATE rate_plans SET name = sqlc.arg(name)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetRatePlanState :execrows
UPDATE rate_plans
SET status = sqlc.arg(status),
    current_version = sqlc.arg(current_version),
    active_version = sqlc.narg(active_version),
    activated_at = sqlc.narg(activated_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: TouchRatePlan :exec
UPDATE rate_plans SET updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SoftDeleteRatePlan :execrows
UPDATE rate_plans SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListRatePlans :many
SELECT p.id, p.uuid, p.scope, p.branch_id, p.channel, p.name, p.status, p.current_version, p.active_version,
    p.activated_at, p.created_at, p.updated_at,
    rp.uuid AS product_uuid, rp.code AS product_code, rp.title AS product_title,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    (dv.id IS NOT NULL)::boolean AS has_draft,
    COALESCE(dv.priority, av.priority, 0)::integer AS priority,
    (SELECT COUNT(*) FROM rate_bands rb
     WHERE rb.organization_id = p.organization_id AND rb.rate_plan_version_id = COALESCE(dv.id, av.id))::bigint AS band_count,
    (SELECT MIN(rb.daily_price) FROM rate_bands rb
     WHERE rb.organization_id = p.organization_id AND rb.rate_plan_version_id = COALESCE(dv.id, av.id))::numeric AS price_from
FROM rate_plans p
JOIN rental_products rp ON rp.organization_id = p.organization_id AND rp.id = p.rental_product_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
LEFT JOIN rate_plan_versions dv ON dv.organization_id = p.organization_id AND dv.rate_plan_id = p.id AND dv.status = 'draft'
LEFT JOIN rate_plan_versions av ON av.organization_id = p.organization_id AND av.rate_plan_id = p.id AND av.status = 'active'
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR p.branch_id IS NULL OR p.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR rp.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(scopes)::text[] IS NULL OR p.scope = ANY(sqlc.narg(scopes)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR p.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(has_draft)::boolean IS NULL OR (dv.id IS NOT NULL) = sqlc.narg(has_draft)::boolean)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR rp.code ILIKE '%' || sqlc.narg(q) || '%'
    OR rp.title ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN p.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN p.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'rental_product' AND NOT sqlc.arg(sort_desc)::boolean THEN rp.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'rental_product' AND sqlc.arg(sort_desc)::boolean THEN rp.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'scope' AND NOT sqlc.arg(sort_desc)::boolean THEN p.scope END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'scope' AND sqlc.arg(sort_desc)::boolean THEN p.scope END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN p.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN p.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'priority' AND NOT sqlc.arg(sort_desc)::boolean THEN COALESCE(dv.priority, av.priority, 0) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'priority' AND sqlc.arg(sort_desc)::boolean THEN COALESCE(dv.priority, av.priority, 0) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN p.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
  rp.code ASC, p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRatePlans :one
SELECT COUNT(*)::bigint
FROM rate_plans p
JOIN rental_products rp ON rp.organization_id = p.organization_id AND rp.id = p.rental_product_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
LEFT JOIN rate_plan_versions dv ON dv.organization_id = p.organization_id AND dv.rate_plan_id = p.id AND dv.status = 'draft'
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR p.branch_id IS NULL OR p.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR rp.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(scopes)::text[] IS NULL OR p.scope = ANY(sqlc.narg(scopes)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR p.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(has_draft)::boolean IS NULL OR (dv.id IS NOT NULL) = sqlc.narg(has_draft)::boolean)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR rp.code ILIKE '%' || sqlc.narg(q) || '%'
    OR rp.title ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListRatePlanFacets :many
-- facet: rental_product | branch | scope | channel | status, in the caller's
-- branch scope.
WITH scoped AS (
    SELECT p.organization_id, p.rental_product_id, p.branch_id, p.scope, p.channel, p.status
    FROM rate_plans p
    WHERE p.organization_id = sqlc.arg(organization_id)
      AND p.deleted_at IS NULL
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR p.branch_id IS NULL OR p.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
)
SELECT 'rental_product'::text AS facet, rp.uuid::text AS value, (rp.code || ' · ' || rp.title)::text AS label, COUNT(*)::bigint AS count
FROM scoped s JOIN rental_products rp ON rp.organization_id = s.organization_id AND rp.id = s.rental_product_id
GROUP BY rp.uuid, rp.code, rp.title
UNION ALL
SELECT 'branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'scope'::text, s.scope::text, ''::text, COUNT(*)::bigint FROM scoped s GROUP BY s.scope
UNION ALL
SELECT 'channel'::text, s.channel::text, ''::text, COUNT(*)::bigint FROM scoped s WHERE s.channel IS NOT NULL GROUP BY s.channel
UNION ALL
SELECT 'status'::text, s.status::text, ''::text, COUNT(*)::bigint FROM scoped s GROUP BY s.status;

-- name: CreateRatePlanVersion :one
INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version, priority, change_note, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(rate_plan_id), sqlc.arg(version), sqlc.arg(priority),
    sqlc.arg(change_note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetRatePlanVersionByStatus :one
SELECT v.id, v.uuid, v.rate_plan_id, v.version, v.status, v.priority, v.change_note, v.created_at, v.activated_at,
    v.superseded_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    au.uuid AS activated_by_uuid, au.name AS activated_by_name, au.surname AS activated_by_surname, au.email AS activated_by_email
FROM rate_plan_versions v
LEFT JOIN users cu ON cu.id = v.created_by
LEFT JOIN users au ON au.id = v.activated_by
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.rate_plan_id = sqlc.arg(rate_plan_id)
  AND v.status = sqlc.arg(status);

-- name: ListRatePlanVersions :many
SELECT v.id, v.uuid, v.rate_plan_id, v.version, v.status, v.priority, v.change_note, v.created_at, v.activated_at,
    v.superseded_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    au.uuid AS activated_by_uuid, au.name AS activated_by_name, au.surname AS activated_by_surname, au.email AS activated_by_email
FROM rate_plan_versions v
LEFT JOIN users cu ON cu.id = v.created_by
LEFT JOIN users au ON au.id = v.activated_by
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.rate_plan_id = sqlc.arg(rate_plan_id)
ORDER BY v.version DESC;

-- name: UpdateRatePlanDraft :execrows
UPDATE rate_plan_versions
SET priority = sqlc.arg(priority), change_note = sqlc.arg(change_note)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'draft';

-- name: ActivateRatePlanVersion :execrows
UPDATE rate_plan_versions
SET status = 'active', activated_at = sqlc.arg(activated_at), activated_by = sqlc.narg(activated_by)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'draft';

-- name: SupersedeRatePlanVersion :execrows
UPDATE rate_plan_versions
SET status = 'superseded', superseded_at = sqlc.arg(superseded_at)
WHERE organization_id = sqlc.arg(organization_id) AND rate_plan_id = sqlc.arg(rate_plan_id) AND status = 'active';

-- name: DeleteRatePlanDraft :execrows
DELETE FROM rate_plan_versions
WHERE organization_id = sqlc.arg(organization_id) AND rate_plan_id = sqlc.arg(rate_plan_id) AND status = 'draft';

-- name: ListRateBandsByVersions :many
SELECT rb.rate_plan_version_id, rb.min_days, rb.max_days, rb.daily_price
FROM rate_bands rb
WHERE rb.organization_id = sqlc.arg(organization_id)
  AND rb.rate_plan_version_id = ANY(sqlc.arg(version_ids)::bigint[])
ORDER BY rb.rate_plan_version_id, rb.min_days;

-- name: DeleteRateBands :exec
DELETE FROM rate_bands
WHERE organization_id = sqlc.arg(organization_id) AND rate_plan_version_id = sqlc.arg(rate_plan_version_id);

-- name: InsertRateBand :exec
INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price)
VALUES (sqlc.arg(organization_id), sqlc.arg(rate_plan_version_id), sqlc.arg(min_days), sqlc.narg(max_days), sqlc.arg(daily_price));

-- name: GetRateBand :one
SELECT rb.id, rb.min_days, rb.max_days, rb.daily_price
FROM rate_bands rb
WHERE rb.organization_id = sqlc.arg(organization_id)
  AND rb.rate_plan_version_id = sqlc.arg(rate_plan_version_id)
  AND rb.min_days = sqlc.arg(min_days)
  AND rb.max_days IS NOT DISTINCT FROM sqlc.narg(max_days)::integer;

-- name: UpdateRateBandPrice :execrows
UPDATE rate_bands SET daily_price = sqlc.arg(daily_price)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: DeleteRateBand :execrows
DELETE FROM rate_bands
WHERE organization_id = sqlc.arg(organization_id)
  AND rate_plan_version_id = sqlc.arg(rate_plan_version_id)
  AND min_days = sqlc.arg(min_days)
  AND max_days IS NOT DISTINCT FROM sqlc.narg(max_days)::integer;

-- name: CopyRateBands :exec
INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price)
SELECT rb.organization_id, sqlc.arg(to_version_id)::bigint, rb.min_days, rb.max_days, rb.daily_price
FROM rate_bands rb
WHERE rb.organization_id = sqlc.arg(organization_id) AND rb.rate_plan_version_id = sqlc.arg(from_version_id);

-- name: ListActiveRatePlans :many
-- Active plans (with their active version) of one product or of every
-- product (product_id NULL): engine candidates and activation conflicts.
SELECT p.id, p.uuid, p.rental_product_id, p.scope, p.branch_id, p.channel, p.name,
    b.uuid AS branch_uuid,
    v.id AS version_id, v.uuid AS version_uuid, v.version, v.priority
FROM rate_plans p
JOIN rate_plan_versions v ON v.organization_id = p.organization_id AND v.rate_plan_id = p.id AND v.status = 'active'
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND p.status = 'active'
  AND (sqlc.narg(rental_product_id)::bigint IS NULL OR p.rental_product_id = sqlc.narg(rental_product_id)::bigint)
ORDER BY p.id;

-- name: ListRatePlansForTarget :many
-- Live plans of one scope target (matrix cells, import upserts) with their
-- draft / active version ids.
SELECT p.id, p.uuid, p.rental_product_id, p.status, p.name, p.current_version,
    dv.id AS draft_version_id, dv.priority AS draft_priority,
    av.id AS active_version_id, av.priority AS active_priority
FROM rate_plans p
LEFT JOIN rate_plan_versions dv ON dv.organization_id = p.organization_id AND dv.rate_plan_id = p.id AND dv.status = 'draft'
LEFT JOIN rate_plan_versions av ON av.organization_id = p.organization_id AND av.rate_plan_id = p.id AND av.status = 'active'
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND p.scope = sqlc.arg(scope)
  AND p.branch_id IS NOT DISTINCT FROM sqlc.narg(branch_id)::bigint
  AND p.channel IS NOT DISTINCT FROM sqlc.narg(channel)::text
  AND COALESCE(dv.priority, av.priority, 0) = sqlc.arg(priority)
  AND (sqlc.narg(rental_product_id)::bigint IS NULL OR p.rental_product_id = sqlc.narg(rental_product_id)::bigint)
ORDER BY p.rental_product_id, (p.status = 'active') DESC, p.id DESC;

-- name: ListRatePlansForExport :many
-- One row per band of the effective version (draft, else active).
SELECT p.uuid, p.scope, p.channel, p.name, p.status,
    rp.code AS product_code, rp.title AS product_title,
    b.code AS branch_code,
    v.version, v.status AS version_status, v.priority,
    rb.min_days, rb.max_days, rb.daily_price
FROM rate_plans p
JOIN rental_products rp ON rp.organization_id = p.organization_id AND rp.id = p.rental_product_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
JOIN rate_plan_versions v ON v.organization_id = p.organization_id AND v.rate_plan_id = p.id
    AND v.id = COALESCE(
        (SELECT d.id FROM rate_plan_versions d WHERE d.organization_id = p.organization_id AND d.rate_plan_id = p.id AND d.status = 'draft'),
        (SELECT a.id FROM rate_plan_versions a WHERE a.organization_id = p.organization_id AND a.rate_plan_id = p.id AND a.status = 'active')
    )
JOIN rate_bands rb ON rb.organization_id = p.organization_id AND rb.rate_plan_version_id = v.id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR p.branch_id IS NULL OR p.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR rp.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(scopes)::text[] IS NULL OR p.scope = ANY(sqlc.narg(scopes)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR p.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR rp.code ILIKE '%' || sqlc.narg(q) || '%'
    OR rp.title ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY rp.code, p.scope, b.code NULLS FIRST, p.channel NULLS FIRST, rb.min_days
LIMIT sqlc.arg(limit_count);

-- --------------------------------------------------------------- date rules

-- name: CreateDateRule :one
INSERT INTO date_rules (organization_id, name)
VALUES (sqlc.arg(organization_id), sqlc.arg(name))
RETURNING id, uuid;

-- name: DateRuleNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM date_rules
    WHERE organization_id = sqlc.arg(organization_id)
      AND lower(name) = lower(sqlc.arg(name))
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id)::bigint)
)::boolean;

-- name: GetDateRule :one
SELECT r.id, r.uuid, r.name, r.status, r.current_version, r.active_version, r.activated_at, r.created_at, r.updated_at
FROM date_rules r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid)
  AND r.deleted_at IS NULL;

-- name: LockDateRule :one
SELECT id FROM date_rules
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateDateRuleName :execrows
UPDATE date_rules SET name = sqlc.arg(name)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetDateRuleState :execrows
UPDATE date_rules
SET status = sqlc.arg(status),
    current_version = sqlc.arg(current_version),
    active_version = sqlc.narg(active_version),
    activated_at = sqlc.narg(activated_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: TouchDateRule :exec
UPDATE date_rules SET updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SoftDeleteDateRule :execrows
UPDATE date_rules SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListDateRules :many
-- Effective version = draft, else active. Date filters match rules whose
-- effective range [start, end) overlaps [date_from, date_to].
SELECT r.id, r.uuid, r.name, r.status, r.current_version, r.active_version, r.activated_at, r.created_at, r.updated_at,
    (dv.id IS NOT NULL)::boolean AS has_draft,
    COALESCE(dv.start_date, av.start_date)::date AS start_date,
    COALESCE(dv.end_date, av.end_date)::date AS end_date,
    COALESCE(dv.kind, av.kind)::text AS kind,
    COALESCE(dv.value, av.value)::numeric AS value,
    COALESCE(dv.priority, av.priority, 0)::integer AS priority,
    COALESCE(dv.product_scope, av.product_scope)::text AS product_scope,
    COALESCE(dv.all_branches, av.all_branches, TRUE)::boolean AS all_branches,
    (CASE WHEN dv.id IS NOT NULL THEN dv.channels IS NULL ELSE av.channels IS NULL END)::boolean AS all_channels
FROM date_rules r
LEFT JOIN date_rule_versions dv ON dv.organization_id = r.organization_id AND dv.date_rule_id = r.id AND dv.status = 'draft'
LEFT JOIN date_rule_versions av ON av.organization_id = r.organization_id AND av.date_rule_id = r.id AND av.status = 'active'
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR COALESCE(dv.kind, av.kind) = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(has_draft)::boolean IS NULL OR (dv.id IS NOT NULL) = sqlc.narg(has_draft)::boolean)
  AND (sqlc.narg(date_from)::date IS NULL OR COALESCE(dv.end_date, av.end_date) > sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR COALESCE(dv.start_date, av.start_date) <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(q)::text IS NULL OR r.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN r.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN r.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'start_date' AND NOT sqlc.arg(sort_desc)::boolean THEN COALESCE(dv.start_date, av.start_date) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'start_date' AND sqlc.arg(sort_desc)::boolean THEN COALESCE(dv.start_date, av.start_date) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'priority' AND NOT sqlc.arg(sort_desc)::boolean THEN COALESCE(dv.priority, av.priority, 0) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'priority' AND sqlc.arg(sort_desc)::boolean THEN COALESCE(dv.priority, av.priority, 0) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN r.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN r.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN r.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
  r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDateRules :one
SELECT COUNT(*)::bigint
FROM date_rules r
LEFT JOIN date_rule_versions dv ON dv.organization_id = r.organization_id AND dv.date_rule_id = r.id AND dv.status = 'draft'
LEFT JOIN date_rule_versions av ON av.organization_id = r.organization_id AND av.date_rule_id = r.id AND av.status = 'active'
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR COALESCE(dv.kind, av.kind) = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(has_draft)::boolean IS NULL OR (dv.id IS NOT NULL) = sqlc.narg(has_draft)::boolean)
  AND (sqlc.narg(date_from)::date IS NULL OR COALESCE(dv.end_date, av.end_date) > sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR COALESCE(dv.start_date, av.start_date) <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(q)::text IS NULL OR r.name ILIKE '%' || sqlc.narg(q) || '%');

-- name: ListDateRuleFacets :many
SELECT 'status'::text AS facet, r.status::text AS value, COUNT(*)::bigint AS count
FROM date_rules r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.deleted_at IS NULL
GROUP BY r.status
UNION ALL
SELECT 'kind'::text, COALESCE(dv.kind, av.kind)::text, COUNT(*)::bigint
FROM date_rules r
LEFT JOIN date_rule_versions dv ON dv.organization_id = r.organization_id AND dv.date_rule_id = r.id AND dv.status = 'draft'
LEFT JOIN date_rule_versions av ON av.organization_id = r.organization_id AND av.date_rule_id = r.id AND av.status = 'active'
WHERE r.organization_id = sqlc.arg(organization_id) AND r.deleted_at IS NULL
GROUP BY COALESCE(dv.kind, av.kind);

-- name: CreateDateRuleVersion :one
INSERT INTO date_rule_versions (
    organization_id, date_rule_id, version, start_date, end_date, product_scope, all_branches, channels,
    kind, value, priority, change_note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(date_rule_id), sqlc.arg(version), sqlc.arg(start_date), sqlc.arg(end_date),
    sqlc.arg(product_scope), sqlc.arg(all_branches), sqlc.narg(channels), sqlc.arg(kind), sqlc.arg(value),
    sqlc.arg(priority), sqlc.arg(change_note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: CopyDateRuleVersion :one
-- New draft version N+1 from an existing version (targets copied apart).
INSERT INTO date_rule_versions (
    organization_id, date_rule_id, version, start_date, end_date, product_scope, all_branches, channels,
    kind, value, priority, change_note, created_by
)
SELECT v.organization_id, v.date_rule_id, sqlc.arg(version)::integer, v.start_date, v.end_date, v.product_scope,
    v.all_branches, v.channels, v.kind, v.value, v.priority, '', sqlc.narg(created_by)
FROM date_rule_versions v
WHERE v.organization_id = sqlc.arg(organization_id) AND v.id = sqlc.arg(from_version_id)
RETURNING id, uuid;

-- name: GetDateRuleVersionByStatus :one
SELECT v.id, v.uuid, v.date_rule_id, v.version, v.status, v.start_date, v.end_date, v.product_scope, v.all_branches,
    v.channels, v.kind, v.value, v.priority, v.change_note, v.created_at, v.activated_at, v.superseded_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    au.uuid AS activated_by_uuid, au.name AS activated_by_name, au.surname AS activated_by_surname, au.email AS activated_by_email
FROM date_rule_versions v
LEFT JOIN users cu ON cu.id = v.created_by
LEFT JOIN users au ON au.id = v.activated_by
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.date_rule_id = sqlc.arg(date_rule_id)
  AND v.status = sqlc.arg(status);

-- name: ListDateRuleVersions :many
SELECT v.id, v.uuid, v.date_rule_id, v.version, v.status, v.start_date, v.end_date, v.product_scope, v.all_branches,
    v.channels, v.kind, v.value, v.priority, v.change_note, v.created_at, v.activated_at, v.superseded_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    au.uuid AS activated_by_uuid, au.name AS activated_by_name, au.surname AS activated_by_surname, au.email AS activated_by_email
FROM date_rule_versions v
LEFT JOIN users cu ON cu.id = v.created_by
LEFT JOIN users au ON au.id = v.activated_by
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.date_rule_id = sqlc.arg(date_rule_id)
ORDER BY v.version DESC;

-- name: UpdateDateRuleDraft :execrows
UPDATE date_rule_versions
SET start_date = sqlc.arg(start_date),
    end_date = sqlc.arg(end_date),
    product_scope = sqlc.arg(product_scope),
    all_branches = sqlc.arg(all_branches),
    channels = sqlc.narg(channels),
    kind = sqlc.arg(kind),
    value = sqlc.arg(value),
    priority = sqlc.arg(priority),
    change_note = sqlc.arg(change_note)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'draft';

-- name: ActivateDateRuleVersion :execrows
UPDATE date_rule_versions
SET status = 'active', activated_at = sqlc.arg(activated_at), activated_by = sqlc.narg(activated_by)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'draft';

-- name: SupersedeDateRuleVersion :execrows
UPDATE date_rule_versions
SET status = 'superseded', superseded_at = sqlc.arg(superseded_at)
WHERE organization_id = sqlc.arg(organization_id) AND date_rule_id = sqlc.arg(date_rule_id) AND status = 'active';

-- name: DeleteDateRuleDraft :execrows
DELETE FROM date_rule_versions
WHERE organization_id = sqlc.arg(organization_id) AND date_rule_id = sqlc.arg(date_rule_id) AND status = 'draft';

-- name: ListDateRuleTargets :many
SELECT t.date_rule_version_id, t.rental_product_id, t.segment_id, t.branch_id,
    rp.uuid AS product_uuid, rp.code AS product_code, rp.title AS product_title,
    s.uuid AS segment_uuid, s.code AS segment_code, s.name AS segment_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name
FROM date_rule_version_targets t
LEFT JOIN rental_products rp ON rp.organization_id = t.organization_id AND rp.id = t.rental_product_id
LEFT JOIN vehicle_segments s ON s.organization_id = t.organization_id AND s.id = t.segment_id
LEFT JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.date_rule_version_id = ANY(sqlc.arg(version_ids)::bigint[])
ORDER BY t.date_rule_version_id, t.id;

-- name: DeleteDateRuleTargets :exec
DELETE FROM date_rule_version_targets
WHERE organization_id = sqlc.arg(organization_id) AND date_rule_version_id = sqlc.arg(date_rule_version_id);

-- name: InsertDateRuleTarget :exec
INSERT INTO date_rule_version_targets (organization_id, date_rule_version_id, rental_product_id, segment_id, branch_id)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(date_rule_version_id), sqlc.narg(rental_product_id), sqlc.narg(segment_id),
    sqlc.narg(branch_id)
);

-- name: CopyDateRuleTargets :exec
INSERT INTO date_rule_version_targets (organization_id, date_rule_version_id, rental_product_id, segment_id, branch_id)
SELECT t.organization_id, sqlc.arg(to_version_id)::bigint, t.rental_product_id, t.segment_id, t.branch_id
FROM date_rule_version_targets t
WHERE t.organization_id = sqlc.arg(organization_id) AND t.date_rule_version_id = sqlc.arg(from_version_id);

-- name: ListActiveDateRuleVersions :many
-- Active versions of active rules whose range overlaps [date_from, date_to)
-- (NULL bounds = unbounded): engine candidates and activation overlaps.
SELECT r.id AS rule_id, r.uuid AS rule_uuid, r.name,
    v.id, v.uuid, v.version, v.start_date, v.end_date, v.product_scope, v.all_branches, v.channels, v.kind,
    v.value, v.priority
FROM date_rules r
JOIN date_rule_versions v ON v.organization_id = r.organization_id AND v.date_rule_id = r.id AND v.status = 'active'
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND r.status = 'active'
  AND (sqlc.narg(date_from)::date IS NULL OR v.end_date > sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR v.start_date < sqlc.narg(date_to)::date)
ORDER BY r.id;

-- name: ListDateRulesForExport :many
SELECT r.uuid, r.name, r.status,
    v.version, v.status AS version_status, v.start_date, v.end_date, v.product_scope, v.all_branches, v.channels,
    v.kind, v.value, v.priority
FROM date_rules r
JOIN date_rule_versions v ON v.organization_id = r.organization_id AND v.date_rule_id = r.id
    AND v.id = COALESCE(
        (SELECT d.id FROM date_rule_versions d WHERE d.organization_id = r.organization_id AND d.date_rule_id = r.id AND d.status = 'draft'),
        (SELECT a.id FROM date_rule_versions a WHERE a.organization_id = r.organization_id AND a.date_rule_id = r.id AND a.status = 'active')
    )
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR v.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR v.end_date > sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR v.start_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(q)::text IS NULL OR r.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY v.start_date, r.name
LIMIT sqlc.arg(limit_count);
