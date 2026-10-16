-- Corporate agreements and long-term rentals (TEC-723, RT-A24,
-- docs/design/corporate-agreements.md).
--
-- corporate_agreements: one agreement with a corporate customer: validity,
-- invoice period (monthly = the rent of its rentals is billed per calendar
-- month; per_rental = accrued once at the handover as before), payment term
-- (copied onto the customer's ledger account), open-account permission with
-- its credit limit, coupon eligibility, long-term terms (monthly km
-- allowance, maintenance interval), notes and a status.
-- agreement_contacts: authorised contacts (the corporate customer's
-- contacts, with a role). agreement_drivers: who may drive under the
-- agreement (a driver record, or an individual customer).
--
-- Tariffs: rate_plans gains the scope "agreement" (agreement_id); such a
-- plan prices only sales made under its agreement and beats every public
-- plan (pricing engine). Plan versioning is unchanged.
--
-- Quotes, reservations reference the agreement they were sold under.
--
-- Periodic billing: rentals.rent_billing is "handover" (the rent is one
-- accrual at the handover, key rental:<id>:rent) or "periodic" (one accrual
-- per rental_billing_periods row, key agreement:<a>:r<rental>:p<n>). The two
-- paths are mutually exclusive in the database:
--   * ledger_entries.billing_period_id names the period of a periodic rent
--     accrual; the insert trigger refuses a rent accrual without a period on
--     a periodic rental and one with a period on a handover rental;
--   * rentals.rent_billing cannot change while the rental has a live rent
--     accrual.
-- A posted period is never edited or deleted: an extension or an early
-- termination supersedes it (new revision of the same period number,
-- reversal + new accrual under the same key) or reverses it.
--
-- Every index starts with organization_id; children use composite FKs.

-- Composite FK target for agreement contacts.
ALTER TABLE customer_contacts
    ADD CONSTRAINT uq_customer_contacts_organization_id UNIQUE (organization_id, id);

CREATE TABLE corporate_agreements (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- KA-000001 per organization (advisory-lock serialised).
    code VARCHAR(16) NOT NULL,
    name VARCHAR(255) NOT NULL,
    -- The corporate customer (customer_type corporate, checked by the API).
    customer_id BIGINT NOT NULL,
    -- Managing branch; NULL = head office (every branch).
    branch_id BIGINT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'suspended', 'terminated')),
    -- Validity, Europe/Istanbul calendar days, both inclusive (valid_to
    -- NULL = open ended).
    valid_from DATE NOT NULL,
    valid_to DATE NULL,
    invoice_period VARCHAR(12) NOT NULL DEFAULT 'per_rental'
        CHECK (invoice_period IN ('monthly', 'per_rental')),
    payment_term_days INTEGER NOT NULL DEFAULT 0 CHECK (payment_term_days BETWEEN 0 AND 365),
    -- Open account: a reservation is confirmed without prepayment while
    -- the customer's exposure stays within credit_limit.
    open_account BOOLEAN NOT NULL DEFAULT FALSE,
    credit_limit NUMERIC(14, 2) NULL CHECK (credit_limit IS NULL OR credit_limit >= 0),
    -- Whether campaigns / coupons may apply on top of the agreement price
    -- (and then only campaigns marked corporate_eligible).
    coupon_eligible BOOLEAN NOT NULL DEFAULT FALSE,
    -- Long-term terms: km allowance per (full) month, maintenance interval.
    monthly_km_allowance INTEGER NULL CHECK (monthly_km_allowance IS NULL OR monthly_km_allowance > 0),
    maintenance_interval_days INTEGER NULL CHECK (maintenance_interval_days IS NULL OR maintenance_interval_days > 0),
    maintenance_interval_km INTEGER NULL CHECK (maintenance_interval_km IS NULL OR maintenance_interval_km > 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    notes TEXT NOT NULL DEFAULT '',
    terminated_at TIMESTAMPTZ NULL,
    termination_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_corporate_agreements_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_corporate_agreements_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_corporate_agreements_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_corporate_agreements_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT chk_corporate_agreements_validity CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT chk_corporate_agreements_open_account CHECK (NOT open_account OR credit_limit IS NOT NULL),
    CONSTRAINT chk_corporate_agreements_terminated CHECK ((status = 'terminated') = (terminated_at IS NOT NULL))
);

