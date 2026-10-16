-- Data-loss note: drops every supplier card, supplier invoice (with its
-- lines), supplier payment and payment allocation. The ledger entries
-- posted for supplier invoices and payments, the cash movements of supplier
-- payments and the expense rows generated from invoices are deleted too
-- (the append-only triggers are suspended for that), so supplier balances
-- and the cash balances they touched change. vehicles.supplier_id,
-- expenses.supplier_id / supplier_invoice_id and the accounts → suppliers
-- FK are dropped; the free-text supplier names stay. Invoice attachments
-- stay in storage (unreferenced).

ALTER TABLE ledger_entries DISABLE TRIGGER trg_ledger_entries_append_only;
ALTER TABLE cash_movements DISABLE TRIGGER trg_cash_movements_append_only;

-- Cash movements of supplier payments (and their reversals).
DELETE FROM cash_movements m
WHERE m.reverses_movement_id IN (SELECT p.cash_movement_id FROM supplier_payments p WHERE p.cash_movement_id IS NOT NULL);
UPDATE supplier_payments SET cash_movement_id = NULL, ledger_entry_id = NULL;
DELETE FROM cash_movements m
WHERE m.kind = 'supplier_payment' AND m.ledger_entry_id IN (
    SELECT e.id FROM ledger_entries e WHERE e.source_type IN ('supplier_invoice', 'supplier_payment')
);
-- Ledger entries of supplier invoices and payments (reversals first).
DELETE FROM ledger_entries e
WHERE e.reverses_entry_id IS NOT NULL AND e.source_type IN ('supplier_invoice', 'supplier_payment');
DELETE FROM ledger_entry_links l
WHERE l.entry_id IN (SELECT e.id FROM ledger_entries e WHERE e.source_type IN ('supplier_invoice', 'supplier_payment'));
DELETE FROM ledger_entries e WHERE e.source_type IN ('supplier_invoice', 'supplier_payment');

ALTER TABLE cash_movements ENABLE TRIGGER trg_cash_movements_append_only;
ALTER TABLE ledger_entries ENABLE TRIGGER trg_ledger_entries_append_only;

-- Expense rows generated from invoices.
DELETE FROM expenses WHERE supplier_invoice_id IS NOT NULL;

DROP INDEX IF EXISTS idx_vehicles_organization_supplier;
ALTER TABLE vehicles DROP CONSTRAINT IF EXISTS fk_vehicles_supplier;
ALTER TABLE vehicles DROP COLUMN IF EXISTS supplier_id;

DROP INDEX IF EXISTS idx_expenses_organization_supplier;
DROP INDEX IF EXISTS uq_expenses_supplier_invoice;
ALTER TABLE expenses DROP CONSTRAINT IF EXISTS fk_expenses_supplier_invoice;
ALTER TABLE expenses DROP CONSTRAINT IF EXISTS fk_expenses_supplier;
ALTER TABLE expenses DROP COLUMN IF EXISTS supplier_invoice_id;
ALTER TABLE expenses DROP COLUMN IF EXISTS supplier_id;

DROP INDEX IF EXISTS idx_ledger_entries_organization_supplier_invoice;
DROP INDEX IF EXISTS uq_ledger_entries_supplier_payment;
-- Back to the union before 000105: 000096 values, 'collection' (000101) and
-- 'security_deposit' (deposits, 000103).
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_source_type_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_source_type_check CHECK (source_type IN (
    'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
    'cash_movement', 'manual', 'collection', 'security_deposit'
));
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS fk_ledger_entries_supplier_payment;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS fk_ledger_entries_supplier_invoice;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS supplier_payment_id;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS supplier_invoice_id;

DROP TABLE IF EXISTS supplier_payment_allocations;
DROP TABLE IF EXISTS supplier_payments;
DROP TABLE IF EXISTS supplier_invoice_lines;
DROP TABLE IF EXISTS supplier_invoices;

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS fk_accounts_supplier;
DROP TABLE IF EXISTS suppliers;
