-- Notification engine (TEC-733, docs/design/notification-engine.md):
-- organization notification rules (event -> ordered channels + fallback +
-- reminder offsets), organization message templates (tr / en, with
-- variables), dispatches (one event instance for one recipient) and the
-- delivery log (one row per dispatch and channel: a retry never sends the
-- same event twice on the same channel). Customer web push subscriptions
-- are kept apart from staff subscriptions (push_subscriptions).
--
-- Channels: sms, email, whatsapp, push. A channel without a configured
-- provider is logged as skipped and the next channel of the rule runs.

CREATE TABLE notification_rules (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_code       VARCHAR(64) NOT NULL,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    channels         TEXT[] NOT NULL DEFAULT '{}',
    fallback         BOOLEAN NOT NULL DEFAULT TRUE,
    reminder_offsets INTEGER[] NOT NULL DEFAULT '{}',
    updated_by       BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notification_rules_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_notification_rules_organization_event UNIQUE (organization_id, event_code),
    CONSTRAINT chk_notification_rules_channels CHECK (channels <@ ARRAY['sms', 'email', 'whatsapp', 'push']::text[]),
    CONSTRAINT chk_notification_rules_offsets CHECK (cardinality(reminder_offsets) <= 5)
);

CREATE TRIGGER trg_notification_rules_updated_at
    BEFORE UPDATE ON notification_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_message_templates (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_code      VARCHAR(64) NOT NULL,
    channel         VARCHAR(16) NOT NULL CHECK (channel IN ('sms', 'email', 'whatsapp', 'push')),
    language        VARCHAR(8) NOT NULL CHECK (language IN ('tr', 'en')),
    subject         VARCHAR(200) NOT NULL DEFAULT '',
    body            TEXT NOT NULL,
    updated_by      BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notification_message_templates_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_notification_message_templates_key UNIQUE (organization_id, event_code, channel, language)
);

CREATE TRIGGER trg_notification_message_templates_updated_at
    BEFORE UPDATE ON notification_message_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_dispatches (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id    BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_code         VARCHAR(64) NOT NULL,
    audience           VARCHAR(16) NOT NULL CHECK (audience IN ('customer', 'staff')),
    customer_id        BIGINT,
    user_id            BIGINT REFERENCES users (id) ON DELETE CASCADE,
    ref_type           VARCHAR(32) NOT NULL DEFAULT '',
    ref_uuid           UUID,
    idempotency_key    VARCHAR(200) NOT NULL,
    language           VARCHAR(8) NOT NULL DEFAULT 'tr',
    -- Message variables only (names, references, dates, links that need a
    -- sign-in); never identity, licence, signature or financial detail.
    vars               JSONB NOT NULL DEFAULT '{}',
    channels           TEXT[] NOT NULL DEFAULT '{}',
    fallback           BOOLEAN NOT NULL DEFAULT TRUE,
    next_channel_index INTEGER NOT NULL DEFAULT 0,
    status             VARCHAR(16) NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'sending', 'delivered', 'failed', 'skipped')),
    scheduled_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error         TEXT NOT NULL DEFAULT '',
    completed_at       TIMESTAMPTZ,
    test               BOOLEAN NOT NULL DEFAULT FALSE,
    created_by         BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notification_dispatches_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_notification_dispatches_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_notification_dispatches_idempotency UNIQUE (organization_id, idempotency_key),
    CONSTRAINT fk_notification_dispatches_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_notification_dispatches_recipient CHECK (
        (audience = 'customer' AND customer_id IS NOT NULL) OR (audience = 'staff' AND user_id IS NOT NULL)
    )
);

CREATE INDEX idx_notification_dispatches_organization_due
    ON notification_dispatches (organization_id, scheduled_at) WHERE status = 'pending';
CREATE INDEX idx_notification_dispatches_organization_created
    ON notification_dispatches (organization_id, created_at DESC);
CREATE INDEX idx_notification_dispatches_organization_ref
    ON notification_dispatches (organization_id, ref_type, ref_uuid);

CREATE TRIGGER trg_notification_dispatches_updated_at
    BEFORE UPDATE ON notification_dispatches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_deliveries (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    dispatch_id      BIGINT NOT NULL,
    channel          VARCHAR(16) NOT NULL CHECK (channel IN ('sms', 'email', 'whatsapp', 'push')),
    attempt_no       INTEGER NOT NULL,
    provider         VARCHAR(32) NOT NULL DEFAULT '',
    provider_ref     VARCHAR(128) NOT NULL DEFAULT '',
    -- The channel's own record (an sms_messages uuid), for delivery reports.
    external_uuid    UUID,
    recipient_masked VARCHAR(255) NOT NULL DEFAULT '',
    subject          VARCHAR(200) NOT NULL DEFAULT '',
    body             TEXT NOT NULL DEFAULT '',
    status           VARCHAR(16) NOT NULL DEFAULT 'sending'
                     CHECK (status IN ('sending', 'sent', 'delivered', 'failed', 'skipped')),
    error            TEXT NOT NULL DEFAULT '',
    sent_at          TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notification_deliveries_organization_uuid UNIQUE (organization_id, uuid),
    -- Event + channel idempotency: one attempt per channel and dispatch.
    CONSTRAINT uq_notification_deliveries_dispatch_channel UNIQUE (organization_id, dispatch_id, channel),
    CONSTRAINT fk_notification_deliveries_dispatch FOREIGN KEY (organization_id, dispatch_id)
        REFERENCES notification_dispatches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_notification_deliveries_organization_created
    ON notification_deliveries (organization_id, created_at DESC);
CREATE INDEX idx_notification_deliveries_organization_external
    ON notification_deliveries (organization_id, external_uuid) WHERE external_uuid IS NOT NULL;

CREATE TRIGGER trg_notification_deliveries_updated_at
    BEFORE UPDATE ON notification_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_push_subscriptions (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id     BIGINT NOT NULL,
    endpoint        TEXT NOT NULL,
    key_p256dh      TEXT NOT NULL,
    key_auth        TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_push_subscriptions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customer_push_subscriptions_endpoint UNIQUE (organization_id, customer_id, endpoint),
    CONSTRAINT fk_customer_push_subscriptions_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_customer_push_subscriptions_updated_at
    BEFORE UPDATE ON customer_push_subscriptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
