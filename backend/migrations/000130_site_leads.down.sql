-- Data-loss note: collected leads and handling notes are removed.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.site_leads.read','tenant.site_leads.manage'));
DELETE FROM permissions WHERE slug IN ('tenant.site_leads.read','tenant.site_leads.manage');
DROP TABLE site_leads;
