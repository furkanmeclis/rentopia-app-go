-- Quotes (TEC-704, spec §6.5). A quote is a priced search answer for one
-- rental product: the search input, the billing days, one line per billing
-- day (with the rate plan / date rule versions that priced it), the extras,
-- one-way and delivery fees, the discount, the VAT split, the total and the
-- prepayment, valid until valid_until (organization setting
-- reservation.quote_validity_minutes). A quote consumes no capacity.
--
-- A reservation is created from a quote: the server re-validates it (not
-- expired, same price now, capacity still there) and copies the breakdown
-- onto the reservation as an immutable snapshot. Change and extension
-- quotes belong to an existing reservation (purpose change / extension /
-- upgrade) and carry the difference to the reservation's current total.
--
-- Every index starts with organization_id (tenant isolation guard rail);
-- children use composite foreign keys.

CREATE TABLE quotes (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    purpose VARCHAR(16) NOT NULL DEFAULT 'new' CHECK (purpose IN ('new', 'change', 'extension', 'upgrade')),
    -- open: usable; used: a reservation / change consumed it; superseded: a
    -- newer quote replaced it (price changed at confirmation).
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'used', 'superseded')),
    -- Change / extension quotes: the reservation they change (FK added by
    -- the reservations migration).
    reservation_id BIGINT NULL,
    customer_id BIGINT NULL,
    -- Search input.
    rental_product_id BIGINT NOT NULL,
    product_version INTEGER NOT NULL,
    pickup_branch_id BIGINT NOT NULL,
    return_branch_id BIGINT NOT NULL,
    pickup_point_id BIGINT NULL,
    return_point_id BIGINT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    channel VARCHAR(16) NOT NULL,
    billing_days INTEGER NOT NULL CHECK (billing_days >= 1),
    -- Money (VAT inclusive, TRY): rental (after discount), extras, fees.
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    rental_subtotal NUMERIC(14, 2) NOT NULL,
    discount_total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    rental_total NUMERIC(14, 2) NOT NULL,
    extras_total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    one_way_fee NUMERIC(14, 2) NOT NULL DEFAULT 0,
    delivery_fee NUMERIC(14, 2) NOT NULL DEFAULT 0,
    base_total NUMERIC(14, 2) NOT NULL,
    vat_total NUMERIC(14, 2) NOT NULL,
    total NUMERIC(14, 2) NOT NULL,
    prepayment NUMERIC(14, 2) NOT NULL DEFAULT 0,
    prepayment_rule VARCHAR(16) NOT NULL DEFAULT 'none',
    -- VAT rate in basis points (2000 = %20); NULL = not configured.
    vat_rate_bp INTEGER NULL,
    -- Change quotes: total - the reservation's current total.
    difference NUMERIC(14, 2) NULL,
    -- Versions that priced it: the rate plan version and the settings
    -- (policy) version. Date rule versions are per day line.
    rate_plan_version_uuid UUID NULL,
    rate_plan_version INTEGER NULL,
    settings_version INTEGER NOT NULL DEFAULT 0,
    -- Requested extras ([{code, quantity}], TEC-702 prices them), coupon
    -- code (TEC-703) and the manual discount ({kind, value, reason}).
    extras JSONB NOT NULL DEFAULT '[]'::jsonb,
    coupon_code VARCHAR(64) NULL,
    manual_discount JSONB NULL,
    -- The full engine breakdown (pricing.Breakdown) plus the extension
    -- block for extension quotes; immutable once written.
    breakdown JSONB NOT NULL,
    -- Alternative vehicle of an extension that collides (RT-A31).
    vehicle_id BIGINT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_quotes_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_quotes_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_quotes_range CHECK (ends_at > starts_at),
    CONSTRAINT fk_quotes_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_quotes_product_version
        FOREIGN KEY (organization_id, rental_product_id, product_version)
        REFERENCES rental_product_versions (organization_id, product_id, version),
    CONSTRAINT fk_quotes_pickup_branch
        FOREIGN KEY (organization_id, pickup_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_quotes_return_branch
        FOREIGN KEY (organization_id, return_branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_quotes_pickup_point
        FOREIGN KEY (organization_id, pickup_point_id) REFERENCES service_points (organization_id, id),
    CONSTRAINT fk_quotes_return_point
        FOREIGN KEY (organization_id, return_point_id) REFERENCES service_points (organization_id, id),
    CONSTRAINT fk_quotes_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id) ON DELETE SET NULL (vehicle_id)
);

CREATE INDEX idx_quotes_organization_created ON quotes (organization_id, created_at DESC);
CREATE INDEX idx_quotes_organization_reservation ON quotes (organization_id, reservation_id) WHERE reservation_id IS NOT NULL;
CREATE INDEX idx_quotes_organization_customer ON quotes (organization_id, customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX idx_quotes_organization_pickup_branch ON quotes (organization_id, pickup_branch_id);
CREATE INDEX idx_quotes_organization_return_branch ON quotes (organization_id, return_branch_id);

CREATE TRIGGER trg_quotes_updated_at
    BEFORE UPDATE ON quotes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A quote's price never changes: only status / used_at move.
CREATE FUNCTION quotes_immutable() RETURNS trigger AS $$
BEGIN
    IF (to_jsonb(NEW) - 'status' - 'used_at' - 'updated_at' - 'reservation_id')
       IS DISTINCT FROM (to_jsonb(OLD) - 'status' - 'used_at' - 'updated_at' - 'reservation_id') THEN
        RAISE EXCEPTION 'quotes are immutable' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_quotes_immutable
    BEFORE UPDATE ON quotes
    FOR EACH ROW EXECUTE FUNCTION quotes_immutable();

-- One line per billing day, extra, fee and the discount, in display order.
CREATE TABLE quote_lines (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    quote_id BIGINT NOT NULL,
    position INTEGER NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('day', 'extra', 'one_way', 'delivery', 'discount')),
    -- Day lines: index (1-based), the Istanbul date the day starts on, the
    -- band price, the date rule version applied and its adjustment.
    day_index INTEGER NULL,
    day_date DATE NULL,
    date_rule_version_uuid UUID NULL,
    date_rule_name VARCHAR(255) NOT NULL DEFAULT '',
    code VARCHAR(64) NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL DEFAULT '',
    quantity INTEGER NOT NULL DEFAULT 1,
    unit_price NUMERIC(14, 2) NOT NULL DEFAULT 0,
    adjustment NUMERIC(14, 2) NOT NULL DEFAULT 0,
    amount NUMERIC(14, 2) NOT NULL,
    CONSTRAINT uq_quote_lines_position UNIQUE (organization_id, quote_id, position),
    CONSTRAINT fk_quote_lines_quote
        FOREIGN KEY (organization_id, quote_id) REFERENCES quotes (organization_id, id) ON DELETE CASCADE
);
