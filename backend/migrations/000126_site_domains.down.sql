-- Data-loss note: removes all custom domain verification and provisioning records.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('platform.site_domains.read','platform.site_domains.manage'));
DELETE FROM permissions WHERE slug IN ('platform.site_domains.read','platform.site_domains.manage');
DROP TABLE site_domains;
DROP FUNCTION site_domains_limit();
