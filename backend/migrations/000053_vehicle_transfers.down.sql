-- Data-loss note: drops every vehicle transfer (planned, in transit and the
-- completed / cancelled history) and the busy intervals view.
DROP VIEW IF EXISTS vehicle_busy_intervals;
DROP TABLE IF EXISTS vehicle_transfers;
