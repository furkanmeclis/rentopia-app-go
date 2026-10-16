-- Data-loss note: removes the notification engine permissions from every
-- role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug LIKE 'tenant.notification_settings.%');

DELETE FROM permissions WHERE slug LIKE 'tenant.notification_settings.%';
