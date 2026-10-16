-- Data-loss note: drops every channel mapping, offer, channel booking link,
-- inbound message (sealed payloads and the error queue) and sync log row,
-- and deletes every channel integration account (kind channel, providers
-- yolcu360 / yolcu360_fake) with its jobs. Reservations created from
-- channel bookings stay (channel, external reference, collecting party and
-- commission are reservation columns). The one-active-account-per-kind
-- index and the old kind / provider lists are restored.

DROP TABLE IF EXISTS channel_sync_runs;
DROP TABLE IF EXISTS channel_messages;
DROP TABLE IF EXISTS channel_reservations;
DROP TABLE IF EXISTS channel_offers;
DROP TABLE IF EXISTS channel_mappings;
DROP TABLE IF EXISTS channel_accounts;

DELETE FROM integration_jobs
WHERE integration_account_id IN (
    SELECT id FROM integration_accounts WHERE kind = 'channel' OR provider IN ('yolcu360', 'yolcu360_fake')
);
DELETE FROM integration_accounts WHERE kind = 'channel' OR provider IN ('yolcu360', 'yolcu360_fake');

DROP INDEX IF EXISTS uq_integration_accounts_organization_active_channel;
DROP INDEX IF EXISTS uq_integration_accounts_organization_active_kind;
CREATE UNIQUE INDEX uq_integration_accounts_organization_active_kind
    ON integration_accounts (organization_id, kind) WHERE status = 'active';

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis', 'einvoice'));
