-- Data-loss note: removes the reservations, quotes and resolution task
-- permissions from every role, custom organization roles included.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.reservations.read',
        'tenant.reservations.write',
        'tenant.reservations.cancel',
        'tenant.reservations.confirm',
        'tenant.reservations.export',
        'tenant.reservations.override_eligibility',
        'tenant.quotes.read',
        'tenant.quotes.write',
        'tenant.resolution_tasks.read',
        'tenant.resolution_tasks.write'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.reservations.read',
    'tenant.reservations.write',
    'tenant.reservations.cancel',
    'tenant.reservations.confirm',
    'tenant.reservations.export',
    'tenant.reservations.override_eligibility',
    'tenant.quotes.read',
    'tenant.quotes.write',
    'tenant.resolution_tasks.read',
    'tenant.resolution_tasks.write'
);
