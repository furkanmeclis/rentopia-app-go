-- Pricing permissions (TEC-700 / TEC-701): rate plans, date rules and the
-- price preview. Activation is its own slug so staff who may prepare
-- drafts (or only read) can never put a price live. Exports use the read
-- slug, the price list import the write slug (imports only write drafts).
INSERT INTO permissions (name, slug) VALUES
    ('Read pricing', 'tenant.pricing.read'),
    ('Write pricing drafts', 'tenant.pricing.write'),
    ('Activate pricing', 'tenant.pricing.activate')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.pricing.read', 'tenant.pricing.write', 'tenant.pricing.activate')
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers, sales, accounting and the generic member: read only.
-- Delivery / return staff (teslim_iade): nothing.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.pricing.read'
WHERE r.slug IN ('sube_yoneticisi', 'rezervasyon_satis', 'muhasebe', 'member')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
