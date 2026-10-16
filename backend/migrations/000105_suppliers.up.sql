-- Suppliers, supplier invoices and supplier payments (TEC-721,
-- docs/design/suppliers.md). Builds on the finance ledger (000096) and the
-- cash book (000097):
--
-- suppliers: the supplier card (unvan, VKN / TCKN, tax office, contacts,
-- billing data, IBAN, payment terms, type, status). Archived instead of
-- deleted; the code (TED-000001) is unique per organization.
--
-- supplier_invoices (gider faturası): header totals (net / VAT / total) with
-- optional lines carrying their own VAT rate. Creating an invoice posts a
-- payable accrual (credit) on the supplier's ledger account under the key
-- supplier_invoice:<id> (single accrual, RT-A23 trigger); voiding reverses
-- it. The invoice is also the source of one expense row
-- (expenses.supplier_invoice_id), so expense reports keep covering every
-- cost without a second write path.
--
-- supplier_payments: money out of a cash / bank account, one cash movement
-- (kind supplier_payment) and one ledger entry (kind payment, debit) each,
-- allocated to one or more open invoices (supplier_payment_allocations);
-- an unallocated remainder is an advance on the account.
--
-- Free-text supplier names (vehicles.supplier_name, expenses.supplier_name)
-- become supplier cards: one card per distinct name (trimmed, case
-- insensitive) and organization, linked through the new supplier_id
-- columns. The names stay as they are (backward compatibility).
--
-- Every index starts with organization_id; references are composite FKs.

CREATE TABLE suppliers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    title VARCHAR(255) NOT NULL,
    supplier_type VARCHAR(20) NOT NULL DEFAULT 'other'
        CHECK (supplier_type IN ('service', 'insurance', 'vehicle_supplier', 'parts', 'fuel', 'other')),
    -- VKN (10 digits) or TCKN (11 digits), checksum validated by the API.
    tax_number VARCHAR(11) NOT NULL DEFAULT '',
    tax_office VARCHAR(128) NOT NULL DEFAULT '',
    phone VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    website VARCHAR(255) NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    city VARCHAR(64) NOT NULL DEFAULT '',
    -- Billing: address / e-mail invoices are sent from, e-invoice taxpayer.
    billing_address TEXT NOT NULL DEFAULT '',
    billing_email VARCHAR(255) NOT NULL DEFAULT '',
    e_invoice BOOLEAN NOT NULL DEFAULT FALSE,
    bank_name VARCHAR(128) NOT NULL DEFAULT '',
    iban VARCHAR(34) NOT NULL DEFAULT '',
    -- Default due date of an invoice: invoice date + payment_term_days.
    payment_term_days INTEGER NOT NULL DEFAULT 30 CHECK (payment_term_days BETWEEN 0 AND 365),
    -- [{name, role, phone, email}], at most 10.
    contacts JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(contacts) = 'array'),
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_suppliers_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_suppliers_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_suppliers_organization_code UNIQUE (organization_id, code),
    CONSTRAINT chk_suppliers_tax_number CHECK (tax_number = '' OR tax_number ~ '^[0-9]{10,11}$')
);

CREATE INDEX idx_suppliers_organization_created ON suppliers (organization_id, created_at DESC);
CREATE INDEX idx_suppliers_organization_title ON suppliers (organization_id, lower(title));
CREATE INDEX idx_suppliers_organization_tax ON suppliers (organization_id, tax_number) WHERE tax_number <> '';

CREATE TRIGGER trg_suppliers_updated_at
    BEFORE UPDATE ON suppliers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The supplier account of the ledger points at its card.
ALTER TABLE accounts ADD CONSTRAINT fk_accounts_supplier
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id);

CREATE TABLE supplier_invoices (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL,
    -- The supplier's ledger account the accrual is posted to.
    account_id BIGINT NOT NULL,
    invoice_no VARCHAR(64) NOT NULL,
    invoice_date DATE NOT NULL,
    due_date DATE NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    category_id BIGINT NOT NULL,
    branch_id BIGINT NULL,
    vehicle_id BIGINT NULL,
    description TEXT NOT NULL DEFAULT '',
    net_amount NUMERIC(14, 2) NOT NULL CHECK (net_amount >= 0),
    vat_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (vat_amount >= 0),
    total_amount NUMERIC(14, 2) NOT NULL CHECK (total_amount > 0),
    -- Sum of the live allocations (kept in step by the payment writes).
    paid_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (paid_amount >= 0),
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'partially_paid', 'paid', 'void')),
    attachment_object_key TEXT NULL,
    attachment_name VARCHAR(255) NOT NULL DEFAULT '',
    attachment_content_type VARCHAR(128) NOT NULL DEFAULT '',
    attachment_size_bytes BIGINT NOT NULL DEFAULT 0,
    void_reason TEXT NOT NULL DEFAULT '',
    voided_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_supplier_invoices_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_supplier_invoices_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_supplier_invoices_total CHECK (total_amount = net_amount + vat_amount),
    CONSTRAINT chk_supplier_invoices_paid CHECK (paid_amount <= total_amount),
    CONSTRAINT chk_supplier_invoices_due CHECK (due_date >= invoice_date),
    CONSTRAINT chk_supplier_invoices_void CHECK ((status = 'void') = (voided_at IS NOT NULL)),
    CONSTRAINT fk_supplier_invoices_supplier
        FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id),
    CONSTRAINT fk_supplier_invoices_account
        FOREIGN KEY (organization_id, account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_supplier_invoices_category
        FOREIGN KEY (organization_id, category_id) REFERENCES expense_categories (organization_id, id),
    CONSTRAINT fk_supplier_invoices_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_supplier_invoices_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id)
);

