-- Data-loss note: drops the mirror brand list, sync history and the display
-- brand <-> mirror links (downloaded logo objects stay in storage).
DROP INDEX IF EXISTS idx_catalog_display_brands_unmatched;
DROP INDEX IF EXISTS idx_catalog_display_brands_motomarks;
ALTER TABLE catalog_display_brands
    DROP CONSTRAINT IF EXISTS chk_catalog_display_brands_match_source,
    DROP COLUMN IF EXISTS synced_logo_variants,
    DROP COLUMN IF EXISTS brand_color,
    DROP COLUMN IF EXISTS motomarks_suggested_score,
    DROP COLUMN IF EXISTS motomarks_suggested_id,
    DROP COLUMN IF EXISTS motomarks_match_source,
    DROP COLUMN IF EXISTS motomarks_match_score,
    DROP COLUMN IF EXISTS motomarks_id;

DROP TABLE IF EXISTS brand_logo_sync_runs;
DROP TABLE IF EXISTS brand_logo_sources;
DROP TABLE IF EXISTS brand_logo_sync_settings;
