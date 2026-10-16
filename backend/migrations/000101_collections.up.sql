-- Collections, allocation to open items and refunds (TEC-718,
-- docs/design/collections.md). Builds on the ledger (000096) and the cash
-- book (000097); nothing here posts on its own.
--
-- collections: one receipt of money from a party ("tahsilat"): cash, bank
-- transfer / EFT, physical POS or an online payment (iyzico). It is the
-- document behind exactly one ledger collection entry (ledger_entry_id)
-- and, when the money landed in a till / bank / clearing account, one cash
-- movement. Online collections keep gross, provider commission and net
-- apart; the commission is booked as an expense (category
-- payment_commission) when the net settlement is recorded in the
-- provider's settlement account (settlement_status pending -> settled).
-- The row is mutable only in its document state (refunded amount, status,
-- settlement, void); money facts live in the append-only ledger / cash
-- book.
--
-- collection_allocations: append-only allocate / release rows that apply a
-- collection to open items (live accrual entries: rent and charges). The
-- net allocation of a pair is allocate - release. The insert trigger locks
-- the account row and refuses over-allocation of the item or of the
-- collection (amount - refunded), so concurrent writers cannot allocate the
-- same money twice. The unallocated part of a collection is the party's
-- credit, allocatable later.
--
-- cash_accounts: kind clearing (a provider balance such as iyzico) and
-- provider (the account that receives a provider's net settlement; one
-- active per organization and provider).
--
-- ledger_entries: collection_id (a refund entry names the collection it
-- returns) and source_type collection.

-- Cash accounts: clearing kind and the provider settlement account.
ALTER TABLE cash_accounts DROP CONSTRAINT cash_accounts_kind_check;
ALTER TABLE cash_accounts ADD CONSTRAINT cash_accounts_kind_check CHECK (kind IN ('cash', 'bank', 'clearing'));
ALTER TABLE cash_accounts ADD COLUMN provider VARCHAR(24) NULL;
ALTER TABLE cash_accounts ADD CONSTRAINT chk_cash_accounts_provider CHECK (
    provider IS NULL OR (provider IN ('iyzico') AND kind IN ('bank', 'clearing'))
);
CREATE UNIQUE INDEX uq_cash_accounts_organization_provider ON cash_accounts (organization_id, provider)
    WHERE provider IS NOT NULL AND status = 'active';

-- The expense category of provider commissions, for every organization
-- (new ones: organization hook, model.DefaultCategories).
INSERT INTO expense_categories (organization_id, code, name, is_system, sort_order)
SELECT o.id, 'payment_commission', 'Ödeme komisyonu', TRUE, 75
FROM organizations o
ON CONFLICT (organization_id, code) DO NOTHING;

