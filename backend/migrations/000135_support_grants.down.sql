-- Data-loss note: drops every support access grant and removes the
-- permission from every role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.support_access.manage');

DELETE FROM permissions WHERE slug = 'tenant.support_access.manage';

DROP TABLE IF EXISTS support_grants;
