-- Rentopya brand defaults (docs/brand.md): platform letterhead and the
-- welcome notification seeded with the boilerplate name "App". Only values
-- still at the seeded defaults change; anything an admin edited stays.
ALTER TABLE app_settings
    ALTER COLUMN primary_color SET DEFAULT '#2E358C',
    ALTER COLUMN company_name  SET DEFAULT 'Rentopya',
    ALTER COLUMN tagline       SET DEFAULT 'Araç kiralama yönetim platformu';

UPDATE app_settings SET primary_color = '#2E358C' WHERE id = 1 AND primary_color = '#0F172A';
UPDATE app_settings SET company_name = 'Rentopya' WHERE id = 1 AND company_name = 'App';
UPDATE app_settings SET tagline = 'Araç kiralama yönetim platformu' WHERE id = 1 AND tagline = 'Platform & CMS console';

UPDATE notification_templates SET subject = 'Rentopya''ya hoş geldiniz'
WHERE code = 'auth.welcome' AND language = 'tr' AND subject = 'App''e hoş geldiniz';
UPDATE notification_templates SET body = 'Merhaba {{name}}, Rentopya hesabınız başarıyla oluşturuldu.'
WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'tr'
  AND body = 'Merhaba {{name}}, App hesabınız başarıyla oluşturuldu.';
UPDATE notification_templates SET subject = 'Welcome to Rentopya'
WHERE code = 'auth.welcome' AND language = 'en' AND subject = 'Welcome to App';
UPDATE notification_templates SET body = 'Hello {{name}}, your Rentopya account was created successfully.'
WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'en'
  AND body = 'Hello {{name}}, your App account was created successfully.';