-- An invoice number is used once per supplier (void invoices excepted).
CREATE UNIQUE INDEX uq_supplier_invoices_number ON supplier_invoices (organization_id, supplier_id, lower(invoice_no))
    WHERE status <> 'void';
CREATE INDEX idx_supplier_invoices_organization_supplier ON supplier_invoices (organization_id, supplier_id, invoice_date DESC);
CREATE INDEX idx_supplier_invoices_organization_due ON supplier_invoices (organization_id, due_date)
    WHERE status IN ('open', 'partially_paid');
CREATE INDEX idx_supplier_invoices_organization_branch ON supplier_invoices (organization_id, branch_id);
CREATE INDEX idx_supplier_invoices_organization_date ON supplier_invoices (organization_id, invoice_date DESC, id DESC);

CREATE TRIGGER trg_supplier_invoices_updated_at
    BEFORE UPDATE ON supplier_invoices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- VAT breakdown: one row per line (an invoice without lines carries its
-- totals in the header only).
CREATE TABLE supplier_invoice_lines (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    invoice_id BIGINT NOT NULL,
    line_no INTEGER NOT NULL CHECK (line_no > 0),
    description TEXT NOT NULL DEFAULT '',
    net_amount NUMERIC(14, 2) NOT NULL CHECK (net_amount >= 0),
    -- Percent (20.00 = 20 %).
    vat_rate NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (vat_rate >= 0 AND vat_rate <= 100),
    vat_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (vat_amount >= 0),
    total_amount NUMERIC(14, 2) NOT NULL CHECK (total_amount >= 0),
    CONSTRAINT uq_supplier_invoice_lines UNIQUE (organization_id, invoice_id, line_no),
    CONSTRAINT chk_supplier_invoice_lines_total CHECK (total_amount = net_amount + vat_amount),
    CONSTRAINT fk_supplier_invoice_lines_invoice
        FOREIGN KEY (organization_id, invoice_id) REFERENCES supplier_invoices (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE supplier_payments (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    cash_account_id BIGINT NOT NULL,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    payment_date DATE NOT NULL,
    method VARCHAR(16) NOT NULL DEFAULT 'bank_transfer'
        CHECK (method IN ('cash', 'bank_transfer', 'card', 'cheque', 'other')),
    reference VARCHAR(64) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    -- Copied from the cash account (branch scope without a join).
    branch_id BIGINT NULL,
    ledger_entry_id BIGINT NULL,
    cash_movement_id BIGINT NULL,
    status VARCHAR(8) NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'void')),
    void_reason TEXT NOT NULL DEFAULT '',
    voided_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_supplier_payments_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_supplier_payments_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_supplier_payments_void CHECK ((status = 'void') = (voided_at IS NOT NULL)),
    CONSTRAINT fk_supplier_payments_supplier
        FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id),
    CONSTRAINT fk_supplier_payments_account
        FOREIGN KEY (organization_id, account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_supplier_payments_cash_account
        FOREIGN KEY (organization_id, cash_account_id) REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_supplier_payments_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_supplier_payments_entry
        FOREIGN KEY (organization_id, ledger_entry_id) REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_supplier_payments_movement
        FOREIGN KEY (organization_id, cash_movement_id) REFERENCES cash_movements (organization_id, id)
);

CREATE INDEX idx_supplier_payments_organization_supplier ON supplier_payments (organization_id, supplier_id, payment_date DESC);
CREATE INDEX idx_supplier_payments_organization_date ON supplier_payments (organization_id, payment_date DESC, id DESC);
CREATE INDEX idx_supplier_payments_organization_branch ON supplier_payments (organization_id, branch_id);
CREATE UNIQUE INDEX uq_supplier_payments_movement ON supplier_payments (organization_id, cash_movement_id)
    WHERE cash_movement_id IS NOT NULL;

CREATE TRIGGER trg_supplier_payments_updated_at
    BEFORE UPDATE ON supplier_payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE supplier_payment_allocations (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    payment_id BIGINT NOT NULL,
    invoice_id BIGINT NOT NULL,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_supplier_payment_allocations UNIQUE (organization_id, payment_id, invoice_id),
    CONSTRAINT fk_supplier_payment_allocations_payment
        FOREIGN KEY (organization_id, payment_id) REFERENCES supplier_payments (organization_id, id),
    CONSTRAINT fk_supplier_payment_allocations_invoice
        FOREIGN KEY (organization_id, invoice_id) REFERENCES supplier_invoices (organization_id, id)
);

