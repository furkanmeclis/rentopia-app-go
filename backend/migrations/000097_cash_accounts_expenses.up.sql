-- Cash and bank accounts, movements, expense categories and expenses
-- (TEC-720, docs/design/cash-accounts.md). Record based: there is no live
-- bank API; a bank statement is imported or entered.
--
-- cash_accounts: a till (kind cash) or a bank account (kind bank) of the
-- organization (branch_id NULL = head office) or of a branch, with an
-- opening balance. The balance is opening + movements in - movements out.
--
-- cash_movements: append-only like ledger entries. Only the category (a
-- classification, never an amount) may change afterwards; a wrong movement
-- is reversed (reverses_movement_id, at most once). A transfer is two rows
-- sharing transfer_group, written in one transaction. A movement may point
-- at the ledger entry it settles (a collection deposited into the account)
-- and at the expense it paid.
--
-- expense_categories: per organization, seeded with the defaults
-- (maintenance, insurance, repair, fuel, cleaning, vehicle procurement,
-- operations, other) for every existing organization here and for new ones
-- by the organization hook; organizations rename / add / deactivate them.
--
-- expenses: amount with VAT, date, category, optional branch / vehicle and
-- the cash account that paid it; the receipt / invoice file lives in the
-- private bucket under organizations/{org}/expenses/{expense}/ and is
-- streamed by the API.

CREATE TABLE cash_accounts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('cash', 'bank')),
    branch_id BIGINT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    opening_balance NUMERIC(14, 2) NOT NULL DEFAULT 0,
    opening_date DATE NOT NULL DEFAULT CURRENT_DATE,
    bank_name VARCHAR(128) NOT NULL DEFAULT '',
    iban VARCHAR(34) NOT NULL DEFAULT '',
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cash_accounts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_accounts_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_cash_accounts_organization_code UNIQUE (organization_id, code),
    CONSTRAINT fk_cash_accounts_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id)
);

CREATE INDEX idx_cash_accounts_organization_branch ON cash_accounts (organization_id, branch_id);
CREATE INDEX idx_cash_accounts_organization_created ON cash_accounts (organization_id, created_at DESC);

CREATE TRIGGER trg_cash_accounts_updated_at
    BEFORE UPDATE ON cash_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE expense_categories (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    -- Seeded default (renamable, not removable).
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expense_categories_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_expense_categories_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_expense_categories_organization_code UNIQUE (organization_id, code)
);

CREATE TRIGGER trg_expense_categories_updated_at
    BEFORE UPDATE ON expense_categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Defaults for every existing organization (new ones: organization hook).
INSERT INTO expense_categories (organization_id, code, name, is_system, sort_order)
SELECT o.id, d.code, d.name, TRUE, d.sort_order
FROM organizations o
CROSS JOIN (VALUES
    ('maintenance', 'Bakım', 10),
    ('insurance', 'Sigorta', 20),
    ('repair', 'Onarım', 30),
    ('fuel', 'Yakıt', 40),
    ('cleaning', 'Temizlik', 50),
    ('vehicle_procurement', 'Araç tedariki', 60),
    ('operations', 'Operasyon', 70),
    ('other', 'Diğer', 80)
) AS d (code, name, sort_order)
ON CONFLICT (organization_id, code) DO NOTHING;

CREATE TABLE expenses (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL,
    expense_date DATE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    supplier_name VARCHAR(255) NOT NULL DEFAULT '',
    document_no VARCHAR(64) NOT NULL DEFAULT '',
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    net_amount NUMERIC(14, 2) NOT NULL CHECK (net_amount >= 0),
    -- Percent (20.00 = 20 %).
    vat_rate NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (vat_rate >= 0 AND vat_rate <= 100),
    vat_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (vat_amount >= 0),
    total_amount NUMERIC(14, 2) NOT NULL CHECK (total_amount > 0),
    branch_id BIGINT NULL,
    vehicle_id BIGINT NULL,
    -- The account that paid it (a movement kind expense), if any.
    cash_account_id BIGINT NULL,
    attachment_object_key TEXT NULL,
    attachment_name VARCHAR(255) NOT NULL DEFAULT '',
    attachment_content_type VARCHAR(128) NOT NULL DEFAULT '',
    attachment_size_bytes BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(12) NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded', 'void')),
    void_reason TEXT NOT NULL DEFAULT '',
    voided_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expenses_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_expenses_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_expenses_total CHECK (total_amount = net_amount + vat_amount),
    CONSTRAINT chk_expenses_void CHECK ((status = 'void') = (voided_at IS NOT NULL)),
    CONSTRAINT fk_expenses_category
        FOREIGN KEY (organization_id, category_id) REFERENCES expense_categories (organization_id, id),
    CONSTRAINT fk_expenses_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_expenses_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id),
    CONSTRAINT fk_expenses_cash_account
        FOREIGN KEY (organization_id, cash_account_id) REFERENCES cash_accounts (organization_id, id)
);

CREATE INDEX idx_expenses_organization_date ON expenses (organization_id, expense_date DESC, id DESC);
CREATE INDEX idx_expenses_organization_category ON expenses (organization_id, category_id);
CREATE INDEX idx_expenses_organization_branch ON expenses (organization_id, branch_id);
CREATE INDEX idx_expenses_organization_vehicle ON expenses (organization_id, vehicle_id) WHERE vehicle_id IS NOT NULL;

