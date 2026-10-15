-- Data-loss note: removes the pricing permissions from every role, custom
-- organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.pricing.read', 'tenant.pricing.write', 'tenant.pricing.activate'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.pricing.read', 'tenant.pricing.write', 'tenant.pricing.activate'
);
