-- Rentable products (TEC-691; "alt grup" in the beta doc). A product is what
-- a channel sells. Its guarantee (what the customer is promised) and its
-- rental conditions live in immutable versions: changing either creates
-- version N+1, so a booking can keep pointing at the version it was sold
-- with. Title, description, images and publish status are not versioned.
-- Product resource sets may overlap; no stock count is stored anywhere.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE rental_products (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Stable internal code: channel mappings refer to it.
    code VARCHAR(32) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    segment_id BIGINT NOT NULL,
    -- Representative catalog model / type for the "X veya benzeri" text.
    representative_model_id BIGINT NULL REFERENCES catalog_models (id) ON DELETE SET NULL,
    representative_type_id BIGINT NULL REFERENCES catalog_types (id) ON DELETE SET NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    -- Latest version number (the current one); kept in the same transaction
    -- that inserts the version row.
    current_version INTEGER NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_rental_products_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rental_products_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_rental_products_segment
        FOREIGN KEY (organization_id, segment_id)
        REFERENCES vehicle_segments (organization_id, id)
);

CREATE UNIQUE INDEX uq_rental_products_organization_code
    ON rental_products (organization_id, code)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_rental_products_organization_created
    ON rental_products (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_rental_products_organization_segment
    ON rental_products (organization_id, segment_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_rental_products_updated_at
    BEFORE UPDATE ON rental_products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE rental_product_versions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    version INTEGER NOT NULL CHECK (version >= 1),
    guarantee_type VARCHAR(16) NOT NULL CHECK (guarantee_type IN ('EXACT_VEHICLE', 'EXACT_VARIANT', 'GROUP')),
    -- EXACT_VEHICLE: the guaranteed vehicle. Placeholder without a foreign
    -- key until the vehicles module exists (TEC-691); may stay NULL.
    vehicle_id UUID NULL,
    -- EXACT_VARIANT: the guaranteed catalog model (level 'model') or type
    -- (level 'type'; catalog_model_id then holds the type's model).
    variant_level VARCHAR(8) NULL CHECK (variant_level IN ('model', 'type')),
    catalog_model_id BIGINT NULL REFERENCES catalog_models (id) ON DELETE SET NULL,
    catalog_type_id BIGINT NULL REFERENCES catalog_types (id) ON DELETE SET NULL,
    -- GROUP: the rental group whose protected attributes are guaranteed.
    rental_group_id BIGINT NULL,
    -- Human-readable snapshot of the guaranteed reference at version time
    -- (survives catalog renames / removals).
    guarantee_label TEXT NOT NULL DEFAULT '',
    -- Rental conditions.
    deposit_amount NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (deposit_amount >= 0),
    min_driver_age SMALLINT NOT NULL DEFAULT 21 CHECK (min_driver_age BETWEEN 18 AND 99),
    min_license_years SMALLINT NOT NULL DEFAULT 1 CHECK (min_license_years BETWEEN 0 AND 60),
    -- NULL = unlimited kilometres.
    included_km_per_day INTEGER NULL CHECK (included_km_per_day BETWEEN 0 AND 100000),
    extra_km_fee NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (extra_km_fee >= 0),
    fuel_policy VARCHAR(16) NOT NULL DEFAULT 'same_to_same'
        CHECK (fuel_policy IN ('same_to_same', 'full_to_full', 'prepaid', 'full_to_empty')),
    notes TEXT NOT NULL DEFAULT '',
    change_note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rental_product_versions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rental_product_versions_product_version UNIQUE (organization_id, product_id, version),
    CONSTRAINT fk_rental_product_versions_product
        FOREIGN KEY (organization_id, product_id)
        REFERENCES rental_products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_rental_product_versions_group
        FOREIGN KEY (organization_id, rental_group_id)
        REFERENCES rental_groups (organization_id, id),
    CONSTRAINT chk_rental_product_versions_vehicle
        CHECK (vehicle_id IS NULL OR guarantee_type = 'EXACT_VEHICLE'),
    CONSTRAINT chk_rental_product_versions_variant
        CHECK ((guarantee_type = 'EXACT_VARIANT') = (variant_level IS NOT NULL)),
    CONSTRAINT chk_rental_product_versions_group
        CHECK (rental_group_id IS NULL OR guarantee_type = 'GROUP')
);

CREATE INDEX idx_rental_product_versions_organization_group
    ON rental_product_versions (organization_id, rental_group_id)
    WHERE rental_group_id IS NOT NULL;

-- Past versions are immutable. The only update allowed is the platform
-- catalog clearing a removed model / type (ON DELETE SET NULL).
CREATE FUNCTION rental_product_versions_immutable() RETURNS trigger AS $$
BEGIN
    IF (to_jsonb(NEW) - 'catalog_model_id' - 'catalog_type_id') IS DISTINCT FROM
       (to_jsonb(OLD) - 'catalog_model_id' - 'catalog_type_id')
       OR (NEW.catalog_model_id IS NOT NULL AND NEW.catalog_model_id IS DISTINCT FROM OLD.catalog_model_id)
       OR (NEW.catalog_type_id IS NOT NULL AND NEW.catalog_type_id IS DISTINCT FROM OLD.catalog_type_id) THEN
        RAISE EXCEPTION 'rental product versions are immutable' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rental_product_versions_immutable
    BEFORE UPDATE ON rental_product_versions
    FOR EACH ROW EXECUTE FUNCTION rental_product_versions_immutable();

-- Product images (object storage; bytes in S3, metadata here). Served
-- through the API (public_url); the browser never builds object URLs.
CREATE TABLE rental_product_images (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    object_key TEXT NOT NULL,
    filename VARCHAR(255) NOT NULL DEFAULT '',
    content_type VARCHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rental_product_images_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_rental_product_images_product
        FOREIGN KEY (organization_id, product_id)
        REFERENCES rental_products (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_rental_product_images_organization_product
    ON rental_product_images (organization_id, product_id, sort_order);
