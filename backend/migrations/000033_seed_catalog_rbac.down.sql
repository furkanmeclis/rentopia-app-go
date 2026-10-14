DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('platform.catalog.read', 'platform.catalog.write')
);

DELETE FROM permissions
WHERE slug IN ('platform.catalog.read', 'platform.catalog.write');
