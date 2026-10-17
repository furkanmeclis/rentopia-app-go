-- Data-loss note: drops the publish history of every site. The draft
-- (site_settings.config and images) becomes live again, as before TEC-730:
-- unpublished draft changes go live on rollback.
DROP TABLE IF EXISTS site_setting_versions;

ALTER TABLE site_settings
    DROP CONSTRAINT IF EXISTS uq_site_settings_organization_id,
    DROP COLUMN IF EXISTS published_revision;
