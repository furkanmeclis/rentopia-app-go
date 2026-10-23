-- Support access grants (TEC-678). Organization-scoped: every query names
-- organization_id. "Active" = revoked_at IS NULL AND expires_at > now, with
-- now passed by the caller (request time; no job closes grants).

-- name: CreateSupportGrant :one
INSERT INTO support_grants (organization_id, created_by, scope, expires_at, note)
VALUES (sqlc.arg(organization_id), sqlc.narg(created_by), sqlc.arg(scope), sqlc.arg(expires_at), sqlc.arg(note))
RETURNING *;

-- name: RevokeActiveSupportGrants :execrows
-- A new grant replaces the active one of the organization.
UPDATE support_grants
SET revoked_at = sqlc.arg(now), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now);

-- name: RevokeSupportGrant :one
UPDATE support_grants
SET revoked_at = sqlc.arg(now), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now)
RETURNING *;

-- name: GetSupportGrant :one
SELECT g.*,
       cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
       ru.uuid AS revoked_by_uuid, ru.name AS revoked_by_name, ru.surname AS revoked_by_surname, ru.email AS revoked_by_email
FROM support_grants g
LEFT JOIN users cu ON cu.id = g.created_by
LEFT JOIN users ru ON ru.id = g.revoked_by
WHERE g.organization_id = sqlc.arg(organization_id)
  AND g.uuid = sqlc.arg(uuid);

-- name: ListSupportGrants :many
SELECT g.*,
       cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
       ru.uuid AS revoked_by_uuid, ru.name AS revoked_by_name, ru.surname AS revoked_by_surname, ru.email AS revoked_by_email
FROM support_grants g
LEFT JOIN users cu ON cu.id = g.created_by
LEFT JOIN users ru ON ru.id = g.revoked_by
WHERE g.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(statuses)::text[] IS NULL
    OR (CASE
          WHEN g.revoked_at IS NOT NULL THEN 'revoked'
          WHEN g.expires_at <= sqlc.arg(now) THEN 'expired'
          ELSE 'active'
        END) = ANY(sqlc.narg(statuses)::text[])
  )
ORDER BY g.created_at DESC, g.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountSupportGrants :one
SELECT COUNT(*)::bigint
FROM support_grants g
WHERE g.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(statuses)::text[] IS NULL
    OR (CASE
          WHEN g.revoked_at IS NOT NULL THEN 'revoked'
          WHEN g.expires_at <= sqlc.arg(now) THEN 'expired'
          ELSE 'active'
        END) = ANY(sqlc.narg(statuses)::text[])
  );

-- name: GetActiveSupportGrant :one
-- The grant platform staff act under; the newest wins if two overlap.
SELECT * FROM support_grants
WHERE organization_id = sqlc.arg(organization_id)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetUserSupportAccessSummary :one
-- Impersonation gate: the target's live memberships and how many of those
-- organizations have an active grant.
SELECT
    COUNT(*)::bigint AS memberships,
    (COUNT(*) FILTER (WHERE EXISTS (
        SELECT 1 FROM support_grants g
        WHERE g.organization_id = m.organization_id
          AND g.revoked_at IS NULL
          AND g.expires_at > sqlc.arg(now)
    )))::bigint AS granted
FROM organization_members m
JOIN organizations o ON o.id = m.organization_id AND o.deleted_at IS NULL
WHERE m.user_id = sqlc.arg(user_id);

-- name: ListActiveSupportGrantsPlatform :many
-- Platform view: active grants across organizations (support staff).
SELECT g.uuid, g.scope, g.expires_at, g.note, g.created_at,
       o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
       cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM support_grants g
JOIN organizations o ON o.id = g.organization_id AND o.deleted_at IS NULL
LEFT JOIN users cu ON cu.id = g.created_by
WHERE g.revoked_at IS NULL
  AND g.expires_at > sqlc.arg(now)
ORDER BY g.expires_at ASC, g.id ASC
LIMIT 200;
