-- Integration skeleton (TEC-676, docs/design/payments.md §2). Every external
-- provider an organization connects (iyzico payments now; KABİS, HGS,
-- e-invoice and channel managers later) is an integration_accounts row:
-- the organization's own credentials, sealed with the field cipher under
-- the "integration.credentials" purpose and bound to the organization and
-- the account uuid (associated data), so a ciphertext copied to another
-- row does not open. Only a masked hint is ever returned to the browser.
--
-- integration_jobs records every outbound call and inbound notification
-- that must happen exactly once (idempotency key unique per provider),
-- with attempts and status. organization_id is nullable: platform-level
-- jobs (none yet) have no organization; every query names the scope
-- (mixed_scope_tables.txt).

CREATE TABLE integration_accounts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind VARCHAR(24) NOT NULL CHECK (kind IN ('payment')),
    provider VARCHAR(24) NOT NULL CHECK (provider IN ('iyzico')),
    display_name VARCHAR(120) NOT NULL,
    mode VARCHAR(8) NOT NULL DEFAULT 'sandbox' CHECK (mode IN ('sandbox', 'live')),
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    -- "v<N>:" + base64(nonce | AES-256-GCM); JSON object of the secrets.
    credentials_ciphertext TEXT NOT NULL,
    -- Masked values for display ({"api_key": "sand••••1234"}).
    credentials_hint JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Non-secret provider settings.
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_checked_at TIMESTAMPTZ NULL,
    last_check_status VARCHAR(8) NOT NULL DEFAULT '' CHECK (last_check_status IN ('', 'ok', 'failed')),
    last_check_error TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_integration_accounts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_integration_accounts_organization_id UNIQUE (organization_id, id)
);

-- One active account per organization and kind (payments go to it).
CREATE UNIQUE INDEX uq_integration_accounts_organization_active_kind
    ON integration_accounts (organization_id, kind) WHERE status = 'active';
CREATE INDEX idx_integration_accounts_organization_created
    ON integration_accounts (organization_id, created_at DESC);

CREATE TRIGGER trg_integration_accounts_updated_at
    BEFORE UPDATE ON integration_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE integration_jobs (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NULL,
    direction VARCHAR(8) NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    provider VARCHAR(24) NOT NULL,
    -- payment.webhook, payment.refund, payment.retrieve …
    kind VARCHAR(48) NOT NULL,
    idempotency_key VARCHAR(200) NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'ignored')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    -- Deliveries of the same key (an inbound notification sent again).
    deliveries INTEGER NOT NULL DEFAULT 1 CHECK (deliveries >= 1),
    last_error TEXT NOT NULL DEFAULT '',
    -- References and non-secret facts only (never credentials or card data).
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    ref_type VARCHAR(24) NOT NULL DEFAULT '',
    ref_uuid UUID NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_integration_jobs_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE SET NULL (integration_account_id),
    CONSTRAINT chk_integration_jobs_account_scope CHECK (integration_account_id IS NULL OR organization_id IS NOT NULL)
);

-- Exactly once per provider and key (a webhook delivered three times is
-- one job with deliveries = 3).
CREATE UNIQUE INDEX uq_integration_jobs_provider_key ON integration_jobs (provider, idempotency_key);
CREATE UNIQUE INDEX uq_integration_jobs_uuid ON integration_jobs (uuid);
CREATE INDEX idx_integration_jobs_organization_created ON integration_jobs (organization_id, created_at DESC);
CREATE INDEX idx_integration_jobs_organization_account
    ON integration_jobs (organization_id, integration_account_id, created_at DESC);

CREATE TRIGGER trg_integration_jobs_updated_at
    BEFORE UPDATE ON integration_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
