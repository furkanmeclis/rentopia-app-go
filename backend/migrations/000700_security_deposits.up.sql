-- Security deposits and pre-authorizations (TEC-719, docs/design/deposits.md).
--
-- security_deposits: one deposit / provision taken for a rental (at the
-- handover, or later from the rental page). The amount defaults to the
-- product's deposit_amount (reservation product snapshot). Methods:
--   cash            money in a till (cash account kind cash)
--   bank_transfer   money in a bank account (cash account kind bank)
--   pos_preauth     a provision on a physical POS terminal, reference typed in
--   online_preauth  an iyzico pre-authorization (hosted checkout form)
-- Statuses: pending (online pre-auth not completed yet), held, and the
-- closed states released (pre-auth released, nothing captured), refunded
-- (money paid back, nothing captured), partially_captured (deductions were
-- captured, the remainder released / paid back) and cancelled (never held).
--
-- deposit_deductions: the line items captured from a deposit at its
-- resolution, each with a reason, an optional link to a rental charge
-- and / or an inspection damage, and an optional private document.
--
-- Accounting (never revenue, never a collection while held):
--   ledger_entries gains three kinds, all excluded from the receivable
--   balance because their purpose is 'deposit' (except the capture's
--   collection leg):
--     deposit_hold     credit, purpose deposit: cash / transfer received
--     deposit_return   debit, purpose deposit: the remainder paid back
--     deposit_capture  debit, purpose deposit: a deduction leaves the
--                      liability; its pair is a 'collection' credit
--                      (purpose charge / rent / general) applied to the
--                      charge, both naming the deduction
--   cash_movements gains deposit_in / deposit_out (the till / bank side of
--   deposit_hold / deposit_return), kept apart from collections and refunds.
--   A pre-authorization posts nothing until a deduction is captured.
--
-- Every index starts with organization_id; references are composite FKs.

CREATE TABLE security_deposits (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    -- The party the deposit belongs to (the rental's customer).
    customer_id BIGINT NOT NULL,
    -- The customer's ledger account (set when the first entry posts).
    account_id BIGINT NULL,
    -- Branch the deposit was taken at (branch scope).
    branch_id BIGINT NOT NULL,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    method VARCHAR(16) NOT NULL CHECK (method IN ('cash', 'bank_transfer', 'pos_preauth', 'online_preauth')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'held', 'partially_captured', 'released', 'refunded', 'cancelled'
    )),
    -- Till / bank account holding the money (cash, bank_transfer).
    cash_account_id BIGINT NULL,
    -- Physical POS: provision / authorization number from the slip.
    pos_reference VARCHAR(64) NOT NULL DEFAULT '',
    -- Online pre-auth: provider account and references (no card data).
    provider VARCHAR(16) NOT NULL DEFAULT '',
    integration_account_id BIGINT NULL,
    provider_token VARCHAR(128) NOT NULL DEFAULT '',
    provider_payment_id VARCHAR(64) NOT NULL DEFAULT '',
    payment_page_url TEXT NOT NULL DEFAULT '',
    provider_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Pre-auth hold end as told by the staff / provider (informational).
    authorization_expires_at TIMESTAMPTZ NULL,
    captured_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (captured_amount >= 0),
    returned_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (returned_amount >= 0),
    note TEXT NOT NULL DEFAULT '',
    held_at TIMESTAMPTZ NULL,
    resolved_at TIMESTAMPTZ NULL,
    -- Idempotency key of the resolution (a replay answers the same result).
    resolution_key VARCHAR(64) NOT NULL DEFAULT '',
    resolution_note TEXT NOT NULL DEFAULT '',
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    cancel_reason TEXT NOT NULL DEFAULT '',
    -- Customer notice of the deductions: no template mechanism exists yet,
    -- so the notice is recorded as pending with its content (hook for the
    -- notification templates, docs/design/deposits.md §6).
    customer_notice_status VARCHAR(12) NOT NULL DEFAULT 'none'
        CHECK (customer_notice_status IN ('none', 'pending', 'sent')),
    customer_notice JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_security_deposits_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_security_deposits_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_security_deposits_totals CHECK (captured_amount + returned_amount <= amount),
    CONSTRAINT chk_security_deposits_held CHECK (status IN ('pending', 'cancelled') OR held_at IS NOT NULL),
    CONSTRAINT chk_security_deposits_resolved CHECK (
        (status IN ('partially_captured', 'released', 'refunded', 'cancelled')) = (resolved_at IS NOT NULL)
    ),
    CONSTRAINT chk_security_deposits_cash CHECK (
        (method IN ('cash', 'bank_transfer')) = (cash_account_id IS NOT NULL)
    ),
    CONSTRAINT chk_security_deposits_pos CHECK (method <> 'pos_preauth' OR length(btrim(pos_reference)) > 0),
    CONSTRAINT fk_security_deposits_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id),
    CONSTRAINT fk_security_deposits_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_security_deposits_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_security_deposits_account
        FOREIGN KEY (organization_id, account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_security_deposits_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_security_deposits_cash_account
        FOREIGN KEY (organization_id, cash_account_id) REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_security_deposits_integration_account
        FOREIGN KEY (organization_id, integration_account_id) REFERENCES integration_accounts (organization_id, id)
);

