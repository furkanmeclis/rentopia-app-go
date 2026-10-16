-- Channel manager core (TEC-736, RT-A25, docs/design/channel-manager.md).
--
-- Marketplaces (Yolcu360 first, Obilet next) sell the organization's rental
-- products. Each one is an integration account of kind "channel" (the
-- organization's own credentials, sealed by the integrations vault). The
-- tables below are shared by every channel adapter:
--
--   channel_accounts      per account: sync settings (horizon, intervals,
--                         offline policy, default collecting party and
--                         commission) and the sync state (last push / pull,
--                         cursor, push requested by an event)
--   channel_mappings      internal records ↔ external codes per account:
--                         branch → location code, rental product (optionally
--                         per branch) → product code with stop-sale and a
--                         quota cap, extra → extra code. Several internal
--                         products may share one external code; the internal
--                         code stays permanent
--   channel_offers        what each pushed item stood for (product, version,
--                         location, day, price): a booking that echoes the
--                         offer reference resolves the product from it,
--                         never from the group name
--   channel_reservations  one row per channel booking: the external number is
--                         unique per organization and channel; the latest
--                         applied version reconciles out-of-order messages;
--                         who collected, gross / commission / net
--   channel_messages      every inbound message once (per account and message
--                         id; a repeat only counts a delivery): status,
--                         error queue, the sealed payload (customer data)
--   channel_sync_runs     the sync log (push / pull / reconciliation)
--
-- integration_accounts gains the kind "channel" and the providers yolcu360
-- and yolcu360_fake (full lists rewritten; previous: 000108). One active
-- account per kind stays the rule except for channels: there it is one
-- active account per channel (yolcu360 and its fake count as the same
-- channel), so Yolcu360 and Obilet can both be connected.

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis', 'einvoice', 'channel'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN (
        'iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake',
        'yolcu360', 'yolcu360_fake'
    ));

DROP INDEX uq_integration_accounts_organization_active_kind;
CREATE UNIQUE INDEX uq_integration_accounts_organization_active_kind
    ON integration_accounts (organization_id, kind)
    WHERE status = 'active' AND kind <> 'channel';
CREATE UNIQUE INDEX uq_integration_accounts_organization_active_channel
    ON integration_accounts (organization_id, (regexp_replace(provider, '_fake$', '')))
    WHERE status = 'active' AND kind = 'channel';

