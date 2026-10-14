-- Vehicle usage blocks (TEC-694): maintenance, damage, cleaning, out of
-- service or any other reason a vehicle cannot be used during a time range.
-- One input of the availability engine (TEC-697), read through the
-- vehicle_busy_intervals view (000053).
--
-- during is a half-open range [start, end). The upper bound may be
-- unbounded (open-ended block, "Bakıma al"); closing the block ("Engeli
-- kapat") sets it to the closing time. Blocks of the same vehicle never
-- overlap (EXCLUDE below): a vehicle is blocked for one reason at a time, so
-- the engine and the UI never have to merge or rank overlapping blocks;
-- extending a block or changing its kind is an update of the same row.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE vehicle_blocks (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('maintenance', 'damage', 'cleaning', 'out_of_service', 'other')),
    during TSTZRANGE NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    -- Set by "Engeli kapat" (the upper bound becomes released_at).
    released_at TIMESTAMPTZ NULL,
    released_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_vehicle_blocks_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_vehicle_blocks_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    -- [start, end) with a finite start; the end may be unbounded.
    CONSTRAINT chk_vehicle_blocks_during CHECK (
        NOT isempty(during) AND NOT lower_inf(during) AND lower_inc(during) AND NOT upper_inc(during)
    ),
    CONSTRAINT ex_vehicle_blocks_overlap EXCLUDE USING gist (
        organization_id WITH =,
        vehicle_id WITH =,
        during WITH &&
    ) WHERE (deleted_at IS NULL)
);

CREATE INDEX idx_vehicle_blocks_organization_vehicle
    ON vehicle_blocks (organization_id, vehicle_id, lower(during) DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_vehicle_blocks_updated_at
    BEFORE UPDATE ON vehicle_blocks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
