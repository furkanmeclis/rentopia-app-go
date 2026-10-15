-- Finance ledger core (TEC-717, docs/design/ledger.md): customer and
-- supplier accounts ("cari") and their append-only entries.
--
-- accounts: one per customer (party_kind customer, customer_id) or, with
-- TEC-721, per supplier (party_kind supplier, supplier_id; the FK is added
-- there). The balance is never stored: it is the sum of the entries.
--
-- ledger_entries: append-only. A row is never updated or deleted (trigger
-- ledger_entries_append_only); a correction is a reversal (the exact
-- opposite of one entry, at most one per entry) or an adjustment, both with
-- a reason. Amounts are NUMERIC(14,2) > 0 with a direction (debit = the
-- party owes us more, credit = less); pkg/money in Go.
--
-- Single accrual (RT-A23): a firm debt (rent of a rental, an approved
-- rental charge) is accrued under an accrual_key ("rental:<id>:rent",
-- "rental_charge:<id>"). The trigger ledger_entries_accrual numbers the
-- accruals of one key (accrual_generation) and refuses a new accrual while
-- an earlier one is not reversed; uq_ledger_entries_accrual makes two
-- concurrent writers collide. So at any time a key has at most one live
-- accrual. Invoices (TEC-722) never post: they link to entries
-- (ledger_entry_links).
--
-- Every index starts with organization_id; references are composite FKs.

CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- CR-000001 (customer) / TD-000001 (supplier), unique per organization.
    code VARCHAR(32) NOT NULL,
    party_kind VARCHAR(12) NOT NULL CHECK (party_kind IN ('customer', 'supplier')),
    customer_id BIGINT NULL,
    -- TEC-721: suppliers table + FK (organization_id, supplier_id).
    supplier_id BIGINT NULL,
    -- Display name, refreshed from the party.
    name VARCHAR(255) NOT NULL,
    -- Branch the account was opened at (first posting); NULL = head office.
    branch_id BIGINT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- Days added to the posting date for the due date of a debt.
    payment_term_days INTEGER NOT NULL DEFAULT 0 CHECK (payment_term_days BETWEEN 0 AND 365),
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_accounts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_accounts_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_accounts_organization_code UNIQUE (organization_id, code),
    CONSTRAINT chk_accounts_party CHECK (
        (party_kind = 'customer' AND customer_id IS NOT NULL AND supplier_id IS NULL)
        OR (party_kind = 'supplier' AND customer_id IS NULL)
    ),
    CONSTRAINT fk_accounts_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_accounts_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id)
);

CREATE UNIQUE INDEX uq_accounts_organization_customer ON accounts (organization_id, customer_id) WHERE customer_id IS NOT NULL;
CREATE UNIQUE INDEX uq_accounts_organization_supplier ON accounts (organization_id, supplier_id) WHERE supplier_id IS NOT NULL;
CREATE INDEX idx_accounts_organization_created ON accounts (organization_id, created_at DESC);
CREATE INDEX idx_accounts_organization_branch ON accounts (organization_id, branch_id);

CREATE TRIGGER trg_accounts_updated_at
    BEFORE UPDATE ON accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Composite FK target for ledger entries naming a charge.
ALTER TABLE rental_charges ADD CONSTRAINT uq_rental_charges_organization_id UNIQUE (organization_id, id);

CREATE TABLE ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL,
    kind VARCHAR(12) NOT NULL CHECK (kind IN (
        'accrual', 'collection', 'payment', 'refund', 'offset', 'adjustment', 'reversal'
    )),
    direction VARCHAR(6) NOT NULL CHECK (direction IN ('debit', 'credit')),
    -- What the money is for: rent / charge (accruals), prepayment / deposit
    -- (collections, the deposit leg of an offset), general.
    purpose VARCHAR(12) NOT NULL DEFAULT 'general'
        CHECK (purpose IN ('rent', 'charge', 'prepayment', 'deposit', 'general')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- Posting date and due date (Europe/Istanbul calendar days).
    entry_date DATE NOT NULL,
    due_date DATE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    -- Source document: a generic reference plus typed FKs where it matters.
    source_type VARCHAR(24) NOT NULL CHECK (source_type IN (
        'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
        'cash_movement', 'manual'
    )),
    source_uuid UUID NULL,
    rental_id BIGINT NULL,
    reservation_id BIGINT NULL,
    rental_charge_id BIGINT NULL,
    payment_id BIGINT NULL,
    payment_refund_id BIGINT NULL,
    -- Single accrual: key + generation (set by the trigger).
    accrual_key VARCHAR(64) NULL,
    accrual_generation INTEGER NULL,
    -- Corrections and offsets.
    reverses_entry_id BIGINT NULL,
    adjusts_entry_id BIGINT NULL,
    offset_group UUID NULL,
    branch_id BIGINT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ledger_entries_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_ledger_entries_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_ledger_entries_reversal CHECK ((kind = 'reversal') = (reverses_entry_id IS NOT NULL)),
    CONSTRAINT chk_ledger_entries_accrual CHECK (
        (kind = 'accrual') = (accrual_key IS NOT NULL) AND (accrual_key IS NULL) = (accrual_generation IS NULL)
    ),
    CONSTRAINT chk_ledger_entries_offset CHECK ((kind = 'offset') = (offset_group IS NOT NULL)),
    CONSTRAINT chk_ledger_entries_reason CHECK (kind NOT IN ('reversal', 'adjustment') OR length(btrim(reason)) >= 3),
    CONSTRAINT chk_ledger_entries_due CHECK (due_date >= entry_date - 3650),
    CONSTRAINT fk_ledger_entries_account
        FOREIGN KEY (organization_id, account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_ledger_entries_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id),
    CONSTRAINT fk_ledger_entries_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_ledger_entries_charge
        FOREIGN KEY (organization_id, rental_charge_id) REFERENCES rental_charges (organization_id, id),
    CONSTRAINT fk_ledger_entries_payment
        FOREIGN KEY (organization_id, payment_id) REFERENCES payments (organization_id, id),
    CONSTRAINT fk_ledger_entries_reverses
        FOREIGN KEY (organization_id, reverses_entry_id) REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_ledger_entries_adjusts
        FOREIGN KEY (organization_id, adjusts_entry_id) REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_ledger_entries_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id)
);

