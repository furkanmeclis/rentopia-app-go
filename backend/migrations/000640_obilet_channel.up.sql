-- Obilet channel adapter (TEC-737, docs/design/channel-manager.md §1).
--
-- integration_accounts gains the providers obilet and obilet_fake (full
-- list rewritten; previous: 000118). The kind "channel", the channel tables
-- and the one-active-account-per-channel index (obilet and its fake count as
-- the same channel, next to yolcu360) are already in place.

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN (
        'iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake',
        'yolcu360', 'yolcu360_fake', 'obilet', 'obilet_fake'
    ));
