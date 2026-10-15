-- Data-loss note: drops every rate plan with all versions and duration
-- bands (draft and activated prices). Reservations do not reference them
-- yet (M3).
DROP TABLE IF EXISTS rate_bands;
DROP FUNCTION IF EXISTS rate_bands_immutable();
DROP TABLE IF EXISTS rate_plan_versions;
DROP FUNCTION IF EXISTS rate_plan_versions_immutable();
DROP TABLE IF EXISTS rate_plans;
-- btree_gist stays: vehicle_blocks / vehicle_transfers use it too.
