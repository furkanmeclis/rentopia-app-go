-- Finance permissions (TEC-717 / TEC-720, docs/design/ledger.md §7):
--   tenant.finance.accounts.read|write|export   customer / supplier accounts, statements
--   tenant.finance.ledger.adjust                 reversals, adjustments, deposit offsets
--   tenant.finance.cash.read|write|export|import cash / bank accounts, movements, transfers
--   tenant.finance.cash.bulk.set_category        bulk category of movements
--   tenant.finance.expenses.read|write|export|import
--   tenant.finance.expenses.bulk.set_category    bulk category of expenses
--   tenant.finance.reports.read                  aggregate reports
-- Recording a collection / refund on an account also needs the existing
-- tenant.payment.collect / tenant.payment.refund.
INSERT INTO permissions (name, slug) VALUES
    ('Read finance accounts', 'tenant.finance.accounts.read'),
    ('Write finance accounts', 'tenant.finance.accounts.write'),
    ('Export finance accounts', 'tenant.finance.accounts.export'),
    ('Adjust the ledger', 'tenant.finance.ledger.adjust'),
    ('Read cash accounts', 'tenant.finance.cash.read'),
    ('Write cash accounts', 'tenant.finance.cash.write'),
    ('Export cash accounts', 'tenant.finance.cash.export'),
    ('Import cash accounts', 'tenant.finance.cash.import'),
    ('Bulk categorise cash movements', 'tenant.finance.cash.bulk.set_category'),
    ('Read expenses', 'tenant.finance.expenses.read'),
    ('Write expenses', 'tenant.finance.expenses.write'),
    ('Export expenses', 'tenant.finance.expenses.export'),
    ('Import expenses', 'tenant.finance.expenses.import'),
    ('Bulk categorise expenses', 'tenant.finance.expenses.bulk.set_category'),
    ('Read finance reports', 'tenant.finance.reports.read')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and accounting: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.finance.%'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: their branches' accounts, tills and expenses (no ledger
-- corrections, no imports).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.finance.accounts.read', 'tenant.finance.accounts.write', 'tenant.finance.accounts.export',
    'tenant.finance.cash.read', 'tenant.finance.cash.write', 'tenant.finance.cash.export',
    'tenant.finance.cash.bulk.set_category',
    'tenant.finance.expenses.read', 'tenant.finance.expenses.write', 'tenant.finance.expenses.export',
    'tenant.finance.expenses.bulk.set_category', 'tenant.finance.reports.read'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales and delivery / return: read the customer accounts (balances at the
-- counter).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.finance.accounts.read'
WHERE r.slug IN ('rezervasyon_satis', 'teslim_iade') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
