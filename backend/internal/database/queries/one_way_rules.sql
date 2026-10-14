-- Tenant resource: one-way rules (TEC-695). Every query filters by
-- organization_id (docs/design/tenant-core.md §8).
--
-- Branch scope: a rule is visible when its from OR to branch is in the
-- caller's scope (branch_ids NULL = every branch).

-- name: CreateOneWayRule :one
INSERT INTO one_way_rules (
    organization_id, from_branch_id, to_branch_id, rental_product_id, allowed, fee, note, is_active
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(from_branch_id), sqlc.arg(to_branch_id), sqlc.narg(rental_product_id),
    sqlc.arg(allowed), sqlc.arg(fee), sqlc.arg(note), sqlc.arg(is_active)
)
RETURNING uuid;

-- name: UpdateOneWayRule :execrows
UPDATE one_way_rules
SET from_branch_id = sqlc.arg(from_branch_id),
    to_branch_id = sqlc.arg(to_branch_id),
    rental_product_id = sqlc.narg(rental_product_id),
    allowed = sqlc.arg(allowed),
    fee = sqlc.arg(fee),
    note = sqlc.arg(note),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: SetOneWayRuleActive :execrows
UPDATE one_way_rules
SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: SoftDeleteOneWayRule :execrows
UPDATE one_way_rules
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: OneWayRouteTaken :one
-- Another live rule with the same route and product (NULL = generic).
SELECT EXISTS (
    SELECT 1 FROM one_way_rules
    WHERE organization_id = sqlc.arg(organization_id)
      AND from_branch_id = sqlc.arg(from_branch_id)
      AND to_branch_id = sqlc.arg(to_branch_id)
      AND COALESCE(rental_product_id, 0) = COALESCE(sqlc.narg(rental_product_id)::bigint, 0)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: GetOneWayRule :one
SELECT r.id, r.uuid, r.from_branch_id, r.to_branch_id, r.rental_product_id, r.allowed, r.fee, r.note,
    r.is_active, r.created_at, r.updated_at,
    fb.uuid AS from_branch_uuid, fb.code AS from_branch_code, fb.name AS from_branch_name,
    tb.uuid AS to_branch_uuid, tb.code AS to_branch_code, tb.name AS to_branch_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title
FROM one_way_rules r
JOIN branches fb ON fb.organization_id = r.organization_id AND fb.id = r.from_branch_id
JOIN branches tb ON tb.organization_id = r.organization_id AND tb.id = r.to_branch_id
LEFT JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid)
  AND r.deleted_at IS NULL;

-- name: ListOneWayRules :many
SELECT r.id, r.uuid, r.from_branch_id, r.to_branch_id, r.rental_product_id, r.allowed, r.fee, r.note,
    r.is_active, r.created_at, r.updated_at,
    fb.uuid AS from_branch_uuid, fb.code AS from_branch_code, fb.name AS from_branch_name,
    tb.uuid AS to_branch_uuid, tb.code AS to_branch_code, tb.name AS to_branch_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title
