-- Data-loss note: drops display brands (merges, renames and logo keys made
-- in the admin; uploaded logo objects stay in storage). catalog_brands keeps
-- display_name, so the pre-000040 behaviour is restored.
DROP INDEX IF EXISTS idx_catalog_models_group_key;
ALTER TABLE catalog_models DROP COLUMN IF EXISTS group_key;

DROP INDEX IF EXISTS idx_catalog_brands_display_brand;
ALTER TABLE catalog_brands DROP COLUMN IF EXISTS display_brand_id;

DROP TABLE IF EXISTS catalog_display_brands;
