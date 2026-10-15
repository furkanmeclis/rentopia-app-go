-- Integrations, payments and approval permissions (TEC-676 / TEC-707 /
-- TEC-708). Collecting at the branch and refunding reuse the slugs seeded
-- by 000041 (tenant.payment.collect, tenant.payment.refund, TEC-689); only
-- the new capabilities get new slugs:
--   tenant.integrations.manage     connect / test / disable provider accounts
--   tenant.payments.read           payments, refunds and payment links
--   tenant.payments.create_link    create / cancel pay-by-link links, verify
--   tenant.reservations.approve    staff approval queue (accept / reject)
INSERT INTO permissions (name, slug) VALUES
    ('Manage integrations', 'tenant.integrations.manage'),
    ('Read payments', 'tenant.payments.read'),
    ('Create payment links', 'tenant.payments.create_link'),
    ('Approve reservation requests', 'tenant.reservations.approve')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.integrations.manage', 'tenant.payments.read', 'tenant.payments.create_link', 'tenant.reservations.approve'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers and sales: payments, links, approvals (no provider
-- accounts: those are organization-wide credentials).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.payments.read', 'tenant.payments.create_link', 'tenant.reservations.approve')
WHERE r.slug IN ('sube_yoneticisi', 'rezervasyon_satis') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Accounting (refunds through tenant.payment.refund) and delivery / return
-- (collection through tenant.payment.collect): read payments.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.payments.read'
WHERE r.slug IN ('muhasebe', 'teslim_iade') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
