-- name: ListSiteMedia :many
SELECT * FROM site_media WHERE organization_id = sqlc.arg(organization_id)
ORDER BY id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountSiteMedia :one
SELECT count(*) FROM site_media WHERE organization_id = sqlc.arg(organization_id);

-- name: GetSiteMedia :one
SELECT * FROM site_media WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: InsertSiteMedia :one
INSERT INTO site_media (organization_id, uuid, mime, size, width, height, alt, object_key, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(uuid), sqlc.arg(mime), sqlc.arg(size), sqlc.arg(width), sqlc.arg(height), sqlc.arg(alt), sqlc.arg(object_key), sqlc.narg(created_by)) RETURNING *;

-- name: PatchSiteMedia :one
UPDATE site_media SET alt = sqlc.arg(alt)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) RETURNING *;

-- name: DeleteSiteMedia :one
DELETE FROM site_media WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) RETURNING object_key;

-- name: GetPublicSiteMedia :one
-- Global public UUID lookup: the site, organization and published/draft
-- reference jointly authorize the bytes. The service validates the token.
SELECT m.*, s.config AS draft_config, v.config AS published_config, s.enabled, s.published_revision,
 o.status AS organization_status, o.access_starts_at, o.access_ends_at
FROM site_settings s
JOIN organizations o ON o.id = s.organization_id AND o.deleted_at IS NULL
JOIN site_media m ON m.organization_id = s.organization_id AND m.uuid = sqlc.arg(media_uuid)
LEFT JOIN site_setting_versions v ON v.organization_id = s.organization_id AND v.revision = s.published_revision
WHERE s.uuid = sqlc.arg(site_uuid);
