-- Toll pass permissions (TEC-725, docs/design/tolls.md §7):
--   tenant.tolls.read     passes, review queue, vehicle tags, settings view
--   tenant.tolls.export   export the pass list
--   tenant.tolls.import   file import and "fetch now" from the provider
--   tenant.tolls.review   assign / unassign / ignore / re-match a pass
-- Vehicle tags are vehicle data (tenant.vehicles.write); the processing
-- fee is an organization setting (tenant.settings.write).
INSERT INTO permissions (name, slug) VALUES
    ('Read toll passes', 'tenant.tolls.read'),
    ('Export toll passes', 'tenant.tolls.export'),
    ('Import toll passes', 'tenant.tolls.import'),
    ('Review toll passes', 'tenant.tolls.review')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and accounting: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.tolls.read', 'tenant.tolls.export', 'tenant.tolls.import', 'tenant.tolls.review')
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: read, export and review the passes of their branches
-- (imports are organization-wide files).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.tolls.read', 'tenant.tolls.export', 'tenant.tolls.review')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales, delivery / return and members: read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.tolls.read'
WHERE r.slug IN ('rezervasyon_satis', 'teslim_iade', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
