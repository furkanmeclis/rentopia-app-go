DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE slug = 'teslim_iade' AND scope = 'tenant' AND organization_id IS NULL)
  AND permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.one_way.read');
