-- Data-loss note: removes the customers permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.customers.read',
        'tenant.customers.write',
        'tenant.customers.reveal',
        'tenant.customers.export',
        'tenant.customers.import',
        'tenant.customers.anonymize'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.customers.read',
    'tenant.customers.write',
    'tenant.customers.reveal',
    'tenant.customers.export',
    'tenant.customers.import',
    'tenant.customers.anonymize'
);