CREATE TABLE collections (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- TH-000001, unique per organization (advisory-lock serialised).
    code VARCHAR(32) NOT NULL,
    account_id BIGINT NOT NULL,
    method VARCHAR(16) NOT NULL CHECK (method IN ('cash', 'bank_transfer', 'card_pos', 'online', 'other')),
    -- manual (taken at the branch) or the online provider.
    provider VARCHAR(24) NOT NULL DEFAULT 'manual' CHECK (provider IN ('manual', 'iyzico')),
    purpose VARCHAR(12) NOT NULL DEFAULT 'general' CHECK (purpose IN ('general', 'prepayment', 'deposit')),
    status VARCHAR(20) NOT NULL DEFAULT 'recorded'
        CHECK (status IN ('recorded', 'partially_refunded', 'refunded', 'void')),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- Credited to the party (the amount owed that was paid).
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    -- What the payer paid on the provider (installment surcharge included),
    -- the provider commission and the net settlement. Manual: gross = net =
    -- amount, commission 0.
    gross_amount NUMERIC(14, 2) NOT NULL CHECK (gross_amount >= 0),
    commission_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),
    net_amount NUMERIC(14, 2) NOT NULL,
    refunded_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (refunded_amount >= 0 AND refunded_amount <= amount),
    collection_date DATE NOT NULL,
    -- Receipt / bank / POS slip / provider payment reference.
    reference_no VARCHAR(64) NOT NULL DEFAULT '',
    payer_name VARCHAR(255) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    branch_id BIGINT NULL,
    -- The till / bank / clearing account the money landed in.
    cash_account_id BIGINT NULL,
    rental_id BIGINT NULL,
    reservation_id BIGINT NULL,
    payment_id BIGINT NULL,
    ledger_entry_id BIGINT NOT NULL,
    cash_movement_id BIGINT NULL,
    commission_expense_id BIGINT NULL,
    -- none (manual), pending (online, settlement account not known yet),
    -- settled (gross in + commission expense out written).
    settlement_status VARCHAR(12) NOT NULL DEFAULT 'none' CHECK (settlement_status IN ('none', 'pending', 'settled')),
    settled_at TIMESTAMPTZ NULL,
    -- Client retry key of the create request.
    idempotency_key UUID NULL,
    void_reason TEXT NOT NULL DEFAULT '',
    voided_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_collections_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_collections_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_collections_organization_code UNIQUE (organization_id, code),
    CONSTRAINT uq_collections_organization_entry UNIQUE (organization_id, ledger_entry_id),
    CONSTRAINT chk_collections_void CHECK ((status = 'void') = (voided_at IS NOT NULL)),
    CONSTRAINT chk_collections_online CHECK ((method = 'online') = (provider <> 'manual')),
    CONSTRAINT chk_collections_settled CHECK ((settlement_status = 'settled') = (settled_at IS NOT NULL)),
    CONSTRAINT fk_collections_account
        FOREIGN KEY (organization_id, account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_collections_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_collections_cash_account
        FOREIGN KEY (organization_id, cash_account_id) REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_collections_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id),
    CONSTRAINT fk_collections_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_collections_payment
        FOREIGN KEY (organization_id, payment_id) REFERENCES payments (organization_id, id),
    CONSTRAINT fk_collections_entry
        FOREIGN KEY (organization_id, ledger_entry_id) REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_collections_movement
        FOREIGN KEY (organization_id, cash_movement_id) REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_collections_commission_expense
        FOREIGN KEY (organization_id, commission_expense_id) REFERENCES expenses (organization_id, id)
);

-- One collection per payment and per client retry key.
CREATE UNIQUE INDEX uq_collections_organization_payment ON collections (organization_id, payment_id)
    WHERE payment_id IS NOT NULL;
CREATE UNIQUE INDEX uq_collections_organization_idempotency ON collections (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_collections_organization_date ON collections (organization_id, collection_date DESC, id DESC);
CREATE INDEX idx_collections_organization_account ON collections (organization_id, account_id, collection_date);
CREATE INDEX idx_collections_organization_branch ON collections (organization_id, branch_id, collection_date);
CREATE INDEX idx_collections_organization_cash_account ON collections (organization_id, cash_account_id)
    WHERE cash_account_id IS NOT NULL;
CREATE INDEX idx_collections_organization_rental ON collections (organization_id, rental_id) WHERE rental_id IS NOT NULL;
CREATE INDEX idx_collections_organization_reservation ON collections (organization_id, reservation_id)
    WHERE reservation_id IS NOT NULL;
CREATE INDEX idx_collections_organization_settlement ON collections (organization_id, provider)
    WHERE settlement_status = 'pending';

CREATE TRIGGER trg_collections_updated_at
    BEFORE UPDATE ON collections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A refund entry names the collection it returns.
ALTER TABLE ledger_entries ADD COLUMN collection_id BIGINT NULL;
ALTER TABLE ledger_entries ADD CONSTRAINT fk_ledger_entries_collection
    FOREIGN KEY (organization_id, collection_id) REFERENCES collections (organization_id, id);
CREATE INDEX idx_ledger_entries_organization_collection ON ledger_entries (organization_id, collection_id)
    WHERE collection_id IS NOT NULL;
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_source_type_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_source_type_check CHECK (source_type IN (
    'rental', 'rental_charge', 'reservation', 'payment', 'payment_refund', 'invoice', 'toll_charge',
    'cash_movement', 'manual', 'collection'
));

CREATE TABLE collection_allocations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    collection_id BIGINT NOT NULL,
    -- The open item: a live accrual (debit) of the collection's account.
    entry_id BIGINT NOT NULL,
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('allocate', 'release')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    -- Who decided: a member (manual / auto = oldest first), a refund or a
    -- void giving the money back, an accrual reversed by its rental.
    source VARCHAR(20) NOT NULL CHECK (source IN ('manual', 'auto', 'refund', 'void', 'accrual_reversed', 'released')),
    -- The client key of the allocate request (a retry allocates nothing
    -- more).
    request_key UUID NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_collection_allocations_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT chk_collection_allocations_request CHECK (request_key IS NULL OR kind = 'allocate'),
    CONSTRAINT fk_collection_allocations_collection
        FOREIGN KEY (organization_id, collection_id) REFERENCES collections (organization_id, id),
    CONSTRAINT fk_collection_allocations_entry
        FOREIGN KEY (organization_id, entry_id) REFERENCES ledger_entries (organization_id, id)
);