CREATE TRIGGER trg_expenses_updated_at
    BEFORE UPDATE ON expenses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cash_movements (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    cash_account_id BIGINT NOT NULL,
    kind VARCHAR(20) NOT NULL CHECK (kind IN (
        'transfer_in', 'transfer_out', 'collection', 'supplier_payment', 'expense', 'refund', 'adjustment', 'reversal'
    )),
    direction VARCHAR(4) NOT NULL CHECK (direction IN ('in', 'out')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    movement_date DATE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    -- Copied from the account (branch scope without a join).
    branch_id BIGINT NULL,
    -- The one mutable column: a classification (bulk "set category").
    category_id BIGINT NULL,
    ledger_entry_id BIGINT NULL,
    expense_id BIGINT NULL,
    -- TEC-721: the supplier account a supplier payment settles.
    supplier_account_id BIGINT NULL,
    transfer_group UUID NULL,
    counter_account_id BIGINT NULL,
    reverses_movement_id BIGINT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cash_movements_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_movements_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_cash_movements_direction CHECK (
        kind IN ('adjustment', 'reversal')
        OR (kind IN ('transfer_in', 'collection') AND direction = 'in')
        OR (kind IN ('transfer_out', 'supplier_payment', 'expense', 'refund') AND direction = 'out')
    ),
    CONSTRAINT chk_cash_movements_transfer CHECK (
        (kind IN ('transfer_in', 'transfer_out')) = (transfer_group IS NOT NULL AND counter_account_id IS NOT NULL)
    ),
    CONSTRAINT chk_cash_movements_reversal CHECK ((kind = 'reversal') = (reverses_movement_id IS NOT NULL)),
    CONSTRAINT chk_cash_movements_reason CHECK (kind NOT IN ('reversal', 'adjustment') OR length(btrim(reason)) >= 3),
    CONSTRAINT fk_cash_movements_account
        FOREIGN KEY (organization_id, cash_account_id) REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_movements_counter
        FOREIGN KEY (organization_id, counter_account_id) REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_movements_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_cash_movements_category
        FOREIGN KEY (organization_id, category_id) REFERENCES expense_categories (organization_id, id),
    CONSTRAINT fk_cash_movements_entry
        FOREIGN KEY (organization_id, ledger_entry_id) REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_cash_movements_expense
        FOREIGN KEY (organization_id, expense_id) REFERENCES expenses (organization_id, id),
    CONSTRAINT fk_cash_movements_supplier_account
        FOREIGN KEY (organization_id, supplier_account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_cash_movements_reverses
        FOREIGN KEY (organization_id, reverses_movement_id) REFERENCES cash_movements (organization_id, id)
);

CREATE UNIQUE INDEX uq_cash_movements_reverses ON cash_movements (organization_id, reverses_movement_id)
    WHERE reverses_movement_id IS NOT NULL;
-- A collection entry is deposited once.
CREATE UNIQUE INDEX uq_cash_movements_entry ON cash_movements (organization_id, ledger_entry_id)
    WHERE ledger_entry_id IS NOT NULL AND kind IN ('collection', 'refund');
-- An expense is paid once.
CREATE UNIQUE INDEX uq_cash_movements_expense ON cash_movements (organization_id, expense_id)
    WHERE expense_id IS NOT NULL AND kind = 'expense';
CREATE INDEX idx_cash_movements_organization_account ON cash_movements (organization_id, cash_account_id, movement_date, id);
CREATE INDEX idx_cash_movements_organization_created ON cash_movements (organization_id, created_at DESC);
CREATE INDEX idx_cash_movements_organization_branch ON cash_movements (organization_id, branch_id, movement_date);
CREATE INDEX idx_cash_movements_organization_transfer ON cash_movements (organization_id, transfer_group)
    WHERE transfer_group IS NOT NULL;

-- Append-only except the category; organization deletion cascades.
CREATE FUNCTION cash_movements_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
    ELSIF (to_jsonb(NEW) - 'category_id') = (to_jsonb(OLD) - 'category_id') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'cash movements are append-only (post a reversal with a reason)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'cash_movements_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cash_movements_append_only
    BEFORE UPDATE OR DELETE ON cash_movements
    FOR EACH ROW EXECUTE FUNCTION cash_movements_append_only();

-- A reversal is the exact opposite of one movement of the same account.
CREATE FUNCTION cash_movements_insert_rules() RETURNS trigger AS $$
DECLARE
    orig cash_movements%ROWTYPE;
BEGIN
    IF NEW.kind = 'reversal' THEN
        SELECT * INTO orig FROM cash_movements
         WHERE organization_id = NEW.organization_id AND id = NEW.reverses_movement_id;
        IF NOT FOUND OR orig.kind = 'reversal' OR orig.cash_account_id <> NEW.cash_account_id
           OR orig.direction = NEW.direction OR orig.amount <> NEW.amount OR orig.currency <> NEW.currency THEN
            RAISE EXCEPTION 'a reversal is the exact opposite of one movement of the same account'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_cash_movements_reversal';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cash_movements_insert_rules
    BEFORE INSERT ON cash_movements
    FOR EACH ROW EXECUTE FUNCTION cash_movements_insert_rules();
