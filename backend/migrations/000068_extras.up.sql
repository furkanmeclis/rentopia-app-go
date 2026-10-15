-- Extras catalog (TEC-702, beta doc §7): additional services sold with a
-- rental (additional driver, child seat, km package, delivery …).
--
-- charge_type decides how the engine prices a line (docs/design/extras.md):
--   per_day     unit price x quantity x billing days
--   per_unit    unit price x quantity (once per rental)
--   per_rental  unit price once (quantity is always 1)
-- max_total caps the line total (e.g. "child seat 150 TL/day, at most
-- 1 000 TL per rental"). channels NULL = every sales channel (orgsettings
-- codes). Physical extras (is_physical) have a stock per branch and timed
-- allocations (migration 000069).
--
-- Prices live in extra_prices: one row per (extra, rental product) and a
-- default row (rental_product_id NULL) used when the product has none.
-- Rows are effective-dated and immutable: a price change closes the
-- current row (valid_to) and inserts the next one, so a quote can keep the
-- price row uuid it was priced with (snapshot). Rows of one extra and
-- product never overlap in time (EXCLUDE).
--
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE extras (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    extra_type VARCHAR(24) NOT NULL
        CHECK (extra_type IN ('additional_driver', 'child_seat', 'km_package', 'delivery', 'other')),
    charge_type VARCHAR(16) NOT NULL CHECK (charge_type IN ('per_day', 'per_unit', 'per_rental')),
    min_quantity INTEGER NOT NULL DEFAULT 1 CHECK (min_quantity BETWEEN 1 AND 100),
    max_quantity INTEGER NOT NULL DEFAULT 1 CHECK (max_quantity BETWEEN 1 AND 100),
    -- Cap of the line total (NULL = no cap).
    max_total NUMERIC(14, 2) NULL CHECK (max_total >= 0),
    -- Sales channel codes; NULL = every channel.
    channels VARCHAR(16)[] NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    is_physical BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 10000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_extras_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_extras_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_extras_quantity CHECK (max_quantity >= min_quantity),
    CONSTRAINT chk_extras_per_rental CHECK (charge_type <> 'per_rental' OR (min_quantity = 1 AND max_quantity = 1))
);

-- Codes are unique per organization among live extras (case-insensitive).
CREATE UNIQUE INDEX uq_extras_organization_code
    ON extras (organization_id, lower(code))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_extras_organization_created
    ON extras (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_extras_organization_status
    ON extras (organization_id, status, sort_order)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_extras_updated_at
    BEFORE UPDATE ON extras
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE extra_prices (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    extra_id BIGINT NOT NULL,
    -- NULL = the default price of the extra.
    rental_product_id BIGINT NULL,
    price NUMERIC(14, 2) NOT NULL CHECK (price >= 0),
    -- [valid_from, valid_to); valid_to NULL = open.
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_extra_prices_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_extra_prices_extra
        FOREIGN KEY (organization_id, extra_id)
        REFERENCES extras (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_extra_prices_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_extra_prices_range CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT ex_extra_prices_overlap EXCLUDE USING gist (
        organization_id WITH =,
        extra_id WITH =,
        (COALESCE(rental_product_id, 0)) WITH =,
        tstzrange(valid_from, valid_to) WITH &&
    )
);

CREATE INDEX idx_extra_prices_organization_extra
    ON extra_prices (organization_id, extra_id, rental_product_id, valid_from DESC);

CREATE INDEX idx_extra_prices_organization_product
    ON extra_prices (organization_id, rental_product_id)
    WHERE rental_product_id IS NOT NULL;

-- Price rows are immutable: only an open or future end (valid_to) may be
-- set or moved earlier, which is how a price change closes the current
-- row. created_by may still be cleared by ON DELETE SET NULL.
CREATE FUNCTION extra_prices_immutable() RETURNS trigger AS $$
BEGIN
    IF NEW.organization_id IS DISTINCT FROM OLD.organization_id
       OR NEW.uuid IS DISTINCT FROM OLD.uuid
       OR NEW.extra_id IS DISTINCT FROM OLD.extra_id
       OR NEW.rental_product_id IS DISTINCT FROM OLD.rental_product_id
       OR NEW.price IS DISTINCT FROM OLD.price
       OR NEW.valid_from IS DISTINCT FROM OLD.valid_from
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR (NEW.created_by IS NOT NULL AND NEW.created_by IS DISTINCT FROM OLD.created_by)
       OR (OLD.valid_to IS NOT NULL AND (NEW.valid_to IS NULL OR NEW.valid_to > OLD.valid_to)) THEN
        RAISE EXCEPTION 'extra prices are immutable; close the row and insert a new one' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_extra_prices_immutable
    BEFORE UPDATE ON extra_prices
    FOR EACH ROW EXECUTE FUNCTION extra_prices_immutable();
