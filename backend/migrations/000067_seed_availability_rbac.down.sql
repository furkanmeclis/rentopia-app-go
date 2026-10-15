-- Data-loss note: removes the availability / capacity rule permissions from
-- every role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.availability.read',
        'tenant.availability.hold',
        'tenant.capacity_rules.read',
        'tenant.capacity_rules.write'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.availability.read',
    'tenant.availability.hold',
    'tenant.capacity_rules.read',
    'tenant.capacity_rules.write'
);
