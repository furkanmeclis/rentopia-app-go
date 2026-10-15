-- Data-loss note: drops every customer / supplier account ("cari"), every
-- ledger entry (accruals, collections, refunds, offsets, adjustments,
-- reversals) and every document link of the entries. Balances cannot be
-- rebuilt afterwards except by posting again from rentals and payments.
DROP TABLE IF EXISTS ledger_entry_links;
DROP TABLE IF EXISTS ledger_entries;
DROP FUNCTION IF EXISTS ledger_entries_insert_rules();
DROP FUNCTION IF EXISTS ledger_entries_append_only();
ALTER TABLE payment_refunds DROP CONSTRAINT IF EXISTS uq_payment_refunds_organization_id;
ALTER TABLE rental_charges DROP CONSTRAINT IF EXISTS uq_rental_charges_organization_id;
DROP TABLE IF EXISTS accounts;