CREATE TABLE channel_accounts (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    -- Sales channel code of the bookings (orgsettings sales channels).
    channel VARCHAR(16) NOT NULL,
    push_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    pull_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    -- Account-wide stop-sale: every item goes out with 0 units.
    stop_sale BOOLEAN NOT NULL DEFAULT FALSE,
    horizon_days INTEGER NOT NULL DEFAULT 30 CHECK (horizon_days BETWEEN 1 AND 90),
    push_interval_minutes INTEGER NOT NULL DEFAULT 15 CHECK (push_interval_minutes BETWEEN 5 AND 1440),
    pull_interval_minutes INTEGER NOT NULL DEFAULT 5 CHECK (pull_interval_minutes BETWEEN 1 AND 1440),
    -- How long a push stays valid on the channel; afterwards the offline
    -- policy applies (the last known capacity is never sold indefinitely).
    validity_minutes INTEGER NOT NULL DEFAULT 120 CHECK (validity_minutes BETWEEN 15 AND 2880),
    offline_policy VARCHAR(8) NOT NULL DEFAULT 'close' CHECK (offline_policy IN ('close', 'quota')),
    offline_quota INTEGER NOT NULL DEFAULT 0 CHECK (offline_quota BETWEEN 0 AND 100),
    -- Defaults when a message does not say: who collected, commission %.
    collected_by VARCHAR(8) NOT NULL DEFAULT 'channel' CHECK (collected_by IN ('channel', 'us')),
    commission_rate NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (commission_rate >= 0 AND commission_rate <= 100),
    -- An event (a sale, a cancellation, a mapping change) asked for a push.
    push_requested_at TIMESTAMPTZ NULL,
    last_push_at TIMESTAMPTZ NULL,
    last_push_ok_at TIMESTAMPTZ NULL,
    last_push_status VARCHAR(16) NOT NULL DEFAULT '',
    last_push_error TEXT NOT NULL DEFAULT '',
    -- valid_until of the last accepted push (the channel stops selling the
    -- pushed capacity after it).
    pushed_valid_until TIMESTAMPTZ NULL,
    last_pull_at TIMESTAMPTZ NULL,
    last_pull_ok_at TIMESTAMPTZ NULL,
    last_pull_status VARCHAR(16) NOT NULL DEFAULT '',
    last_pull_error TEXT NOT NULL DEFAULT '',
    pull_cursor TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    -- A worker holds the account's sync until then (crash-safe claim).
    locked_until TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_channel_accounts_organization_account UNIQUE (organization_id, integration_account_id),
    CONSTRAINT fk_channel_accounts_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_channel_accounts_updated_at
    BEFORE UPDATE ON channel_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE channel_mappings (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    entity_type VARCHAR(16) NOT NULL CHECK (entity_type IN ('branch', 'product', 'extra')),
    -- branch: the location; product: optional location scope (NULL = every
    -- mapped location).
    branch_id BIGINT NULL,
    rental_product_id BIGINT NULL,
    extra_id BIGINT NULL,
    external_code VARCHAR(64) NOT NULL,
    external_name VARCHAR(255) NOT NULL DEFAULT '',
    -- Several products on one external code: the lowest priority number
    -- with free capacity is offered first.
    priority INTEGER NOT NULL DEFAULT 0,
    -- Products only: stop-sale and a cap on the units offered per day.
    stop_sale BOOLEAN NOT NULL DEFAULT FALSE,
    max_quota INTEGER NULL CHECK (max_quota IS NULL OR max_quota >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_channel_mappings_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_channel_mappings_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_channel_mappings_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_mappings_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_mappings_product
        FOREIGN KEY (organization_id, rental_product_id) REFERENCES rental_products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_mappings_extra
        FOREIGN KEY (organization_id, extra_id) REFERENCES extras (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_channel_mappings_entity CHECK (
        (entity_type = 'branch' AND branch_id IS NOT NULL AND rental_product_id IS NULL AND extra_id IS NULL)
        OR (entity_type = 'product' AND rental_product_id IS NOT NULL AND extra_id IS NULL)
        OR (entity_type = 'extra' AND extra_id IS NOT NULL AND rental_product_id IS NULL AND branch_id IS NULL)
    ),
    CONSTRAINT chk_channel_mappings_product_only
        CHECK (entity_type = 'product' OR (stop_sale = FALSE AND max_quota IS NULL)),
    CONSTRAINT chk_channel_mappings_code CHECK (length(btrim(external_code)) > 0)
);

CREATE UNIQUE INDEX uq_channel_mappings_branch
    ON channel_mappings (organization_id, integration_account_id, branch_id) WHERE entity_type = 'branch';
CREATE UNIQUE INDEX uq_channel_mappings_branch_code
    ON channel_mappings (organization_id, integration_account_id, external_code) WHERE entity_type = 'branch';
CREATE UNIQUE INDEX uq_channel_mappings_extra
    ON channel_mappings (organization_id, integration_account_id, extra_id) WHERE entity_type = 'extra';
CREATE UNIQUE INDEX uq_channel_mappings_extra_code
    ON channel_mappings (organization_id, integration_account_id, external_code) WHERE entity_type = 'extra';
CREATE UNIQUE INDEX uq_channel_mappings_product
    ON channel_mappings (organization_id, integration_account_id, rental_product_id, COALESCE(branch_id, 0))
    WHERE entity_type = 'product';
CREATE INDEX idx_channel_mappings_organization_code
    ON channel_mappings (organization_id, integration_account_id, entity_type, external_code);
CREATE INDEX idx_channel_mappings_organization_created
    ON channel_mappings (organization_id, created_at DESC);
CREATE INDEX idx_channel_mappings_organization_branch ON channel_mappings (organization_id, branch_id);
CREATE INDEX idx_channel_mappings_organization_product ON channel_mappings (organization_id, rental_product_id);
CREATE INDEX idx_channel_mappings_organization_extra ON channel_mappings (organization_id, extra_id);

CREATE TRIGGER trg_channel_mappings_updated_at
    BEFORE UPDATE ON channel_mappings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE channel_offers (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    -- Deterministic reference of (product, version, location, day, price):
    -- an unchanged item keeps its reference across pushes.
    offer_ref VARCHAR(64) NOT NULL,
    mapping_id BIGINT NOT NULL,
    rental_product_id BIGINT NOT NULL,
    product_version INTEGER NOT NULL,
    branch_id BIGINT NOT NULL,
    external_code VARCHAR(64) NOT NULL,
    location_code VARCHAR(64) NOT NULL,
    stay_date DATE NOT NULL,
    daily_price NUMERIC(14, 2) NULL,
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    available INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_channel_offers_organization_ref UNIQUE (organization_id, integration_account_id, offer_ref),
    CONSTRAINT fk_channel_offers_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_offers_mapping
        FOREIGN KEY (organization_id, mapping_id) REFERENCES channel_mappings (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_offers_product
        FOREIGN KEY (organization_id, rental_product_id) REFERENCES rental_products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_offers_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_channel_offers_organization_lookup
    ON channel_offers (organization_id, integration_account_id, external_code, location_code, stay_date);
CREATE INDEX idx_channel_offers_organization_mapping ON channel_offers (organization_id, mapping_id);
CREATE INDEX idx_channel_offers_organization_created ON channel_offers (organization_id, created_at);

CREATE TABLE channel_reservations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    channel VARCHAR(16) NOT NULL,
    external_ref VARCHAR(64) NOT NULL,
    -- The Rentopia reservation (NULL: not created yet, or a cancellation
    -- that arrived before its booking).
    reservation_id BIGINT NULL,
    customer_id BIGINT NULL,
    -- pending (nothing applied yet), active, cancelled, failed (the last
    -- message could not be applied; see the error queue).
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'cancelled', 'failed')),
    -- The channel's state in the newest message received.
    channel_status VARCHAR(16) NOT NULL DEFAULT 'confirmed' CHECK (channel_status IN ('confirmed', 'cancelled')),
    -- Newest version received / applied (out-of-order messages with a
    -- version at or below applied_version change nothing).
    last_version BIGINT NOT NULL DEFAULT 0,
    applied_version BIGINT NOT NULL DEFAULT 0,
    product_code VARCHAR(64) NOT NULL DEFAULT '',
    pickup_code VARCHAR(64) NOT NULL DEFAULT '',
    return_code VARCHAR(64) NOT NULL DEFAULT '',
    offer_ref VARCHAR(64) NOT NULL DEFAULT '',
    rental_product_id BIGINT NULL,
    starts_at TIMESTAMPTZ NULL,
    ends_at TIMESTAMPTZ NULL,
    collected_by VARCHAR(8) NOT NULL DEFAULT 'channel' CHECK (collected_by IN ('channel', 'us')),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    gross_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    commission_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    net_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    prepaid_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- Our reservation total after the last apply and the difference to the
    -- channel's gross (reconciliation).
    local_total NUMERIC(14, 2) NULL,
    price_difference NUMERIC(14, 2) NULL,
    last_message_at TIMESTAMPTZ NULL,
    last_error_code VARCHAR(48) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_channel_reservations_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_channel_reservations_organization_id UNIQUE (organization_id, id),
    -- The external booking number is unique per organization and channel.
    CONSTRAINT uq_channel_reservations_external UNIQUE (organization_id, channel, external_ref),
    CONSTRAINT fk_channel_reservations_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_reservations_reservation
        FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE SET NULL (reservation_id),
    CONSTRAINT fk_channel_reservations_customer
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE SET NULL (customer_id),
    CONSTRAINT fk_channel_reservations_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id) ON DELETE SET NULL (rental_product_id)
);

CREATE UNIQUE INDEX uq_channel_reservations_organization_reservation
    ON channel_reservations (organization_id, reservation_id) WHERE reservation_id IS NOT NULL;
CREATE INDEX idx_channel_reservations_organization_account
    ON channel_reservations (organization_id, integration_account_id, starts_at);
CREATE INDEX idx_channel_reservations_organization_customer ON channel_reservations (organization_id, customer_id);
CREATE INDEX idx_channel_reservations_organization_product ON channel_reservations (organization_id, rental_product_id);

CREATE TRIGGER trg_channel_reservations_updated_at
    BEFORE UPDATE ON channel_reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE channel_messages (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    channel_reservation_id BIGINT NULL,
    channel VARCHAR(16) NOT NULL,
    -- The channel's message id (idempotency key per account).
    message_key VARCHAR(128) NOT NULL,
    external_ref VARCHAR(64) NOT NULL,
    message_type VARCHAR(16) NOT NULL CHECK (message_type IN ('booking', 'modification', 'cancellation')),
    version BIGINT NOT NULL DEFAULT 0,
    occurred_at TIMESTAMPTZ NULL,
    source VARCHAR(12) NOT NULL DEFAULT 'pull' CHECK (source IN ('pull', 'webhook', 'simulation')),
    -- received → applied | duplicate (same version as the applied one) |
    -- stale (older than the applied version) | failed (error queue) |
    -- ignored (a person dismissed it).
    status VARCHAR(16) NOT NULL DEFAULT 'received'
        CHECK (status IN ('received', 'applied', 'duplicate', 'stale', 'failed', 'ignored')),
    error_code VARCHAR(48) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    -- How many times the channel delivered this message id.
    deliveries INTEGER NOT NULL DEFAULT 1,
    processed_at TIMESTAMPTZ NULL,
    acked_at TIMESTAMPTZ NULL,
    ack_error TEXT NOT NULL DEFAULT '',
    -- "v<N>:" + base64(nonce | AES-256-GCM) of the full message (JSON,
    -- with the customer's contact data). Purpose "channels.message".
    payload_enc TEXT NOT NULL DEFAULT '',
    -- Masked facts for display (codes, dates, amounts, masked name).
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    resolution_note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_channel_messages_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_channel_messages_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_channel_messages_key UNIQUE (organization_id, integration_account_id, message_key),
    CONSTRAINT fk_channel_messages_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_messages_reservation
        FOREIGN KEY (organization_id, channel_reservation_id)
        REFERENCES channel_reservations (organization_id, id) ON DELETE SET NULL (channel_reservation_id)
);

CREATE INDEX idx_channel_messages_organization_created ON channel_messages (organization_id, created_at DESC);
CREATE INDEX idx_channel_messages_organization_status ON channel_messages (organization_id, status);
CREATE INDEX idx_channel_messages_organization_reservation ON channel_messages (organization_id, channel_reservation_id);
CREATE INDEX idx_channel_messages_organization_external ON channel_messages (organization_id, channel, external_ref);

CREATE TRIGGER trg_channel_messages_updated_at
    BEFORE UPDATE ON channel_messages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE channel_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NOT NULL,
    direction VARCHAR(12) NOT NULL CHECK (direction IN ('push', 'pull', 'reconcile')),
    -- event (a sale / cancellation / mapping change), schedule, manual.
    trigger VARCHAR(12) NOT NULL CHECK (trigger IN ('event', 'schedule', 'manual')),
    status VARCHAR(12) NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'partial', 'failed')),
    items_total INTEGER NOT NULL DEFAULT 0,
    items_ok INTEGER NOT NULL DEFAULT 0,
    items_failed INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    -- Non-sensitive facts (receipt reference, item errors, cursor).
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_channel_sync_runs_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_channel_sync_runs_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_channel_sync_runs_organization_started
    ON channel_sync_runs (organization_id, started_at DESC);
CREATE INDEX idx_channel_sync_runs_organization_account
    ON channel_sync_runs (organization_id, integration_account_id, started_at DESC);
