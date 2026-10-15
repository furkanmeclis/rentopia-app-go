-- Payments (TEC-707, docs/design/payments.md §3). Card data never touches
-- Rentopia: a customer pays on the provider's hosted checkout (iyzico
-- Checkout Form, 3DS) opened from a pay-by-link page. A payment row is one
-- checkout attempt (or a manual collection at the branch); it is confirmed
-- only by a signature-verified webhook followed by a provider retrieve, or
-- by a provider retrieve (staff "verify", the pay page poll). The return
-- page of the provider is never proof.
--
-- Idempotency: the provider payment id is unique per organization and
-- provider, the status moves pending -> succeeded once (conditional
-- update), and the reservation is told once (applied_at).
--
-- Money: NUMERIC(14,2) like reservations; the amount owed (amount), what
-- the customer paid on the provider (gross_amount, may include installment
-- interest), the provider commission and the net payout are separate, and
-- refunds accumulate in refunded_amount (rows in payment_refunds).

CREATE TABLE payment_links (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- The public bearer of /pay/{token} (random, 32 bytes base64url).
    token VARCHAR(64) NOT NULL,
    -- Subject: a reservation now; rentals and current accounts later.
    subject_type VARCHAR(16) NOT NULL DEFAULT 'reservation' CHECK (subject_type IN ('reservation')),
    reservation_id BIGINT NULL,
    integration_account_id BIGINT NOT NULL,
    purpose VARCHAR(16) NOT NULL DEFAULT 'prepayment' CHECK (purpose IN ('prepayment', 'balance', 'custom')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paid', 'expired', 'cancelled')),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ NULL,
    cancelled_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payment_links_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_payment_links_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_payment_links_subject CHECK (subject_type <> 'reservation' OR reservation_id IS NOT NULL),
    CONSTRAINT fk_payment_links_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_payment_links_account
        FOREIGN KEY (organization_id, integration_account_id) REFERENCES integration_accounts (organization_id, id)
);

-- The token is resolved before the organization is known (public page):
-- the one global index of the module; every later query is scoped.
CREATE UNIQUE INDEX uq_payment_links_token ON payment_links (token);
-- One active link per reservation (a new link cancels the previous one).
CREATE UNIQUE INDEX uq_payment_links_organization_reservation_active
    ON payment_links (organization_id, reservation_id) WHERE status = 'active';
CREATE INDEX idx_payment_links_organization_created ON payment_links (organization_id, created_at DESC);
CREATE INDEX idx_payment_links_organization_expiry ON payment_links (organization_id, expires_at) WHERE status = 'active';

CREATE TRIGGER trg_payment_links_updated_at
    BEFORE UPDATE ON payment_links
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    subject_type VARCHAR(16) NOT NULL DEFAULT 'reservation' CHECK (subject_type IN ('reservation')),
    reservation_id BIGINT NULL,
    payment_link_id BIGINT NULL,
    integration_account_id BIGINT NULL,
    provider VARCHAR(24) NOT NULL CHECK (provider IN ('iyzico', 'manual')),
    method VARCHAR(16) NOT NULL CHECK (method IN ('checkout_form', 'cash', 'card_pos', 'bank_transfer')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed', 'cancelled', 'partially_refunded', 'refunded')),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    gross_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (gross_amount >= 0),
    commission_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),
    net_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    refunded_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (refunded_amount >= 0 AND refunded_amount <= amount),
    -- Our order reference sent to the provider (conversationId / basketId).
    conversation_id VARCHAR(64) NOT NULL,
    -- Checkout session token of the provider.
    provider_token VARCHAR(255) NULL,
    provider_payment_id VARCHAR(64) NULL,
    provider_status VARCHAR(32) NOT NULL DEFAULT '',
    -- Masked, non-sensitive facts (card family, last four, installments).
    provider_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    failure_reason TEXT NOT NULL DEFAULT '',
    verified_via VARCHAR(12) NOT NULL DEFAULT '' CHECK (verified_via IN ('', 'webhook', 'retrieve', 'manual')),
    verified_at TIMESTAMPTZ NULL,
    -- The reservation was told (PaymentRecorder) and what happened.
    applied_at TIMESTAMPTZ NULL,
    outcome VARCHAR(24) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payments_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_payments_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_payments_subject CHECK (subject_type <> 'reservation' OR reservation_id IS NOT NULL),
    CONSTRAINT chk_payments_provider_account CHECK (provider = 'manual' OR integration_account_id IS NOT NULL),
    CONSTRAINT fk_payments_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_payments_link
        FOREIGN KEY (organization_id, payment_link_id) REFERENCES payment_links (organization_id, id) ON DELETE SET NULL (payment_link_id),
    CONSTRAINT fk_payments_account
        FOREIGN KEY (organization_id, integration_account_id) REFERENCES integration_accounts (organization_id, id)
);

-- RT-A16: one payment per provider payment id, whatever the deliveries.
CREATE UNIQUE INDEX uq_payments_organization_provider_payment
    ON payments (organization_id, provider, provider_payment_id) WHERE provider_payment_id IS NOT NULL;
CREATE UNIQUE INDEX uq_payments_organization_provider_token
    ON payments (organization_id, provider, provider_token) WHERE provider_token IS NOT NULL;
CREATE UNIQUE INDEX uq_payments_organization_conversation ON payments (organization_id, conversation_id);
CREATE INDEX idx_payments_organization_created ON payments (organization_id, created_at DESC);
CREATE INDEX idx_payments_organization_reservation ON payments (organization_id, reservation_id);
CREATE INDEX idx_payments_organization_pending ON payments (organization_id, created_at) WHERE status = 'pending';

CREATE TRIGGER trg_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payment_refunds (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    payment_id BIGINT NOT NULL,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    status VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    provider_refund_id VARCHAR(64) NULL,
    reason TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payment_refunds_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_payment_refunds_payment
        FOREIGN KEY (organization_id, payment_id) REFERENCES payments (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_payment_refunds_organization_payment ON payment_refunds (organization_id, payment_id, created_at);

CREATE TRIGGER trg_payment_refunds_updated_at
    BEFORE UPDATE ON payment_refunds
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
