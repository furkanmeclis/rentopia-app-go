-- Customer portal (TEC-715, docs/design/customer-portal.md).
--
-- customer_portal_sessions: a short-lived customer session created after a
-- one-time code (purpose customer_portal, subject reservation:<uuid>). The
-- browser holds an opaque bearer token in an HttpOnly cookie set by the
-- BFF; only its SHA-256 is stored. The session names the organization, the
-- customer (renter) and, when the person signed in as a driver of the
-- reservation, the driver: a renter sees the customer's reservations in the
-- organization, a driver only the reservations they drive. It is never a
-- staff session (no user, no JWT).
--
-- customer_requests: a change or cancellation request a customer sent from
-- the portal. Staff see and resolve them (reservation detail, the requests
-- list); nothing changes on the reservation by itself unless the policy
-- snapshot of the reservation explicitly allows self-cancellation.

CREATE TABLE customer_portal_sessions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL,
    -- Set when the person signed in with a driver's contact.
    driver_id BIGINT NULL,
    -- The reservation whose reference was used to sign in.
    reservation_id BIGINT NOT NULL,
    -- SHA-256 hex of the bearer token (256 random bits).
    token_hash VARCHAR(64) NOT NULL,
    channel VARCHAR(8) NOT NULL CHECK (channel IN ('sms', 'email')),
    contact_masked VARCHAR(64) NOT NULL,
    otp_challenge_uuid UUID NOT NULL,
    ip_address INET NULL,
    user_agent VARCHAR(512) NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Sliding idle expiry, never beyond hard_expires_at.
    expires_at TIMESTAMPTZ NOT NULL,
    hard_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    revoked_reason VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_portal_sessions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customer_portal_sessions_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_customer_portal_sessions_expiry CHECK (expires_at <= hard_expires_at),
    CONSTRAINT fk_customer_portal_sessions_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_portal_sessions_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_portal_sessions_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

-- Deliberately global: the API resolves the bearer token before it knows
-- the organization (then compares it with the organization in the URL), as
-- payment_links.token and document_approval_links.token_hash.
CREATE UNIQUE INDEX uq_customer_portal_sessions_token_hash ON customer_portal_sessions (token_hash);
CREATE INDEX idx_customer_portal_sessions_organization_customer
    ON customer_portal_sessions (organization_id, customer_id, created_at DESC);
CREATE INDEX idx_customer_portal_sessions_organization_expiry
    ON customer_portal_sessions (organization_id, hard_expires_at);

CREATE TRIGGER trg_customer_portal_sessions_updated_at
    BEFORE UPDATE ON customer_portal_sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_requests (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    -- Who asked: the session's customer (and driver when signed in as one).
    customer_id BIGINT NOT NULL,
    driver_id BIGINT NULL,
    session_id BIGINT NULL,
    request_type VARCHAR(8) NOT NULL CHECK (request_type IN ('change', 'cancel')),
    status VARCHAR(12) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'rejected')),
    message TEXT NOT NULL DEFAULT '' CHECK (char_length(message) <= 2000),
    -- Change requests: the dates the customer asks for (optional).
    requested_starts_at TIMESTAMPTZ NULL,
    requested_ends_at TIMESTAMPTZ NULL,
    -- Cancellation requests: the policy text and fee shown to the customer,
    -- copied from the reservation's policy snapshot at the time.
    policy_text TEXT NOT NULL DEFAULT '',
    cancellation_fee NUMERIC(14, 2) NULL CHECK (cancellation_fee IS NULL OR cancellation_fee >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- self_cancelled: the policy snapshot allowed it and the reservation was
    -- cancelled at once (the request is then resolved).
    outcome VARCHAR(16) NOT NULL DEFAULT '' CHECK (outcome IN ('', 'self_cancelled')),
    contact_masked VARCHAR(64) NOT NULL DEFAULT '',
    resolution_note TEXT NOT NULL DEFAULT '',
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_requests_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customer_requests_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_customer_requests_range CHECK (
        requested_starts_at IS NULL OR requested_ends_at IS NULL OR requested_ends_at > requested_starts_at
    ),
    CONSTRAINT chk_customer_requests_resolved CHECK ((status = 'open') = (resolved_at IS NULL)),
    CONSTRAINT fk_customer_requests_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_requests_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_requests_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE SET NULL (driver_id),
    CONSTRAINT fk_customer_requests_session
        FOREIGN KEY (organization_id, session_id) REFERENCES customer_portal_sessions (organization_id, id)
            ON DELETE SET NULL (session_id)
);

-- One open request per reservation and type.
CREATE UNIQUE INDEX uq_customer_requests_open
    ON customer_requests (organization_id, reservation_id, request_type)
    WHERE status = 'open';
CREATE INDEX idx_customer_requests_organization_status
    ON customer_requests (organization_id, status, created_at DESC);
CREATE INDEX idx_customer_requests_organization_reservation
    ON customer_requests (organization_id, reservation_id, created_at DESC);

CREATE TRIGGER trg_customer_requests_updated_at
    BEFORE UPDATE ON customer_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
