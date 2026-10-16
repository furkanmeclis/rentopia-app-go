-- Data-loss note: drops every corporate agreement, its contacts, permitted
-- drivers, agreement tariffs (rate plans of scope "agreement" and their
-- versions and bands), the billing periods of periodically billed rentals,
-- and the agreement links of quotes and reservations. Ledger entries stay;
-- their billing period link is dropped.

DROP TRIGGER IF EXISTS trg_rentals_rent_billing_fixed ON rentals;
DROP FUNCTION IF EXISTS rentals_rent_billing_fixed();
DROP TRIGGER IF EXISTS trg_ledger_entries_rent_billing ON ledger_entries;
DROP FUNCTION IF EXISTS ledger_entries_rent_billing();
DROP INDEX IF EXISTS idx_ledger_entries_organization_billing_period;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS fk_ledger_entries_billing_period;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS billing_period_id;

DROP TABLE IF EXISTS rental_billing_periods;
DROP FUNCTION IF EXISTS rental_billing_periods_guard();

ALTER TABLE rentals DROP COLUMN IF EXISTS rent_billing;

DROP INDEX IF EXISTS idx_reservations_organization_agreement;
ALTER TABLE reservations DROP CONSTRAINT IF EXISTS chk_reservations_open_account;
ALTER TABLE reservations DROP CONSTRAINT IF EXISTS fk_reservations_agreement;
ALTER TABLE reservations DROP COLUMN IF EXISTS open_account;
ALTER TABLE reservations DROP COLUMN IF EXISTS agreement_id;

ALTER TABLE quotes DROP CONSTRAINT IF EXISTS fk_quotes_agreement;
ALTER TABLE quotes DROP COLUMN IF EXISTS agreement_id;

-- Agreement tariffs go with their agreements (bands of activated versions
-- are frozen by a trigger, switched off for this cleanup only).
ALTER TABLE rate_bands DISABLE TRIGGER trg_rate_bands_immutable;
DELETE FROM rate_bands b USING rate_plan_versions v, rate_plans p
 WHERE b.organization_id = v.organization_id AND b.rate_plan_version_id = v.id
   AND v.organization_id = p.organization_id AND v.rate_plan_id = p.id AND p.scope = 'agreement';
DELETE FROM rate_plan_versions v USING rate_plans p
 WHERE v.organization_id = p.organization_id AND v.rate_plan_id = p.id AND p.scope = 'agreement';
DELETE FROM rate_plans WHERE scope = 'agreement';
ALTER TABLE rate_bands ENABLE TRIGGER trg_rate_bands_immutable;
DROP INDEX IF EXISTS idx_rate_plans_organization_agreement;
ALTER TABLE rate_plans DROP CONSTRAINT IF EXISTS chk_rate_plans_agreement_scope;
ALTER TABLE rate_plans DROP CONSTRAINT IF EXISTS fk_rate_plans_agreement;
ALTER TABLE rate_plans DROP CONSTRAINT rate_plans_scope_check;
ALTER TABLE rate_plans ADD CONSTRAINT rate_plans_scope_check
    CHECK (scope IN ('common', 'branch', 'channel', 'branch_channel'));
ALTER TABLE rate_plans DROP COLUMN IF EXISTS agreement_id;

DROP TABLE IF EXISTS agreement_drivers;
DROP TABLE IF EXISTS agreement_contacts;
DROP TABLE IF EXISTS corporate_agreements;

ALTER TABLE customer_contacts DROP CONSTRAINT IF EXISTS uq_customer_contacts_organization_id;
