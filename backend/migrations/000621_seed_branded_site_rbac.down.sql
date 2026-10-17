-- Data-loss note: removes the branded site permissions from every role,
-- custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.site.read', 'tenant.site.manage'));

DELETE FROM permissions WHERE slug IN ('tenant.site.read', 'tenant.site.manage');
