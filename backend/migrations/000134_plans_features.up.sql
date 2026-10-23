-- Plans and feature entitlements (TEC-674, RT-A28).
--
-- A plan (Yönetim = management, Komple = complete) enables a set of
-- features; an organization has one plan (organization_plan, with an
-- optional contract period) and may carry per-feature overrides
-- (organization_features: enabled true / false beats the plan default).
-- The panel itself (customers, fleet, reservations, rentals, contracts,
-- finance, documents, customer portal links, e-mail notifications) is not a
-- feature: every plan has it. The feature keys must match
-- internal/platform/features (catalog_test.go reads this file).
--
-- Turning a feature off never deletes data: domains, keys, channel
-- accounts and history stay and simply stop being served.

CREATE TABLE features (
    key        TEXT PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)?$'),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO features (key, sort_order) VALUES
    ('site', 10),
    ('widget', 20),
    ('public_api', 30),
    ('channel.yolcu360', 40),
    ('channel.obilet', 50),
    ('kabis', 60),
    ('hgs', 70),
    ('einvoice', 80),
    ('whatsapp', 90),
    ('sms', 100)
ON CONFLICT (key) DO NOTHING;

CREATE TABLE plans (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    code       TEXT NOT NULL UNIQUE CHECK (code ~ '^[a-z][a-z0-9_]*$'),
    name       TEXT NOT NULL CHECK (btrim(name) <> ''),
    name_en    TEXT NOT NULL CHECK (btrim(name_en) <> ''),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_plans_updated_at BEFORE UPDATE ON plans FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE plan_features (
    plan_id     BIGINT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    feature_key TEXT NOT NULL REFERENCES features(key) ON DELETE CASCADE,
    PRIMARY KEY (plan_id, feature_key)
);

INSERT INTO plans (code, name, name_en, sort_order) VALUES
    ('management', 'Yönetim', 'Management', 10),
    ('complete', 'Komple', 'Complete', 20)
ON CONFLICT (code) DO NOTHING;

-- Komple enables every feature; Yönetim none of the external / channel
-- features.
INSERT INTO plan_features (plan_id, feature_key)
SELECT p.id, f.key FROM plans p CROSS JOIN features f WHERE p.code = 'complete'
ON CONFLICT DO NOTHING;

CREATE TABLE organization_plan (
    organization_id  BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    plan_id          BIGINT NOT NULL REFERENCES plans(id),
    period_starts_at TIMESTAMPTZ,
    period_ends_at   TIMESTAMPTZ,
    updated_by       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT organization_plan_period_check
        CHECK (period_starts_at IS NULL OR period_ends_at IS NULL OR period_ends_at > period_starts_at)
);
CREATE INDEX idx_organization_plan_plan ON organization_plan(plan_id);
CREATE TRIGGER trg_organization_plan_updated_at BEFORE UPDATE ON organization_plan FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE organization_features (
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    feature_key     TEXT NOT NULL REFERENCES features(key) ON DELETE CASCADE,
    enabled         BOOLEAN NOT NULL,
    updated_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, feature_key)
);
CREATE TRIGGER trg_organization_features_updated_at BEFORE UPDATE ON organization_features FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Every existing organization keeps everything it has today (the pilot
-- must not lose a feature on deploy): Komple.
INSERT INTO organization_plan (organization_id, plan_id)
SELECT o.id, p.id FROM organizations o CROSS JOIN plans p WHERE p.code = 'complete'
ON CONFLICT (organization_id) DO NOTHING;

INSERT INTO permissions (name, slug) VALUES
    ('Read organization plans and features', 'platform.organizations.plan.read'),
    ('Change organization plans and features', 'platform.organizations.plan.write')
ON CONFLICT (slug) DO NOTHING;

-- super_admin, and every platform role that already reads / writes
-- organizations, gets the matching plan permission.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p
    ON p.slug IN ('platform.organizations.plan.read', 'platform.organizations.plan.write')
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, np.id
FROM role_permissions rp
JOIN permissions op ON op.id = rp.permission_id
JOIN permissions np ON np.slug = CASE op.slug
    WHEN 'platform.organizations.read' THEN 'platform.organizations.plan.read'
    WHEN 'platform.organizations.write' THEN 'platform.organizations.plan.write'
END
WHERE op.slug IN ('platform.organizations.read', 'platform.organizations.write')
ON CONFLICT DO NOTHING;