CREATE UNIQUE INDEX uq_corporate_agreements_organization_code
    ON corporate_agreements (organization_id, lower(code));
CREATE INDEX idx_corporate_agreements_organization_customer
    ON corporate_agreements (organization_id, customer_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_corporate_agreements_organization_branch
    ON corporate_agreements (organization_id, branch_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_corporate_agreements_organization_created
    ON corporate_agreements (organization_id, created_at DESC) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_corporate_agreements_updated_at
    BEFORE UPDATE ON corporate_agreements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE agreement_contacts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    agreement_id BIGINT NOT NULL,
    customer_contact_id BIGINT NOT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'operations'
        CHECK (role IN ('signatory', 'billing', 'operations', 'other')),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agreement_contacts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_agreement_contacts_contact UNIQUE (organization_id, agreement_id, customer_contact_id),
    CONSTRAINT fk_agreement_contacts_agreement
        FOREIGN KEY (organization_id, agreement_id) REFERENCES corporate_agreements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agreement_contacts_contact
        FOREIGN KEY (organization_id, customer_contact_id) REFERENCES customer_contacts (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE agreement_drivers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    agreement_id BIGINT NOT NULL,
    -- Exactly one: a driver record, or an individual customer (every driver
    -- of that customer may drive).
    driver_id BIGINT NULL,
    customer_id BIGINT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agreement_drivers_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_agreement_drivers_agreement
        FOREIGN KEY (organization_id, agreement_id) REFERENCES corporate_agreements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agreement_drivers_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agreement_drivers_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_agreement_drivers_one CHECK ((driver_id IS NULL) <> (customer_id IS NULL))
);

CREATE UNIQUE INDEX uq_agreement_drivers_driver
    ON agreement_drivers (organization_id, agreement_id, driver_id) WHERE driver_id IS NOT NULL;
CREATE UNIQUE INDEX uq_agreement_drivers_customer
    ON agreement_drivers (organization_id, agreement_id, customer_id) WHERE customer_id IS NOT NULL;

-- Agreement tariffs: scope "agreement" on rate plans (full list of scopes).
ALTER TABLE rate_plans ADD COLUMN agreement_id BIGINT NULL;
ALTER TABLE rate_plans DROP CONSTRAINT rate_plans_scope_check;
ALTER TABLE rate_plans ADD CONSTRAINT rate_plans_scope_check
    CHECK (scope IN ('common', 'branch', 'channel', 'branch_channel', 'agreement'));
ALTER TABLE rate_plans
    ADD CONSTRAINT fk_rate_plans_agreement
        FOREIGN KEY (organization_id, agreement_id) REFERENCES corporate_agreements (organization_id, id),
    ADD CONSTRAINT chk_rate_plans_agreement_scope CHECK ((scope = 'agreement') = (agreement_id IS NOT NULL));
CREATE INDEX idx_rate_plans_organization_agreement
    ON rate_plans (organization_id, agreement_id) WHERE deleted_at IS NULL AND agreement_id IS NOT NULL;

-- Sales under an agreement.
ALTER TABLE quotes ADD COLUMN agreement_id BIGINT NULL;
ALTER TABLE quotes
    ADD CONSTRAINT fk_quotes_agreement
        FOREIGN KEY (organization_id, agreement_id) REFERENCES corporate_agreements (organization_id, id);

ALTER TABLE reservations
    ADD COLUMN agreement_id BIGINT NULL,
    -- Confirmed without prepayment on the agreement's open account.
    ADD COLUMN open_account BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE reservations
    ADD CONSTRAINT fk_reservations_agreement
        FOREIGN KEY (organization_id, agreement_id) REFERENCES corporate_agreements (organization_id, id),
    ADD CONSTRAINT chk_reservations_open_account CHECK (NOT open_account OR agreement_id IS NOT NULL);
CREATE INDEX idx_reservations_organization_agreement
    ON reservations (organization_id, agreement_id) WHERE agreement_id IS NOT NULL;

-- How the rent of a rental reaches the ledger (fixed when the rental is
-- created from its reservation's agreement).
ALTER TABLE rentals ADD COLUMN rent_billing VARCHAR(12) NOT NULL DEFAULT 'handover'
    CHECK (rent_billing IN ('handover', 'periodic'));

CREATE TABLE rental_billing_periods (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    agreement_id BIGINT NULL,
    period_no INTEGER NOT NULL CHECK (period_no >= 1),
    -- 1 for the first plan of the period, +1 for each adjustment.
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    -- Billing day indices [day_from, day_to) from the rental start, and
    -- their Istanbul dates [starts_on, ends_on).
    day_from INTEGER NOT NULL CHECK (day_from >= 0),
    day_to INTEGER NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    days INTEGER NOT NULL CHECK (days >= 1),
    partial BOOLEAN NOT NULL DEFAULT FALSE,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    km_allowance INTEGER NULL,
    accrual_key VARCHAR(64) NOT NULL,
    -- planned: not posted yet (replaced freely by a re-plan); posted: its
    -- accrual is in the ledger (ledger_entry_id); superseded: replaced by a
    -- new revision (accrual reversed and accrued again); reversed: no longer
    -- due (early termination, accrual reversed).
    status VARCHAR(12) NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'posted', 'superseded', 'reversed')),
    ledger_entry_id BIGINT NULL,
    posted_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rental_billing_periods_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rental_billing_periods_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_rental_billing_periods_revision UNIQUE (organization_id, rental_id, period_no, revision),
    CONSTRAINT fk_rental_billing_periods_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id),
    CONSTRAINT fk_rental_billing_periods_agreement
        FOREIGN KEY (organization_id, agreement_id) REFERENCES corporate_agreements (organization_id, id),
    CONSTRAINT fk_rental_billing_periods_entry
        FOREIGN KEY (organization_id, ledger_entry_id) REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT chk_rental_billing_periods_days CHECK (day_to > day_from AND days = day_to - day_from AND ends_on > starts_on),
    CONSTRAINT chk_rental_billing_periods_posted CHECK (
        (status = 'planned') = (posted_at IS NULL) AND (status = 'planned') = (ledger_entry_id IS NULL)
    ),
    CONSTRAINT chk_rental_billing_periods_closed CHECK ((status IN ('superseded', 'reversed')) = (closed_at IS NOT NULL))
);

