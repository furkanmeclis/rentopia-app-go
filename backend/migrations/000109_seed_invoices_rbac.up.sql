-- Invoice permissions (TEC-722, docs/design/einvoice.md §8):
--   tenant.invoices.read                    invoices list, detail, preview,
--                                           XML / PDF downloads, attempts
--   tenant.invoices.write                   create / edit drafts ("Fatura
--                                           kes"), validate, finalize (send),
--                                           retry a send
--   tenant.invoices.cancel                  cancel (draft, unsent, failed,
--                                           e-Arşiv) and return (IADE)
--   tenant.invoices.export                  list export
--   tenant.invoices.settings                series, supplier address, tax
--                                           mapping (line kind → VAT)
--   tenant.invoices.bulk.create_for_rentals bulk "invoice the selected
--                                           rentals"
--   tenant.invoices.bulk.send               bulk finalize + send of drafts
-- The integrator account (enable, adapter, credentials) is an integration
-- account: tenant.integrations.manage.
INSERT INTO permissions (name, slug) VALUES
    ('Read invoices', 'tenant.invoices.read'),
    ('Write invoices', 'tenant.invoices.write'),
    ('Cancel and return invoices', 'tenant.invoices.cancel'),
    ('Export invoices', 'tenant.invoices.export'),
    ('Manage invoice settings', 'tenant.invoices.settings'),
    ('Bulk invoice rentals', 'tenant.invoices.bulk.create_for_rentals'),
    ('Bulk send invoices', 'tenant.invoices.bulk.send')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and accounting: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.invoices.read', 'tenant.invoices.write', 'tenant.invoices.cancel', 'tenant.invoices.export',
    'tenant.invoices.settings', 'tenant.invoices.bulk.create_for_rentals', 'tenant.invoices.bulk.send'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: invoice their branches' rentals (no cancel / return,
-- no settings).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.invoices.read', 'tenant.invoices.write', 'tenant.invoices.export',
    'tenant.invoices.bulk.create_for_rentals', 'tenant.invoices.bulk.send'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales and members: read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.invoices.read'
WHERE r.slug IN ('rezervasyon_satis', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
