-- Named date rules (TEC-701): "YAZ", "BAYRAM" … adjust the daily price of
-- the days that start inside [start_date, end_date) (the end day is
-- excluded) by a signed percentage or amount. The pricing engine applies the
-- single highest-priority matching rule per billing day; activation refuses
-- an equal-priority overlap with another active rule.
--
-- Versioned like rate plans: the rule row holds the name and pointers, the
-- price-affecting fields live in date_rule_versions (one draft, one active,
-- frozen after activation). Product / segment / branch selections are child
-- rows of the version (composite foreign keys: never another organization's
-- record); channels are settings codes (text[]).
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE date_rules (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'inactive')),
    current_version INTEGER NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    active_version INTEGER NULL CHECK (active_version >= 1),
    activated_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_date_rules_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_date_rules_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_date_rules_active_version CHECK (status = 'draft' OR active_version IS NOT NULL)
);

-- Names are unique per organization among live rules (case-insensitive).
CREATE UNIQUE INDEX uq_date_rules_organization_name
    ON date_rules (organization_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_date_rules_organization_created
    ON date_rules (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_date_rules_updated_at
    BEFORE UPDATE ON date_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE date_rule_versions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    date_rule_id BIGINT NOT NULL,
    version INTEGER NOT NULL CHECK (version >= 1),
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'superseded')),
    -- [start_date, end_date): end_date is the first day NOT covered.
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    product_scope VARCHAR(16) NOT NULL DEFAULT 'all' CHECK (product_scope IN ('all', 'segments', 'products')),
    all_branches BOOLEAN NOT NULL DEFAULT TRUE,
    -- NULL = every channel; otherwise the selected settings channel codes.
    channels TEXT[] NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('percent', 'amount')),
    -- Signed: a percentage (-100..1000) for percent, an amount for amount.
    value NUMERIC(14, 2) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 1000),
    change_note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    activated_at TIMESTAMPTZ NULL,
    superseded_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_date_rule_versions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_date_rule_versions_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_date_rule_versions_rule_version UNIQUE (organization_id, date_rule_id, version),
    CONSTRAINT fk_date_rule_versions_rule
        FOREIGN KEY (organization_id, date_rule_id)
        REFERENCES date_rules (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_date_rule_versions_range CHECK (end_date > start_date),
    CONSTRAINT chk_date_rule_versions_percent CHECK (kind <> 'percent' OR value BETWEEN -100 AND 1000)
);

CREATE UNIQUE INDEX uq_date_rule_versions_organization_draft
    ON date_rule_versions (organization_id, date_rule_id) WHERE status = 'draft';
CREATE UNIQUE INDEX uq_date_rule_versions_organization_active
    ON date_rule_versions (organization_id, date_rule_id) WHERE status = 'active';
-- Engine lookups: active versions overlapping a rental window.
CREATE INDEX idx_date_rule_versions_organization_active_range
    ON date_rule_versions (organization_id, start_date, end_date) WHERE status = 'active';

-- Selected products / segments / branches of a version (exactly one
-- reference per row).
CREATE TABLE date_rule_version_targets (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    date_rule_version_id BIGINT NOT NULL,
    rental_product_id BIGINT NULL,
    segment_id BIGINT NULL,
    branch_id BIGINT NULL,
    CONSTRAINT fk_date_rule_version_targets_version
        FOREIGN KEY (organization_id, date_rule_version_id)
        REFERENCES date_rule_versions (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_date_rule_version_targets_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id),
    CONSTRAINT fk_date_rule_version_targets_segment
        FOREIGN KEY (organization_id, segment_id)
        REFERENCES vehicle_segments (organization_id, id),
    CONSTRAINT fk_date_rule_version_targets_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT chk_date_rule_version_targets_one CHECK (num_nonnulls(rental_product_id, segment_id, branch_id) = 1)
);

CREATE INDEX idx_date_rule_version_targets_organization_version
    ON date_rule_version_targets (organization_id, date_rule_version_id);

CREATE FUNCTION date_rule_versions_immutable() RETURNS trigger AS $$
BEGIN
    IF OLD.status = 'draft' THEN
        RETURN NEW;
    END IF;
    IF (to_jsonb(NEW) - 'status' - 'superseded_at' - 'created_by' - 'activated_by') IS DISTINCT FROM
       (to_jsonb(OLD) - 'status' - 'superseded_at' - 'created_by' - 'activated_by')
       OR (NEW.created_by IS NOT NULL AND NEW.created_by IS DISTINCT FROM OLD.created_by)
       OR (NEW.activated_by IS NOT NULL AND NEW.activated_by IS DISTINCT FROM OLD.activated_by)
       OR NOT (NEW.status = OLD.status OR (OLD.status = 'active' AND NEW.status = 'superseded')) THEN
        RAISE EXCEPTION 'date rule versions are immutable once activated' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_date_rule_versions_immutable
    BEFORE UPDATE ON date_rule_versions
    FOR EACH ROW EXECUTE FUNCTION date_rule_versions_immutable();

CREATE FUNCTION date_rule_version_targets_immutable() RETURNS trigger AS $$
DECLARE
    parent_status TEXT;
    row_data date_rule_version_targets;
BEGIN
    IF TG_OP = 'DELETE' THEN
        row_data := OLD;
    ELSE
        row_data := NEW;
    END IF;
    SELECT status INTO parent_status
    FROM date_rule_versions
    WHERE organization_id = row_data.organization_id AND id = row_data.date_rule_version_id;
    IF parent_status IS NOT NULL AND parent_status <> 'draft' THEN
        RAISE EXCEPTION 'targets of an activated date rule version are immutable' USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_date_rule_version_targets_immutable
    BEFORE INSERT OR UPDATE OR DELETE ON date_rule_version_targets
    FOR EACH ROW EXECUTE FUNCTION date_rule_version_targets_immutable();
