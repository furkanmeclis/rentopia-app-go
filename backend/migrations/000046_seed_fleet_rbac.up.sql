-- Fleet permissions (segments, rental groups, rental products). Granted to
-- the owner / admin template roles; members read.
INSERT INTO permissions (name, slug) VALUES
    ('Read fleet', 'tenant.fleet.read'),
    ('Write fleet', 'tenant.fleet.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.fleet.read', 'tenant.fleet.write')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.fleet.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Rentopia template roles (000041): head office manages the fleet
-- definitions; every other role reads them.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.fleet.read', 'tenant.fleet.write')
WHERE r.slug = 'merkez_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.fleet.read'
WHERE r.slug IN ('sube_yoneticisi', 'rezervasyon_satis', 'muhasebe', 'teslim_iade')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
