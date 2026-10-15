-- SMS sending and OTP (TEC-734, docs/design/sms.md).
--
-- sms_messages is the tenant's send log: one row per SMS handed to a
-- provider through the integration job runner. It never holds a full
-- phone number (masked display value + a keyed blind index for counting)
-- and never an OTP code: OTP bodies are stored redacted ("Kodunuz: ••••••")
-- and every body also as a keyed hash. Delivery reports update status /
-- dlr_* through the job's CheckStatus. The number and text the provider
-- needs are kept sealed only until the send is final.
--
-- sms_otp_challenges is one issued one-time code: the code exists only as
-- HMAC(server key, challenge uuid + code); verification compares in
-- constant time, counts attempts and expires.

CREATE TABLE sms_messages (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NULL,
    integration_job_id BIGINT NULL REFERENCES integration_jobs (id) ON DELETE SET NULL,
    -- otp: one-time codes; transactional: reservation / contract notices;
    -- informational: optional notices that honour the customer's İYS SMS
    -- consent (marketing never goes through this table).
    class VARCHAR(16) NOT NULL CHECK (class IN ('otp', 'transactional', 'informational')),
    purpose VARCHAR(48) NOT NULL,
    provider VARCHAR(24) NOT NULL,
    phone_masked VARCHAR(32) NOT NULL,
    -- BlindIndex("sms.phone", org, normalized phone): rate limits, lookups.
    phone_hash VARCHAR(64) NOT NULL,
    -- Redacted for OTPs; the plain text for other classes.
    body_redacted TEXT NOT NULL,
    body_hash VARCHAR(64) NOT NULL,
    -- Sealed {"to", "body"} for the provider call (field cipher purpose
    -- "sms.payload", bound to the organization and the message uuid);
    -- emptied as soon as the send is final (delivered / failed) or the OTP
    -- is no longer usable.
    payload_ciphertext TEXT NOT NULL DEFAULT '',
    customer_id BIGINT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'submitted', 'delivered', 'undelivered', 'failed', 'unknown', 'blocked')),
    provider_message_id VARCHAR(200) NOT NULL DEFAULT '',
    dlr_status VARCHAR(32) NOT NULL DEFAULT '',
    dlr_at TIMESTAMPTZ NULL,
    error TEXT NOT NULL DEFAULT '',
    ref_type VARCHAR(24) NOT NULL DEFAULT '',
    ref_uuid UUID NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sms_messages_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_sms_messages_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_sms_messages_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE SET NULL (integration_account_id),
    CONSTRAINT fk_sms_messages_customer
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE SET NULL (customer_id)
);

CREATE INDEX idx_sms_messages_organization_created ON sms_messages (organization_id, created_at DESC);
CREATE INDEX idx_sms_messages_organization_phone ON sms_messages (organization_id, phone_hash, created_at DESC);
CREATE INDEX idx_sms_messages_organization_job ON sms_messages (organization_id, integration_job_id);

CREATE TRIGGER trg_sms_messages_updated_at
    BEFORE UPDATE ON sms_messages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sms_otp_challenges (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- contract_approval (TEC-711) …
    purpose VARCHAR(48) NOT NULL,
    -- What the code proves control over ("contract:<uuid>").
    subject_ref VARCHAR(160) NOT NULL,
    phone_masked VARCHAR(32) NOT NULL,
    phone_hash VARCHAR(64) NOT NULL,
    -- BlindIndex("sms.otp.code", "org:<id>|otp:<uuid>", code), hex.
    code_hash VARCHAR(64) NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'verified', 'expired', 'locked', 'superseded')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts >= 1),
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ NULL,
    sms_message_id BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sms_otp_challenges_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_sms_otp_challenges_message
        FOREIGN KEY (organization_id, sms_message_id)
        REFERENCES sms_messages (organization_id, id) ON DELETE SET NULL (sms_message_id)
);

-- At most one open challenge per purpose and subject (a resend supersedes).
CREATE UNIQUE INDEX uq_sms_otp_challenges_open
    ON sms_otp_challenges (organization_id, purpose, subject_ref)
    WHERE status = 'pending';
CREATE INDEX idx_sms_otp_challenges_organization_phone
    ON sms_otp_challenges (organization_id, phone_hash, created_at DESC);
CREATE INDEX idx_sms_otp_challenges_organization_created
    ON sms_otp_challenges (organization_id, created_at DESC);

CREATE TRIGGER trg_sms_otp_challenges_updated_at
    BEFORE UPDATE ON sms_otp_challenges
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
