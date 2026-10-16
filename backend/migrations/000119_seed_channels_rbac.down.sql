-- Data-loss note: removes the channel manager permissions from every role,
-- custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.channels.read', 'tenant.channels.write', 'tenant.channels.sync')
);

DELETE FROM permissions WHERE slug IN ('tenant.channels.read', 'tenant.channels.write', 'tenant.channels.sync');
