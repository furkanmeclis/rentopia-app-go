-- Generic integration job runner (TEC-751, docs/design/integration-jobs.md).
--
-- integration_jobs (000077) gains the runner's states and bookkeeping. The
-- status column keeps its existing values; the runner's vocabulary maps
-- onto it:
--   pending             = pending_submit (nothing sent yet, or the provider
--                         confirmed the previous submit was not done)
--   processing          = a worker holds the job (payments use it as before)
--   submitted           = the provider accepted the work; the final outcome
--                         comes from CheckStatus (e.g. an SMS delivery report)
--   succeeded / failed  = final (failed always means "the provider confirmed
--                         it was not done", never "unknown")
--   needs_verification  = the outcome is unknown (timeout, 5xx after the
--                         request left); only CheckStatus may move it on
--   ignored             = payments: a notification that needs no action
--
-- integration_attempts holds one row per provider call (submit or
-- check_status) with a short request / response summary. Summaries carry
-- references and non-secret facts only: never credentials, message bodies,
-- OTP codes, full phone numbers or card data. Mixed scope like the jobs
-- (organization_id NULL = a platform job; mixed_scope_tables.txt).

ALTER TABLE integration_jobs DROP CONSTRAINT integration_jobs_status_check;
ALTER TABLE integration_jobs ALTER COLUMN status TYPE VARCHAR(24);
ALTER TABLE integration_jobs ADD CONSTRAINT integration_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'submitted', 'succeeded', 'failed', 'ignored', 'needs_verification'));

ALTER TABLE integration_jobs
    ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    ADD COLUMN next_attempt_at TIMESTAMPTZ NULL,
    -- The provider's reference for the submitted work (Netgsm jobid …);
    -- CheckStatus looks it up.
    ADD COLUMN provider_ref VARCHAR(200) NOT NULL DEFAULT '',
    ADD COLUMN submitted_at TIMESTAMPTZ NULL,
    -- A worker's claim on the job (crash-safe: an expired claim is free).
    ADD COLUMN locked_until TIMESTAMPTZ NULL,
    -- critical = OTP / transactional (own queue), normal = everything else.
    ADD COLUMN priority VARCHAR(8) NOT NULL DEFAULT 'normal' CHECK (priority IN ('critical', 'normal'));

-- Due work for the sweep (pending / submitted / needs_verification).
CREATE INDEX idx_integration_jobs_due
    ON integration_jobs (status, next_attempt_at)
    WHERE status IN ('pending', 'submitted', 'needs_verification');

CREATE TABLE integration_attempts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_job_id BIGINT NOT NULL REFERENCES integration_jobs (id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL CHECK (attempt_no >= 1),
    operation VARCHAR(16) NOT NULL CHECK (operation IN ('submit', 'check_status')),
    -- What triggered the call: the queue worker, the sweep, a staff retry,
    -- or an inline call from the API (payment refunds, no queue).
    trigger VARCHAR(12) NOT NULL DEFAULT 'worker' CHECK (trigger IN ('worker', 'sweep', 'retry', 'inline')),
    outcome VARCHAR(16) NOT NULL
        CHECK (outcome IN ('succeeded', 'accepted', 'pending', 'failed', 'not_found', 'uncertain', 'unsupported')),
    request_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_integration_attempts_window CHECK (finished_at >= started_at)
);

-- Global by design: attempts are addressed by uuid only inside a job the
-- caller already resolved in its organization.
CREATE UNIQUE INDEX uq_integration_attempts_uuid ON integration_attempts (uuid);
CREATE INDEX idx_integration_attempts_organization_job
    ON integration_attempts (organization_id, integration_job_id, id);

-- SMS accounts (TEC-734): Netgsm, and the development log provider.
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log'));
