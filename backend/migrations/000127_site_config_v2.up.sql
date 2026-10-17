-- Additive normalization preserves all legacy content and presentation fields.
CREATE FUNCTION site_config_v2(c jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_set(
  COALESCE(c, '{}'::jsonb) || jsonb_build_object(
   'config_version', 2,
   'template', COALESCE(c->>'template', 'technowide-design'),
   'templates', jsonb_build_object('technowide-design', jsonb_build_object(
    'sections', '[{"id":"hero","enabled":true},{"id":"search","enabled":true},{"id":"campaigns","enabled":false},{"id":"fleet","enabled":true},{"id":"advantages","enabled":true},{"id":"newsletter","enabled":false},{"id":"guide","enabled":false},{"id":"about","enabled":true},{"id":"faq","enabled":true},{"id":"app_promo","enabled":false}]'::jsonb,
    'settings', '{"heroLayout":"image"}'::jsonb
   )) || COALESCE(c->'templates','{}'::jsonb),
   'theme', COALESCE(c->'theme','{}'::jsonb)
  ), '{theme,radius}', to_jsonb(CASE c#>>'{theme,radius}' WHEN 'none' THEN 'sharp' WHEN 'full' THEN 'pill' ELSE COALESCE(c#>>'{theme,radius}','md') END)
 );
$$;
UPDATE site_settings SET config = site_config_v2(config) WHERE config IS DISTINCT FROM site_config_v2(config);
UPDATE site_setting_versions SET config = site_config_v2(config) WHERE config IS DISTINCT FROM site_config_v2(config);
