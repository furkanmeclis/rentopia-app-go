-- Reservations (TEC-706, spec §8.3, §8.4, §15.1). A reservation sells one
-- unit of a rental product for a range: it carries a human-readable
-- reference, the places and times, the sold product version (its guarantee
-- and conditions as a snapshot), the customer roles (contract party,
-- invoice addressee, drivers), the channel data, the accepted quote as an
-- immutable price snapshot with the policy (settings) version, and its
-- capacity commitment in the availability engine (vehicle_allocations).
--
-- status (who may move it, see docs/design/reservations.md):
--   request -> awaiting_review | awaiting_payment | confirmed | cancelled | expired
--   awaiting_review -> awaiting_payment | confirmed | cancelled | expired
--   awaiting_payment -> confirmed | cancelled | expired
--   confirmed -> cancelled | no_show
-- payment_status is separate (payments arrive in M3).
-- capacity_commitment: none (request), hold (option with expires_at,
-- review / payment window) or firm (confirmed). Every transition is a row in
-- reservation_events.
--
-- The plate plan lives in the allocation: planned vehicle = the allocation's
-- vehicle, assigned (promised) vehicle = the same vehicle when plate_fixed.
-- reservation_vehicle_assignments keeps the history of staff plate changes
-- and product changes (sold vs assigned side by side).
--
-- Branch scope: a reservation is visible when its pick-up OR return branch
-- is in the member's scope. Every index starts with organization_id.

-- Per organization and (Istanbul) year reference counter.
CREATE TABLE reservation_sequences (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    last_value BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_reservation_sequences PRIMARY KEY (organization_id, year)
);

