-- Data-loss note: removes the media library metadata, retaining stored objects.
DROP TRIGGER trg_site_settings_media ON site_settings;
DROP TRIGGER trg_site_versions_media ON site_setting_versions;
DROP FUNCTION site_config_media_guard();
DROP TABLE site_media;
DROP FUNCTION site_media_guard();
DROP FUNCTION site_config_media_ids(jsonb);
