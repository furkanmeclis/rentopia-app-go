-- Platform vehicle catalog imported from the TSB (Turkish Insurance
-- Association) monthly kasko value list. Platform-level: not organization scoped.

CREATE TABLE catalog_brands (
    id           BIGSERIAL    PRIMARY KEY,
    uuid         UUID         NOT NULL DEFAULT gen_random_uuid(),
    tsb_code     TEXT         NOT NULL,
    tsb_name     TEXT         NOT NULL,
    display_name TEXT         NOT NULL,
    category     TEXT         NOT NULL DEFAULT 'passenger',
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_catalog_brands_uuid UNIQUE (uuid),
    CONSTRAINT uq_catalog_brands_tsb_code UNIQUE (tsb_code),
    CONSTRAINT chk_catalog_brands_category CHECK (category IN ('passenger', 'light_commercial', 'other')),
    CONSTRAINT chk_catalog_brands_display_name CHECK (btrim(display_name) <> '')
);

CREATE INDEX idx_catalog_brands_category ON catalog_brands (category, is_active);

CREATE TRIGGER trg_catalog_brands_set_updated_at
    BEFORE UPDATE ON catalog_brands
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE catalog_models (
    id         BIGSERIAL    PRIMARY KEY,
    uuid       UUID         NOT NULL DEFAULT gen_random_uuid(),
    brand_id   BIGINT       NOT NULL REFERENCES catalog_brands (id) ON DELETE CASCADE,
    name       TEXT         NOT NULL,
    source     TEXT         NOT NULL DEFAULT 'auto',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_catalog_models_uuid UNIQUE (uuid),
    CONSTRAINT chk_catalog_models_source CHECK (source IN ('auto', 'manual')),
    CONSTRAINT chk_catalog_models_name CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX uq_catalog_models_brand_name ON catalog_models (brand_id, lower(name));

CREATE TRIGGER trg_catalog_models_set_updated_at
    BEFORE UPDATE ON catalog_models
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE catalog_types (
    id                     BIGSERIAL    PRIMARY KEY,
    uuid                   UUID         NOT NULL DEFAULT gen_random_uuid(),
    brand_id               BIGINT       NOT NULL REFERENCES catalog_brands (id) ON DELETE CASCADE,
    model_id               BIGINT       NULL REFERENCES catalog_models (id) ON DELETE SET NULL,
    tsb_type_code          TEXT         NOT NULL,
    raw_name               TEXT         NOT NULL,
    years                  INTEGER[]    NOT NULL DEFAULT '{}',
    suggested_transmission TEXT         NULL,
    suggested_fuel         TEXT         NULL,
    first_seen_period      CHAR(7)      NOT NULL,
    last_seen_period       CHAR(7)      NOT NULL,
    is_active              BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_catalog_types_uuid UNIQUE (uuid),
    CONSTRAINT uq_catalog_types_brand_code UNIQUE (brand_id, tsb_type_code),
    CONSTRAINT chk_catalog_types_transmission CHECK (suggested_transmission IS NULL OR suggested_transmission IN ('manual', 'automatic')),
    CONSTRAINT chk_catalog_types_fuel CHECK (suggested_fuel IS NULL OR suggested_fuel IN ('petrol', 'diesel', 'hybrid', 'electric', 'lpg')),
    CONSTRAINT chk_catalog_types_first_period CHECK (first_seen_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_catalog_types_last_period CHECK (last_seen_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_catalog_types_model ON catalog_types (model_id);
CREATE INDEX idx_catalog_types_unresolved ON catalog_types (brand_id) WHERE model_id IS NULL;
CREATE INDEX idx_catalog_types_years ON catalog_types USING GIN (years);

CREATE TRIGGER trg_catalog_types_set_updated_at
    BEFORE UPDATE ON catalog_types
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Kasko values are history: one row per type, model year and monthly period.
-- Imports never delete older periods.
CREATE TABLE catalog_valuations (
    type_id    BIGINT         NOT NULL REFERENCES catalog_types (id) ON DELETE CASCADE,
    model_year INTEGER        NOT NULL,
    period     CHAR(7)        NOT NULL,
    value      NUMERIC(14, 2) NOT NULL,
    PRIMARY KEY (type_id, model_year, period),
    CONSTRAINT chk_catalog_valuations_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_catalog_valuations_value CHECK (value > 0)
);

CREATE INDEX idx_catalog_valuations_period ON catalog_valuations (period);

-- Admin-curated "type name prefix -> model" rules; longest prefix wins.
CREATE TABLE catalog_model_rules (
    id         BIGSERIAL    PRIMARY KEY,
    brand_id   BIGINT       NOT NULL REFERENCES catalog_brands (id) ON DELETE CASCADE,
    prefix     TEXT         NOT NULL,
    model_id   BIGINT       NOT NULL REFERENCES catalog_models (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_catalog_model_rules_brand_prefix UNIQUE (brand_id, prefix),
    CONSTRAINT chk_catalog_model_rules_prefix CHECK (btrim(prefix) <> '')
);

CREATE INDEX idx_catalog_model_rules_model ON catalog_model_rules (model_id);

CREATE TRIGGER trg_catalog_model_rules_set_updated_at
    BEFORE UPDATE ON catalog_model_rules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE catalog_imports (
    id          BIGSERIAL    PRIMARY KEY,
    uuid        UUID         NOT NULL DEFAULT gen_random_uuid(),
    period      CHAR(7)      NULL,
    filename    TEXT         NOT NULL DEFAULT '',
    file_key    TEXT         NULL,
    status      TEXT         NOT NULL DEFAULT 'queued',
    counts      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    diff        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error       TEXT         NULL,
    created_by  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    applied_at  TIMESTAMPTZ  NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_catalog_imports_uuid UNIQUE (uuid),
    CONSTRAINT chk_catalog_imports_status CHECK (status IN (
        'queued', 'processing', 'ready', 'applying', 'applied', 'failed'
    )),
    CONSTRAINT chk_catalog_imports_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_catalog_imports_created_at ON catalog_imports (created_at DESC);

CREATE TRIGGER trg_catalog_imports_set_updated_at
    BEFORE UPDATE ON catalog_imports
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
