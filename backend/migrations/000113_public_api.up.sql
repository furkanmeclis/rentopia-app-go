-- Public booking API (TEC-728, docs/design/public-api.md): /v1/public/*
-- for the branded site, the embeddable widget and an organization's own
-- frontends or partners.
--
-- public_api_keys: an organization's credentials.
--   secret       server-to-server key (rtp_sk_…); only its SHA-256 is
--                stored and it is shown once. Never accepted from a browser.
--   publishable  site / widget key (rtp_pk_…) that may appear in a web page;
--                it only mints short-lived booking session tokens for the
--                origins listed in allowed_origins. Kept in plain text so the
--                panel can show the embed snippet again.
-- Each key sells as one sales channel (web | widget | api, orgsettings
-- codes), has scopes, a per-minute rate limit, and is rotated by issuing a
-- successor while the old key keeps working until expires_at.
--
-- public_api_idempotency: the stored answer of every write made with an
-- Idempotency-Key (24 h), per key and session; the body is sealed.
-- public_quotes / public_reservations: which quotes and reservations the
-- public API made (the key, the channel and, for reservations, the hash of
-- the customer's reservation access token).
-- public_webhook_endpoints / public_webhook_deliveries: signed reservation
-- event webhooks (secret sealed with its own purpose) and their delivery
-- queue with retries.

CREATE TABLE public_api_keys (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind VARCHAR(12) NOT NULL CHECK (kind IN ('secret', 'publishable')),
    name VARCHAR(120) NOT NULL CHECK (char_length(btrim(name)) > 0),
    channel VARCHAR(16) NOT NULL CHECK (channel IN ('web', 'widget', 'api')),
    -- Display only: the first characters of the key and its last four.
    key_prefix VARCHAR(24) NOT NULL,
    key_last4 VARCHAR(4) NOT NULL,
    -- SHA-256 hex of the full key (256 / 192 random bits).
    key_hash VARCHAR(64) NOT NULL,
    -- The full publishable key (not a secret: it is embedded in pages).
    -- Always NULL for secret keys.
    publishable_key VARCHAR(64) NULL,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    -- Publishable keys: the exact origins (scheme://host[:port]) allowed to
    -- mint sessions. Secret keys: empty.
    allowed_origins TEXT[] NOT NULL DEFAULT '{}',
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 120 CHECK (rate_limit_per_minute BETWEEN 10 AND 6000),
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    -- Set on rotation: the old key keeps working until then.
    expires_at TIMESTAMPTZ NULL,
    rotated_from_id BIGINT NULL,
    last_used_at TIMESTAMPTZ NULL,
    last_used_ip INET NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    revoked_at TIMESTAMPTZ NULL,
    revoked_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_public_api_keys_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_public_api_keys_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_public_api_keys_publishable CHECK ((kind = 'publishable') = (publishable_key IS NOT NULL)),
    CONSTRAINT chk_public_api_keys_origins CHECK (kind = 'publishable' OR cardinality(allowed_origins) = 0),
    CONSTRAINT chk_public_api_keys_revoked CHECK ((status = 'revoked') = (revoked_at IS NOT NULL)),
    CONSTRAINT fk_public_api_keys_rotated_from
        FOREIGN KEY (organization_id, rotated_from_id)
        REFERENCES public_api_keys (organization_id, id) ON DELETE SET NULL (rotated_from_id)
);

-- Deliberately global: the API resolves a key before it knows the
-- organization (like customer_portal_sessions.token_hash).
CREATE UNIQUE INDEX uq_public_api_keys_key_hash ON public_api_keys (key_hash);
CREATE INDEX idx_public_api_keys_organization_created ON public_api_keys (organization_id, created_at DESC);

CREATE TRIGGER trg_public_api_keys_updated_at
    BEFORE UPDATE ON public_api_keys
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE public_api_idempotency (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    key_id BIGINT NOT NULL,
    -- The booking session id for browser sessions, '' for secret keys.
    subject VARCHAR(64) NOT NULL DEFAULT '',
    idempotency_key VARCHAR(255) NOT NULL,
    method VARCHAR(8) NOT NULL,
    path VARCHAR(255) NOT NULL,
    -- SHA-256 hex of method, path and body.
    request_hash VARCHAR(64) NOT NULL,
    state VARCHAR(12) NOT NULL DEFAULT 'processing' CHECK (state IN ('processing', 'completed')),
    response_status INTEGER NULL,
    -- The answer body, sealed (it may carry a reservation access token).
    response_enc TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_public_api_idempotency_key UNIQUE (organization_id, key_id, subject, idempotency_key),
    CONSTRAINT fk_public_api_idempotency_key
        FOREIGN KEY (organization_id, key_id) REFERENCES public_api_keys (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_public_api_idempotency_organization_expiry ON public_api_idempotency (organization_id, expires_at);

CREATE TABLE public_quotes (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    quote_id BIGINT NOT NULL,
    key_id BIGINT NULL,
    channel VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_public_quotes_organization_quote UNIQUE (organization_id, quote_id),
    CONSTRAINT fk_public_quotes_quote
        FOREIGN KEY (organization_id, quote_id) REFERENCES quotes (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_public_quotes_key
        FOREIGN KEY (organization_id, key_id) REFERENCES public_api_keys (organization_id, id) ON DELETE SET NULL (key_id)
);

CREATE TABLE public_reservations (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    key_id BIGINT NULL,
    channel VARCHAR(16) NOT NULL,
    -- SHA-256 hex of the reservation access token handed to the booker
    -- (X-Reservation-Token on status / request / payment calls).
    access_token_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_public_reservations_organization_reservation UNIQUE (organization_id, reservation_id),
    CONSTRAINT fk_public_reservations_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_public_reservations_key
        FOREIGN KEY (organization_id, key_id) REFERENCES public_api_keys (organization_id, id) ON DELETE SET NULL (key_id)
);

CREATE INDEX idx_public_reservations_organization_created ON public_reservations (organization_id, created_at DESC);

CREATE TABLE public_webhook_endpoints (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL CHECK (char_length(btrim(name)) > 0),
    url TEXT NOT NULL CHECK (char_length(url) BETWEEN 8 AND 2048),
    -- Signing secret (whsec_…), sealed under purpose public_api_webhooks.
    secret_enc TEXT NOT NULL,
    secret_last4 VARCHAR(4) NOT NULL,
    events TEXT[] NOT NULL DEFAULT '{}',
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    last_success_at TIMESTAMPTZ NULL,
    last_failure_at TIMESTAMPTZ NULL,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_public_webhook_endpoints_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_public_webhook_endpoints_organization_id UNIQUE (organization_id, id)
);

CREATE INDEX idx_public_webhook_endpoints_organization_created
    ON public_webhook_endpoints (organization_id, created_at DESC);

CREATE TRIGGER trg_public_webhook_endpoints_updated_at
    BEFORE UPDATE ON public_webhook_endpoints
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE public_webhook_deliveries (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    endpoint_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    -- The event id (the same for every endpoint the event goes to).
    event_uuid UUID NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_status_code INTEGER NULL,
    last_error TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_public_webhook_deliveries_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_public_webhook_deliveries_endpoint
        FOREIGN KEY (organization_id, endpoint_id) REFERENCES public_webhook_endpoints (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_public_webhook_deliveries_organization_endpoint
    ON public_webhook_deliveries (organization_id, endpoint_id, created_at DESC);
-- The delivery sweep runs over every organization (like idx_integration_jobs_due).
CREATE INDEX idx_public_webhook_deliveries_due
    ON public_webhook_deliveries (next_attempt_at)
    WHERE status = 'pending';

CREATE TRIGGER trg_public_webhook_deliveries_updated_at
    BEFORE UPDATE ON public_webhook_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
