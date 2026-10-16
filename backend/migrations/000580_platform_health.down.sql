DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'platform.health.read');
DELETE FROM permissions WHERE slug = 'platform.health.read';

DROP INDEX IF EXISTS idx_integration_jobs_problem_updated;
DROP TABLE IF EXISTS platform_health_alert_cursors;
