-- Data-loss note: drops integration_attempts (every recorded provider call
-- with its summaries and timings), the runner columns of integration_jobs
-- (max_attempts, next_attempt_at, provider_ref, submitted_at, locked_until,
-- priority) and every SMS integration account (kind sms: Netgsm / log, with
-- their sealed credentials) together with their jobs. Remaining jobs in the
-- runner's own states (submitted, needs_verification) are rewritten to
-- failed so the old CHECK holds; their unknown outcome is lost.
DROP TABLE IF EXISTS integration_attempts;

DELETE FROM integration_jobs
WHERE integration_account_id IN (SELECT id FROM integration_accounts WHERE kind = 'sms');
DELETE FROM integration_jobs WHERE kind LIKE 'sms.%';
DELETE FROM integration_accounts WHERE kind = 'sms' OR provider IN ('netgsm', 'log');

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment'));

UPDATE integration_jobs SET status = 'failed', last_error = 'runner state dropped by a migration down'
WHERE status IN ('submitted', 'needs_verification');

DROP INDEX IF EXISTS idx_integration_jobs_due;
ALTER TABLE integration_jobs
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS submitted_at,
    DROP COLUMN IF EXISTS provider_ref,
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS max_attempts;

ALTER TABLE integration_jobs DROP CONSTRAINT integration_jobs_status_check;
ALTER TABLE integration_jobs ALTER COLUMN status TYPE VARCHAR(12);
ALTER TABLE integration_jobs ADD CONSTRAINT integration_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'ignored'));
