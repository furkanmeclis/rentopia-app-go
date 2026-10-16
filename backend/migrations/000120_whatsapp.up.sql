-- WhatsApp channel (TEC-735, docs/design/whatsapp.md): one wuzapi user /
-- WhatsApp Web session per organization behind an integration account
-- (kind whatsapp; the generated session token is sealed in the account's
-- credentials), the session state and the message log with delivery and
-- read receipts.

-- Full lists rewritten (previous: 000118).
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis', 'einvoice', 'channel', 'whatsapp'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN (
        'iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake',
        'yolcu360', 'yolcu360_fake', 'wuzapi', 'whatsapp_fake'
    ));

CREATE TABLE whatsapp_sessions (
    id                     BIGSERIAL PRIMARY KEY,
    uuid                   UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id        BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    provider               VARCHAR(24) NOT NULL,
    status                 VARCHAR(16) NOT NULL DEFAULT 'created'
                           CHECK (status IN ('created', 'pairing', 'connected', 'disconnected', 'logged_out')),
    -- SHA-256 (hex) of the secret path segment of the session's webhook URL.
    webhook_token_hash     CHAR(64) NOT NULL,
    -- The linked number, masked ("+90 532 *** ** 33").
    phone_masked           VARCHAR(32) NOT NULL DEFAULT '',
    rate_limit_per_minute  INTEGER NOT NULL DEFAULT 20 CHECK (rate_limit_per_minute BETWEEN 1 AND 60),
    -- Who accepted the number risk notice (unofficial WhatsApp Web
    -- session: the number may be banned).
    risk_acknowledged_at   TIMESTAMPTZ NOT NULL,
    risk_acknowledged_by   BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    connected_at           TIMESTAMPTZ NULL,
    disconnected_at        TIMESTAMPTZ NULL,
    last_status_at         TIMESTAMPTZ NULL,
    last_error             TEXT NOT NULL DEFAULT '',
    -- When staff were last told that the session dropped.
    alerted_at             TIMESTAMPTZ NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_whatsapp_sessions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_whatsapp_sessions_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_whatsapp_sessions_account UNIQUE (organization_id, integration_account_id),
    CONSTRAINT uq_whatsapp_sessions_webhook UNIQUE (webhook_token_hash),
    CONSTRAINT fk_whatsapp_sessions_account FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_whatsapp_sessions_updated_at
    BEFORE UPDATE ON whatsapp_sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE whatsapp_messages (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    session_id          BIGINT NOT NULL,
    kind                VARCHAR(12) NOT NULL CHECK (kind IN ('text', 'document')),
    -- "notify.<event>" for engine messages, "test" for test sends.
    purpose             VARCHAR(80) NOT NULL DEFAULT '',
    -- The notification engine attempt (notification_deliveries.uuid).
    delivery_uuid       UUID NULL,
    customer_id         BIGINT NULL REFERENCES customers (id) ON DELETE SET NULL,
    recipient_masked    VARCHAR(32) NOT NULL,
    -- SHA-256 (hex) of the E.164 number: per-recipient rate limit without
    -- keeping the number.
    recipient_hash      CHAR(64) NOT NULL,
    -- The WhatsApp message id (set by us on send; receipts carry it).
    provider_message_id VARCHAR(64) NOT NULL,
    status              VARCHAR(12) NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'sent', 'delivered', 'read', 'failed')),
    error               TEXT NOT NULL DEFAULT '',
    test                BOOLEAN NOT NULL DEFAULT false,
    sent_at             TIMESTAMPTZ NULL,
    delivered_at        TIMESTAMPTZ NULL,
    read_at             TIMESTAMPTZ NULL,
    failed_at           TIMESTAMPTZ NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_whatsapp_messages_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_whatsapp_messages_provider_id UNIQUE (organization_id, provider_message_id),
    CONSTRAINT fk_whatsapp_messages_session FOREIGN KEY (organization_id, session_id)
        REFERENCES whatsapp_sessions (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_whatsapp_messages_organization_created ON whatsapp_messages (organization_id, created_at DESC);
CREATE INDEX idx_whatsapp_messages_session_created ON whatsapp_messages (organization_id, session_id, created_at);
CREATE INDEX idx_whatsapp_messages_awaiting ON whatsapp_messages (organization_id, sent_at) WHERE status = 'sent';

CREATE TRIGGER trg_whatsapp_messages_updated_at
    BEFORE UPDATE ON whatsapp_messages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
