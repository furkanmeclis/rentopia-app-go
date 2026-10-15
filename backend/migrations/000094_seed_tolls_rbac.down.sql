-- Data-loss note: removes the toll pass permissions from every role,
-- custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.tolls.read', 'tenant.tolls.export', 'tenant.tolls.import', 'tenant.tolls.review'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.tolls.read', 'tenant.tolls.export', 'tenant.tolls.import', 'tenant.tolls.review'
);
