-- Branded site (TEC-729, docs/design/branded-site.md). Every query names
-- organization_id; the host lookups are global by design (the host
-- resolves the organization), like the public API key lookup by hash.

-- name: GetSiteSettings :one
SELECT s.*, k.uuid AS api_key_uuid
FROM site_settings s
LEFT JOIN public_api_keys k ON k.organization_id = s.organization_id AND k.id = s.api_key_id
WHERE s.organization_id = sqlc.arg(organization_id);

-- name: InsertSiteSettings :one
-- First save of an organization's site. ON CONFLICT DO NOTHING: a
-- concurrent first save returns no row and the caller answers a conflict.
INSERT INTO site_settings (
    organization_id, enabled, subdomain, default_locale, config, api_key_id, updated_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(enabled), sqlc.arg(subdomain), sqlc.arg(default_locale),
    sqlc.arg(config), sqlc.narg(api_key_id), sqlc.narg(updated_by)
)
ON CONFLICT (organization_id) DO NOTHING
RETURNING *;

-- name: UpdateSiteSettings :one
-- Saves the site when the caller saw the current version (optimistic
-- concurrency); no row = the version moved.
UPDATE site_settings
SET enabled = sqlc.arg(enabled),
    subdomain = sqlc.arg(subdomain),
    default_locale = sqlc.arg(default_locale),
    config = sqlc.arg(config),
    api_key_id = sqlc.narg(api_key_id),
    updated_by = sqlc.narg(updated_by),
    version = version + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: SiteSubdomainTaken :one
-- Whether another organization's site already uses the subdomain.
SELECT EXISTS (
    SELECT 1 FROM site_settings
    WHERE subdomain = sqlc.arg(subdomain)
      AND organization_id <> sqlc.arg(organization_id)
)::boolean AS taken;

-- name: GetSitePublicAPIKeyID :one
-- The internal id of a key the site settings link to.
SELECT id FROM public_api_keys
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid);

-- name: ResolveSiteBySubdomain :one
-- The enabled site of a subdomain with its organization and linked key
-- (global lookup by the host; the organization comes from the row).
SELECT s.id, s.uuid, s.organization_id, s.enabled, s.subdomain, s.default_locale, s.config,
    s.logo_object_key, s.favicon_object_key, s.cover_object_key, s.version, s.updated_at,
    o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
    o.status AS organization_status, o.access_starts_at, o.access_ends_at,
    o.logo_object_key AS organization_logo_object_key,
    k.uuid AS api_key_uuid, k.kind AS api_key_kind, k.channel AS api_key_channel,
    k.publishable_key, k.status AS api_key_status, k.expires_at AS api_key_expires_at
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN public_api_keys k ON k.organization_id = s.organization_id AND k.id = s.api_key_id
WHERE s.subdomain = sqlc.arg(subdomain)
  AND s.organization_id = o.id
  AND s.enabled
  AND o.deleted_at IS NULL;

-- name: ResolveSiteByOrganization :one
-- The enabled site of an organization (a verified custom domain names the
-- organization; TEC-731).
SELECT s.id, s.uuid, s.organization_id, s.enabled, s.subdomain, s.default_locale, s.config,
    s.logo_object_key, s.favicon_object_key, s.cover_object_key, s.version, s.updated_at,
    o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
    o.status AS organization_status, o.access_starts_at, o.access_ends_at,
    o.logo_object_key AS organization_logo_object_key,
    k.uuid AS api_key_uuid, k.kind AS api_key_kind, k.channel AS api_key_channel,
    k.publishable_key, k.status AS api_key_status, k.expires_at AS api_key_expires_at
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN public_api_keys k ON k.organization_id = s.organization_id AND k.id = s.api_key_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND o.id = s.organization_id
  AND s.enabled
  AND o.deleted_at IS NULL;

-- name: GetSiteAssetKeys :one
-- The image keys of an enabled site by its public uuid (asset stream).
SELECT s.organization_id, s.logo_object_key, s.favicon_object_key, s.cover_object_key,
    o.logo_object_key AS organization_logo_object_key
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id
WHERE s.uuid = sqlc.arg(uuid)
  AND s.organization_id = o.id
  AND s.enabled
  AND o.deleted_at IS NULL;
