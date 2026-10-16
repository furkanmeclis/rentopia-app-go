-- Data-loss note: drops collection_allocations (every allocation of a
-- collection to an open item and its history) and collections (the
-- collection documents: method, reference, payer, note, gross / commission
-- / net, settlement and void state). The ledger entries, cash movements
-- and commission expenses they produced stay. Refund entries lose their
-- collection link (ledger_entries.collection_id); entries with source_type
-- collection become manual. Clearing cash accounts become bank accounts and
-- lose their provider; the payment_commission expense category stays.

DROP TABLE IF EXISTS collection_allocations;
DROP FUNCTION IF EXISTS collection_allocations_guard();
DROP FUNCTION IF EXISTS collection_allocations_append_only();

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS fk_ledger_entries_collection;
DROP INDEX IF EXISTS idx_ledger_entries_organization_collection;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS collection_id;

DROP TABLE IF EXISTS collections;
DROP FUNCTION IF EXISTS collections_refund_guard();

-- Entries of source collection go back to manual (the append-only trigger
-- is lifted for this one rewrite).
ALTER TABLE ledger_entries DISABLE TRIGGER trg_ledger_entries_append_only;
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_source_type_check;
UPDATE ledger_entries SET source_type = 'manual' WHERE source_type = 'collection';
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_source_type_check CHECK (source_type IN (
    'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
    'cash_movement', 'manual'
));
ALTER TABLE ledger_entries ENABLE TRIGGER trg_ledger_entries_append_only;

DROP INDEX IF EXISTS uq_cash_accounts_organization_provider;
ALTER TABLE cash_accounts DROP CONSTRAINT IF EXISTS chk_cash_accounts_provider;
ALTER TABLE cash_accounts DROP COLUMN IF EXISTS provider;
ALTER TABLE cash_accounts DROP CONSTRAINT cash_accounts_kind_check;
UPDATE cash_accounts SET kind = 'bank' WHERE kind = 'clearing';
ALTER TABLE cash_accounts ADD CONSTRAINT cash_accounts_kind_check CHECK (kind IN ('cash', 'bank'));