-- One open deposit per rental.
CREATE UNIQUE INDEX uq_security_deposits_open ON security_deposits (organization_id, rental_id)
    WHERE status IN ('pending', 'held');
CREATE INDEX idx_security_deposits_organization_created ON security_deposits (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_security_deposits_organization_status ON security_deposits (organization_id, status, held_at);
CREATE INDEX idx_security_deposits_organization_branch ON security_deposits (organization_id, branch_id);
CREATE INDEX idx_security_deposits_organization_rental ON security_deposits (organization_id, rental_id);
CREATE INDEX idx_security_deposits_organization_customer ON security_deposits (organization_id, customer_id);

CREATE TRIGGER trg_security_deposits_updated_at
    BEFORE UPDATE ON security_deposits
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE deposit_deductions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    deposit_id BIGINT NOT NULL,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    -- What the captured money settles: a rental charge, the rent, or the
    -- account in general (the purpose of the collection entry).
    applies_to VARCHAR(8) NOT NULL DEFAULT 'charge' CHECK (applies_to IN ('charge', 'rent', 'general')),
    reason TEXT NOT NULL CHECK (length(btrim(reason)) >= 3),
    rental_charge_id BIGINT NULL,
    inspection_damage_id BIGINT NULL,
    document_object_key TEXT NULL,
    document_name VARCHAR(255) NOT NULL DEFAULT '',
    document_content_type VARCHAR(128) NOT NULL DEFAULT '',
    document_size_bytes BIGINT NOT NULL DEFAULT 0,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_deposit_deductions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_deposit_deductions_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_deposit_deductions_charge CHECK (applies_to <> 'charge' OR rental_charge_id IS NOT NULL),
    CONSTRAINT fk_deposit_deductions_deposit
        FOREIGN KEY (organization_id, deposit_id) REFERENCES security_deposits (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_deposit_deductions_charge
        FOREIGN KEY (organization_id, rental_charge_id) REFERENCES rental_charges (organization_id, id),
    -- A draft form deleted later must not take the deduction with it.
    CONSTRAINT fk_deposit_deductions_damage
        FOREIGN KEY (organization_id, inspection_damage_id) REFERENCES inspection_damages (organization_id, id)
        ON DELETE SET NULL (inspection_damage_id)
);

CREATE INDEX idx_deposit_deductions_organization_deposit ON deposit_deductions (organization_id, deposit_id, id);
CREATE INDEX idx_deposit_deductions_organization_charge ON deposit_deductions (organization_id, rental_charge_id)
    WHERE rental_charge_id IS NOT NULL;

CREATE TRIGGER trg_deposit_deductions_updated_at
    BEFORE UPDATE ON deposit_deductions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Ledger: deposit kinds, the security_deposit source and links. The kind
-- column grows to fit 'deposit_capture' (a metadata-only change).
ALTER TABLE ledger_entries ALTER COLUMN kind TYPE VARCHAR(16);
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_kind_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_kind_check CHECK (kind IN (
    'accrual', 'collection', 'payment', 'refund', 'offset', 'adjustment', 'reversal',
    'deposit_hold', 'deposit_return', 'deposit_capture'
));
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_source_type_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_source_type_check CHECK (source_type IN (
    'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
    'cash_movement', 'manual', 'security_deposit'
));
ALTER TABLE ledger_entries
    ADD COLUMN security_deposit_id BIGINT NULL,
    ADD COLUMN deposit_deduction_id BIGINT NULL,
    ADD CONSTRAINT fk_ledger_entries_security_deposit
        FOREIGN KEY (organization_id, security_deposit_id) REFERENCES security_deposits (organization_id, id),
    ADD CONSTRAINT fk_ledger_entries_deposit_deduction
        FOREIGN KEY (organization_id, deposit_deduction_id) REFERENCES deposit_deductions (organization_id, id),
    -- Deposit kinds always name their deposit; the deposit legs carry the
    -- deposit purpose so the receivable balance never includes them.
    ADD CONSTRAINT chk_ledger_entries_deposit CHECK (
        kind NOT IN ('deposit_hold', 'deposit_return', 'deposit_capture')
        OR (security_deposit_id IS NOT NULL AND purpose = 'deposit')
    ),
    ADD CONSTRAINT chk_ledger_entries_deposit_capture CHECK (
        kind <> 'deposit_capture' OR deposit_deduction_id IS NOT NULL
    );

-- Idempotent deposit postings: one hold and one return per deposit, one
-- capture pair per deduction (a replayed resolution posts nothing twice).
CREATE UNIQUE INDEX uq_ledger_entries_deposit_hold ON ledger_entries (organization_id, security_deposit_id)
    WHERE kind = 'deposit_hold';
CREATE UNIQUE INDEX uq_ledger_entries_deposit_return ON ledger_entries (organization_id, security_deposit_id)
    WHERE kind = 'deposit_return';
CREATE UNIQUE INDEX uq_ledger_entries_deposit_deduction ON ledger_entries (organization_id, deposit_deduction_id, kind)
    WHERE deposit_deduction_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_organization_deposit ON ledger_entries (organization_id, security_deposit_id)
    WHERE security_deposit_id IS NOT NULL;

-- Cash book: deposit money in / out, apart from collections and refunds.
ALTER TABLE cash_movements DROP CONSTRAINT cash_movements_kind_check;
ALTER TABLE cash_movements ADD CONSTRAINT cash_movements_kind_check CHECK (kind IN (
    'transfer_in', 'transfer_out', 'collection', 'supplier_payment', 'expense', 'refund', 'adjustment', 'reversal',
    'deposit_in', 'deposit_out'
));
ALTER TABLE cash_movements DROP CONSTRAINT chk_cash_movements_direction;
ALTER TABLE cash_movements ADD CONSTRAINT chk_cash_movements_direction CHECK (
    kind IN ('adjustment', 'reversal')
    OR (kind IN ('transfer_in', 'collection', 'deposit_in') AND direction = 'in')
    OR (kind IN ('transfer_out', 'supplier_payment', 'expense', 'refund', 'deposit_out') AND direction = 'out')
);
DROP INDEX uq_cash_movements_entry;
-- A collection / refund / deposit entry is recorded in a cash account once.
CREATE UNIQUE INDEX uq_cash_movements_entry ON cash_movements (organization_id, ledger_entry_id)
    WHERE ledger_entry_id IS NOT NULL AND kind IN ('collection', 'refund', 'deposit_in', 'deposit_out');
