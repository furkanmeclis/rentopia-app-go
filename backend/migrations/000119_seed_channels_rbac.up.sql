-- Channel manager permissions (TEC-736, docs/design/channel-manager.md §7):
--   tenant.channels.read   the channel screen: accounts with their sync
--                          state, mappings, the message / error queue, the
--                          sync log and the reconciliation report
--   tenant.channels.write  mappings (create, edit, stop-sale, quota,
--                          delete) and the sync settings of an account
--   tenant.channels.sync   push / pull now, retry or dismiss a message,
--                          simulate a booking on a development adapter
-- The channel account itself (enable, adapter, credentials) is an
-- integration account: tenant.integrations.manage.
INSERT INTO permissions (name, slug) VALUES
    ('Read channel manager', 'tenant.channels.read'),
    ('Manage channel mappings and settings', 'tenant.channels.write'),
    ('Run channel sync and resolve messages', 'tenant.channels.sync')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin and head office: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.channels.read', 'tenant.channels.write', 'tenant.channels.sync')
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Reservation sales: read, run the sync and resolve the queue.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.channels.read', 'tenant.channels.sync')
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Branch managers and accounting (reconciliation): read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.channels.read'
WHERE r.slug IN ('sube_yoneticisi', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
