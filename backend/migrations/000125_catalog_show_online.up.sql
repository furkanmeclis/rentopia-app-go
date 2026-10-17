-- Online publish state of the catalog (TEC-730, docs/design/branded-site.md
-- §3): whether a branch or a vehicle subgroup (rental product) is offered
-- on the online channels (the public booking API: branded site, widget,
-- API keys). Default on, so nothing changes for existing data. The panel
-- switches them on Ayarlar → Markalı site → Katalog.
--
-- Extras need no column: their sales channels (extras.channels, NULL =
-- every channel) already say whether the `web` channel sells them.
-- Service points keep their own online_selectable flag.
ALTER TABLE branches
    ADD COLUMN show_online BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE rental_products
    ADD COLUMN show_online BOOLEAN NOT NULL DEFAULT TRUE;
