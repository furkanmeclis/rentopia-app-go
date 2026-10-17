-- Data-loss note: drops every organization's site settings (theme, content,
-- subdomain). The linked publishable keys stay in public_api_keys; the
-- sites stop resolving.
DROP TABLE IF EXISTS site_settings;
