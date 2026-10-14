-- Hand-over / return staff see the one-way rules (they explain one-way fees
-- to customers at the counter); they still cannot change them.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.one_way.read'
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
