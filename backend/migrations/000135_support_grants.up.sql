-- Time-boxed, recorded support access (TEC-678). Platform staff may enter an
-- organization (impersonate a member, act in the tenant shell) only while
-- the organization has an active grant: not revoked and not expired. Expiry
-- is checked at request time (expires_at > now()); no job closes grants.
--   scope read_only  every non-GET tenant request answers 403 SUPPORT_READ_ONLY
--   scope full       the impersonated member's own permissions apply
-- created_by / revoked_by are the tenant members (users) who opened or
-- closed the grant; a new grant replaces the active one (revoked_by = opener).
CREATE TABLE support_grants (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    scope           TEXT        NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ NULL,
    revoked_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    note            TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_support_grants_org_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT chk_support_grants_scope CHECK (scope IN ('read_only', 'full')),
    CONSTRAINT chk_support_grants_note CHECK (char_length(note) <= 500)
);

CREATE INDEX idx_support_grants_org_created ON support_grants (organization_id, created_at DESC);
-- Request-time lookup of the active grant of an organization.
CREATE INDEX idx_support_grants_org_open ON support_grants (organization_id, expires_at DESC)
    WHERE revoked_at IS NULL;

CREATE TRIGGER trg_support_grants_set_updated_at
    BEFORE UPDATE ON support_grants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Opening, listing and revoking support access: owner and admin.
INSERT INTO permissions (name, slug) VALUES
    ('Manage support access', 'tenant.support_access.manage')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.support_access.manage'
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
