-- Data-loss note: removes the integrations, payments and approval
-- permissions from every role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.integrations.manage', 'tenant.payments.read', 'tenant.payments.create_link', 'tenant.reservations.approve'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.integrations.manage', 'tenant.payments.read', 'tenant.payments.create_link', 'tenant.reservations.approve'
);
