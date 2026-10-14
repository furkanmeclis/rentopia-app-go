-- One-way rules (TEC-695): may a rental picked up at from_branch be returned
-- at to_branch, and for which fee (TRY). A rule with a rental product
-- ("alt grup" in the beta doc) overrides the generic rule of the same route
-- for that product. Routes are directional (A -> B and B -> A are two
-- rules). Every index starts with organization_id.

CREATE TABLE one_way_rules (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    from_branch_id BIGINT NOT NULL,
    to_branch_id BIGINT NOT NULL,
    -- NULL = every product (generic rule of the route).
    rental_product_id BIGINT NULL,
    allowed BOOLEAN NOT NULL DEFAULT TRUE,
    -- TRY; 0 when allowed for free. Ignored when allowed is false.
    fee NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (fee >= 0),
    note TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_one_way_rules_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_one_way_rules_from_branch
        FOREIGN KEY (organization_id, from_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_one_way_rules_to_branch
        FOREIGN KEY (organization_id, to_branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_one_way_rules_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id),
    CONSTRAINT chk_one_way_rules_branches CHECK (from_branch_id <> to_branch_id)
);

-- One live rule per route and product; a NULL product counts as one value
-- (one generic rule per route).
CREATE UNIQUE INDEX uq_one_way_rules_organization_route
    ON one_way_rules (organization_id, from_branch_id, to_branch_id, COALESCE(rental_product_id, 0))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_one_way_rules_organization_to_branch
    ON one_way_rules (organization_id, to_branch_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_one_way_rules_organization_created
    ON one_way_rules (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_one_way_rules_updated_at
    BEFORE UPDATE ON one_way_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
