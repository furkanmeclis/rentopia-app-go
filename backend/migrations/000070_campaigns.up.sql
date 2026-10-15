-- Campaigns and coupons (TEC-703, beta doc §6.4, RT-A12).
--
-- campaign_type
--   coupon           applied when the customer / staff enters `code`
--                    (unique per organization among live campaigns,
--                    case-insensitive);
--   repeat_customer  applied automatically when the customer has at least
--                    min_completed_rentals completed rentals in the
--                    organization (rentals arrive in M3; until then the
--                    counter answers 0).
-- discount_kind percent (value 0..100) or amount (value in TRY). One
-- campaign per rental, on the rental subtotal; max_discount caps it and the
-- rental total never goes negative (pricing engine).
-- Validity: [start_date, end_date] inclusive, Europe/Istanbul calendar of
-- the moment the code is used (NULL = open).
-- channels NULL = every sales channel. product_scope all / segments /
-- products with campaign_targets. customer_uuids: optional list of customer
-- uuids (no foreign key yet: the customers table arrives with TEC-705).
--
-- Usage: used_count is an atomic counter
--   UPDATE campaigns SET used_count = used_count + 1
--   WHERE id = $1 AND (usage_limit IS NULL OR used_count < usage_limit)
-- and the CHECK below makes exceeding usage_limit impossible even for a
-- writer that forgets the condition. Redemptions (one per reservation)
-- record reserved -> consumed / released; release_on_cancel decides
-- whether a released redemption gives its usage back.
--
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE campaigns (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    campaign_type VARCHAR(24) NOT NULL CHECK (campaign_type IN ('coupon', 'repeat_customer')),
    code VARCHAR(32) NULL,
    discount_kind VARCHAR(16) NOT NULL CHECK (discount_kind IN ('percent', 'amount')),
    discount_value NUMERIC(14, 2) NOT NULL CHECK (discount_value > 0),
    max_discount NUMERIC(14, 2) NULL CHECK (max_discount > 0),
    min_subtotal NUMERIC(14, 2) NULL CHECK (min_subtotal >= 0),
    start_date DATE NULL,
    end_date DATE NULL,
    channels VARCHAR(16)[] NULL,
    product_scope VARCHAR(16) NOT NULL DEFAULT 'all' CHECK (product_scope IN ('all', 'segments', 'products')),
    customer_uuids UUID[] NULL,
    usage_limit INTEGER NULL CHECK (usage_limit BETWEEN 1 AND 10000000),
    per_customer_limit INTEGER NULL CHECK (per_customer_limit BETWEEN 1 AND 1000),
    used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    min_completed_rentals INTEGER NOT NULL DEFAULT 1 CHECK (min_completed_rentals BETWEEN 1 AND 1000),
    -- A cancelled / refunded reservation gives its usage back.
    release_on_cancel BOOLEAN NOT NULL DEFAULT TRUE,
    -- May be used on top of corporate agreement prices.
    corporate_eligible BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_campaigns_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_campaigns_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_campaigns_code CHECK ((campaign_type = 'coupon') = (code IS NOT NULL)),
    CONSTRAINT chk_campaigns_percent CHECK (discount_kind <> 'percent' OR discount_value <= 100),
    CONSTRAINT chk_campaigns_dates CHECK (start_date IS NULL OR end_date IS NULL OR end_date >= start_date),
    CONSTRAINT chk_campaigns_usage CHECK (usage_limit IS NULL OR used_count <= usage_limit)
);

CREATE UNIQUE INDEX uq_campaigns_organization_code
    ON campaigns (organization_id, lower(code))
    WHERE deleted_at IS NULL AND code IS NOT NULL;

CREATE INDEX idx_campaigns_organization_created
    ON campaigns (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_campaigns_organization_type
    ON campaigns (organization_id, campaign_type, is_active)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_campaigns_updated_at
    BEFORE UPDATE ON campaigns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Product scope targets: exactly one of product / segment per row.
CREATE TABLE campaign_targets (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    campaign_id BIGINT NOT NULL,
    rental_product_id BIGINT NULL,
    segment_id BIGINT NULL,
    CONSTRAINT fk_campaign_targets_campaign
        FOREIGN KEY (organization_id, campaign_id)
        REFERENCES campaigns (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_targets_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_targets_segment
        FOREIGN KEY (organization_id, segment_id)
        REFERENCES vehicle_segments (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_campaign_targets_one CHECK ((rental_product_id IS NULL) <> (segment_id IS NULL))
);

CREATE INDEX idx_campaign_targets_organization_campaign
    ON campaign_targets (organization_id, campaign_id);

CREATE TABLE campaign_redemptions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    campaign_id BIGINT NOT NULL,
    -- The reservation (TEC-706) the discount belongs to; no foreign key
    -- until the reservations table exists.
    reservation_uuid UUID NOT NULL,
    -- Customer (TEC-705); no foreign key yet.
    customer_uuid UUID NULL,
    code VARCHAR(32) NULL,
    subtotal NUMERIC(14, 2) NOT NULL CHECK (subtotal >= 0),
    discount_amount NUMERIC(14, 2) NOT NULL CHECK (discount_amount >= 0),
    status VARCHAR(16) NOT NULL DEFAULT 'reserved' CHECK (status IN ('reserved', 'consumed', 'released')),
    release_reason VARCHAR(32) NOT NULL DEFAULT '',
    -- TRUE when the release gave the usage back (release_on_cancel).
    usage_returned BOOLEAN NOT NULL DEFAULT FALSE,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consumed_at TIMESTAMPTZ NULL,
    released_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_campaign_redemptions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_campaign_redemptions_campaign
        FOREIGN KEY (organization_id, campaign_id)
        REFERENCES campaigns (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_campaign_redemptions_released CHECK ((status = 'released') = (released_at IS NOT NULL))
);

-- One discount per reservation (a released one may be replaced).
CREATE UNIQUE INDEX uq_campaign_redemptions_organization_reservation
    ON campaign_redemptions (organization_id, reservation_uuid)
    WHERE status <> 'released';

CREATE INDEX idx_campaign_redemptions_organization_campaign
    ON campaign_redemptions (organization_id, campaign_id, created_at DESC);

CREATE INDEX idx_campaign_redemptions_organization_customer
    ON campaign_redemptions (organization_id, campaign_id, customer_uuid)
    WHERE customer_uuid IS NOT NULL;
