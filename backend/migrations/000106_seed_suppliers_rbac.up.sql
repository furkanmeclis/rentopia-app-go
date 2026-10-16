-- Supplier permissions (TEC-721, docs/design/suppliers.md):
--   tenant.suppliers.read|write|export|import   supplier cards
--   tenant.suppliers.bulk.archive               bulk archive / activate
-- Supplier invoices reuse tenant.finance.expenses.* (an invoice is an
-- expense document), supplier payments tenant.finance.cash.* (money out of
-- a till / bank account) and the supplier statement
-- tenant.finance.accounts.read.
INSERT INTO permissions (name, slug) VALUES
    ('Read suppliers', 'tenant.suppliers.read'),
    ('Write suppliers', 'tenant.suppliers.write'),
    ('Export suppliers', 'tenant.suppliers.export'),
    ('Import suppliers', 'tenant.suppliers.import'),
    ('Bulk archive suppliers', 'tenant.suppliers.bulk.archive')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and accounting: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.suppliers.%'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: read, write and export (no imports, no bulk archive).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.suppliers.read', 'tenant.suppliers.write', 'tenant.suppliers.export')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Members: read (who supplied a vehicle).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.suppliers.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
