-- Drops the vehicle profitability report indexes (TEC-727). No data is
-- lost: the report itself owns no table.
DROP INDEX IF EXISTS idx_vehicle_transfers_organization_arrival;
DROP INDEX IF EXISTS idx_vehicle_maintenance_organization_done_cost;
DROP INDEX IF EXISTS idx_rental_vehicles_organization_out;
DROP INDEX IF EXISTS idx_ledger_entries_organization_entry_date;
