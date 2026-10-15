-- Rate plans (TEC-700): the daily price of a rental product ("alt grup")
-- by rental length (duration bands), optionally overridden per branch,
-- sales channel or both. Precedence in the pricing engine: branch+channel →
-- branch → channel → common, then the highest priority.
--
-- Versioning (docs/design/pricing.md): the plan row holds what never
-- affects a price (product, scope target, name) and the pointers; prices
-- (priority + bands) live in rate_plan_versions. Edits go into the single
-- draft version; activation freezes it (status active), the previous active
-- version becomes superseded. Non-draft versions and their bands are
-- immutable (triggers below), so a reservation can keep pointing at the
-- version it was priced with.
--
-- Channels are the settings' sales channel codes (orgsettings), not rows.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE rate_plans (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_product_id BIGINT NOT NULL,
    scope VARCHAR(16) NOT NULL CHECK (scope IN ('common', 'branch', 'channel', 'branch_channel')),
    branch_id BIGINT NULL,
    channel VARCHAR(32) NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    -- draft: never activated; active: the engine uses active_version;
    -- inactive: switched off (versions kept).
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'inactive')),
    -- Highest version number (draft included).
    current_version INTEGER NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    -- Version number of the active (frozen) version; NULL until the first
    -- activation. The "current pointer" of the versioned record pattern.
    active_version INTEGER NULL CHECK (active_version >= 1),
    activated_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_rate_plans_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rate_plans_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_rate_plans_product
        FOREIGN KEY (organization_id, rental_product_id)
        REFERENCES rental_products (organization_id, id),
    CONSTRAINT fk_rate_plans_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT chk_rate_plans_branch_scope
        CHECK ((scope IN ('branch', 'branch_channel')) = (branch_id IS NOT NULL)),
    CONSTRAINT chk_rate_plans_channel_scope
        CHECK ((scope IN ('channel', 'branch_channel')) = (channel IS NOT NULL)),
    CONSTRAINT chk_rate_plans_active_version
        CHECK (status = 'draft' OR active_version IS NOT NULL)
);

CREATE INDEX idx_rate_plans_organization_product
    ON rate_plans (organization_id, rental_product_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_rate_plans_organization_branch
    ON rate_plans (organization_id, branch_id)
    WHERE deleted_at IS NULL AND branch_id IS NOT NULL;

CREATE INDEX idx_rate_plans_organization_created
    ON rate_plans (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_rate_plans_updated_at
    BEFORE UPDATE ON rate_plans
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE rate_plan_versions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    rate_plan_id BIGINT NOT NULL,
    version INTEGER NOT NULL CHECK (version >= 1),
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'superseded')),
    -- Higher wins inside the same scope target.
    priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 1000),
    change_note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    activated_at TIMESTAMPTZ NULL,
    superseded_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_rate_plan_versions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_rate_plan_versions_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_rate_plan_versions_plan_version UNIQUE (organization_id, rate_plan_id, version),
    CONSTRAINT fk_rate_plan_versions_plan
        FOREIGN KEY (organization_id, rate_plan_id)
        REFERENCES rate_plans (organization_id, id) ON DELETE CASCADE
);

-- At most one draft and one active version per plan.
CREATE UNIQUE INDEX uq_rate_plan_versions_organization_draft
    ON rate_plan_versions (organization_id, rate_plan_id) WHERE status = 'draft';
CREATE UNIQUE INDEX uq_rate_plan_versions_organization_active
    ON rate_plan_versions (organization_id, rate_plan_id) WHERE status = 'active';

-- Duration bands of a version: min_days..max_days inclusive (max NULL =
-- open-ended). Bands of one version never overlap (EXCLUDE). Online gap
-- checks run at activation (they depend on the settings).
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE rate_bands (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    rate_plan_version_id BIGINT NOT NULL,
    min_days INTEGER NOT NULL CHECK (min_days BETWEEN 1 AND 3650),
    max_days INTEGER NULL CHECK (max_days BETWEEN 1 AND 3650),
    daily_price NUMERIC(14, 2) NOT NULL CHECK (daily_price >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_rate_bands_version
        FOREIGN KEY (organization_id, rate_plan_version_id)
        REFERENCES rate_plan_versions (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_rate_bands_range CHECK (max_days IS NULL OR max_days >= min_days),
    CONSTRAINT ex_rate_bands_overlap EXCLUDE USING gist (
        organization_id WITH =,
        rate_plan_version_id WITH =,
        int4range(min_days, max_days, '[]') WITH &&
    )
);

CREATE INDEX idx_rate_bands_organization_version
    ON rate_bands (organization_id, rate_plan_version_id, min_days);

-- Frozen versions: once a version leaves 'draft' its content never changes.
-- The only transition left is active -> superseded (status, superseded_at).
-- created_by / activated_by may still be cleared by ON DELETE SET NULL.
CREATE FUNCTION rate_plan_versions_immutable() RETURNS trigger AS $$
BEGIN
    IF OLD.status = 'draft' THEN
        RETURN NEW;
    END IF;
    IF NEW.organization_id IS DISTINCT FROM OLD.organization_id
       OR NEW.rate_plan_id IS DISTINCT FROM OLD.rate_plan_id
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.priority IS DISTINCT FROM OLD.priority
       OR NEW.change_note IS DISTINCT FROM OLD.change_note
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
       OR (NEW.created_by IS NOT NULL AND NEW.created_by IS DISTINCT FROM OLD.created_by)
       OR (NEW.activated_by IS NOT NULL AND NEW.activated_by IS DISTINCT FROM OLD.activated_by)
       OR NOT (NEW.status = OLD.status OR (OLD.status = 'active' AND NEW.status = 'superseded')) THEN
        RAISE EXCEPTION 'rate plan versions are immutable once activated' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rate_plan_versions_immutable
    BEFORE UPDATE ON rate_plan_versions
    FOR EACH ROW EXECUTE FUNCTION rate_plan_versions_immutable();

-- Bands of a frozen version cannot be added, changed or removed. A cascade
-- from a deleted version (organization removal) finds no parent and passes.
CREATE FUNCTION rate_bands_immutable() RETURNS trigger AS $$
DECLARE
    parent_status TEXT;
    row_data rate_bands;
BEGIN
    IF TG_OP = 'DELETE' THEN
        row_data := OLD;
    ELSE
        row_data := NEW;
    END IF;
    SELECT status INTO parent_status
    FROM rate_plan_versions
    WHERE organization_id = row_data.organization_id AND id = row_data.rate_plan_version_id;
    IF parent_status IS NOT NULL AND parent_status <> 'draft' THEN
        RAISE EXCEPTION 'bands of an activated rate plan version are immutable' USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'UPDATE' AND (OLD.rate_plan_version_id IS DISTINCT FROM NEW.rate_plan_version_id
                             OR OLD.organization_id IS DISTINCT FROM NEW.organization_id) THEN
        RAISE EXCEPTION 'bands cannot move between versions' USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rate_bands_immutable
    BEFORE INSERT OR UPDATE OR DELETE ON rate_bands
    FOR EACH ROW EXECUTE FUNCTION rate_bands_immutable();
