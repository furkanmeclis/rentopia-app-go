-- Fleet vehicles (TEC-692): the physical reality of a vehicle only. What is
-- sold lives in rental products; when a vehicle is busy lives in the later
-- capacity tables. Every index starts with organization_id (tenant
-- isolation guard rail).
--
-- Pools: fleet_pool is a short per-organization code (default MAIN).
-- Vehicles of different pools never share capacity; there is no pool table
-- until pools need more than a code.

CREATE TABLE vehicles (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Display plate ("34 ABC 123"); it may change over the vehicle's life.
    plate VARCHAR(20) NOT NULL,
    -- Uniqueness / search key: upper-case letters and digits only.
    plate_key VARCHAR(20) GENERATED ALWAYS AS (upper(regexp_replace(plate, '[^A-Za-z0-9]', '', 'g'))) STORED,
    vin VARCHAR(17) NOT NULL DEFAULT '',
    -- Platform catalog type, or a manual entry when the vehicle is not in
    -- the catalog. Catalog types are never deleted while a vehicle uses one.
    catalog_type_id BIGINT NULL REFERENCES catalog_types (id) ON DELETE RESTRICT,
    manual_entry BOOLEAN NOT NULL DEFAULT FALSE,
    manual_brand VARCHAR(128) NOT NULL DEFAULT '',
    manual_model VARCHAR(128) NOT NULL DEFAULT '',
    manual_type VARCHAR(255) NOT NULL DEFAULT '',
    model_year SMALLINT NOT NULL CHECK (model_year BETWEEN 1950 AND 2100),
    -- Verified physical attributes (feed rental group rules).
    transmission VARCHAR(16) NULL CHECK (transmission IN ('manual', 'automatic')),
    fuel VARCHAR(16) NULL CHECK (fuel IN ('petrol', 'diesel', 'hybrid', 'electric', 'lpg')),
    body_type VARCHAR(16) NULL CHECK (body_type IN (
        'hatchback', 'sedan', 'suv', 'crossover', 'station_wagon', 'mpv', 'van', 'pickup', 'coupe', 'cabrio'
    )),
    seats SMALLINT NULL CHECK (seats BETWEEN 1 AND 60),
    luggage SMALLINT NULL CHECK (luggage BETWEEN 0 AND 30),
    air_conditioning BOOLEAN NOT NULL DEFAULT TRUE,
    -- TRUE once a user confirmed the attributes (form save); imports leave
    -- it FALSE.
    attributes_verified BOOLEAN NOT NULL DEFAULT FALSE,
    color VARCHAR(64) NOT NULL DEFAULT '',
    home_branch_id BIGINT NOT NULL,
    current_branch_id BIGINT NOT NULL,
    fleet_pool VARCHAR(32) NOT NULL DEFAULT 'MAIN' CHECK (fleet_pool ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    ownership VARCHAR(16) NOT NULL DEFAULT 'owned' CHECK (ownership IN ('owned', 'supplied')),
    supplier_name VARCHAR(255) NOT NULL DEFAULT '',
    -- Outside [in_service_date, out_of_service_date] the vehicle does not count.
    in_service_date DATE NULL,
    out_of_service_date DATE NULL,
    odometer_km INTEGER NOT NULL DEFAULT 0 CHECK (odometer_km BETWEEN 0 AND 5000000),
    -- Percent (0-100); NULL = unknown.
    fuel_level SMALLINT NULL CHECK (fuel_level BETWEEN 0 AND 100),
    status VARCHAR(16) NOT NULL DEFAULT 'available' CHECK (status IN (
        'available', 'preparing', 'rented', 'return_check', 'cleaning',
        'maintenance', 'damaged', 'in_transfer', 'out_of_service'
    )),
    status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_vehicles_organization_uuid UNIQUE (organization_id, uuid),
    -- Target of the composite foreign keys from vehicle_events / vehicle_rental_groups.
    CONSTRAINT uq_vehicles_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vehicles_home_branch
        FOREIGN KEY (organization_id, home_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_vehicles_current_branch
        FOREIGN KEY (organization_id, current_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT chk_vehicles_catalog_or_manual CHECK (
        (manual_entry AND catalog_type_id IS NULL AND btrim(manual_brand) <> '' AND btrim(manual_model) <> '')
        OR (NOT manual_entry AND catalog_type_id IS NOT NULL)
    ),
    CONSTRAINT chk_vehicles_service_dates CHECK (
        in_service_date IS NULL OR out_of_service_date IS NULL OR out_of_service_date >= in_service_date
    ),
    CONSTRAINT chk_vehicles_vin CHECK (vin = '' OR vin ~ '^[A-HJ-NPR-Z0-9]{17}$')
);

-- One active, live vehicle per plate in an organization.
CREATE UNIQUE INDEX uq_vehicles_organization_plate
    ON vehicles (organization_id, plate_key)
    WHERE deleted_at IS NULL AND is_active;

CREATE INDEX idx_vehicles_organization_created
    ON vehicles (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vehicles_organization_home_branch
    ON vehicles (organization_id, home_branch_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vehicles_organization_current_branch
    ON vehicles (organization_id, current_branch_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vehicles_organization_status
    ON vehicles (organization_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vehicles_organization_vin
    ON vehicles (organization_id, vin)
    WHERE deleted_at IS NULL AND vin <> '';

CREATE INDEX idx_vehicles_organization_catalog_type
    ON vehicles (organization_id, catalog_type_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_vehicles_updated_at
    BEFORE UPDATE ON vehicles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Operational status history: the initial status and every transition.
-- Rows are never updated.
CREATE TABLE vehicle_events (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    vehicle_id BIGINT NOT NULL,
    event_type VARCHAR(16) NOT NULL DEFAULT 'status' CHECK (event_type IN ('created', 'status')),
    from_status VARCHAR(16) NULL,
    to_status VARCHAR(16) NOT NULL,
    odometer_km INTEGER NULL CHECK (odometer_km BETWEEN 0 AND 5000000),
    fuel_level SMALLINT NULL CHECK (fuel_level BETWEEN 0 AND 100),
    note TEXT NOT NULL DEFAULT '',
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vehicle_events_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_vehicle_events_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_vehicle_events_organization_vehicle
    ON vehicle_events (organization_id, vehicle_id, created_at DESC);

-- Rental group memberships, validated with fleet/eligibility on insert.
CREATE TABLE vehicle_rental_groups (
    organization_id BIGINT NOT NULL,
    vehicle_id BIGINT NOT NULL,
    rental_group_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, vehicle_id, rental_group_id),
    CONSTRAINT fk_vehicle_rental_groups_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vehicle_rental_groups_group
        FOREIGN KEY (organization_id, rental_group_id)
        REFERENCES rental_groups (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_vehicle_rental_groups_organization_group
    ON vehicle_rental_groups (organization_id, rental_group_id);
