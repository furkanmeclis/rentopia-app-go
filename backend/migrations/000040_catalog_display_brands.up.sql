-- Display brands: what users see. TSB lists some makes under several brand
-- codes (FIAT + TOFAS-FIAT, FORD + FORD /USA, RENAULT + RENAULT (OYAK),
-- VOLVO + VOLVO-TR); each TSB brand keeps its code and points at one
-- display brand, so imports stay keyed by the TSB code.

CREATE TABLE catalog_display_brands (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    name             TEXT         NOT NULL,
    slug             TEXT         NOT NULL,
    logo_object_key  TEXT         NULL,
    is_active        BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_catalog_display_brands_uuid UNIQUE (uuid),
    CONSTRAINT uq_catalog_display_brands_slug UNIQUE (slug),
    CONSTRAINT chk_catalog_display_brands_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_catalog_display_brands_slug CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);

CREATE UNIQUE INDEX uq_catalog_display_brands_name ON catalog_display_brands (lower(name));

CREATE TRIGGER trg_catalog_display_brands_set_updated_at
    BEFORE UPDATE ON catalog_display_brands
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Backfill: one display brand per distinct display name (case-insensitive).
WITH names AS (
    SELECT DISTINCT ON (lower(btrim(display_name))) btrim(display_name) AS name
    FROM catalog_brands
    ORDER BY lower(btrim(display_name)), id
), slugs AS (
    SELECT name,
        COALESCE(NULLIF(btrim(regexp_replace(
            lower(translate(name, 'ıİşŞğĞüÜöÖçÇ', 'iissgguuoocc')),
            '[^a-z0-9]+', '-', 'g'), '-'), ''), 'brand') AS base
    FROM names
), numbered AS (
    SELECT name, base, row_number() OVER (PARTITION BY base ORDER BY name) AS n
    FROM slugs
)
INSERT INTO catalog_display_brands (name, slug)
SELECT name, CASE WHEN n = 1 THEN base ELSE base || '-' || n END
FROM numbered;

ALTER TABLE catalog_brands
    ADD COLUMN display_brand_id BIGINT NULL REFERENCES catalog_display_brands (id) ON DELETE RESTRICT;

UPDATE catalog_brands b
SET display_brand_id = d.id,
    display_name = d.name
FROM catalog_display_brands d
WHERE lower(btrim(b.display_name)) = lower(d.name);

ALTER TABLE catalog_brands ALTER COLUMN display_brand_id SET NOT NULL;

CREATE INDEX idx_catalog_brands_display_brand ON catalog_brands (display_brand_id);

-- Model grouping key: models with the same normalized name under the TSB
-- brands of one display brand are presented as one model ("Egea" once for
-- Fiat even though FIAT and TOFAS-FIAT both have it).
ALTER TABLE catalog_models
    ADD COLUMN group_key TEXT GENERATED ALWAYS AS (lower(regexp_replace(btrim(name), '\s+', ' ', 'g'))) STORED;

CREATE INDEX idx_catalog_models_group_key ON catalog_models (group_key);
