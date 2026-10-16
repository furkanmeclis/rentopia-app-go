-- Collections permission (TEC-718, docs/design/collections.md):
--   tenant.finance.collections.bulk.allocate  bulk "allocate the credit to
--                                              the oldest open items"
-- Everything else reuses existing slugs: the list / detail need
-- tenant.finance.accounts.read, recording a collection
-- tenant.payment.collect, allocating tenant.finance.accounts.write, a
-- refund tenant.payment.refund, a void tenant.finance.ledger.adjust and
-- the export tenant.finance.accounts.export.
INSERT INTO permissions (name, slug) VALUES
    ('Bulk allocate collections', 'tenant.finance.collections.bulk.allocate')
ON CONFLICT (slug) DO NOTHING;

-- The roles that may allocate (accounts.write): owner / admin, head office,
-- accounting and branch managers.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.finance.collections.bulk.allocate'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'muhasebe', 'sube_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
