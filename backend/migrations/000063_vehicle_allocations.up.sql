-- Vehicle allocations (TEC-697): every capacity commitment of the
-- availability engine. A commitment always carries a provisional plate plan
-- (vehicle_id), so the set of active commitments is satisfiable by
-- construction: the plan itself is the witness. See
-- docs/design/availability-engine.md.
--
-- kind:   hold (timed option, expires_at) | reservation (confirmed, with or
--         without a fixed plate) | rental (M3: the vehicle is handed over).
-- status: active | released (cancel / release) | expired (hold timed out).
--         Only active rows consume capacity; a row leaves "active" exactly
--         once (every transition is UPDATE ... WHERE status = 'active').
-- period: [starts_at, ends_at + preparation time of the return branch). Two
--         active allocations of a vehicle never overlap (EXCLUDE below).
--         The constraint is DEFERRABLE so a reassignment plan (moving several
--         flexible commitments between vehicles) can be applied in any order
--         inside one transaction and is checked at commit.
--
-- Blocks and transfers are not mirrored here: they stay in their own tables
-- and win over sales (a block can always be recorded; the allocations it
-- overlaps are reported as affected). The engine treats them as hard
-- constraints for new plans and the guard trigger below refuses an
-- allocation that overlaps one (defence in depth, same error code as the
-- EXCLUDE so callers retry the next candidate).
--
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE vehicle_allocations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    -- What was sold: the product and the immutable version whose guarantee
    -- the commitment keeps (reassignment only moves it to vehicles that
    -- satisfy this version).
    rental_product_id BIGINT NOT NULL,
    product_version INTEGER NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('hold', 'reservation', 'rental')),
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'released', 'expired')),
    -- Customer range (pick-up -> return) and the preparation buffer.
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    prep_minutes INTEGER NOT NULL DEFAULT 0 CHECK (prep_minutes BETWEEN 0 AND 10080),
    period TSTZRANGE NOT NULL,
    pickup_branch_id BIGINT NOT NULL,
    return_branch_id BIGINT NOT NULL,
    -- Delivery points of the branches (same stock as the branch).
    pickup_point_id BIGINT NULL,
    return_point_id BIGINT NULL,
    -- Pool of the vehicle chosen at sale time; reassignment stays in it
    -- (independent pools never share capacity).
    fleet_pool VARCHAR(32) NOT NULL,
    -- Sales channel (orgsettings.SalesChannels).
    channel VARCHAR(16) NOT NULL,
    -- TRUE once a plate is promised (EXACT_VEHICLE sale or plate
    -- assignment): reassignment never moves it.
    plate_fixed BOOLEAN NOT NULL DEFAULT FALSE,
    -- Holds only: the option lapses at expires_at.
    expires_at TIMESTAMPTZ NULL,
    -- Related record (TEC-706 reservations, M3 rentals): set when the hold
    -- is confirmed.
    ref_type VARCHAR(32) NULL,
    ref_uuid UUID NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    released_at TIMESTAMPTZ NULL,
    released_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    release_reason VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vehicle_allocations_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_vehicle_allocations_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vehicle_allocations_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vehicle_allocations_product_version
        FOREIGN KEY (organization_id, rental_product_id, product_version)
        REFERENCES rental_product_versions (organization_id, product_id, version),
    CONSTRAINT fk_vehicle_allocations_pickup_branch
        FOREIGN KEY (organization_id, pickup_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_vehicle_allocations_return_branch
        FOREIGN KEY (organization_id, return_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT chk_vehicle_allocations_range CHECK (ends_at > starts_at),
    CONSTRAINT chk_vehicle_allocations_period CHECK (
        NOT isempty(period) AND lower_inc(period) AND NOT upper_inc(period)
        AND lower(period) = starts_at AND upper(period) >= ends_at
    ),
    CONSTRAINT chk_vehicle_allocations_hold_expiry CHECK (kind <> 'hold' OR expires_at IS NOT NULL),
    CONSTRAINT chk_vehicle_allocations_released CHECK ((status = 'active') = (released_at IS NULL)),
    CONSTRAINT ex_vehicle_allocations_overlap EXCLUDE USING gist (
        organization_id WITH =,
        vehicle_id WITH =,
        period WITH &&
    ) WHERE (status = 'active') DEFERRABLE INITIALLY IMMEDIATE
);

-- Service points must belong to the organization (a point of another
-- organization can never be referenced). Their branch is checked by the use
-- case (the point's branch is the pickup / return branch).
ALTER TABLE service_points
    ADD CONSTRAINT uq_service_points_organization_id UNIQUE (organization_id, id);

ALTER TABLE vehicle_allocations
    ADD CONSTRAINT fk_vehicle_allocations_pickup_point
        FOREIGN KEY (organization_id, pickup_point_id)
        REFERENCES service_points (organization_id, id) ON DELETE SET NULL (pickup_point_id),
    ADD CONSTRAINT fk_vehicle_allocations_return_point
        FOREIGN KEY (organization_id, return_point_id)
        REFERENCES service_points (organization_id, id) ON DELETE SET NULL (return_point_id);

CREATE INDEX idx_vehicle_allocations_organization_vehicle
    ON vehicle_allocations (organization_id, vehicle_id, lower(period))
    WHERE status = 'active';

CREATE INDEX idx_vehicle_allocations_organization_product
    ON vehicle_allocations (organization_id, rental_product_id, lower(period))
    WHERE status = 'active';

-- Expiry sweep and the holds list.
CREATE INDEX idx_vehicle_allocations_organization_hold_expiry
    ON vehicle_allocations (organization_id, expires_at)
    WHERE kind = 'hold' AND status = 'active';

CREATE INDEX idx_vehicle_allocations_organization_created
    ON vehicle_allocations (organization_id, created_at DESC);

CREATE INDEX idx_vehicle_allocations_organization_ref
    ON vehicle_allocations (organization_id, ref_uuid)
    WHERE ref_uuid IS NOT NULL;

CREATE TRIGGER trg_vehicle_allocations_updated_at
    BEFORE UPDATE ON vehicle_allocations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Defence in depth: an active allocation never lands on a deleted /
-- inactive vehicle or on a block / open transfer of its vehicle. Fires when
-- a row is inserted or moved (vehicle or period changes), not on status
-- changes, so releasing an allocation that a later block overlaps works.
-- Blocks and transfers themselves are not checked against allocations:
-- operational facts win, the allocations they overlap become "affected".
CREATE FUNCTION vehicle_allocations_guard() RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'active' THEN
        RETURN NEW;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM vehicles v
        WHERE v.organization_id = NEW.organization_id AND v.id = NEW.vehicle_id
          AND v.deleted_at IS NULL AND v.is_active
    ) THEN
        RAISE EXCEPTION 'vehicle % is not available for allocations', NEW.vehicle_id
            USING ERRCODE = 'exclusion_violation', CONSTRAINT = 'vehicle_allocations_guard';
    END IF;
    IF EXISTS (
        SELECT 1 FROM vehicle_blocks b
        WHERE b.organization_id = NEW.organization_id AND b.vehicle_id = NEW.vehicle_id
          AND b.deleted_at IS NULL AND b.during && NEW.period
    ) OR EXISTS (
        SELECT 1 FROM vehicle_transfers t
        WHERE t.organization_id = NEW.organization_id AND t.vehicle_id = NEW.vehicle_id
          AND t.status IN ('planned', 'in_transit')
          AND CASE t.status
                WHEN 'planned' THEN t.planned
                ELSE tstzrange(t.actual_departure, GREATEST(upper(t.planned), NOW(), t.actual_departure), '[)')
              END && NEW.period
    ) THEN
        RAISE EXCEPTION 'vehicle % is blocked or in transfer during %', NEW.vehicle_id, NEW.period
            USING ERRCODE = 'exclusion_violation', CONSTRAINT = 'vehicle_allocations_guard';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vehicle_allocations_guard
    BEFORE INSERT OR UPDATE OF vehicle_id, period ON vehicle_allocations
    FOR EACH ROW EXECUTE FUNCTION vehicle_allocations_guard();

-- vehicle_busy_intervals gains the allocations (fleet-availability-inputs
-- §1): kind = hold | reservation | rental, during = the period (with the
-- preparation buffer), branch_after_id = the return branch. Same column
-- list and order.
CREATE OR REPLACE VIEW vehicle_busy_intervals AS
SELECT
    b.organization_id,
    b.vehicle_id,
    'block'::text AS kind,
    b.uuid AS ref_uuid,
    b.during,
    NULL::bigint AS branch_after_id
FROM vehicle_blocks b
WHERE b.deleted_at IS NULL
UNION ALL
SELECT
    t.organization_id,
    t.vehicle_id,
    'transfer'::text AS kind,
    t.uuid AS ref_uuid,
    CASE t.status
        WHEN 'planned' THEN t.planned
        WHEN 'in_transit' THEN tstzrange(
            t.actual_departure, GREATEST(upper(t.planned), NOW(), t.actual_departure), '[)'
        )
        ELSE tstzrange(t.actual_departure, t.actual_arrival, '[)')
    END AS during,
    t.to_branch_id AS branch_after_id
FROM vehicle_transfers t
WHERE t.status <> 'cancelled'
UNION ALL
SELECT
    a.organization_id,
    a.vehicle_id,
    a.kind::text AS kind,
    a.uuid AS ref_uuid,
    a.period AS during,
    a.return_branch_id AS branch_after_id
FROM vehicle_allocations a
WHERE a.status = 'active'
  AND (a.kind <> 'hold' OR a.expires_at > NOW());