CREATE UNIQUE INDEX uq_collection_allocations_request
    ON collection_allocations (organization_id, collection_id, request_key, entry_id) WHERE request_key IS NOT NULL;
CREATE INDEX idx_collection_allocations_organization_collection
    ON collection_allocations (organization_id, collection_id, entry_id);
CREATE INDEX idx_collection_allocations_organization_entry ON collection_allocations (organization_id, entry_id);

-- Append-only; organization deletion cascades.
CREATE FUNCTION collection_allocations_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND pg_trigger_depth() > 1 THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'collection allocations are append-only (release an allocation instead)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'collection_allocations_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_collection_allocations_append_only
    BEFORE UPDATE OR DELETE ON collection_allocations
    FOR EACH ROW EXECUTE FUNCTION collection_allocations_append_only();

-- The allocation guard. It locks the account row first, so every writer of
-- one account is serialised; the sums below then read committed rows (each
-- statement of a volatile function takes a fresh snapshot).
CREATE FUNCTION collection_allocations_guard() RETURNS trigger AS $$
DECLARE
    c collections%ROWTYPE;
    e ledger_entries%ROWTYPE;
    entry_net NUMERIC;
    coll_net NUMERIC;
    pair_net NUMERIC;
BEGIN
    SELECT * INTO c FROM collections WHERE organization_id = NEW.organization_id AND id = NEW.collection_id;
    SELECT * INTO e FROM ledger_entries WHERE organization_id = NEW.organization_id AND id = NEW.entry_id;
    IF NOT FOUND OR e.account_id <> c.account_id THEN
        RAISE EXCEPTION 'an allocation applies a collection to an open item of the same account'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collection_allocations_account';
    END IF;
    PERFORM 1 FROM accounts WHERE organization_id = NEW.organization_id AND id = c.account_id FOR UPDATE;
    -- Re-read the collection after the lock (refunds change it).
    SELECT * INTO c FROM collections WHERE organization_id = NEW.organization_id AND id = NEW.collection_id;
    SELECT COALESCE(SUM(CASE WHEN kind = 'allocate' THEN amount ELSE -amount END), 0) INTO pair_net
      FROM collection_allocations
     WHERE organization_id = NEW.organization_id AND collection_id = NEW.collection_id AND entry_id = NEW.entry_id;
    IF NEW.kind = 'release' THEN
        IF pair_net - NEW.amount < 0 THEN
            RAISE EXCEPTION 'cannot release more than is allocated'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collection_allocations_release';
        END IF;
        RETURN NEW;
    END IF;
    IF e.kind <> 'accrual' OR e.direction <> 'debit' OR EXISTS (
        SELECT 1 FROM ledger_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id
    ) THEN
        RAISE EXCEPTION 'only a live accrual is an open item'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collection_allocations_item';
    END IF;
    IF c.status = 'void' OR c.purpose = 'deposit' OR c.currency <> e.currency THEN
        RAISE EXCEPTION 'the collection cannot be allocated'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collection_allocations_collection';
    END IF;
    SELECT COALESCE(SUM(CASE WHEN kind = 'allocate' THEN amount ELSE -amount END), 0) INTO entry_net
      FROM collection_allocations WHERE organization_id = NEW.organization_id AND entry_id = NEW.entry_id;
    IF entry_net + NEW.amount > e.amount THEN
        RAISE EXCEPTION 'the open item is already paid'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collection_allocations_item_open';
    END IF;
    SELECT COALESCE(SUM(CASE WHEN kind = 'allocate' THEN amount ELSE -amount END), 0) INTO coll_net
      FROM collection_allocations WHERE organization_id = NEW.organization_id AND collection_id = NEW.collection_id;
    IF coll_net + NEW.amount > c.amount - c.refunded_amount THEN
        RAISE EXCEPTION 'the collection is already allocated'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collection_allocations_credit';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_collection_allocations_guard
    BEFORE INSERT ON collection_allocations
    FOR EACH ROW EXECUTE FUNCTION collection_allocations_guard();

