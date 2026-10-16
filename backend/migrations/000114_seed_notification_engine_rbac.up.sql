-- Notification engine permissions (TEC-733):
--   tenant.notification_settings.read   rules, templates, delivery history
--   tenant.notification_settings.write  edit rules / templates, test sends
INSERT INTO permissions (name, slug) VALUES
    ('Read notification settings', 'tenant.notification_settings.read'),
    ('Write notification settings', 'tenant.notification_settings.write')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.notification_settings.%'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: read (delivery history of their customers).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.notification_settings.read'
WHERE r.slug IN ('sube_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
