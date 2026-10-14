-- Rental groups (TEC-691): the protected attributes an "X veya benzeri"
-- product guarantees, plus the catalog models / types a vehicle must match
-- to be eligible. No stock count: capacity is computed later.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE rental_groups (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Stable internal code: channel mappings refer to it.
    code VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    segment_id BIGINT NOT NULL,
    -- Protected attributes: NULL means "not guaranteed".
    transmission VARCHAR(16) NULL CHECK (transmission IN ('manual', 'automatic')),
    fuel VARCHAR(16) NULL CHECK (fuel IN ('petrol', 'diesel', 'hybrid', 'electric', 'lpg')),
    body_type VARCHAR(16) NULL CHECK (body_type IN (
        'hatchback', 'sedan', 'suv', 'crossover', 'station_wagon', 'mpv', 'van', 'pickup', 'coupe', 'cabrio'
    )),
    min_seats SMALLINT NULL CHECK (min_seats BETWEEN 1 AND 60),
    min_luggage SMALLINT NULL CHECK (min_luggage BETWEEN 0 AND 30),
    air_conditioning BOOLEAN NOT NULL DEFAULT FALSE,
    -- ACRISS (SIPP) code: classification only, never a guarantee.
    acriss_code VARCHAR(4) NULL CHECK (acriss_code ~ '^[A-Z]{4}$'),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_rental_groups_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rental_groups_organization_id UNIQUE (organization_id, id),
    -- A group always belongs to a segment of the same organization.
    CONSTRAINT fk_rental_groups_segment
        FOREIGN KEY (organization_id, segment_id)
        REFERENCES vehicle_segments (organization_id, id)
);

CREATE UNIQUE INDEX uq_rental_groups_organization_code
    ON rental_groups (organization_id, code)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_rental_groups_organization_created
    ON rental_groups (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_rental_groups_organization_segment
    ON rental_groups (organization_id, segment_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_rental_groups_updated_at
    BEFORE UPDATE ON rental_groups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Membership criteria: the platform catalog models / types a group accepts.
-- Empty lists = any vehicle that has the protected attributes.
-- A catalog row removed by the platform drops out of the criteria.
CREATE TABLE rental_group_catalog_models (
    organization_id BIGINT NOT NULL,
    rental_group_id BIGINT NOT NULL,
    catalog_model_id BIGINT NOT NULL REFERENCES catalog_models (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, rental_group_id, catalog_model_id),
    CONSTRAINT fk_rental_group_catalog_models_group
        FOREIGN KEY (organization_id, rental_group_id)
        REFERENCES rental_groups (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_rental_group_catalog_models_organization_model
    ON rental_group_catalog_models (organization_id, catalog_model_id);

CREATE TABLE rental_group_catalog_types (
    organization_id BIGINT NOT NULL,
    rental_group_id BIGINT NOT NULL,
    catalog_type_id BIGINT NOT NULL REFERENCES catalog_types (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, rental_group_id, catalog_type_id),
    CONSTRAINT fk_rental_group_catalog_types_group
        FOREIGN KEY (organization_id, rental_group_id)
        REFERENCES rental_groups (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_rental_group_catalog_types_organization_type
    ON rental_group_catalog_types (organization_id, catalog_type_id);
