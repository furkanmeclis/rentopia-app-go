INSERT INTO permissions (name, slug) VALUES
    ('Read vehicle catalog', 'platform.catalog.read'),
    ('Write vehicle catalog', 'platform.catalog.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('platform.catalog.read', 'platform.catalog.write')
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;
