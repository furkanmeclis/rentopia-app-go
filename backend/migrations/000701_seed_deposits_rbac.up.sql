-- Deposit permissions (TEC-719, docs/design/deposits.md §7):
--   tenant.finance.deposits.read     deposits list, rental deposit card
--   tenant.finance.deposits.write    take a deposit, start / verify an
--                                    online pre-auth, release in full,
--                                    cancel a pending one
--   tenant.finance.deposits.deduct   capture deductions (with a reason)
--   tenant.finance.deposits.export   export the deposits list
INSERT INTO permissions (name, slug) VALUES
    ('Read security deposits', 'tenant.finance.deposits.read'),
    ('Take and release security deposits', 'tenant.finance.deposits.write'),
    ('Deduct from security deposits', 'tenant.finance.deposits.deduct'),
    ('Export security deposits', 'tenant.finance.deposits.export')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and accounting: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.finance.deposits.%'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: their branches' deposits, deductions included.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.finance.deposits.read', 'tenant.finance.deposits.write', 'tenant.finance.deposits.deduct',
    'tenant.finance.deposits.export'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff take the deposit at the handover and release it
-- after a clean return; deductions stay with the managers.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.finance.deposits.read', 'tenant.finance.deposits.write')
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales: read (the deposit due at the counter).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.finance.deposits.read'
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
