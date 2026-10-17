-- Branded site permissions (TEC-729, docs/design/branded-site.md):
--   tenant.site.read     the site settings (theme, content, address)
--   tenant.site.manage   switch the site on / off, change its address,
--                        theme and content
-- The site sells on the organization's behalf (it holds a publishable key),
-- so managing it is limited to owner / admin and head office, like the
-- public API keys.
INSERT INTO permissions (name, slug) VALUES
    ('Read branded site settings', 'tenant.site.read'),
    ('Manage the branded site', 'tenant.site.manage')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.site.read', 'tenant.site.manage')
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
