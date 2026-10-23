-- Restore idempotency gaps (TEC-962, docs/operations/backup-restore.md
-- RT-A32): bookkeeping for work that was done after the backup the
-- database was restored from.
--
-- 1. integration_jobs: a staff / operator "reconciled" resolution with who,
--    when and why (POST /v1/tenant/integration-jobs/{uuid}/reconcile, the
--    bulk route and cmd/integration-jobs-reconcile). The status value
--    exists since 000099.
-- 2. document_versions.pdf_requested_at: when the PDF render was last
--    requested, so a pending render whose queue task was lost is requested
--    again by the worker sweep (NULL = requested before this column).
-- 3. payment_webhook_orphans: provider notifications that name no payment
--    of ours (a payment created after the backup). Mixed scope: the
--    organization is known when the signature verified with one of its
--    accounts, otherwise the row is platform-level (organization_id NULL).
-- 4. payment_reconciliation_items: refunds the provider shows and our
--    ledger does not (refund_mismatch: the refund guard refused to refund
--    again), and refunds made while the provider's refunds could not be
--    read (unverified: the refund went ahead; staff check it later).

ALTER TABLE integration_jobs
    ADD COLUMN reconciled_at TIMESTAMPTZ NULL,
    ADD COLUMN reconciled_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN reconcile_note TEXT NOT NULL DEFAULT '',
    ADD COLUMN reconcile_external_ref VARCHAR(200) NOT NULL DEFAULT '';

ALTER TABLE document_versions
    ADD COLUMN pdf_requested_at TIMESTAMPTZ NULL;

-- The sweep: pending renders across organizations, oldest first.
CREATE INDEX idx_document_versions_pdf_pending
    ON document_versions (pdf_requested_at)
    WHERE pdf_status = 'pending';

CREATE TABLE payment_webhook_orphans (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    -- NULL = no account's secret verified the signature (platform-level).
    organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NULL,
    provider VARCHAR(24) NOT NULL,
    -- One row per notification (provider payment / token + event + status);
    -- a delivery of the same notification counts deliveries.
    dedupe_key VARCHAR(300) NOT NULL,
    event_type VARCHAR(64) NOT NULL DEFAULT '',
    provider_status VARCHAR(32) NOT NULL DEFAULT '',
    provider_payment_id VARCHAR(64) NOT NULL DEFAULT '',
    conversation_id VARCHAR(64) NOT NULL DEFAULT '',
    provider_token VARCHAR(255) NOT NULL DEFAULT '',
    reference_code VARCHAR(64) NOT NULL DEFAULT '',
    merchant_id VARCHAR(64) NOT NULL DEFAULT '',
    signature_verified BOOLEAN NOT NULL DEFAULT FALSE,
    -- The notification body (iyzico sends references only: no card data).
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    deliveries INTEGER NOT NULL DEFAULT 1 CHECK (deliveries >= 1),
    status VARCHAR(12) NOT NULL DEFAULT 'unmatched' CHECK (status IN ('unmatched', 'matched')),
    matched_payment_id BIGINT NULL,
    matched_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    matched_at TIMESTAMPTZ NULL,
    match_note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_payment_webhook_orphans_account_scope
        CHECK (integration_account_id IS NULL OR organization_id IS NOT NULL),
    CONSTRAINT chk_payment_webhook_orphans_matched
        CHECK (status <> 'matched' OR organization_id IS NOT NULL),
    CONSTRAINT fk_payment_webhook_orphans_account
        FOREIGN KEY (organization_id, integration_account_id) REFERENCES integration_accounts (organization_id, id)
        ON DELETE SET NULL (integration_account_id),
    CONSTRAINT fk_payment_webhook_orphans_payment
        FOREIGN KEY (organization_id, matched_payment_id) REFERENCES payments (organization_id, id)
        ON DELETE SET NULL (matched_payment_id)
);

-- Global like uq_integration_jobs_provider_key: the organization is not
-- known when a notification arrives.
CREATE UNIQUE INDEX uq_payment_webhook_orphans_provider_key ON payment_webhook_orphans (provider, dedupe_key);
CREATE UNIQUE INDEX uq_payment_webhook_orphans_uuid ON payment_webhook_orphans (uuid);
CREATE INDEX idx_payment_webhook_orphans_organization_created
    ON payment_webhook_orphans (organization_id, created_at DESC);
CREATE INDEX idx_payment_webhook_orphans_unmatched
    ON payment_webhook_orphans (organization_id) WHERE status = 'unmatched';

CREATE TRIGGER trg_payment_webhook_orphans_updated_at
    BEFORE UPDATE ON payment_webhook_orphans
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payment_reconciliation_items (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    payment_id BIGINT NOT NULL,
    kind VARCHAR(24) NOT NULL CHECK (kind IN ('refund_mismatch', 'unverified')),
    status VARCHAR(12) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- What our ledger and the provider showed when the item was opened.
    our_refunded NUMERIC(14, 2) NOT NULL DEFAULT 0,
    provider_refunded NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- The refund that was refused / made (amount, reason) and the provider
    -- facts (or the lookup error of an unverified one).
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- record_refund (the provider's refund entered in our ledger) | dismiss.
    resolution VARCHAR(16) NOT NULL DEFAULT '' CHECK (resolution IN ('', 'record_refund', 'dismiss')),
    resolution_note TEXT NOT NULL DEFAULT '',
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payment_reconciliation_items_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_payment_reconciliation_items_payment
        FOREIGN KEY (organization_id, payment_id) REFERENCES payments (organization_id, id) ON DELETE CASCADE
);

-- One open item per payment and kind (a second refused refund reuses it).
CREATE UNIQUE INDEX uq_payment_reconciliation_items_open
    ON payment_reconciliation_items (organization_id, payment_id, kind) WHERE status = 'open';
CREATE INDEX idx_payment_reconciliation_items_organization_created
    ON payment_reconciliation_items (organization_id, created_at DESC);

CREATE TRIGGER trg_payment_reconciliation_items_updated_at
    BEFORE UPDATE ON payment_reconciliation_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
