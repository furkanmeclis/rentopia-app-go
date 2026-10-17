-- Storefront publish state of the catalog (TEC-730,
-- docs/design/branded-site.md §3.3): whether a branch, a vehicle subgroup
-- (rental product) or an extra is shown on the branded site and the
-- embeddable widget (TEC-732), i.e. to public API callers that use a
-- publishable key / browser booking session. Integrations calling the
-- public API with secret server keys ignore it. Default on, so nothing
-- changes for existing data. The panel switches them on
-- Ayarlar → Markalı site → Katalog.
--
-- Extras keep their sales-channel rule (extras.channels) for every caller;
-- show_online only adds the storefront filter on top. Service points keep
-- their own online_selectable flag.
ALTER TABLE branches
    ADD COLUMN show_online BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE rental_products
    ADD COLUMN show_online BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE extras
    ADD COLUMN show_online BOOLEAN NOT NULL DEFAULT TRUE;
