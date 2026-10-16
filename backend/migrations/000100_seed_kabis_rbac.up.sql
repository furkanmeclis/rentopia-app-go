-- KABİS notification permissions (TEC-726, docs/design/kabis.md §7):
--   tenant.kabis.read       the KABİS screen, a notification with its
--                           attempt history, the rental chip, attachments
--   tenant.kabis.retry      retry a notification (status check first; a
--                           blocked one re-validates its data, a waiting
--                           one looks for a working adapter)
--   tenant.kabis.reconcile  mark a notification "reported manually" or
--                           "reconciled" (reason, reference, attachment)
-- The KABİS account itself (enable, adapter, credentials) is an
-- integration account: tenant.integrations.manage.
INSERT INTO permissions (name, slug) VALUES
    ('Read KABİS notifications', 'tenant.kabis.read'),
    ('Retry KABİS notifications', 'tenant.kabis.retry'),
    ('Reconcile KABİS notifications', 'tenant.kabis.reconcile')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.kabis.read', 'tenant.kabis.retry', 'tenant.kabis.reconcile')
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers: read and retry the notifications of their branches.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.kabis.read', 'tenant.kabis.retry')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff and members: read (the rental chip).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.kabis.read'
WHERE r.slug IN ('teslim_iade', 'rezervasyon_satis', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
