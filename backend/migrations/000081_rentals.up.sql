-- Rentals (TEC-709, spec §8.1, §8.4, §4.3). A rental is born from a
-- confirmed reservation once its vehicle (plate) is fixed: it carries the
-- physical side of the sale — the actual vehicle(s), the drivers, the actual
-- pick-up and return (time, branch / point, km, fuel), the late / early
-- return facts and the charges proposed under the reservation's policy
-- version. The planned side (planned times and places, product, price) stays
-- on the reservation, which remains the single source of the plan.
--
-- status:
--   preparation -> active (handover) | cancelled (before the handover)
--   active      -> return_check (vehicle back: km / fuel in, actual time and
--                  branch)
--   return_check -> returned (return check done; the vehicle goes to
--                  cleaning / available)
-- Financial close is separate (M4: closed_at stays NULL until then).
--
-- Vehicles: rental_vehicles holds one leg per vehicle (handover and return
-- record each). A mid-rental vehicle swap returns the current leg and opens
-- a new one; the rental continues. The availability engine allocation of the
-- current leg is rentals.allocation_id (kind rental after the handover).
--
-- Branch scope: a rental is visible when its reservation's pick-up OR
-- return branch, or its actual return branch, is in the member's scope.
-- Every index starts with organization_id.

CREATE TABLE rentals (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- The reservation reference (one rental per reservation).
    reference VARCHAR(32) NOT NULL,
    reservation_id BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'preparation'
        CHECK (status IN ('preparation', 'active', 'return_check', 'returned', 'cancelled')),
    -- Current vehicle and its allocation (the engine's plate plan).
    vehicle_id BIGINT NOT NULL,
    allocation_id BIGINT NULL,
    -- Actual pick-up (handover) and return. Planned values are the
    -- reservation's starts_at / ends_at / branches.
    actual_pickup_at TIMESTAMPTZ NULL,
    actual_pickup_branch_id BIGINT NULL,
    actual_pickup_point_id BIGINT NULL,
    actual_return_at TIMESTAMPTZ NULL,
    actual_return_branch_id BIGINT NULL,
    actual_return_point_id BIGINT NULL,
    -- Late return: when the planned return (+ tolerance) passed without a
    -- return; minutes late at the return.
    overdue_at TIMESTAMPTZ NULL,
    late_minutes INTEGER NOT NULL DEFAULT 0 CHECK (late_minutes >= 0),
    -- Policy (settings) version of the reservation and its policy part
    -- (late tolerance, extra hour fee, early return / extension texts):
    -- charges are computed only from this snapshot.
    settings_version INTEGER NOT NULL DEFAULT 0,
    policy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Handover without an approved contract (tenant.signature.waive).
    contract_waiver JSONB NULL,
    return_note TEXT NOT NULL DEFAULT '',
    damage_note TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    cancel_reason TEXT NOT NULL DEFAULT '',
    -- M4 financial close.
    closed_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    delivered_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    returned_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    checked_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    checked_at TIMESTAMPTZ NULL,
    cancelled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rentals_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rentals_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_rentals_delivered CHECK (status IN ('preparation', 'cancelled') OR actual_pickup_at IS NOT NULL),
    CONSTRAINT chk_rentals_returned CHECK (status NOT IN ('return_check', 'returned') OR actual_return_at IS NOT NULL),
    CONSTRAINT fk_rentals_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_rentals_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id),
    CONSTRAINT fk_rentals_allocation
        FOREIGN KEY (organization_id, allocation_id) REFERENCES vehicle_allocations (organization_id, id),
    CONSTRAINT fk_rentals_actual_pickup_branch
        FOREIGN KEY (organization_id, actual_pickup_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_rentals_actual_return_branch
        FOREIGN KEY (organization_id, actual_return_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_rentals_actual_pickup_point
        FOREIGN KEY (organization_id, actual_pickup_point_id) REFERENCES service_points (organization_id, id),
    CONSTRAINT fk_rentals_actual_return_point
        FOREIGN KEY (organization_id, actual_return_point_id) REFERENCES service_points (organization_id, id)
);

-- One live rental per reservation (a cancelled one may be followed by a new
-- one after the plate changes again).
CREATE UNIQUE INDEX uq_rentals_organization_reservation
    ON rentals (organization_id, reservation_id) WHERE status <> 'cancelled';
CREATE INDEX idx_rentals_organization_created ON rentals (organization_id, created_at DESC);
CREATE INDEX idx_rentals_organization_status ON rentals (organization_id, status);
CREATE INDEX idx_rentals_organization_vehicle ON rentals (organization_id, vehicle_id);
CREATE INDEX idx_rentals_organization_reservation ON rentals (organization_id, reservation_id);
CREATE INDEX idx_rentals_organization_return_branch
    ON rentals (organization_id, actual_return_branch_id) WHERE actual_return_branch_id IS NOT NULL;
CREATE INDEX idx_rentals_organization_allocation
    ON rentals (organization_id, allocation_id) WHERE allocation_id IS NOT NULL;
-- Overdue sweep: active rentals.
CREATE INDEX idx_rentals_organization_active ON rentals (organization_id, status) WHERE status = 'active';

CREATE TRIGGER trg_rentals_updated_at
    BEFORE UPDATE ON rentals
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One leg per vehicle: handover (km / fuel out, time, branch) and return
-- (km / fuel in, time, branch). ended_at NULL = the current leg.
CREATE TABLE rental_vehicles (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    vehicle_id BIGINT NOT NULL,
    allocation_id BIGINT NULL,
    -- handover (first leg), swap (a vehicle change during the rental)
    reason VARCHAR(16) NOT NULL DEFAULT 'handover' CHECK (reason IN ('handover', 'swap')),
    out_at TIMESTAMPTZ NULL,
    out_branch_id BIGINT NULL,
    out_km INTEGER NULL CHECK (out_km BETWEEN 0 AND 5000000),
    out_fuel SMALLINT NULL CHECK (out_fuel BETWEEN 0 AND 100),
    in_at TIMESTAMPTZ NULL,
    in_branch_id BIGINT NULL,
    in_km INTEGER NULL CHECK (in_km BETWEEN 0 AND 5000000),
    in_fuel SMALLINT NULL CHECK (in_fuel BETWEEN 0 AND 100),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rental_vehicles_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT chk_rental_vehicles_km CHECK (in_km IS NULL OR out_km IS NULL OR in_km >= out_km),
    CONSTRAINT fk_rental_vehicles_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_rental_vehicles_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id),
    CONSTRAINT fk_rental_vehicles_allocation
        FOREIGN KEY (organization_id, allocation_id) REFERENCES vehicle_allocations (organization_id, id),
    CONSTRAINT fk_rental_vehicles_out_branch
        FOREIGN KEY (organization_id, out_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_rental_vehicles_in_branch
        FOREIGN KEY (organization_id, in_branch_id) REFERENCES branches (organization_id, id)
);

CREATE INDEX idx_rental_vehicles_organization_rental ON rental_vehicles (organization_id, rental_id, id);
CREATE INDEX idx_rental_vehicles_organization_vehicle ON rental_vehicles (organization_id, vehicle_id);

-- Drivers of the rental (copied from the reservation at creation; a driver
-- row anonymized later keeps the masked snapshot).
CREATE TABLE rental_drivers (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    driver_id BIGINT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'primary' CHECK (role IN ('primary', 'additional')),
    snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_rental_drivers_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_rental_drivers_driver
        FOREIGN KEY (organization_id, driver_id) REFERENCES drivers (organization_id, id) ON DELETE SET NULL (driver_id)
);

CREATE INDEX idx_rental_drivers_organization_rental ON rental_drivers (organization_id, rental_id);
CREATE INDEX idx_rental_drivers_organization_driver
    ON rental_drivers (organization_id, driver_id) WHERE driver_id IS NOT NULL;

-- Timeline: every status change and operational fact.
CREATE TABLE rental_events (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    action VARCHAR(48) NOT NULL,
    from_status VARCHAR(16) NULL,
    to_status VARCHAR(16) NULL,
    reason TEXT NOT NULL DEFAULT '',
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_rental_events_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_rental_events_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_rental_events_organization_rental ON rental_events (organization_id, rental_id, created_at, id);

-- Charges beyond the sold price (late return, damage, early return
-- adjustment, other). A policy charge is computed only from the rental's
-- policy snapshot (source = policy, calculation explains it); a manual one
-- needs a reason. Every charge needs an authorized approval (or a waiver)
-- before M4 invoices it.
CREATE TABLE rental_charges (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    kind VARCHAR(24) NOT NULL CHECK (kind IN ('late_return', 'early_return', 'damage', 'fuel', 'mileage', 'other')),
    source VARCHAR(8) NOT NULL CHECK (source IN ('policy', 'manual')),
    status VARCHAR(12) NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'approved', 'waived')),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    amount NUMERIC(14, 2) NOT NULL CHECK (amount >= 0),
    settings_version INTEGER NOT NULL DEFAULT 0,
    calculation JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason TEXT NOT NULL DEFAULT '',
    decided_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ NULL,
    decision_note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rental_charges_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT chk_rental_charges_decided CHECK ((status = 'proposed') = (decided_at IS NULL)),
    CONSTRAINT fk_rental_charges_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_rental_charges_organization_rental ON rental_charges (organization_id, rental_id, created_at);
-- One policy late fee per rental (a re-computed one replaces the proposal).
CREATE UNIQUE INDEX uq_rental_charges_policy_late
    ON rental_charges (organization_id, rental_id) WHERE kind = 'late_return' AND source = 'policy' AND status <> 'waived';
