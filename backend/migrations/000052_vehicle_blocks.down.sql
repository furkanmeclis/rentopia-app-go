-- Data-loss note: drops every vehicle block (maintenance / damage / cleaning
-- history). The btree_gist extension stays: other objects may use it, and
-- dropping a shared extension is not this migration's call.
DROP TABLE IF EXISTS vehicle_blocks;
