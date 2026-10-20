-- Restores the boilerplate defaults. Values still holding the Rentopya
-- defaults go back; edited values are left alone.
UPDATE notification_templates SET body = 'Hello {{name}}, your App account was created successfully.'
WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'en'
  AND body = 'Hello {{name}}, your Rentopya account was created successfully.';
UPDATE notification_templates SET subject = 'Welcome to App'
WHERE code = 'auth.welcome' AND language = 'en' AND subject = 'Welcome to Rentopya';
UPDATE notification_templates SET body = 'Merhaba {{name}}, App hesabınız başarıyla oluşturuldu.'
WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'tr'
  AND body = 'Merhaba {{name}}, Rentopya hesabınız başarıyla oluşturuldu.';
UPDATE notification_templates SET subject = 'App''e hoş geldiniz'
WHERE code = 'auth.welcome' AND language = 'tr' AND subject = 'Rentopya''ya hoş geldiniz';

UPDATE app_settings SET tagline = 'Platform & CMS console' WHERE id = 1 AND tagline = 'Araç kiralama yönetim platformu';
UPDATE app_settings SET company_name = 'App' WHERE id = 1 AND company_name = 'Rentopya';
UPDATE app_settings SET primary_color = '#0F172A' WHERE id = 1 AND primary_color = '#2E358C';

ALTER TABLE app_settings
    ALTER COLUMN primary_color SET DEFAULT '#0F172A',
    ALTER COLUMN company_name  SET DEFAULT 'App',
    ALTER COLUMN tagline       SET DEFAULT 'Platform & CMS console';
