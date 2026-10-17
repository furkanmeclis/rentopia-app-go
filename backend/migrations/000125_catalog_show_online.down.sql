-- Data-loss note: forgets which branches, subgroups and extras were hidden
-- from the branded site / widget; after rollback every active one is
-- offered there again.
ALTER TABLE extras DROP COLUMN IF EXISTS show_online;

ALTER TABLE rental_products DROP COLUMN IF EXISTS show_online;

ALTER TABLE branches DROP COLUMN IF EXISTS show_online;
