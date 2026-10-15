-- Remote contract approval (TEC-711, docs/design/contracts.md §7): staff
-- create a time-limited link for the current version of a document; the
-- customer opens /d/{token}, receives an SMS code on the number staff chose
-- (renter or a driver of the reservation) and approves exactly that
-- version and content hash.
--
-- document_approval_links: one row per link. The bearer token is never
-- stored: token_hash (SHA-256 hex of 32 random bytes) is the lookup key,
-- token_ciphertext keeps it sealed (field cipher purpose
-- "documents.approval_link") so staff can copy / send it again. The
-- number the code goes to is a snapshot (masked, keyed hash, sealed). One
-- pending link per document; a new version of the document supersedes the
-- open link of the previous one. status pending -> approved | expired |
-- revoked | superseded (expiry is also applied lazily by expires_at).
--
-- document_approvals gains the remote approval method (remote_sms) and the
-- evidence of a remote approval: the verified number (masked + keyed hash),
-- the OTP challenge, the driver (when a driver approved), IP address and
-- user agent, the link; plus the approval PDF (the version with an approval
-- page), stored privately like version PDFs.
--
-- Every index starts with organization_id except uq_document_approval_links_token_hash:
-- the public page resolves the token to its organization (as payment_links.token).

CREATE TABLE document_approval_links (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    document_id BIGINT NOT NULL,
    document_version_id BIGINT NOT NULL,
    content_hash CHAR(64) NOT NULL,
    token_hash CHAR(64) NOT NULL CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    token_ciphertext TEXT NOT NULL,
    -- Who approves: the renter (reservation customer) or a driver.
    signer_role VARCHAR(8) NOT NULL CHECK (signer_role IN ('renter', 'driver')),
    signer_name VARCHAR(200) NOT NULL,
    customer_id BIGINT NULL,
    driver_id BIGINT NULL,
    phone_masked VARCHAR(32) NOT NULL,
    phone_hash VARCHAR(64) NOT NULL,
    phone_ciphertext TEXT NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'expired', 'revoked', 'superseded')),
    expires_at TIMESTAMPTZ NOT NULL,
    -- Per-link counters (limits that hold without Redis).
    codes_sent INTEGER NOT NULL DEFAULT 0 CHECK (codes_sent >= 0),
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    sms_sent_count INTEGER NOT NULL DEFAULT 0 CHECK (sms_sent_count >= 0),
    last_sms_at TIMESTAMPTZ NULL,
    approved_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    revoked_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    revoked_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_approval_links_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_document_approval_links_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_document_approval_links_approved CHECK (status <> 'approved' OR approved_at IS NOT NULL),
    CONSTRAINT chk_document_approval_links_closed CHECK (status NOT IN ('revoked', 'superseded') OR revoked_at IS NOT NULL),
    CONSTRAINT chk_document_approval_links_signer CHECK (
        (signer_role = 'renter' AND driver_id IS NULL) OR signer_role = 'driver'
    ),
    CONSTRAINT fk_document_approval_links_document
        FOREIGN KEY (organization_id, document_id) REFERENCES documents (organization_id, id) ON DELETE CASCADE,
    -- The link names the exact content (as approvals do).
    CONSTRAINT fk_document_approval_links_version_hash
        FOREIGN KEY (organization_id, document_version_id, content_hash)
        REFERENCES document_versions (organization_id, id, content_hash) ON DELETE CASCADE,
    CONSTRAINT fk_document_approval_links_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id) ON DELETE SET NULL (customer_id),
    CONSTRAINT fk_document_approval_links_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE SET NULL (driver_id)
);

-- Global on purpose: the public page knows only the token.
CREATE UNIQUE INDEX uq_document_approval_links_token_hash ON document_approval_links (token_hash);
-- One open link per document.
CREATE UNIQUE INDEX uq_document_approval_links_pending
    ON document_approval_links (organization_id, document_id) WHERE status = 'pending';
CREATE INDEX idx_document_approval_links_organization_document
    ON document_approval_links (organization_id, document_id, created_at DESC);
CREATE INDEX idx_document_approval_links_organization_version
    ON document_approval_links (organization_id, document_version_id);

CREATE TRIGGER trg_document_approval_links_updated_at
    BEFORE UPDATE ON document_approval_links
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE document_approvals DROP CONSTRAINT document_approvals_method_check;
ALTER TABLE document_approvals
    ADD CONSTRAINT document_approvals_method_check
        CHECK (method IN ('in_person', 'click', 'signature', 'remote_sms'));

ALTER TABLE document_approvals
    ADD COLUMN driver_id BIGINT NULL,
    ADD COLUMN link_id BIGINT NULL,
    ADD COLUMN otp_challenge_uuid UUID NULL,
    ADD COLUMN verified_phone_masked VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN verified_phone_hash VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN ip_address INET NULL,
    ADD COLUMN user_agent VARCHAR(512) NOT NULL DEFAULT '',
    -- The approval PDF: the version with an approval page (private key).
    ADD COLUMN pdf_object_key TEXT NULL,
    ADD COLUMN pdf_size_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN pdf_sha256 CHAR(64) NULL,
    ADD CONSTRAINT fk_document_approvals_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE SET NULL (driver_id),
    ADD CONSTRAINT fk_document_approvals_link
        FOREIGN KEY (organization_id, link_id) REFERENCES document_approval_links (organization_id, id) ON DELETE CASCADE,
    -- A remote approval always carries the verified number and its link.
    ADD CONSTRAINT chk_document_approvals_remote CHECK (
        method <> 'remote_sms' OR (verified_phone_hash <> '' AND link_id IS NOT NULL AND otp_challenge_uuid IS NOT NULL)
    );

CREATE UNIQUE INDEX uq_document_approvals_organization_link
    ON document_approvals (organization_id, link_id) WHERE link_id IS NOT NULL;
