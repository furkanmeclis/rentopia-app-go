-- Data-loss note: drops every rental product, all of its versions and image
-- metadata (the image objects stay in storage).
DROP TABLE IF EXISTS rental_product_images;
DROP TRIGGER IF EXISTS trg_rental_product_versions_immutable ON rental_product_versions;
DROP TABLE IF EXISTS rental_product_versions;
DROP FUNCTION IF EXISTS rental_product_versions_immutable();
DROP TABLE IF EXISTS rental_products;
