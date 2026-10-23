-- Data-loss note: drops every organization's plan, period and feature
-- overrides (organizations then have no entitlement data).
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('platform.organizations.plan.read', 'platform.organizations.plan.write')
);
DELETE FROM permissions WHERE slug IN ('platform.organizations.plan.read', 'platform.organizations.plan.write');
DROP TABLE organization_features;
DROP TABLE organization_plan;
DROP TABLE plan_features;
DROP TABLE plans;
DROP TABLE features;