-- A refund never leaves more allocated than the collection still holds.
CREATE FUNCTION collections_refund_guard() RETURNS trigger AS $$
DECLARE
    allocated NUMERIC;
BEGIN
    IF NEW.refunded_amount = OLD.refunded_amount AND NEW.status = OLD.status THEN
        RETURN NEW;
    END IF;
    SELECT COALESCE(SUM(CASE WHEN kind = 'allocate' THEN amount ELSE -amount END), 0) INTO allocated
      FROM collection_allocations WHERE organization_id = NEW.organization_id AND collection_id = NEW.id;
    IF allocated > NEW.amount - NEW.refunded_amount OR (NEW.status = 'void' AND allocated > 0) THEN
        RAISE EXCEPTION 'release the allocations before refunding or voiding the collection'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_collections_allocated';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_collections_refund_guard
    BEFORE UPDATE ON collections
    FOR EACH ROW EXECUTE FUNCTION collections_refund_guard();

-- Existing collection entries become collection documents (unallocated:
-- their money is the party's credit). Online collections posted before
-- this migration are not settled retroactively (settlement none).
INSERT INTO collections (
    organization_id, code, account_id, method, provider, purpose, status, currency, amount, gross_amount,
    commission_amount, net_amount, refunded_amount, collection_date, reference_no, branch_id, cash_account_id,
    rental_id, reservation_id, payment_id, ledger_entry_id, cash_movement_id, voided_at, void_reason,
    created_by, created_at
)
SELECT e.organization_id,
    'TH-' || lpad(row_number() OVER (PARTITION BY e.organization_id ORDER BY e.id)::text, 6, '0'),
    e.account_id,
    CASE WHEN p.provider = 'iyzico' THEN 'online'
         WHEN p.method IN ('cash', 'card_pos', 'bank_transfer') THEN p.method
         ELSE 'other' END,
    CASE WHEN p.provider = 'iyzico' THEN 'iyzico' ELSE 'manual' END,
    CASE WHEN e.purpose IN ('general', 'prepayment', 'deposit') THEN e.purpose ELSE 'general' END,
    CASE WHEN rev.id IS NOT NULL THEN 'void'
         WHEN COALESCE(rf.refunded, 0) >= e.amount THEN 'refunded'
         WHEN COALESCE(rf.refunded, 0) > 0 THEN 'partially_refunded'
         ELSE 'recorded' END,
    e.currency, e.amount,
    CASE WHEN p.provider = 'iyzico' AND p.gross_amount > 0 THEN p.gross_amount ELSE e.amount END,
    CASE WHEN p.provider = 'iyzico' THEN p.commission_amount ELSE 0 END,
    CASE WHEN p.provider = 'iyzico' AND p.gross_amount > 0 THEN p.net_amount ELSE e.amount END,
    CASE WHEN rev.id IS NOT NULL THEN 0 ELSE LEAST(COALESCE(rf.refunded, 0), e.amount) END,
    e.entry_date, COALESCE(p.provider_payment_id, ''), e.branch_id, mv.cash_account_id,
    e.rental_id, e.reservation_id, e.payment_id, e.id, mv.id,
    CASE WHEN rev.id IS NOT NULL THEN rev.created_at END,
    COALESCE(rev.reason, ''),
    e.created_by, e.created_at
FROM ledger_entries e
LEFT JOIN payments p ON p.organization_id = e.organization_id AND p.id = e.payment_id
LEFT JOIN ledger_entries rev ON rev.organization_id = e.organization_id AND rev.reverses_entry_id = e.id
LEFT JOIN cash_movements mv ON mv.organization_id = e.organization_id AND mv.ledger_entry_id = e.id AND mv.kind = 'collection'
LEFT JOIN LATERAL (
    SELECT SUM(r.amount) AS refunded FROM ledger_entries r
    WHERE r.organization_id = e.organization_id AND r.kind = 'refund' AND e.payment_id IS NOT NULL
      AND r.payment_id = e.payment_id
) rf ON TRUE
WHERE e.kind = 'collection';
