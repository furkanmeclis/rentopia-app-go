-- Brand logo sync from the motomarks-mirror service (TEC-995 / TEC-996).
-- The mirror's CDN is never exposed to browsers: every run asks the live
-- endpoint (URL + token live here, editable by the platform admin), writes
-- the brand list to brand_logo_sources and downloads the badge / wordmark /
-- full files into this app's own object storage. Display brands are matched
-- to a mirror brand automatically (score >= threshold) or by hand.

CREATE TABLE brand_logo_sync_settings (
    id                    SMALLINT     PRIMARY KEY DEFAULT 1,
    base_url              TEXT         NOT NULL DEFAULT '',
    api_token_enc         TEXT         NULL,
    enabled               BOOLEAN      NOT NULL DEFAULT TRUE,
    auto_match_min_score  INTEGER      NOT NULL DEFAULT 85,
    last_run_at           TIMESTAMPTZ  NULL,
    last_etag             TEXT         NULL,
    last_status           TEXT         NULL,
    last_error            TEXT         NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT brand_logo_sync_settings_singleton CHECK (id = 1),
    CONSTRAINT chk_brand_logo_sync_min_score CHECK (auto_match_min_score BETWEEN 0 AND 100)
);

INSERT INTO brand_logo_sync_settings (id) VALUES (1);

CREATE TRIGGER trg_brand_logo_sync_settings_set_updated_at
    BEFORE UPDATE ON brand_logo_sync_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- One row per mirror brand. logos keeps the variant map the mirror returned
-- (source URLs for the next download), *_object_key the copies in our
-- storage. source_hash changes when any variant URL or the name changes.
CREATE TABLE brand_logo_sources (
    id                  BIGSERIAL    PRIMARY KEY,
    motomarks_id        TEXT         NOT NULL,
    name                TEXT         NOT NULL,
    color               TEXT         NOT NULL DEFAULT '',
    colors              JSONB        NOT NULL DEFAULT '[]'::jsonb,
    vehicle_types       TEXT[]       NOT NULL DEFAULT '{}',
    logos               JSONB        NOT NULL DEFAULT '{}'::jsonb,
    source_hash         TEXT         NOT NULL DEFAULT '',
    badge_object_key    TEXT         NULL,
    wordmark_object_key TEXT         NULL,
    full_object_key     TEXT         NULL,
    synced_at           TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_brand_logo_sources_motomarks_id UNIQUE (motomarks_id),
    CONSTRAINT chk_brand_logo_sources_motomarks_id CHECK (btrim(motomarks_id) <> '')
);

CREATE INDEX idx_brand_logo_sources_name ON brand_logo_sources (lower(name));

CREATE TRIGGER trg_brand_logo_sources_set_updated_at
    BEFORE UPDATE ON brand_logo_sources
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE brand_logo_sync_runs (
    id            BIGSERIAL    PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    status        TEXT         NOT NULL DEFAULT 'queued',
    triggered_by  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    started_at    TIMESTAMPTZ  NULL,
    finished_at   TIMESTAMPTZ  NULL,
    not_modified  BOOLEAN      NOT NULL DEFAULT FALSE,
    requests      INTEGER      NOT NULL DEFAULT 0,
    brands_seen   INTEGER      NOT NULL DEFAULT 0,
    downloaded    INTEGER      NOT NULL DEFAULT 0,
    matched       INTEGER      NOT NULL DEFAULT 0,
    unmatched     INTEGER      NOT NULL DEFAULT 0,
    error         TEXT         NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_brand_logo_sync_runs_uuid UNIQUE (uuid),
    CONSTRAINT chk_brand_logo_sync_runs_status CHECK (status IN ('queued', 'running', 'done', 'failed'))
);

CREATE INDEX idx_brand_logo_sync_runs_created_at ON brand_logo_sync_runs (created_at DESC);
-- One run at a time.
CREATE UNIQUE INDEX uq_brand_logo_sync_runs_active ON brand_logo_sync_runs ((TRUE))
    WHERE status IN ('queued', 'running');

-- Display brand <-> mirror brand link. match_source: none (never matched),
-- auto (score >= threshold), manual (admin picked), skip (admin: no logo).
-- A manual or skip choice is never overwritten by a later run. The
-- suggestion columns keep the best candidate below the threshold for the
-- "unmatched brands" queue. synced_logo_variants is denormalized so lists
-- can build logo URLs without joining the sources.
ALTER TABLE catalog_display_brands
    ADD COLUMN motomarks_id TEXT NULL REFERENCES brand_logo_sources (motomarks_id) ON DELETE SET NULL,
    ADD COLUMN motomarks_match_score NUMERIC(5, 2) NULL,
    ADD COLUMN motomarks_match_source TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN motomarks_suggested_id TEXT NULL,
    ADD COLUMN motomarks_suggested_score NUMERIC(5, 2) NULL,
    ADD COLUMN brand_color TEXT NULL,
    ADD COLUMN synced_logo_variants TEXT[] NOT NULL DEFAULT '{}',
    ADD CONSTRAINT chk_catalog_display_brands_match_source
        CHECK (motomarks_match_source IN ('none', 'auto', 'manual', 'skip'));

CREATE INDEX idx_catalog_display_brands_motomarks ON catalog_display_brands (motomarks_id);
CREATE INDEX idx_catalog_display_brands_unmatched ON catalog_display_brands (motomarks_match_source)
    WHERE motomarks_match_source = 'none';
