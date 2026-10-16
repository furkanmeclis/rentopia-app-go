-- Data-loss note: drops every security deposit / pre-authorization record
-- and every deposit deduction (their documents stay in storage,
-- unreferenced). The ledger entries the deposits posted (deposit_hold,
-- deposit_return, deposit_capture and the collections that name a
-- deduction) and the deposit_in / deposit_out cash movements are deleted
-- too: the append-only triggers are switched off for that one statement
-- each, so customer balances and till balances lose those postings. The
-- collection documents of deductions (TEC-718) and their allocations go
-- with them: the items they paid show as open again.

ALTER TABLE collection_allocations DISABLE TRIGGER trg_collection_allocations_append_only;
DELETE FROM collection_allocations a
USING collections c, ledger_entries e
WHERE c.organization_id = a.organization_id AND c.id = a.collection_id
  AND e.organization_id = c.organization_id AND e.id = c.ledger_entry_id
  AND (e.security_deposit_id IS NOT NULL OR e.deposit_deduction_id IS NOT NULL);
ALTER TABLE collection_allocations ENABLE TRIGGER trg_collection_allocations_append_only;
DELETE FROM collections c
USING ledger_entries e
WHERE e.organization_id = c.organization_id AND e.id = c.ledger_entry_id
  AND (e.security_deposit_id IS NOT NULL OR e.deposit_deduction_id IS NOT NULL);

ALTER TABLE cash_movements DISABLE TRIGGER trg_cash_movements_append_only;
DELETE FROM cash_movements m
WHERE m.kind IN ('deposit_in', 'deposit_out')
   OR m.ledger_entry_id IN (
        SELECT e.id FROM ledger_entries e
        WHERE e.organization_id = m.organization_id
          AND (e.security_deposit_id IS NOT NULL OR e.deposit_deduction_id IS NOT NULL)
   );
ALTER TABLE cash_movements ENABLE TRIGGER trg_cash_movements_append_only;

ALTER TABLE ledger_entries DISABLE TRIGGER trg_ledger_entries_append_only;
DELETE FROM ledger_entries WHERE security_deposit_id IS NOT NULL OR deposit_deduction_id IS NOT NULL;
ALTER TABLE ledger_entries ENABLE TRIGGER trg_ledger_entries_append_only;

DROP INDEX IF EXISTS uq_cash_movements_entry;
CREATE UNIQUE INDEX uq_cash_movements_entry ON cash_movements (organization_id, ledger_entry_id)
    WHERE ledger_entry_id IS NOT NULL AND kind IN ('collection', 'refund');
ALTER TABLE cash_movements DROP CONSTRAINT chk_cash_movements_direction;
ALTER TABLE cash_movements ADD CONSTRAINT chk_cash_movements_direction CHECK (
    kind IN ('adjustment', 'reversal')
    OR (kind IN ('transfer_in', 'collection') AND direction = 'in')
    OR (kind IN ('transfer_out', 'supplier_payment', 'expense', 'refund') AND direction = 'out')
);
ALTER TABLE cash_movements DROP CONSTRAINT cash_movements_kind_check;
ALTER TABLE cash_movements ADD CONSTRAINT cash_movements_kind_check CHECK (kind IN (
    'transfer_in', 'transfer_out', 'collection', 'supplier_payment', 'expense', 'refund', 'adjustment', 'reversal'
));

DROP INDEX IF EXISTS idx_ledger_entries_organization_deposit;
DROP INDEX IF EXISTS uq_ledger_entries_deposit_deduction;
DROP INDEX IF EXISTS uq_ledger_entries_deposit_return;
DROP INDEX IF EXISTS uq_ledger_entries_deposit_hold;
ALTER TABLE ledger_entries
    DROP CONSTRAINT chk_ledger_entries_deposit_capture,
    DROP CONSTRAINT chk_ledger_entries_deposit,
    DROP CONSTRAINT fk_ledger_entries_deposit_deduction,
    DROP CONSTRAINT fk_ledger_entries_security_deposit,
    DROP COLUMN deposit_deduction_id,
    DROP COLUMN security_deposit_id;
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_source_type_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_source_type_check CHECK (source_type IN (
    'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
    'cash_movement', 'manual', 'collection'
));
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_kind_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_kind_check CHECK (kind IN (
    'accrual', 'collection', 'payment', 'refund', 'offset', 'adjustment', 'reversal'
));
ALTER TABLE ledger_entries ALTER COLUMN kind TYPE VARCHAR(12);

DROP TABLE IF EXISTS deposit_deductions;
DROP TABLE IF EXISTS security_deposits;