CREATE TABLE reservations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- PREFIX-YYYY-NNNNNN (prefix = settings reservation.reference_prefix,
    -- else from the organization slug).
    reference VARCHAR(32) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN (
        'request', 'awaiting_review', 'awaiting_payment', 'confirmed', 'cancelled', 'expired', 'no_show'
    )),
    payment_status VARCHAR(16) NOT NULL DEFAULT 'unpaid'
        CHECK (payment_status IN ('unpaid', 'partially_paid', 'paid', 'refunded')),
    capacity_commitment VARCHAR(8) NOT NULL DEFAULT 'none' CHECK (capacity_commitment IN ('none', 'hold', 'firm')),
    -- Current allocation (provisional / fixed plate plan). Kept after a
    -- release for history; only an active allocation holds capacity.
    allocation_id BIGINT NULL,
    -- Places and times.
    pickup_branch_id BIGINT NOT NULL,
    return_branch_id BIGINT NOT NULL,
    pickup_point_id BIGINT NULL,
    return_point_id BIGINT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    -- Current product version (upgrades move it) and the product sold at
    -- creation (never changes).
    rental_product_id BIGINT NOT NULL,
    product_version INTEGER NOT NULL,
    sold_rental_product_id BIGINT NOT NULL,
    sold_product_version INTEGER NOT NULL,
    -- Snapshot of the guarantee and conditions of the current version
    -- ({guarantee_type, guarantee_label, min_driver_age, min_license_years,
    -- deposit_amount, included_km_per_day, extra_km_fee, fuel_policy}) and
    -- of the sold one (side by side after an upgrade).
    guarantee_type VARCHAR(16) NOT NULL,
    product_snapshot JSONB NOT NULL,
    sold_snapshot JSONB NOT NULL,
    -- Customer roles: contract party, invoice addressee (may differ).
    customer_id BIGINT NOT NULL,
    invoice_customer_id BIGINT NULL,
    -- Channel data.
    channel VARCHAR(16) NOT NULL,
    external_reference VARCHAR(64) NULL,
    collecting_party VARCHAR(8) NOT NULL DEFAULT 'us' CHECK (collecting_party IN ('us', 'channel')),
    commission_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (commission_amount >= 0),
    net_receivable NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- Money snapshot (VAT inclusive) and payment progress (M3 payments).
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    total_amount NUMERIC(14, 2) NOT NULL,
    prepayment_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    paid_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- Accepted quote (latest change included) and its snapshot: breakdown,
    -- lines, extras, discount; extensions append to it.
    quote_id BIGINT NULL,
    quote_snapshot JSONB NOT NULL,
    -- Policy (settings) version the reservation was sold under, with the
    -- policy texts and windows of that version.
    settings_version INTEGER NOT NULL DEFAULT 0,
    policy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Deadline of the current waiting state (review / payment window).
    review_deadline TIMESTAMPTZ NULL,
    payment_deadline TIMESTAMPTZ NULL,
    -- Equipment holds (extras module, TEC-702) tied to the allocation; a
    -- date change / upgrade re-reserves them for the new allocation.
    equipment_allocations UUID[] NOT NULL DEFAULT '{}',
    -- Staff member responsible (bulk "assign staff").
    assigned_user_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    -- Driver eligibility override (permission + reason), when used.
    eligibility_override JSONB NULL,
    confirmed_at TIMESTAMPTZ NULL,
    cancelled_at TIMESTAMPTZ NULL,
    cancel_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reservations_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_reservations_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_reservations_organization_reference UNIQUE (organization_id, reference),
    CONSTRAINT chk_reservations_range CHECK (ends_at > starts_at),
    -- A hold / firm commitment always names its allocation; terminal states
    -- hold nothing.
    CONSTRAINT chk_reservations_commitment CHECK (capacity_commitment = 'none' OR allocation_id IS NOT NULL),
    CONSTRAINT chk_reservations_terminal_commitment CHECK (
        status NOT IN ('cancelled', 'expired', 'no_show') OR capacity_commitment = 'none'
    ),
    CONSTRAINT fk_reservations_allocation
        FOREIGN KEY (organization_id, allocation_id) REFERENCES vehicle_allocations (organization_id, id),
    CONSTRAINT fk_reservations_pickup_branch
        FOREIGN KEY (organization_id, pickup_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_reservations_return_branch
        FOREIGN KEY (organization_id, return_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_reservations_pickup_point
        FOREIGN KEY (organization_id, pickup_point_id) REFERENCES service_points (organization_id, id),
    CONSTRAINT fk_reservations_return_point
        FOREIGN KEY (organization_id, return_point_id) REFERENCES service_points (organization_id, id),
    CONSTRAINT fk_reservations_product_version
        FOREIGN KEY (organization_id, rental_product_id, product_version)
        REFERENCES rental_product_versions (organization_id, product_id, version),
    CONSTRAINT fk_reservations_sold_product_version
        FOREIGN KEY (organization_id, sold_rental_product_id, sold_product_version)
        REFERENCES rental_product_versions (organization_id, product_id, version),
    CONSTRAINT fk_reservations_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_reservations_invoice_customer
        FOREIGN KEY (organization_id, invoice_customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_reservations_quote
        FOREIGN KEY (organization_id, quote_id) REFERENCES quotes (organization_id, id)
);

-- External reservation numbers are unique per organization and channel.
CREATE UNIQUE INDEX uq_reservations_organization_channel_external
    ON reservations (organization_id, channel, external_reference)
    WHERE external_reference IS NOT NULL;
CREATE UNIQUE INDEX uq_reservations_organization_allocation
    ON reservations (organization_id, allocation_id) WHERE allocation_id IS NOT NULL;
CREATE INDEX idx_reservations_organization_created ON reservations (organization_id, created_at DESC);
CREATE INDEX idx_reservations_organization_starts ON reservations (organization_id, starts_at);
CREATE INDEX idx_reservations_organization_pickup_branch ON reservations (organization_id, pickup_branch_id);
CREATE INDEX idx_reservations_organization_return_branch ON reservations (organization_id, return_branch_id);
CREATE INDEX idx_reservations_organization_status ON reservations (organization_id, status);
CREATE INDEX idx_reservations_organization_customer ON reservations (organization_id, customer_id);
CREATE INDEX idx_reservations_organization_product ON reservations (organization_id, rental_product_id);
-- The reservation a new-sale quote became (quote lists) and the FK check
-- when a quote is removed.
CREATE INDEX idx_reservations_organization_quote ON reservations (organization_id, quote_id) WHERE quote_id IS NOT NULL;
-- Expiry sweep: waiting reservations by deadline.
CREATE INDEX idx_reservations_organization_waiting
    ON reservations (organization_id, status, review_deadline, payment_deadline)
    WHERE status IN ('awaiting_review', 'awaiting_payment');

CREATE TRIGGER trg_reservations_updated_at
    BEFORE UPDATE ON reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Change / extension quotes point at their reservation.
ALTER TABLE quotes
    ADD CONSTRAINT fk_quotes_reservation
    FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE;

-- Drivers of a reservation (customers doc: the actual driver(s)). A
-- driver row anonymized / removed later keeps the reservation line
-- (driver_id SET NULL) with its masked snapshot.
CREATE TABLE reservation_drivers (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    driver_id BIGINT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'primary' CHECK (role IN ('primary', 'additional')),
    -- Masked display snapshot ({name, licence_hint}).
    snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Eligibility at the time of the check (customers EligibleForConditions)
    -- and the staff override when it was refused.
    eligible BOOLEAN NOT NULL,
    reasons TEXT[] NOT NULL DEFAULT '{}',
    override_reason TEXT NOT NULL DEFAULT '',
    overridden_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_reservation_drivers_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_reservation_drivers_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE SET NULL (driver_id)
);

CREATE UNIQUE INDEX uq_reservation_drivers_driver
    ON reservation_drivers (organization_id, reservation_id, driver_id) WHERE driver_id IS NOT NULL;
CREATE INDEX idx_reservation_drivers_organization_driver
    ON reservation_drivers (organization_id, driver_id) WHERE driver_id IS NOT NULL;

-- Every transition and change: who, when, from, to, reason, data.
CREATE TABLE reservation_events (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    action VARCHAR(48) NOT NULL,
    from_status VARCHAR(20) NULL,
    to_status VARCHAR(20) NULL,
    reason TEXT NOT NULL DEFAULT '',
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_reservation_events_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_reservation_events_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_reservation_events_organization_reservation
    ON reservation_events (organization_id, reservation_id, created_at, id);

-- Plate / product assignment history: the sold product next to what was
-- planned or promised over time. ended_at NULL = current.
CREATE TABLE reservation_vehicle_assignments (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    vehicle_id BIGINT NULL,
    rental_product_id BIGINT NOT NULL,
    product_version INTEGER NOT NULL,
    -- planned (provisional plate) | fixed (promised plate)
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('planned', 'fixed')),
    -- engine (sale), staff (plate change), drag (calendar), replan (after a
    -- block / change), upgrade (product change), extension
    source VARCHAR(16) NOT NULL CHECK (source IN ('engine', 'staff', 'drag', 'replan', 'upgrade', 'extension')),
    note TEXT NOT NULL DEFAULT '',
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    ended_at TIMESTAMPTZ NULL,
    CONSTRAINT fk_reservation_vehicle_assignments_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_reservation_vehicle_assignments_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id) ON DELETE SET NULL (vehicle_id),
    CONSTRAINT fk_reservation_vehicle_assignments_product_version
        FOREIGN KEY (organization_id, rental_product_id, product_version)
        REFERENCES rental_product_versions (organization_id, product_id, version)
);

CREATE INDEX idx_reservation_vehicle_assignments_organization_reservation
    ON reservation_vehicle_assignments (organization_id, reservation_id, started_at);
