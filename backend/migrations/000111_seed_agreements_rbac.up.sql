-- Corporate agreement permissions (TEC-723, docs/design/corporate-agreements.md):
--   tenant.agreements.read    list, detail, tariffs, rentals, billing plan, statement
--   tenant.agreements.write   create / edit agreements, contacts, permitted drivers, status
--   tenant.agreements.export  agreement list export
-- Agreement tariffs are rate plans (tenant.pricing.write / activate);
-- overriding a tariff in a quote stays tenant.price.override.
INSERT INTO permissions (name, slug) VALUES
    ('Read corporate agreements', 'tenant.agreements.read'),
    ('Write corporate agreements', 'tenant.agreements.write'),
    ('Export corporate agreements', 'tenant.agreements.export')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.agreements.%'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers and accounting: read and export (accounting follows the
-- billing plans and statements).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.agreements.read', 'tenant.agreements.export')
WHERE r.slug IN ('sube_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales, delivery / return and members: read (the reservation wizard offers
-- the agreement of a corporate customer).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.agreements.read'
WHERE r.slug IN ('rezervasyon_satis', 'teslim_iade', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
