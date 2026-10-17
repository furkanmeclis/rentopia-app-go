CREATE TABLE site_domains (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    host TEXT NOT NULL UNIQUE CHECK (host = lower(host) AND host !~ '[^a-z0-9.-]' AND host NOT LIKE '%.'),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','failed','disabled')),
    verification_token TEXT NOT NULL,
    last_checked_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    attention_since TIMESTAMPTZ,
    tls_status TEXT NOT NULL DEFAULT 'pending' CHECK (tls_status IN ('pending','provisioning','active','failed','manual')),
    provisioner TEXT NOT NULL CHECK (provisioner IN ('manual','dokploy','traefik_file')),
    provisioner_id TEXT NOT NULL DEFAULT '',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until TIMESTAMPTZ,
    origins_dirty BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_site_domains_org ON site_domains(organization_id);
CREATE INDEX idx_site_domains_due ON site_domains(next_check_at);
CREATE TRIGGER trg_site_domains_updated_at BEFORE UPDATE ON site_domains FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- Serialize inserts on the parent, including concurrent requests to different API processes.
CREATE FUNCTION site_domains_limit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM id FROM organizations WHERE id = NEW.organization_id FOR UPDATE;
    IF (SELECT count(*) FROM site_domains WHERE organization_id = NEW.organization_id) >= 3 THEN
        RAISE EXCEPTION 'maximum three site domains' USING ERRCODE = '23514', CONSTRAINT = 'site_domains_limit';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_site_domains_limit BEFORE INSERT ON site_domains FOR EACH ROW EXECUTE FUNCTION site_domains_limit();
INSERT INTO permissions(name, slug) VALUES
 ('Read custom site domains', 'platform.site_domains.read'),
 ('Manage custom site domains', 'platform.site_domains.manage') ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
 AND p.slug IN ('platform.site_domains.read','platform.site_domains.manage') ON CONFLICT DO NOTHING;
