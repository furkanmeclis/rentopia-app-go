-- Customers and drivers permissions (TEC-705). Reveal, export, import and
-- anonymize are separate slugs so roles get them one by one: reading a
-- customer shows masked identity / contact values only.
INSERT INTO permissions (name, slug) VALUES
    ('Read customers and drivers', 'tenant.customers.read'),
    ('Write customers and drivers', 'tenant.customers.write'),
    ('Reveal customer identity and contact data', 'tenant.customers.reveal'),
    ('Export customers', 'tenant.customers.export'),
    ('Import customers', 'tenant.customers.import'),
    ('Anonymize customers (data subject requests)', 'tenant.customers.anonymize')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.customers.read',
    'tenant.customers.write',
    'tenant.customers.reveal',
    'tenant.customers.export',
    'tenant.customers.import',
    'tenant.customers.anonymize'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: read / write / reveal / export.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.customers.read', 'tenant.customers.write', 'tenant.customers.reveal', 'tenant.customers.export'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales: read / write / reveal.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.customers.read', 'tenant.customers.write', 'tenant.customers.reveal')
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return: read / reveal (identity and licence check at handover).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.customers.read', 'tenant.customers.reveal')
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Accounting: read / export.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.customers.read', 'tenant.customers.export')
WHERE r.slug = 'muhasebe' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Generic member: read (masked values only).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.customers.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
