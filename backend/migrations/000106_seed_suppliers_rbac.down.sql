-- Data-loss note: removes the supplier permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug LIKE 'tenant.suppliers.%');

DELETE FROM permissions WHERE slug LIKE 'tenant.suppliers.%';
