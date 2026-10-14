-- Data-loss note: drops every vehicle, its status history and its rental
-- group memberships.
DROP TABLE IF EXISTS vehicle_rental_groups;
DROP TABLE IF EXISTS vehicle_events;
DROP TABLE IF EXISTS vehicles;
