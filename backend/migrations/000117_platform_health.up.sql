-- Platform usage and connection health (TEC-739, §2.2 / §15.1).
--
-- The dashboard reads usage (active vehicles, monthly reservations,
-- members, tracked file storage) and connection health (integration
-- accounts, integration jobs, and any health source a module registers
-- later) live from the existing tables; nothing is copied.
--
-- platform_health_alert_cursors remembers, per health source, the newest
-- critical issue the platform administrators were already alerted about,
-- so the periodic alert sweep reports each failure once. Platform-level
-- state: no organization column.

CREATE TABLE platform_health_alert_cursors (
    source VARCHAR(48) PRIMARY KEY,
    -- Newest issue time already alerted (issues at or before it are old).
    last_seen_at TIMESTAMPTZ NOT NULL,
    last_alert_at TIMESTAMPTZ NULL,
    last_alert_count INTEGER NOT NULL DEFAULT 0 CHECK (last_alert_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_platform_health_alert_cursors_updated_at
    BEFORE UPDATE ON platform_health_alert_cursors
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Problem jobs across every organization, newest first (the health source
-- and the alert sweep).
CREATE INDEX idx_integration_jobs_problem_updated
    ON integration_jobs (status, updated_at DESC)
    WHERE status IN ('failed', 'needs_verification', 'waiting_dependency', 'blocked');

-- platform.health.read: the usage / health dashboard and the failed job
-- alerts (holders are the alert recipients).
INSERT INTO permissions (name, slug) VALUES
    ('Read platform usage and health', 'platform.health.read')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'platform.health.read'
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;
