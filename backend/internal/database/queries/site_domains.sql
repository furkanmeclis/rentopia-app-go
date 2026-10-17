-- name: CreateSiteDomain :one
INSERT INTO site_domains(organization_id,host,verification_token,provisioner,created_by,created_at,next_check_at)
VALUES ($1,$2,$3,$4,$5,sqlc.arg(now)::timestamptz,sqlc.arg(now)::timestamptz) RETURNING *;

-- name: ListSiteDomains :many
SELECT * FROM site_domains WHERE organization_id = $1 ORDER BY created_at,id;

-- name: ListPlatformSiteDomains :many
SELECT id, uuid, organization_id, host, status, verification_token, last_checked_at, verified_at, last_error, attention_since, tls_status, provisioner, provisioner_id, created_by, attempts, next_check_at, lease_until, origins_dirty, created_at, updated_at FROM site_domains ORDER BY created_at DESC,id DESC LIMIT $1 OFFSET $2;

-- name: GetSiteDomain :one
SELECT * FROM site_domains WHERE organization_id = $1 AND uuid = $2;

-- name: GetPlatformSiteDomain :one
SELECT id, uuid, organization_id, host, status, verification_token, last_checked_at, verified_at, last_error, attention_since, tls_status, provisioner, provisioner_id, created_by, attempts, next_check_at, lease_until, origins_dirty, created_at, updated_at FROM site_domains WHERE uuid = $1;

-- name: LookupSiteDomain :one
SELECT organization_id FROM site_domains WHERE host = $1 AND status = 'verified';

-- name: RequestSiteDomainCheck :execrows
UPDATE site_domains SET next_check_at = $3, origins_dirty = true WHERE uuid = $1 AND organization_id = $2 AND status <> 'disabled';

-- name: DisableSiteDomain :execrows
UPDATE site_domains SET status = 'disabled', origins_dirty = true, next_check_at = $3
WHERE organization_id = $1 AND uuid = $2;

-- name: ClaimDueSiteDomains :many
WITH due AS (
 SELECT id, organization_id FROM site_domains WHERE next_check_at <= $1 AND (lease_until IS NULL OR lease_until <= $1)
 AND (status IN ('pending','verified','disabled') OR origins_dirty OR tls_status IN ('failed','provisioning'))
 ORDER BY next_check_at,id LIMIT 50 FOR UPDATE SKIP LOCKED
)
UPDATE site_domains d SET lease_until = $1::timestamptz + interval '10 minutes'
FROM due WHERE d.id = due.id AND d.organization_id = due.organization_id RETURNING d.*;

-- name: UpdateSiteDomainCheck :one
UPDATE site_domains SET status = $3, last_checked_at = $4, verified_at = $5,
 attention_since = CASE WHEN $3 = 'failed' THEN coalesce(attention_since,$4::timestamptz)
 WHEN tls_status <> 'failed' THEN NULL ELSE attention_since END,
 last_error = $6, attempts = attempts + 1, next_check_at = $7, origins_dirty = true
WHERE id = $1 AND status = $2 AND organization_id = $8 RETURNING *;

-- name: UpdateSiteDomainTLS :exec
UPDATE site_domains SET tls_status = CASE WHEN provisioner = 'manual' AND tls_status = 'active' AND status = 'verified' THEN 'active' ELSE $2 END, provisioner_id = $3, last_error = $4,
 attention_since = CASE WHEN $2 = 'failed' THEN coalesce(attention_since,now())
 WHEN status <> 'failed' AND $2 <> 'provisioning' THEN NULL ELSE attention_since END WHERE id = $1 AND organization_id = $5;

-- name: MarkSiteDomainOriginsSynced :exec
UPDATE site_domains SET origins_dirty = false WHERE id = $1 AND status = $2 AND organization_id = $3;

-- name: DeleteDisabledSiteDomain :exec
DELETE FROM site_domains WHERE id = $1 AND organization_id = $2 AND status = 'disabled' AND NOT origins_dirty;

-- name: MarkSiteDomainTLSActive :execrows
UPDATE site_domains SET tls_status = 'active', last_error = '', attention_since = NULL
WHERE uuid = $1 AND organization_id = $2 AND provisioner = 'manual' AND status = 'verified';

-- name: RetrySiteDomainReconciliation :exec
UPDATE site_domains SET next_check_at = $3
WHERE organization_id = $1 AND id = $2;

-- name: ListSiteDomainHealthIssues :many
SELECT id, uuid, organization_id, host, status, verification_token, last_checked_at,
 verified_at, last_error, attention_since, tls_status, provisioner, provisioner_id, created_by, attempts,
 next_check_at, lease_until, origins_dirty, created_at, updated_at
FROM site_domains WHERE status = 'failed' OR tls_status IN ('failed','manual')
ORDER BY updated_at DESC,id DESC LIMIT $1;

-- name: SiteDomainHealthCounts :one
SELECT count(organization_id) FILTER (WHERE status = 'verified')::bigint AS verified,
 count(organization_id) FILTER (WHERE status = 'pending')::bigint AS pending,
 count(organization_id) FILTER (WHERE status = 'failed')::bigint AS failed
FROM site_domains;

-- name: ListSiteDomainKeys :many
-- Include rotation predecessors: their booking sessions must lose removed origins too.
WITH RECURSIVE linked AS (
 SELECT k.id,k.uuid,k.rotated_from_id FROM public_api_keys k
 JOIN site_settings s ON s.api_key_id = k.id AND s.organization_id = k.organization_id
 WHERE k.organization_id = $1
 UNION
 SELECT k.id,k.uuid,k.rotated_from_id FROM public_api_keys k
 JOIN linked l ON k.rotated_from_id = l.id OR k.id = l.rotated_from_id
 WHERE k.organization_id = $1
)
SELECT uuid FROM linked;

-- name: ReleaseSiteDomainLease :exec
UPDATE site_domains SET lease_until = NULL WHERE organization_id = $1 AND id = $2;
