-- Data-loss note: removes the rentals and contracts permissions from every
-- role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.rentals.read', 'tenant.rentals.write', 'tenant.rentals.close',
        'tenant.contracts.read', 'tenant.contracts.write', 'tenant.contracts.activate'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.rentals.read', 'tenant.rentals.write', 'tenant.rentals.close',
    'tenant.contracts.read', 'tenant.contracts.write', 'tenant.contracts.activate'
);