FROM one_way_rules r
JOIN branches fb ON fb.organization_id = r.organization_id AND fb.id = r.from_branch_id
JOIN branches tb ON tb.organization_id = r.organization_id AND tb.id = r.to_branch_id
LEFT JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(allowed)::boolean IS NULL OR r.allowed = sqlc.narg(allowed))
  AND (sqlc.narg(from_branch_uuids)::uuid[] IS NULL OR fb.uuid = ANY(sqlc.narg(from_branch_uuids)::uuid[]))
  AND (sqlc.narg(to_branch_uuids)::uuid[] IS NULL OR tb.uuid = ANY(sqlc.narg(to_branch_uuids)::uuid[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR fb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR fb.code ILIKE '%' || sqlc.narg(q) || '%'
    OR tb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR tb.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR r.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'fee' AND NOT sqlc.arg(sort_desc)::boolean THEN r.fee END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'fee' AND sqlc.arg(sort_desc)::boolean THEN r.fee END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'from_branch' AND NOT sqlc.arg(sort_desc)::boolean THEN fb.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'from_branch' AND sqlc.arg(sort_desc)::boolean THEN fb.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'to_branch' AND NOT sqlc.arg(sort_desc)::boolean THEN tb.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'to_branch' AND sqlc.arg(sort_desc)::boolean THEN tb.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN r.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN r.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN r.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
  r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOneWayRules :one
SELECT COUNT(*)::bigint
FROM one_way_rules r
JOIN branches fb ON fb.organization_id = r.organization_id AND fb.id = r.from_branch_id
JOIN branches tb ON tb.organization_id = r.organization_id AND tb.id = r.to_branch_id
LEFT JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(allowed)::boolean IS NULL OR r.allowed = sqlc.narg(allowed))
  AND (sqlc.narg(from_branch_uuids)::uuid[] IS NULL OR fb.uuid = ANY(sqlc.narg(from_branch_uuids)::uuid[]))
  AND (sqlc.narg(to_branch_uuids)::uuid[] IS NULL OR tb.uuid = ANY(sqlc.narg(to_branch_uuids)::uuid[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR fb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR fb.code ILIKE '%' || sqlc.narg(q) || '%'
    OR tb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR tb.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR r.note ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListOneWayRuleUUIDs :many
-- Bulk "all matching" scope.
SELECT r.uuid
FROM one_way_rules r
JOIN branches fb ON fb.organization_id = r.organization_id AND fb.id = r.from_branch_id
JOIN branches tb ON tb.organization_id = r.organization_id AND tb.id = r.to_branch_id
LEFT JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(allowed)::boolean IS NULL OR r.allowed = sqlc.narg(allowed))
  AND (sqlc.narg(from_branch_uuids)::uuid[] IS NULL OR fb.uuid = ANY(sqlc.narg(from_branch_uuids)::uuid[]))
  AND (sqlc.narg(to_branch_uuids)::uuid[] IS NULL OR tb.uuid = ANY(sqlc.narg(to_branch_uuids)::uuid[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR fb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR fb.code ILIKE '%' || sqlc.narg(q) || '%'
    OR tb.name ILIKE '%' || sqlc.narg(q) || '%'
    OR tb.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR r.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY r.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListOneWayRuleFacets :many
-- facet: from_branch | to_branch | is_active | allowed, in the caller's
-- branch scope.
WITH scoped AS (
    SELECT r.organization_id, r.from_branch_id, r.to_branch_id, r.is_active, r.allowed
    FROM one_way_rules r
    WHERE r.organization_id = sqlc.arg(organization_id)
      AND r.deleted_at IS NULL
      AND (
        sqlc.narg(branch_ids)::bigint[] IS NULL
        OR r.from_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR r.to_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
      )
)
SELECT 'from_branch'::text AS facet, b.uuid::text AS value, b.name::text AS label, COUNT(*)::bigint AS count
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.from_branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'to_branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.to_branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'is_active'::text, s.is_active::text, ''::text, COUNT(*)::bigint
FROM scoped s GROUP BY s.is_active
UNION ALL
SELECT 'allowed'::text, s.allowed::text, ''::text, COUNT(*)::bigint
FROM scoped s GROUP BY s.allowed;

-- name: ListOneWayRulesForRoute :many
-- Active rules of a route: the product rule (when product_id is given)
-- first, then the generic rule. The use case takes the first row.
SELECT r.uuid, r.rental_product_id, r.allowed, r.fee
FROM one_way_rules r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.from_branch_id = sqlc.arg(from_branch_id)
  AND r.to_branch_id = sqlc.arg(to_branch_id)
  AND r.deleted_at IS NULL
  AND r.is_active
  AND (r.rental_product_id IS NULL OR r.rental_product_id = sqlc.narg(rental_product_id)::bigint)
ORDER BY (r.rental_product_id IS NULL) ASC, r.id ASC;
