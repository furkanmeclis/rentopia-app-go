-- Vehicle permissions (TEC-692). Every bulk action (status, move branch,
-- rental group, activate / deactivate) is behind tenant.vehicles.bulk.
INSERT INTO permissions (name, slug) VALUES
    ('Read vehicles', 'tenant.vehicles.read'),
    ('Write vehicles', 'tenant.vehicles.write'),
    ('Export vehicles', 'tenant.vehicles.export'),
    ('Import vehicles', 'tenant.vehicles.import'),
    ('Bulk update vehicles', 'tenant.vehicles.bulk')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and the head office manager: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.vehicles.read',
    'tenant.vehicles.write',
    'tenant.vehicles.export',
    'tenant.vehicles.import',
    'tenant.vehicles.bulk'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch manager: read / write / export / bulk (no import).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.vehicles.read',
    'tenant.vehicles.write',
    'tenant.vehicles.export',
    'tenant.vehicles.bulk'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff: read + write (status and km changes).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.vehicles.read', 'tenant.vehicles.write')
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales, accounting and the generic member role: read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.vehicles.read'
WHERE r.slug IN ('rezervasyon_satis', 'muhasebe', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
