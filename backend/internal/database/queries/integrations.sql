-- Integration accounts and jobs (TEC-676). Accounts are organization-owned
-- (every query filters by organization_id); jobs are mixed scope
-- (organization_id NULL = platform job) and every query names the scope.

-- name: InsertIntegrationAccount :one
INSERT INTO integration_accounts (
    uuid, organization_id, kind, provider, display_name, mode, status, credentials_ciphertext, credentials_hint,
    settings, created_by, updated_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(provider), sqlc.arg(display_name),
    sqlc.arg(mode), 'active', sqlc.arg(credentials_ciphertext), sqlc.arg(credentials_hint), sqlc.arg(settings),
    sqlc.narg(created_by), sqlc.narg(created_by)
)
RETURNING id;

-- name: GetIntegrationAccount :one
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.display_name, a.mode, a.status,
    a.credentials_ciphertext, a.credentials_hint, a.settings, a.last_checked_at, a.last_check_status,
    a.last_check_error, a.created_at, a.updated_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM integration_accounts a
LEFT JOIN users cu ON cu.id = a.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: GetIntegrationAccountByID :one
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.display_name, a.mode, a.status,
    a.credentials_ciphertext, a.credentials_hint, a.settings
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id);

-- name: GetActiveIntegrationAccount :one
-- The organization's active account of a kind (payments go to it).
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.display_name, a.mode, a.status,
    a.credentials_ciphertext, a.credentials_hint, a.settings
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.kind = sqlc.arg(kind)
  AND a.status = 'active';

-- name: ListIntegrationAccounts :many
SELECT a.uuid, a.kind, a.provider, a.display_name, a.mode, a.status, a.credentials_hint, a.last_checked_at,
    a.last_check_status, a.last_check_error, a.created_at, a.updated_at,
    COUNT(*) OVER ()::bigint AS total
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(providers)::text[] IS NULL OR a.provider = ANY(sqlc.narg(providers)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR a.display_name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND NOT sqlc.arg(sort_desc)::boolean THEN a.display_name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND sqlc.arg(sort_desc)::boolean THEN a.display_name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN a.created_at END DESC,
    a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: UpdateIntegrationAccount :execrows
UPDATE integration_accounts
SET display_name = sqlc.arg(display_name),
    mode = sqlc.arg(mode),
    credentials_ciphertext = sqlc.arg(credentials_ciphertext),
    credentials_hint = sqlc.arg(credentials_hint),
    settings = sqlc.arg(settings),
    updated_by = sqlc.narg(updated_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetIntegrationAccountStatus :execrows
UPDATE integration_accounts
SET status = sqlc.arg(status), updated_by = sqlc.narg(updated_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetIntegrationAccountCheck :exec
UPDATE integration_accounts
SET last_checked_at = sqlc.arg(checked_at), last_check_status = sqlc.arg(check_status),
    last_check_error = sqlc.arg(check_error)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: UpsertIntegrationJob :one
-- Records a job once per provider and idempotency key; a repeated key
-- counts the delivery and returns the existing row (inserted = false).
INSERT INTO integration_jobs (
    organization_id, integration_account_id, direction, provider, kind, idempotency_key, payload, ref_type, ref_uuid
) VALUES (
    sqlc.narg(organization_id), sqlc.narg(integration_account_id), sqlc.arg(direction), sqlc.arg(provider),
    sqlc.arg(kind), sqlc.arg(idempotency_key), sqlc.arg(payload), sqlc.arg(ref_type), sqlc.narg(ref_uuid)
)
ON CONFLICT (provider, idempotency_key) DO UPDATE
SET deliveries = integration_jobs.deliveries + 1
RETURNING id, uuid, organization_id, status, attempts, deliveries, (xmax = 0)::boolean AS inserted;

-- name: ClaimIntegrationJob :execrows
-- Moves a pending / failed job to processing (one worker at a time).
UPDATE integration_jobs
SET status = 'processing', attempts = attempts + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('pending', 'failed');

-- name: FinishIntegrationJob :exec
UPDATE integration_jobs
SET status = sqlc.arg(status)::text, last_error = sqlc.arg(last_error), result = sqlc.arg(result),
    completed_at = CASE WHEN sqlc.arg(status)::text IN ('succeeded', 'ignored') THEN NOW() ELSE completed_at END
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListIntegrationJobs :many
SELECT j.uuid, j.direction, j.provider, j.kind, j.status, j.attempts, j.deliveries, j.last_error, j.ref_type,
    j.ref_uuid, j.completed_at, j.created_at, j.updated_at,
    COUNT(*) OVER ()::bigint AS total
FROM integration_jobs j
WHERE j.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_id)::bigint IS NULL OR j.integration_account_id = sqlc.narg(account_id)::bigint)
ORDER BY j.created_at DESC, j.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
