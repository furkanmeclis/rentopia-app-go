-- Branded site draft / publish (TEC-730, docs/design/branded-site.md §3).
--
-- site_settings now holds two kinds of fields:
--   live     enabled, subdomain, default_locale, api_key_id: take effect on
--            save (switching the site on or off, its address and key).
--   draft    config and the *_object_key images: the working copy the panel
--            edits and previews. Visitors never see it.
-- Publishing copies the draft into site_setting_versions (an immutable
-- snapshot with a per-organization revision number) and points
-- site_settings.published_revision at it. The resolver serves the
-- published snapshot only. Restoring a revision copies its snapshot back
-- into the draft (publish again to make it live).
--
-- Image objects are shared by the draft and any snapshot that names them;
-- the API deletes an object only when no snapshot and no draft refers to it.
ALTER TABLE site_settings
    ADD COLUMN published_revision INTEGER NULL CHECK (published_revision > 0),
    ADD CONSTRAINT uq_site_settings_organization_id UNIQUE (organization_id, id);

CREATE TABLE site_setting_versions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    site_id BIGINT NOT NULL,
    -- 1, 2, 3 … per organization.
    revision INTEGER NOT NULL CHECK (revision > 0),
    config JSONB NOT NULL CHECK (jsonb_typeof(config) = 'object'),
    logo_object_key TEXT NULL,
    favicon_object_key TEXT NULL,
    cover_object_key TEXT NULL,
    -- site_settings.version of the draft that was published.
    draft_version INTEGER NOT NULL CHECK (draft_version > 0),
    published_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_site_setting_versions_revision UNIQUE (organization_id, revision),
    CONSTRAINT uq_site_setting_versions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_site_setting_versions_site
        FOREIGN KEY (organization_id, site_id)
        REFERENCES site_settings (organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_site_setting_versions_uuid ON site_setting_versions (uuid);

-- backfill:begin
-- Sites saved before TEC-730 were live on save: their current document
-- becomes revision 1 and stays published, so nothing changes for visitors.
INSERT INTO site_setting_versions (
    organization_id, site_id, revision, config, logo_object_key, favicon_object_key, cover_object_key,
    draft_version, published_by, published_at
)
SELECT s.organization_id, s.id, 1, s.config, s.logo_object_key, s.favicon_object_key, s.cover_object_key,
    s.version, s.updated_by, s.updated_at
FROM site_settings s
WHERE s.published_revision IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM site_setting_versions v WHERE v.organization_id = s.organization_id
  );

UPDATE site_settings s
SET published_revision = 1
WHERE s.published_revision IS NULL
  AND EXISTS (
      SELECT 1 FROM site_setting_versions v WHERE v.organization_id = s.organization_id AND v.revision = 1
  );
-- backfill:end
