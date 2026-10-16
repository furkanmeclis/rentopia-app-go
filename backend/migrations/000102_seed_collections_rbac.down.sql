-- Data-loss note: removes tenant.finance.collections.bulk.allocate from
-- every role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.finance.collections.bulk.allocate');

DELETE FROM permissions WHERE slug = 'tenant.finance.collections.bulk.allocate';
