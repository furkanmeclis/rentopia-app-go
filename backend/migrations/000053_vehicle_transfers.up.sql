-- Vehicle transfers between branches (TEC-695) and the busy intervals view
-- the availability engine (TEC-697) reads.
--
-- planned is the half-open range [planned departure, planned arrival).
-- Lifecycle: planned -> in_transit (actual_departure, vehicle status
-- in_transfer) -> completed (actual_arrival, vehicle current branch =
-- to_branch); planned / in_transit -> cancelled. Transfers are never
-- deleted: a cancelled transfer is the record that it was cancelled.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE vehicle_transfers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    from_branch_id BIGINT NOT NULL,
    to_branch_id BIGINT NOT NULL,
    -- The member who drives / organises the transfer.
    responsible_user_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    planned TSTZRANGE NOT NULL,
    actual_departure TIMESTAMPTZ NULL,
    actual_arrival TIMESTAMPTZ NULL,
    -- TRY.
    cost NUMERIC(14, 2) NULL CHECK (cost >= 0),
    status VARCHAR(16) NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'in_transit', 'completed', 'cancelled')),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vehicle_transfers_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_vehicle_transfers_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vehicle_transfers_from_branch
        FOREIGN KEY (organization_id, from_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_vehicle_transfers_to_branch
        FOREIGN KEY (organization_id, to_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT chk_vehicle_transfers_branches CHECK (from_branch_id <> to_branch_id),
    -- Both planned bounds are finite: [departure, arrival).
    CONSTRAINT chk_vehicle_transfers_planned CHECK (
        NOT isempty(planned) AND NOT lower_inf(planned) AND NOT upper_inf(planned)
        AND lower_inc(planned) AND NOT upper_inc(planned)
    ),
    CONSTRAINT chk_vehicle_transfers_actual CHECK (
        actual_arrival IS NULL OR (actual_departure IS NOT NULL AND actual_arrival >= actual_departure)
    ),
    CONSTRAINT chk_vehicle_transfers_lifecycle CHECK (
        (status = 'planned' AND actual_departure IS NULL AND actual_arrival IS NULL)
        OR (status = 'in_transit' AND actual_departure IS NOT NULL AND actual_arrival IS NULL)
        OR (status = 'completed' AND actual_departure IS NOT NULL AND actual_arrival IS NOT NULL)
        OR status = 'cancelled'
    ),
    -- One open (planned / in transit) transfer of a vehicle at a time.
    CONSTRAINT ex_vehicle_transfers_overlap EXCLUDE USING gist (
        organization_id WITH =,
        vehicle_id WITH =,
        planned WITH &&
    ) WHERE (status IN ('planned', 'in_transit'))
);

CREATE INDEX idx_vehicle_transfers_organization_vehicle
    ON vehicle_transfers (organization_id, vehicle_id, lower(planned) DESC);

CREATE INDEX idx_vehicle_transfers_organization_from_branch
    ON vehicle_transfers (organization_id, from_branch_id);

CREATE INDEX idx_vehicle_transfers_organization_to_branch
    ON vehicle_transfers (organization_id, to_branch_id);

CREATE INDEX idx_vehicle_transfers_organization_status
    ON vehicle_transfers (organization_id, status, lower(planned));

CREATE TRIGGER trg_vehicle_transfers_updated_at
    BEFORE UPDATE ON vehicle_transfers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The one place the availability engine reads "when is vehicle V busy"
-- (docs/design/fleet-availability-inputs.md). kind: block | transfer
-- (reservations join in M2). during is half-open [start, end); an unbounded
-- upper means "until further notice". branch_after_id is the branch the
-- vehicle is at once the interval ends (transfers: to_branch; blocks: NULL =
-- where it already was).
--
-- Transfer intervals: planned -> the planned range; in_transit -> from the
-- actual departure until the planned arrival, or until now when the arrival
-- is late (the vehicle stays busy until someone completes it); completed ->
-- [actual departure, actual arrival). Cancelled transfers and deleted
-- blocks are not busy.
CREATE VIEW vehicle_busy_intervals AS
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
WHERE t.status <> 'cancelled';
