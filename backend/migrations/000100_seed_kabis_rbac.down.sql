-- Data-loss note: removes the KABİS permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.kabis.read', 'tenant.kabis.retry', 'tenant.kabis.reconcile')
);

DELETE FROM permissions WHERE slug IN ('tenant.kabis.read', 'tenant.kabis.retry', 'tenant.kabis.reconcile');
