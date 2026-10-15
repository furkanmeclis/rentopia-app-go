-- Data-loss note: drops every KABİS notification (sealed payloads, masked
-- summaries, staff resolutions and their attachment references; the
-- attachment objects stay in storage), deletes every KABİS integration job
-- (kabis.* kinds) with its attempts and every KABİS account (kind kabis,
-- providers kabis / kabis_fake). Other jobs still in blocked or
-- waiting_dependency are rewritten to failed, reported_manually /
-- reconciled ones to succeeded (none exist outside KABİS), so the old
-- status list holds again.

DROP INDEX IF EXISTS idx_integration_jobs_waiting;
DROP TABLE IF EXISTS kabis_notifications;

DELETE FROM integration_jobs WHERE kind LIKE 'kabis.%';
DELETE FROM integration_jobs
WHERE integration_account_id IN (SELECT id FROM integration_accounts WHERE kind = 'kabis' OR provider IN ('kabis', 'kabis_fake'));
DELETE FROM integration_accounts WHERE kind = 'kabis' OR provider IN ('kabis', 'kabis_fake');

UPDATE integration_jobs SET status = 'failed' WHERE status IN ('blocked', 'waiting_dependency');
UPDATE integration_jobs SET status = 'succeeded' WHERE status IN ('reported_manually', 'reconciled');

ALTER TABLE integration_jobs DROP CONSTRAINT integration_jobs_status_check;
ALTER TABLE integration_jobs ADD CONSTRAINT integration_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'submitted', 'succeeded', 'failed', 'ignored', 'needs_verification'));

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log', 'hgs_fake'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll'));
