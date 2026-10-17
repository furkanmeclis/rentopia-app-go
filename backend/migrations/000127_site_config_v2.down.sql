-- Data-loss note: removes template selections; additive editorial content is retained.
UPDATE site_settings SET config = jsonb_set(config - 'config_version' - 'template' - 'templates', '{theme,radius}', to_jsonb(CASE config#>>'{theme,radius}' WHEN 'sharp' THEN 'none' WHEN 'pill' THEN 'full' ELSE config#>>'{theme,radius}' END));
UPDATE site_setting_versions SET config = jsonb_set(config - 'config_version' - 'template' - 'templates', '{theme,radius}', to_jsonb(CASE config#>>'{theme,radius}' WHEN 'sharp' THEN 'none' WHEN 'pill' THEN 'full' ELSE config#>>'{theme,radius}' END));
DROP FUNCTION site_config_v2(jsonb);
