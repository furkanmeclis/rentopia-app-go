-- Availability engine permissions (TEC-697): capacity search, holds
-- ("opsiyon") and channel capacity rules.
INSERT INTO permissions (name, slug) VALUES
    ('Search availability', 'tenant.availability.read'),
    ('Create / extend / release holds', 'tenant.availability.hold'),
    ('Read channel capacity rules', 'tenant.capacity_rules.read'),
    ('Write channel capacity rules', 'tenant.capacity_rules.write')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and branch managers: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.availability.read',
    'tenant.availability.hold',
    'tenant.capacity_rules.read',
    'tenant.capacity_rules.write'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'sube_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales: search and put options (and read the channel rules they sell under).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.availability.read', 'tenant.availability.hold', 'tenant.capacity_rules.read')
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Accounting, delivery / return staff and the generic member: read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.availability.read', 'tenant.capacity_rules.read')
WHERE r.slug IN ('muhasebe', 'teslim_iade', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
