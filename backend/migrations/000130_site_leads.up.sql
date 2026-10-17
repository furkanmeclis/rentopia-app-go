CREATE TABLE site_leads (
 id bigserial PRIMARY KEY,
 organization_id bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 uuid uuid NOT NULL DEFAULT gen_random_uuid(),
 kind text NOT NULL CHECK (kind IN ('newsletter','contact','corporate','franchise')),
 name text NOT NULL DEFAULT '' CHECK (char_length(name) <= 160),
 email text NOT NULL CHECK (char_length(email) <= 254),
 phone text NOT NULL DEFAULT '' CHECK (char_length(phone) <= 40),
 company text NOT NULL DEFAULT '' CHECK (char_length(company) <= 200),
 city text NOT NULL DEFAULT '' CHECK (char_length(city) <= 100),
 message text NOT NULL DEFAULT '' CHECK (char_length(message) <= 5000),
 fleet_size integer CHECK (fleet_size BETWEEN 0 AND 100000),
 locale text NOT NULL DEFAULT 'tr' CHECK (locale IN ('tr','en')),
 consent_kvkk boolean NOT NULL CHECK (consent_kvkk),
 consent_marketing boolean NOT NULL DEFAULT false,
 source_host text NOT NULL DEFAULT '' CHECK (char_length(source_host) <= 253),
 ip_hash text NOT NULL CHECK (ip_hash ~ '^[0-9a-f]{64}$'),
 user_agent text NOT NULL DEFAULT '' CHECK (char_length(user_agent) <= 300),
 status text NOT NULL DEFAULT 'new' CHECK (status IN ('new','in_progress','done','spam')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 handled_by bigint REFERENCES users(id) ON DELETE SET NULL,
 handled_at timestamptz,
 note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 5000),
 UNIQUE (organization_id, uuid)
);
CREATE INDEX idx_site_leads_list ON site_leads(organization_id, created_at DESC, id DESC);
CREATE INDEX idx_site_leads_filters ON site_leads(organization_id, kind, status, created_at DESC);
CREATE UNIQUE INDEX uq_site_leads_newsletter ON site_leads(organization_id, lower(email)) WHERE kind = 'newsletter';
CREATE TRIGGER trg_site_leads_updated BEFORE UPDATE ON site_leads FOR EACH ROW EXECUTE FUNCTION set_updated_at();
INSERT INTO permissions(name, slug) VALUES
 ('Read site leads','tenant.site_leads.read'), ('Manage site leads','tenant.site_leads.manage') ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
 SELECT r.id,p.id FROM roles r JOIN permissions p ON p.slug IN ('tenant.site_leads.read','tenant.site_leads.manage')
 WHERE r.slug IN ('owner','admin','merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL ON CONFLICT DO NOTHING;
