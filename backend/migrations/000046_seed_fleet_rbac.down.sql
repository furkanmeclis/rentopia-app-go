DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.fleet.read', 'tenant.fleet.write')
);

DELETE FROM permissions WHERE slug IN ('tenant.fleet.read', 'tenant.fleet.write');
