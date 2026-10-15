-- Data-loss note: removes the extras and campaigns permissions from every
-- role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.extras.read', 'tenant.extras.write', 'tenant.campaigns.read', 'tenant.campaigns.write'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.extras.read', 'tenant.extras.write', 'tenant.campaigns.read', 'tenant.campaigns.write'
);
