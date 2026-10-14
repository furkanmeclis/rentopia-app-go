-- Data-loss note: removes the vehicle permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.vehicles.read',
        'tenant.vehicles.write',
        'tenant.vehicles.export',
        'tenant.vehicles.import',
        'tenant.vehicles.bulk'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.vehicles.read',
    'tenant.vehicles.write',
    'tenant.vehicles.export',
    'tenant.vehicles.import',
    'tenant.vehicles.bulk'
);
