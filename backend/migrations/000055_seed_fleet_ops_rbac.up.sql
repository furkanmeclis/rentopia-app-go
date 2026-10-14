-- Fleet operations permissions (TEC-694 / TEC-695): vehicle blocks,
-- transfers and one-way rules. Separate from tenant.vehicles.* so delivery
-- staff can block / move vehicles without editing them, and sales /
-- accounting can read rules and transfers without seeing the block
-- controls. Exports use the read slug; bulk actions the write slug.
INSERT INTO permissions (name, slug) VALUES
    ('Read vehicle blocks', 'tenant.blocks.read'),
    ('Write vehicle blocks', 'tenant.blocks.write'),
    ('Read vehicle transfers', 'tenant.transfers.read'),
    ('Write vehicle transfers', 'tenant.transfers.write'),
    ('Read one-way rules', 'tenant.one_way.read'),
    ('Write one-way rules', 'tenant.one_way.write')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and branch managers: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.blocks.read',
    'tenant.blocks.write',
    'tenant.transfers.read',
    'tenant.transfers.write',
    'tenant.one_way.read',
    'tenant.one_way.write'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'sube_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff: block and transfer vehicles.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.blocks.read',
    'tenant.blocks.write',
    'tenant.transfers.read',
    'tenant.transfers.write'
)
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales and the generic member role: read everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.blocks.read', 'tenant.transfers.read', 'tenant.one_way.read')
WHERE r.slug IN ('rezervasyon_satis', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Accounting: transfer costs and one-way fees.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.transfers.read', 'tenant.one_way.read')
WHERE r.slug = 'muhasebe' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
