-- Rentals, operation tasks and contracts permissions (TEC-709 / TEC-710).
--
-- tenant.rentals.read   rentals list / detail, operation tasks list
-- tenant.rentals.write  create from a reservation, handover, return,
--                       extension, vehicle swap, charge proposals, work
--                       operation tasks, record document approvals
-- tenant.rentals.close  finish the return check, approve / waive charges,
--                       cancel a rental before the handover
-- tenant.contracts.read      templates and documents
-- tenant.contracts.write     edit templates, prepare / refresh documents
-- tenant.contracts.activate  activate a template version
-- Existing sensitive slugs are reused: tenant.document.download (PDF),
-- tenant.signature.waive (handover without an approved contract),
-- tenant.damage.approve (damage charges).
INSERT INTO permissions (name, slug) VALUES
    ('Read rentals and operation tasks', 'tenant.rentals.read'),
    ('Hand over, return and change rentals', 'tenant.rentals.write'),
    ('Finish returns and decide rental charges', 'tenant.rentals.close'),
    ('Read contract templates and documents', 'tenant.contracts.read'),
    ('Edit contract templates and prepare documents', 'tenant.contracts.write'),
    ('Activate contract templates', 'tenant.contracts.activate')
ON CONFLICT (slug) DO NOTHING;

-- Owner / admin, head office and branch managers: everything.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.rentals.read', 'tenant.rentals.write', 'tenant.rentals.close',
    'tenant.contracts.read', 'tenant.contracts.write', 'tenant.contracts.activate'
)
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'sube_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff: work rentals, read contracts.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.rentals.read', 'tenant.rentals.write', 'tenant.contracts.read')
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales desk and accounting: read rentals and contracts.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.rentals.read', 'tenant.contracts.read')
WHERE r.slug IN ('rezervasyon_satis', 'muhasebe') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Generic member: read rentals.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.rentals.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
