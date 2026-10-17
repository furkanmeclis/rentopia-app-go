-- Data-loss note: forgets which branches and subgroups were hidden from the
-- online channels; after rollback every active one is offered again.
ALTER TABLE rental_products DROP COLUMN IF EXISTS show_online;

ALTER TABLE branches DROP COLUMN IF EXISTS show_online;