-- One live revision per period number (planned or posted).
CREATE UNIQUE INDEX uq_rental_billing_periods_live
    ON rental_billing_periods (organization_id, rental_id, period_no) WHERE status IN ('planned', 'posted');
CREATE INDEX idx_rental_billing_periods_organization_agreement
    ON rental_billing_periods (organization_id, agreement_id) WHERE agreement_id IS NOT NULL;
CREATE INDEX idx_rental_billing_periods_organization_due
    ON rental_billing_periods (organization_id, starts_on) WHERE status = 'planned';

CREATE TRIGGER trg_rental_billing_periods_updated_at
    BEFORE UPDATE ON rental_billing_periods
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A period that reached the ledger is history: its figures never change and
-- it is never deleted. Allowed: planned -> posted, posted -> superseded /
-- reversed. Planned rows are free (they never touched the ledger). An
-- organization delete still cascades (pg_trigger_depth() > 1).
CREATE FUNCTION rental_billing_periods_guard() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'planned' AND pg_trigger_depth() <= 1 THEN
            RAISE EXCEPTION 'a posted billing period is never deleted (supersede or reverse it)'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'rental_billing_periods_guard';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.status = 'planned' THEN
        RETURN NEW;
    END IF;
    IF NEW.rental_id <> OLD.rental_id OR NEW.period_no <> OLD.period_no OR NEW.revision <> OLD.revision
       OR NEW.day_from <> OLD.day_from OR NEW.day_to <> OLD.day_to OR NEW.amount <> OLD.amount
       OR NEW.accrual_key <> OLD.accrual_key OR NEW.ledger_entry_id IS DISTINCT FROM OLD.ledger_entry_id
       OR NOT (NEW.status = OLD.status OR (OLD.status = 'posted' AND NEW.status IN ('superseded', 'reversed'))) THEN
        RAISE EXCEPTION 'a posted billing period is immutable (supersede or reverse it)'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'rental_billing_periods_guard';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rental_billing_periods_guard
    BEFORE UPDATE OR DELETE ON rental_billing_periods
    FOR EACH ROW EXECUTE FUNCTION rental_billing_periods_guard();

