-- Fleet classification (TEC-691): vehicle segments. A segment ("araç grubu"
-- in the beta doc) is a commercial class such as Ekonomi or SUV. It holds no
-- stock count: capacity is computed later from eligible vehicles.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE vehicle_segments (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 100000),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_vehicle_segments_organization_uuid UNIQUE (organization_id, uuid),
    -- Target of the composite foreign keys from rental_groups / rental_products.
    CONSTRAINT uq_vehicle_segments_organization_id UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX uq_vehicle_segments_organization_code
    ON vehicle_segments (organization_id, code)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vehicle_segments_organization_sort
    ON vehicle_segments (organization_id, sort_order, code)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_vehicle_segments_updated_at
    BEFORE UPDATE ON vehicle_segments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
