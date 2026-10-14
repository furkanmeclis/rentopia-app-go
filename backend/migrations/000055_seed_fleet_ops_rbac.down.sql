-- Data-loss note: removes the block / transfer / one-way permissions from
-- every role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.blocks.read',
        'tenant.blocks.write',
        'tenant.transfers.read',
        'tenant.transfers.write',
        'tenant.one_way.read',
        'tenant.one_way.write'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.blocks.read',
    'tenant.blocks.write',
    'tenant.transfers.read',
    'tenant.transfers.write',
    'tenant.one_way.read',
    'tenant.one_way.write'
);
