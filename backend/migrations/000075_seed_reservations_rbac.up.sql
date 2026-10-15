-- Reservations, quotes and resolution tasks permissions (TEC-704 / TEC-706 /
-- TEC-698). Cancel, confirm, export and the driver eligibility override are
-- separate slugs so roles get them one by one. Manual discounts keep using
-- tenant.discount.apply, price overrides tenant.price.override.
INSERT INTO permissions (name, slug) VALUES
    ('Read reservations', 'tenant.reservations.read'),
    ('Create and edit reservations', 'tenant.reservations.write'),
    ('Cancel reservations', 'tenant.reservations.cancel'),
    ('Confirm reservations', 'tenant.reservations.confirm'),
    ('Export reservations', 'tenant.reservations.export'),
    ('Override driver eligibility on reservations', 'tenant.reservations.override_eligibility'),
    ('Read quotes', 'tenant.quotes.read'),
    ('Create quotes', 'tenant.quotes.write'),
    ('Read resolution tasks', 'tenant.resolution_tasks.read'),
    ('Resolve and assign resolution tasks', 'tenant.resolution_tasks.write')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and branch managers: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.reservations.read',
    'tenant.reservations.write',
    'tenant.reservations.cancel',
    'tenant.reservations.confirm',
    'tenant.reservations.export',
    'tenant.reservations.override_eligibility',
    'tenant.quotes.read',
    'tenant.quotes.write',
    'tenant.resolution_tasks.read',
    'tenant.resolution_tasks.write'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'sube_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales: reservations (no eligibility override), quotes, tasks read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.reservations.read',
    'tenant.reservations.write',
    'tenant.reservations.cancel',
    'tenant.reservations.confirm',
    'tenant.reservations.export',
    'tenant.quotes.read',
    'tenant.quotes.write',
    'tenant.resolution_tasks.read'
)
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return: read reservations, work the resolution tasks.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.reservations.read', 'tenant.resolution_tasks.read', 'tenant.resolution_tasks.write'
)
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Accounting: read / export reservations, read quotes.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.reservations.read', 'tenant.reservations.export', 'tenant.quotes.read')
WHERE r.slug = 'muhasebe' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Generic member: read reservations.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.reservations.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