-- The period a periodic rent accrual (and its reversal) belongs to.
ALTER TABLE ledger_entries ADD COLUMN billing_period_id BIGINT NULL;
ALTER TABLE ledger_entries
    ADD CONSTRAINT fk_ledger_entries_billing_period
        FOREIGN KEY (organization_id, billing_period_id) REFERENCES rental_billing_periods (organization_id, id);
CREATE INDEX idx_ledger_entries_organization_billing_period
    ON ledger_entries (organization_id, billing_period_id) WHERE billing_period_id IS NOT NULL;

-- Handover accrual and periodic accrual are mutually exclusive.
CREATE FUNCTION ledger_entries_rent_billing() RETURNS trigger AS $$
DECLARE
    mode VARCHAR(12);
    p rental_billing_periods%ROWTYPE;
BEGIN
    IF NEW.kind <> 'accrual' OR NEW.purpose <> 'rent' OR NEW.rental_id IS NULL THEN
        IF NEW.billing_period_id IS NOT NULL AND NEW.kind NOT IN ('accrual', 'reversal') THEN
            RAISE EXCEPTION 'only a rent accrual and its reversal name a billing period'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_ledger_entries_rent_billing';
        END IF;
        RETURN NEW;
    END IF;
    SELECT rent_billing INTO mode FROM rentals
     WHERE organization_id = NEW.organization_id AND id = NEW.rental_id;
    IF mode = 'periodic' THEN
        IF NEW.billing_period_id IS NULL THEN
            RAISE EXCEPTION 'rental % is billed per period: its rent is accrued per billing period, never at the handover', NEW.rental_id
                USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_ledger_entries_rent_billing';
        END IF;
        SELECT * INTO p FROM rental_billing_periods
         WHERE organization_id = NEW.organization_id AND id = NEW.billing_period_id;
        IF NOT FOUND OR p.rental_id <> NEW.rental_id OR p.accrual_key <> NEW.accrual_key THEN
            RAISE EXCEPTION 'the billing period does not belong to this rental accrual'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_ledger_entries_rent_billing';
        END IF;
    ELSIF NEW.billing_period_id IS NOT NULL THEN
        RAISE EXCEPTION 'rental % is accrued at the handover: it has no billing periods', NEW.rental_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_ledger_entries_rent_billing';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_entries_rent_billing
    BEFORE INSERT ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_rent_billing();

CREATE FUNCTION rentals_rent_billing_fixed() RETURNS trigger AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM ledger_entries e
         WHERE e.organization_id = NEW.organization_id AND e.rental_id = NEW.id
           AND e.kind = 'accrual' AND e.purpose = 'rent'
           AND NOT EXISTS (
               SELECT 1 FROM ledger_entries r
                WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id
           )
    ) THEN
        RAISE EXCEPTION 'rental % has a live rent accrual: its billing mode cannot change', NEW.id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'chk_rentals_rent_billing_fixed';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rentals_rent_billing_fixed
    BEFORE UPDATE OF rent_billing ON rentals
    FOR EACH ROW WHEN (OLD.rent_billing IS DISTINCT FROM NEW.rent_billing)
    EXECUTE FUNCTION rentals_rent_billing_fixed();
