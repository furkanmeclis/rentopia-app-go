-- Data-loss note: drops every toll pass, its revisions and timeline, every
-- vehicle toll tag and the organizations' toll settings; deletes every
-- toll and toll_fee rental charge (reversals included) and the toll
-- provider accounts with their integration jobs and attempts. Remaining
-- rental charges lose the pass / reversal links (none exist outside tolls).

ALTER TABLE toll_passes DROP CONSTRAINT fk_toll_passes_charge;
ALTER TABLE toll_passes DROP CONSTRAINT fk_toll_passes_fee_charge;

DELETE FROM rental_charges WHERE kind IN ('toll', 'toll_fee') OR source = 'toll' OR reversal_of_id IS NOT NULL;
DROP INDEX IF EXISTS idx_rental_charges_organization_toll_pass;
DROP INDEX IF EXISTS uq_rental_charges_reversal;
ALTER TABLE rental_charges
    DROP CONSTRAINT fk_rental_charges_reversal_of,
    DROP CONSTRAINT fk_rental_charges_toll_pass,
    DROP CONSTRAINT chk_rental_charges_amount,
    DROP COLUMN reversal_of_id,
    DROP COLUMN toll_pass_id;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_amount_check CHECK (amount >= 0);
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_source_check;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_source_check CHECK (source IN ('policy', 'manual'));
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_kind_check;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_kind_check
    CHECK (kind IN ('late_return', 'early_return', 'damage', 'fuel', 'mileage', 'equipment', 'other'));
ALTER TABLE rental_charges DROP CONSTRAINT uq_rental_charges_organization_id;

DROP TABLE IF EXISTS toll_settings;
DROP TABLE IF EXISTS toll_pass_events;
DROP TABLE IF EXISTS toll_pass_revisions;
DROP TABLE IF EXISTS toll_passes;
DROP TABLE IF EXISTS vehicle_toll_tags;

DELETE FROM integration_jobs
WHERE integration_account_id IN (SELECT id FROM integration_accounts WHERE kind = 'toll' OR provider = 'hgs_fake');
DELETE FROM integration_jobs WHERE kind LIKE 'toll.%';
DELETE FROM integration_accounts WHERE kind = 'toll' OR provider = 'hgs_fake';
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms'));
