-- Data-loss note: drops every reservation with its drivers, event history,
-- plate assignment history and reference counters, and every change /
-- extension quote (quotes.reservation_id cascade). The allocations they held
-- stay in vehicle_allocations (release active ones before rolling back, or
-- they keep consuming capacity without a reservation).
ALTER TABLE quotes DROP CONSTRAINT IF EXISTS fk_quotes_reservation;
DELETE FROM quotes WHERE reservation_id IS NOT NULL;
DROP TABLE IF EXISTS reservation_vehicle_assignments;
DROP TABLE IF EXISTS reservation_events;
DROP TABLE IF EXISTS reservation_drivers;
DROP TABLE IF EXISTS reservations;
DROP TABLE IF EXISTS reservation_sequences;
