-- Data-loss note: removes the invoice permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.invoices.read', 'tenant.invoices.write', 'tenant.invoices.cancel', 'tenant.invoices.export',
        'tenant.invoices.settings', 'tenant.invoices.bulk.create_for_rentals', 'tenant.invoices.bulk.send'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.invoices.read', 'tenant.invoices.write', 'tenant.invoices.cancel', 'tenant.invoices.export',
    'tenant.invoices.settings', 'tenant.invoices.bulk.create_for_rentals', 'tenant.invoices.bulk.send'
);
