-- Data-loss note: drops the branch stock counts of physical extras and
-- every equipment allocation (holds, reservations and their history).
DROP TRIGGER IF EXISTS trg_vehicle_allocations_cascade_equipment ON vehicle_allocations;
DROP FUNCTION IF EXISTS vehicle_allocations_cascade_equipment();
DROP TABLE IF EXISTS equipment_allocations;
DROP TABLE IF EXISTS extra_branch_stock;
