-- Data-loss note: removes the corporate agreement permissions from every
-- role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug LIKE 'tenant.agreements.%');

DELETE FROM permissions WHERE slug LIKE 'tenant.agreements.%';
