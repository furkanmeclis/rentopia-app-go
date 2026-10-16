-- KABİS rental notifications (TEC-726, RT-A21, docs/design/kabis.md).
--
-- KABİS is the police vehicle rental notification system. Its official
-- connection method is not verified yet (TEC-682), so no undocumented
-- endpoint is assumed: the official adapter slot ("kabis") answers
-- "waiting on dependency", and a development / test adapter ("kabis_fake")
-- exists outside production only.
--
-- kabis_notifications is one notification per rental and event, built only
-- from ACTUAL events (the handover = rental_start, the return = rental_end;
-- correction = a reported start whose person / vehicle data changed). A
-- reservation is never reported. Each notification has exactly one
-- integration_jobs row (the status, attempts, last error, provider
-- reference); the person data the authority needs (identity, licence
-- numbers) is sealed with the field cipher ("kabis.payload" purpose, bound
-- to the organization and the notification) and only masked facts are
-- stored in the clear (summary).
--
-- integration_jobs gains four statuses (generic, see
-- docs/design/integration-jobs.md §3):
--   blocked            = required data is missing; nothing is sent until a
--                        person completes it (missing fields listed on the
--                        domain record)
--   waiting_dependency = no working adapter for the organization (no
--                        account, or the adapter is not verified yet);
--                        nothing is sent, the job stays visible
--   reported_manually  = staff filed the work through the official manual
--                        channel (reason, reference, optional attachment)
--   reconciled         = staff confirmed with the provider that the work
--                        was done (reason, reference)
-- The full list is written out (000086 introduced the runner's states).

ALTER TABLE integration_jobs DROP CONSTRAINT integration_jobs_status_check;
ALTER TABLE integration_jobs ADD CONSTRAINT integration_jobs_status_check
    CHECK (status IN (
        'pending', 'processing', 'submitted', 'succeeded', 'failed', 'ignored', 'needs_verification',
        'blocked', 'waiting_dependency', 'reported_manually', 'reconciled'
    ));

-- KABİS accounts: the official slot and the development adapter. Full
-- lists: 000077 (payment / iyzico), 000087 (sms / netgsm, log), 000093
-- (toll / hgs_fake).
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake'));

CREATE TABLE kabis_notifications (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    -- The branch where the event happened (handover / return / swap).
    branch_id BIGINT NOT NULL,
    -- The rental's own branch (the reservation's planned pick-up branch).
    -- Branch scope: visible when either branch is in the member's scope.
    rental_branch_id BIGINT NOT NULL,
    event VARCHAR(16) NOT NULL CHECK (event IN ('rental_start', 'rental_end', 'correction')),
    -- 1 for the start / end; corrections count up per rental.
    sequence INTEGER NOT NULL DEFAULT 1 CHECK (sequence >= 1),
    -- What produced it: handover, return, drivers_changed, vehicle_swap.
    cause VARCHAR(24) NOT NULL DEFAULT '',
    -- The notification a correction amends.
    corrects_id BIGINT NULL,
    integration_job_id BIGINT NOT NULL REFERENCES integration_jobs (id) ON DELETE CASCADE,
    -- The actual event time (never a planned time).
    occurred_at TIMESTAMPTZ NOT NULL,
    plate VARCHAR(32) NOT NULL DEFAULT '',
    -- "v<N>:" + base64(nonce | AES-256-GCM) of the full payload (JSON).
    payload_enc TEXT NOT NULL DEFAULT '',
    -- SHA-256 of the plaintext payload (change detection for corrections).
    payload_hash VARCHAR(64) NOT NULL DEFAULT '',
    -- Masked facts for display (names, masked identity / licence numbers,
    -- plate, place, time). Never a full identity or licence number.
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Required fields that were missing at the last validation (blocked).
    missing_fields TEXT[] NOT NULL DEFAULT '{}',
    -- Why a job waits on a dependency: no_account, account_disabled,
    -- adapter_unverified, adapter_unavailable.
    hold_reason VARCHAR(32) NOT NULL DEFAULT '',
    -- Last status check with the provider (CheckStatus attempt).
    last_verified_at TIMESTAMPTZ NULL,
    -- Staff resolution (reported_manually / reconciled).
    resolution VARCHAR(20) NULL CHECK (resolution IN ('reported_manually', 'reconciled')),
    resolution_reason TEXT NOT NULL DEFAULT '',
    resolution_reference VARCHAR(120) NOT NULL DEFAULT '',
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    -- Optional attachment of the resolution (private storage, streamed by
    -- the API only): organizations/{org}/kabis/{notification}/{file}.{ext}.
    attachment_key TEXT NULL,
    attachment_name VARCHAR(255) NOT NULL DEFAULT '',
    attachment_content_type VARCHAR(100) NOT NULL DEFAULT '',
    attachment_size BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kabis_notifications_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_kabis_notifications_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_kabis_notifications_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_kabis_notifications_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_kabis_notifications_rental_branch
        FOREIGN KEY (organization_id, rental_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_kabis_notifications_corrects
        FOREIGN KEY (organization_id, corrects_id)
        REFERENCES kabis_notifications (organization_id, id) ON DELETE SET NULL (corrects_id),
    CONSTRAINT chk_kabis_notifications_start_end CHECK (event = 'correction' OR corrects_id IS NULL)
);

-- Exactly one notification per rental, event and sequence (the handover
-- hook runs once per rental; a repeated call creates nothing).
CREATE UNIQUE INDEX uq_kabis_notifications_rental_event
    ON kabis_notifications (organization_id, rental_id, event, sequence);
CREATE UNIQUE INDEX uq_kabis_notifications_organization_job
    ON kabis_notifications (organization_id, integration_job_id);
CREATE INDEX idx_kabis_notifications_organization_created
    ON kabis_notifications (organization_id, created_at DESC);
CREATE INDEX idx_kabis_notifications_organization_branch
    ON kabis_notifications (organization_id, branch_id);
CREATE INDEX idx_kabis_notifications_organization_rental_branch
    ON kabis_notifications (organization_id, rental_branch_id);

CREATE TRIGGER trg_kabis_notifications_updated_at
    BEFORE UPDATE ON kabis_notifications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The release sweep finds waiting jobs quickly.
CREATE INDEX idx_integration_jobs_waiting
    ON integration_jobs (organization_id, kind)
    WHERE status = 'waiting_dependency';
