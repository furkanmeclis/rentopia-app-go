-- Data-loss note: removes the deposit permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug LIKE 'tenant.finance.deposits.%');

DELETE FROM permissions WHERE slug LIKE 'tenant.finance.deposits.%';
