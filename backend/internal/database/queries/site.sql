-- Branded site (TEC-729 / TEC-730, docs/design/branded-site.md). Every query
-- names organization_id; the host lookups are global by design (the host
-- resolves the organization), like the public API key lookup by hash.
--
-- site_settings holds the live fields (enabled, subdomain, default_locale,
-- api_key_id) and the draft (config, *_object_key). site_setting_versions
-- holds the published snapshots; published_revision names the live one.

-- name: GetSiteSettings :one
SELECT s.*, k.uuid AS api_key_uuid,
    v.uuid AS published_uuid, v.config AS published_config,
    v.logo_object_key AS published_logo_object_key, v.favicon_object_key AS published_favicon_object_key,
    v.cover_object_key AS published_cover_object_key, v.published_at
FROM site_settings s
LEFT JOIN public_api_keys k ON k.organization_id = s.organization_id AND k.id = s.api_key_id
LEFT JOIN site_setting_versions v ON v.organization_id = s.organization_id AND v.revision = s.published_revision
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
-- Saves the live fields and the draft document when the caller saw the
-- current version (optimistic concurrency); no row = the version moved.
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

-- name: SetSiteKey :one
-- Links another publishable key (renew key); bumps the version.
UPDATE site_settings
SET api_key_id = sqlc.narg(api_key_id),
    updated_by = sqlc.narg(updated_by),
    version = version + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: SetSiteDraftImage :one
-- Replaces (or clears) one draft image; bumps the version.
UPDATE site_settings
SET logo_object_key = CASE WHEN sqlc.arg(kind)::text = 'logo' THEN sqlc.narg(object_key)::text ELSE logo_object_key END,
    favicon_object_key = CASE WHEN sqlc.arg(kind)::text = 'favicon' THEN sqlc.narg(object_key)::text ELSE favicon_object_key END,
    cover_object_key = CASE WHEN sqlc.arg(kind)::text = 'cover' THEN sqlc.narg(object_key)::text ELSE cover_object_key END,
    updated_by = sqlc.narg(updated_by),
    version = version + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: PublishSiteDraft :one
-- Publishes the draft the caller saw: snapshots config and images as the
-- next revision and makes it the published one, in one statement. No row =
-- the version moved; a unique violation = a concurrent publish.
WITH cur AS (
    SELECT s.id, s.organization_id, s.config, s.logo_object_key, s.favicon_object_key, s.cover_object_key, s.version
    FROM site_settings s
    WHERE s.organization_id = sqlc.arg(organization_id)
      AND s.version = sqlc.arg(expected_version)
    FOR UPDATE
), ins AS (
    INSERT INTO site_setting_versions (
        organization_id, site_id, revision, config, logo_object_key, favicon_object_key, cover_object_key,
        draft_version, published_by
    )
    SELECT cur.organization_id, cur.id,
        COALESCE((
            SELECT MAX(x.revision) FROM site_setting_versions x WHERE x.organization_id = sqlc.arg(organization_id)
        ), 0) + 1,
        cur.config, cur.logo_object_key, cur.favicon_object_key, cur.cover_object_key, cur.version,
        sqlc.narg(published_by)
    FROM cur
    RETURNING site_setting_versions.id, site_setting_versions.site_id, site_setting_versions.revision,
        site_setting_versions.uuid
)
UPDATE site_settings
SET published_revision = ins.revision
FROM ins
WHERE site_settings.organization_id = sqlc.arg(organization_id)
  AND site_settings.id = ins.site_id
RETURNING ins.revision, ins.uuid;

-- name: ListSiteVersions :many
-- Publish history, newest first.
SELECT v.uuid, v.revision, v.draft_version, v.published_at,
    (v.logo_object_key IS NOT NULL)::boolean AS has_logo,
    (v.favicon_object_key IS NOT NULL)::boolean AS has_favicon,
    (v.cover_object_key IS NOT NULL)::boolean AS has_cover,
    COALESCE(btrim(u.name || ' ' || u.surname), '')::text AS published_by_name
FROM site_setting_versions v
LEFT JOIN users u ON u.id = v.published_by
WHERE v.organization_id = sqlc.arg(organization_id)
ORDER BY v.revision DESC
LIMIT 200;

-- name: GetSiteVersion :one
SELECT v.*
FROM site_setting_versions v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid);

-- name: RestoreSiteDraft :one
-- Copies a published snapshot back into the draft (publish again to make it
-- live); bumps the version.
UPDATE site_settings s
SET config = v.config,
    logo_object_key = v.logo_object_key,
    favicon_object_key = v.favicon_object_key,
    cover_object_key = v.cover_object_key,
    updated_by = sqlc.narg(updated_by),
    version = s.version + 1
FROM site_setting_versions v
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.version = sqlc.arg(expected_version)
  AND v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
RETURNING s.version;

-- name: SiteObjectKeyInUse :one
-- Whether the draft or any snapshot still names an image object (it is
-- deleted from storage only when nothing does).
SELECT (
    EXISTS (
        SELECT 1 FROM site_settings s
        WHERE s.organization_id = sqlc.arg(organization_id)
          AND sqlc.arg(object_key)::text IN (
              COALESCE(s.logo_object_key, ''), COALESCE(s.favicon_object_key, ''), COALESCE(s.cover_object_key, '')
          )
    ) OR EXISTS (
        SELECT 1 FROM site_setting_versions v
        WHERE v.organization_id = sqlc.arg(organization_id)
          AND sqlc.arg(object_key)::text IN (
              COALESCE(v.logo_object_key, ''), COALESCE(v.favicon_object_key, ''), COALESCE(v.cover_object_key, '')
          )
    )
)::boolean AS in_use;

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

