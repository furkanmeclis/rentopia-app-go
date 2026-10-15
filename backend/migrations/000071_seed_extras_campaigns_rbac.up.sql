-- Extras (TEC-702) and campaigns (TEC-703) permissions. Equipment holds
-- (create / confirm / release) reuse tenant.availability.hold: holding a
-- child seat is the same sales action as holding a car. Applying a manual
-- discount at sale time stays tenant.discount.apply; a coupon by code needs
-- no extra permission.
INSERT INTO permissions (name, slug) VALUES
    ('Read extras', 'tenant.extras.read'),
    ('Write extras, prices and stock', 'tenant.extras.write'),
    ('Read campaigns', 'tenant.campaigns.read'),
    ('Write campaigns', 'tenant.campaigns.write')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.extras.read', 'tenant.extras.write', 'tenant.campaigns.read', 'tenant.campaigns.write'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: extras read / write (branch stock) and campaigns read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.extras.read', 'tenant.extras.write', 'tenant.campaigns.read')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales, accounting and the generic member: read both.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.extras.read', 'tenant.campaigns.read')
WHERE r.slug IN ('rezervasyon_satis', 'muhasebe', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff: extras read (what to hand over).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.extras.read'
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
