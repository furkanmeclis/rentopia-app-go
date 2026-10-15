-- Data-loss note: drops every rental with its vehicle legs (handover /
-- return km, fuel, times), drivers, timeline and charges. Reservations and
-- their allocations stay; allocations already turned into kind 'rental'
-- keep that kind (they still hold the vehicle until their period ends).
DROP TABLE IF EXISTS rental_charges;
DROP TABLE IF EXISTS rental_events;
DROP TABLE IF EXISTS rental_drivers;
DROP TABLE IF EXISTS rental_vehicles;
DROP TABLE IF EXISTS rentals;
