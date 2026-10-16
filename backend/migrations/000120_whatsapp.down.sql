-- Data-loss note: drops the WhatsApp sessions and the message log, and the
-- WhatsApp integration accounts (their wuzapi users stay on the wuzapi
-- server until removed there).
DROP TABLE IF EXISTS whatsapp_messages;
DROP TABLE IF EXISTS whatsapp_sessions;

DELETE FROM integration_accounts WHERE kind = 'whatsapp';

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis', 'einvoice', 'channel'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN (
        'iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake',
        'yolcu360', 'yolcu360_fake'
    ));
