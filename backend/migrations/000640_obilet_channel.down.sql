-- Data-loss note: deletes every Obilet integration account (providers
-- obilet / obilet_fake) with its jobs; their channel settings, mappings,
-- offers, booking links, inbound messages and sync log rows go with them
-- (ON DELETE CASCADE). Reservations created from Obilet bookings stay
-- (channel, external reference, collecting party and commission are
-- reservation columns). The provider list of 000118 is restored.

DELETE FROM integration_jobs
WHERE integration_account_id IN (
    SELECT id FROM integration_accounts WHERE provider IN ('obilet', 'obilet_fake')
);
DELETE FROM integration_accounts WHERE provider IN ('obilet', 'obilet_fake');

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN (
        'iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake',
        'yolcu360', 'yolcu360_fake'
    ));
