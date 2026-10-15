-- Channel capacity rules (TEC-697). Without rules every channel sells from
-- the shared pool (default). Rules are separate concepts:
--
--   sales_limit       a channel may hold at most `quantity` concurrent
--                     commitments of a product; does not reserve anything.
--   guaranteed_quota  `quantity` units of a product are kept for a channel:
--                     other channels cannot take them while unused. The sum
--                     of quotas never exceeds the real capacity (checked on
--                     write against the eligible vehicles, capped again when
--                     evaluated).
--   safety_margin     `quantity` units of a product are never sold (every
--                     channel when channel is NULL, else that channel).
--
-- rental_product_id NULL = the rule applies to every product separately;
-- branch_id NULL = every pick-up branch. When several rules of a type match
-- a channel, the most specific wins (product + branch, product, branch,
-- generic). Every index starts with organization_id (tenant isolation guard
-- rail).

CREATE TABLE channel_capacity_rules (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rule_type VARCHAR(24) NOT NULL CHECK (rule_type IN ('sales_limit', 'guaranteed_quota', 'safety_margin')),
    channel VARCHAR(16) NULL,
    rental_product_id BIGINT NULL,
    branch_id BIGINT NULL,
    quantity INTEGER NOT NULL CHECK (quantity BETWEEN 0 AND 10000),
    note TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_channel_capacity_rules_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_channel_capacity_rules_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_capacity_rules_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    -- Limits and quotas belong to a channel; a margin may cover all.
    CONSTRAINT chk_channel_capacity_rules_channel CHECK (rule_type = 'safety_margin' OR channel IS NOT NULL)
);

-- One live rule per type, channel, product and branch.
CREATE UNIQUE INDEX uq_channel_capacity_rules_scope
    ON channel_capacity_rules (
        organization_id, rule_type, COALESCE(channel, ''), COALESCE(rental_product_id, 0), COALESCE(branch_id, 0)
    )
    WHERE deleted_at IS NULL;

CREATE INDEX idx_channel_capacity_rules_organization_created
    ON channel_capacity_rules (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_channel_capacity_rules_organization_product
    ON channel_capacity_rules (organization_id, rental_product_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_channel_capacity_rules_updated_at
    BEFORE UPDATE ON channel_capacity_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