CREATE INDEX idx_supplier_payment_allocations_invoice ON supplier_payment_allocations (organization_id, invoice_id);

-- Ledger: typed sources for supplier invoices and payments.
ALTER TABLE ledger_entries ADD COLUMN supplier_invoice_id BIGINT NULL;
ALTER TABLE ledger_entries ADD COLUMN supplier_payment_id BIGINT NULL;
ALTER TABLE ledger_entries ADD CONSTRAINT fk_ledger_entries_supplier_invoice
    FOREIGN KEY (organization_id, supplier_invoice_id) REFERENCES supplier_invoices (organization_id, id);
ALTER TABLE ledger_entries ADD CONSTRAINT fk_ledger_entries_supplier_payment
    FOREIGN KEY (organization_id, supplier_payment_id) REFERENCES supplier_payments (organization_id, id);
-- The full union: 000096 values, 'collection' (000101), 'security_deposit'
-- (deposits, 000103) and the two supplier sources.
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_source_type_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_source_type_check CHECK (source_type IN (
    'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
    'cash_movement', 'manual', 'collection', 'security_deposit', 'supplier_invoice', 'supplier_payment'
));
-- One payment entry per supplier payment (idempotent posting).
CREATE UNIQUE INDEX uq_ledger_entries_supplier_payment ON ledger_entries (organization_id, supplier_payment_id)
    WHERE kind = 'payment' AND supplier_payment_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_organization_supplier_invoice ON ledger_entries (organization_id, supplier_invoice_id)
    WHERE supplier_invoice_id IS NOT NULL;

-- Expenses: the supplier (FK next to the kept free-text name) and the
-- invoice an expense row was generated from (one row per invoice).
ALTER TABLE expenses ADD COLUMN supplier_id BIGINT NULL;
ALTER TABLE expenses ADD COLUMN supplier_invoice_id BIGINT NULL;
ALTER TABLE expenses ADD CONSTRAINT fk_expenses_supplier
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id);
ALTER TABLE expenses ADD CONSTRAINT fk_expenses_supplier_invoice
    FOREIGN KEY (organization_id, supplier_invoice_id) REFERENCES supplier_invoices (organization_id, id);
CREATE UNIQUE INDEX uq_expenses_supplier_invoice ON expenses (organization_id, supplier_invoice_id)
    WHERE supplier_invoice_id IS NOT NULL;
CREATE INDEX idx_expenses_organization_supplier ON expenses (organization_id, supplier_id) WHERE supplier_id IS NOT NULL;

-- Vehicles sourced externally (ownership 'supplied') link to their supplier.
ALTER TABLE vehicles ADD COLUMN supplier_id BIGINT NULL;
ALTER TABLE vehicles ADD CONSTRAINT fk_vehicles_supplier
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id);
CREATE INDEX idx_vehicles_organization_supplier ON vehicles (organization_id, supplier_id) WHERE supplier_id IS NOT NULL;

-- Free-text supplier names become supplier cards (one per distinct name and
-- organization). A name used for a supplied vehicle makes a vehicle
-- supplier, any other name an "other" supplier.
WITH names AS (
    SELECT v.organization_id, btrim(v.supplier_name) AS title, 1 AS vehicle
    FROM vehicles v
    WHERE btrim(v.supplier_name) <> ''
    UNION ALL
    SELECT x.organization_id, btrim(x.supplier_name), 0
    FROM expenses x
    WHERE btrim(x.supplier_name) <> ''
), grouped AS (
    SELECT organization_id, lower(title) AS norm, min(title) AS title, max(vehicle) AS vehicle
    FROM names
    GROUP BY organization_id, lower(title)
), numbered AS (
    SELECT g.*, row_number() OVER (PARTITION BY g.organization_id ORDER BY g.norm) AS n
    FROM grouped g
)
INSERT INTO suppliers (organization_id, code, title, supplier_type, note)
SELECT organization_id, 'TED-' || lpad(n::text, 6, '0'), left(title, 255),
    CASE WHEN vehicle = 1 THEN 'vehicle_supplier' ELSE 'other' END,
    'Serbest metin tedarikçi adından oluşturuldu.'
FROM numbered;

UPDATE vehicles v
SET supplier_id = s.id
FROM suppliers s
WHERE s.organization_id = v.organization_id
  AND lower(s.title) = lower(btrim(v.supplier_name))
  AND btrim(v.supplier_name) <> '';

UPDATE expenses x
SET supplier_id = s.id
FROM suppliers s
WHERE s.organization_id = x.organization_id
  AND lower(s.title) = lower(btrim(x.supplier_name))
  AND btrim(x.supplier_name) <> '';