ALTER TABLE payment_refunds ADD CONSTRAINT uq_payment_refunds_organization_id UNIQUE (organization_id, id);
ALTER TABLE ledger_entries ADD CONSTRAINT fk_ledger_entries_refund
    FOREIGN KEY (organization_id, payment_refund_id) REFERENCES payment_refunds (organization_id, id);

-- An entry is reversed at most once.
CREATE UNIQUE INDEX uq_ledger_entries_reverses ON ledger_entries (organization_id, reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;
-- RT-A23: one accrual per key and generation (see the trigger).
CREATE UNIQUE INDEX uq_ledger_entries_accrual ON ledger_entries (organization_id, accrual_key, accrual_generation)
    WHERE kind = 'accrual';
-- Idempotent posting from payments: one collection per payment, one refund
-- entry per provider / manual refund.
CREATE UNIQUE INDEX uq_ledger_entries_payment ON ledger_entries (organization_id, payment_id)
    WHERE kind = 'collection' AND payment_id IS NOT NULL;
CREATE UNIQUE INDEX uq_ledger_entries_payment_refund ON ledger_entries (organization_id, payment_refund_id)
    WHERE kind = 'refund' AND payment_refund_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_organization_account ON ledger_entries (organization_id, account_id, entry_date, id);
CREATE INDEX idx_ledger_entries_organization_created ON ledger_entries (organization_id, created_at DESC);
CREATE INDEX idx_ledger_entries_organization_rental ON ledger_entries (organization_id, rental_id) WHERE rental_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_organization_reservation ON ledger_entries (organization_id, reservation_id)
    WHERE reservation_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_organization_branch ON ledger_entries (organization_id, branch_id, entry_date);
CREATE INDEX idx_ledger_entries_organization_due ON ledger_entries (organization_id, due_date);

-- Append-only. Direct UPDATE / DELETE fail; a delete cascading from the
-- organization (pg_trigger_depth() > 1: fired by the foreign key action)
-- is allowed so an organization can still be removed.
CREATE FUNCTION ledger_entries_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND pg_trigger_depth() > 1 THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'ledger entries are append-only (post a reversal or an adjustment with a reason)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'ledger_entries_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_entries_append_only
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_append_only();

-- Single accrual and reversal rules, checked on insert.
CREATE FUNCTION ledger_entries_insert_rules() RETURNS trigger AS $$
DECLARE
    orig ledger_entries%ROWTYPE;
    prior INTEGER;
    live INTEGER;
BEGIN
    IF NEW.kind = 'accrual' THEN
        SELECT count(*),
               count(*) FILTER (WHERE NOT EXISTS (
                   SELECT 1 FROM ledger_entries r
                   WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id
               ))
          INTO prior, live
          FROM ledger_entries e
         WHERE e.organization_id = NEW.organization_id AND e.kind = 'accrual' AND e.accrual_key = NEW.accrual_key;
        IF live > 0 THEN
            RAISE EXCEPTION 'accrual % is already posted (reverse it before posting a new one)', NEW.accrual_key
                USING ERRCODE = 'unique_violation', CONSTRAINT = 'uq_ledger_entries_accrual';
        END IF;
        NEW.accrual_generation := prior + 1;
    ELSIF NEW.kind = 'reversal' THEN
        SELECT * INTO orig FROM ledger_entries
         WHERE organization_id = NEW.organization_id AND id = NEW.reverses_entry_id;
        IF NOT FOUND OR orig.kind = 'reversal' OR orig.account_id <> NEW.account_id
           OR orig.direction = NEW.direction OR orig.amount <> NEW.amount OR orig.currency <> NEW.currency THEN
            RAISE EXCEPTION 'a reversal is the exact opposite of one entry of the same account'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_ledger_entries_reversal';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_entries_insert_rules
    BEFORE INSERT ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_insert_rules();

-- Documents that point at entries without posting (an invoice of TEC-722
-- links the accruals it bills; RT-A23). Append-only like the entries.
CREATE TABLE ledger_entry_links (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    entry_id BIGINT NOT NULL,
    document_type VARCHAR(16) NOT NULL CHECK (document_type IN ('invoice', 'statement', 'toll_charge')),
    -- The document's reference (an invoice number / uuid until TEC-722
    -- adds its table and FK).
    document_ref VARCHAR(64) NOT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ledger_entry_links UNIQUE (organization_id, entry_id, document_type, document_ref),
    CONSTRAINT fk_ledger_entry_links_entry
        FOREIGN KEY (organization_id, entry_id) REFERENCES ledger_entries (organization_id, id)
);

CREATE INDEX idx_ledger_entry_links_organization_document ON ledger_entry_links (organization_id, document_type, document_ref);
