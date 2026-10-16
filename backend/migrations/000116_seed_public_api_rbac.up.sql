-- Public booking API permissions (TEC-728, docs/design/public-api.md):
--   tenant.public_api.read     API keys (masked), webhook endpoints and deliveries
--   tenant.public_api.manage   create / rotate / revoke keys, manage webhooks
-- The keys sell on the organization's behalf, so managing them is limited
-- to owner / admin and head office.
INSERT INTO permissions (name, slug) VALUES
    ('Read public API settings', 'tenant.public_api.read'),
    ('Manage public API keys and webhooks', 'tenant.public_api.manage')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.public_api.%'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