-- name: GetSiteKeySuccessor :one
-- The newest active key rotated from the site's key (Ayarlar → Public API
-- rotation): the site follows it on the next save.
SELECT k.uuid
FROM public_api_keys k
JOIN public_api_keys old ON old.organization_id = k.organization_id AND old.id = k.rotated_from_id
WHERE k.organization_id = sqlc.arg(organization_id)
  AND old.organization_id = sqlc.arg(organization_id)
  AND old.uuid = sqlc.arg(uuid)
  AND k.status = 'active'
  AND (k.expires_at IS NULL OR k.expires_at > NOW())
ORDER BY k.id DESC
LIMIT 1;

-- name: ResolveSiteBySubdomain :one
-- The site of a subdomain with its organization, linked key, draft and
-- published snapshot (global lookup by the host; the organization comes
-- from the row). The caller serves the published snapshot of an enabled
-- site, or the draft to a valid preview token.
SELECT s.id, s.uuid, s.organization_id, s.enabled, s.subdomain, s.default_locale, s.config,
    s.logo_object_key, s.favicon_object_key, s.cover_object_key, s.version, s.updated_at,
    s.published_revision,
    v.config AS published_config, v.logo_object_key AS published_logo_object_key,
    v.favicon_object_key AS published_favicon_object_key, v.cover_object_key AS published_cover_object_key,
    v.published_at,
    o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
    o.status AS organization_status, o.access_starts_at, o.access_ends_at,
    o.logo_object_key AS organization_logo_object_key,
    k.uuid AS api_key_uuid, k.kind AS api_key_kind, k.channel AS api_key_channel,
    k.publishable_key, k.status AS api_key_status, k.expires_at AS api_key_expires_at
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN public_api_keys k ON k.organization_id = s.organization_id AND k.id = s.api_key_id
LEFT JOIN site_setting_versions v ON v.organization_id = s.organization_id AND v.revision = s.published_revision
WHERE s.subdomain = sqlc.arg(subdomain)
  AND s.organization_id = o.id
  AND o.deleted_at IS NULL;

-- name: ResolveSiteByOrganization :one
-- The site of an organization (a verified custom domain names the
-- organization; TEC-731). Same shape as ResolveSiteBySubdomain.
SELECT s.id, s.uuid, s.organization_id, s.enabled, s.subdomain, s.default_locale, s.config,
    s.logo_object_key, s.favicon_object_key, s.cover_object_key, s.version, s.updated_at,
    s.published_revision,
    v.config AS published_config, v.logo_object_key AS published_logo_object_key,
    v.favicon_object_key AS published_favicon_object_key, v.cover_object_key AS published_cover_object_key,
    v.published_at,
    o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
    o.status AS organization_status, o.access_starts_at, o.access_ends_at,
    o.logo_object_key AS organization_logo_object_key,
    k.uuid AS api_key_uuid, k.kind AS api_key_kind, k.channel AS api_key_channel,
    k.publishable_key, k.status AS api_key_status, k.expires_at AS api_key_expires_at
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN public_api_keys k ON k.organization_id = s.organization_id AND k.id = s.api_key_id
LEFT JOIN site_setting_versions v ON v.organization_id = s.organization_id AND v.revision = s.published_revision
WHERE s.organization_id = sqlc.arg(organization_id)
  AND o.id = s.organization_id
  AND o.deleted_at IS NULL;

-- name: GetSiteAssetKeys :one
-- The image keys of a site by its public uuid (asset stream): the draft
-- and the published snapshot. The caller picks by the preview token.
SELECT s.organization_id, s.enabled, s.published_revision,
    s.logo_object_key, s.favicon_object_key, s.cover_object_key,
    v.logo_object_key AS published_logo_object_key, v.favicon_object_key AS published_favicon_object_key,
    v.cover_object_key AS published_cover_object_key,
    o.uuid AS organization_uuid, o.logo_object_key AS organization_logo_object_key
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN site_setting_versions v ON v.organization_id = s.organization_id AND v.revision = s.published_revision
WHERE s.uuid = sqlc.arg(uuid)
  AND s.organization_id = o.id
  AND o.deleted_at IS NULL;

-- ----------------------------------------------- catalog publish state

-- name: ListSiteCatalogBranches :many
SELECT b.uuid, b.code, b.name, b.city, b.is_active, b.show_online
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
ORDER BY b.city, b.name, b.id;

-- name: ListSiteCatalogProducts :many
SELECT p.uuid, p.code, p.title, p.status, p.is_active, p.show_online,
    s.name AS segment_name
FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND s.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
ORDER BY s.sort_order, s.name, p.title, p.id;

-- name: ListSiteCatalogExtras :many
SELECT e.uuid, e.code, e.name, e.status, e.extra_type, e.channels::varchar[] AS channels
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.deleted_at IS NULL
  AND e.extra_type <> 'delivery'
ORDER BY e.sort_order, e.name, e.id;

-- name: SetBranchShowOnline :one
UPDATE branches
SET show_online = COALESCE(sqlc.narg(show_online)::boolean, show_online)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING uuid, code, name, city, is_active, show_online;

-- name: SetProductShowOnline :one
UPDATE rental_products
SET show_online = COALESCE(sqlc.narg(show_online)::boolean, show_online)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING uuid, code, title, status, is_active, show_online;

-- name: GetSiteCatalogExtra :one
SELECT e.uuid, e.code, e.name, e.status, e.extra_type, e.channels::varchar[] AS channels
FROM extras e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid)
  AND e.deleted_at IS NULL
  AND e.extra_type <> 'delivery';

-- name: SetExtraChannels :one
-- channels NULL = every channel.
UPDATE extras
SET channels = sqlc.narg(channels)::varchar[]
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING uuid, code, name, status, extra_type, channels::varchar[] AS channels;
