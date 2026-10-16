-- Vehicle profitability report (TEC-727, docs/design/vehicle-profitability.md).
-- The report is computed at query time (no cache table): it reads the
-- ledger, collections, expenses, supplier payments, rental legs, blocks,
-- maintenance and transfers of one organization for a period. These
-- indexes back the period scans that no module index served. Every index
-- starts with organization_id (tenant isolation guard rail).

-- Ledger entries of a period (accrual and cash facts by entry date).
CREATE INDEX IF NOT EXISTS idx_ledger_entries_organization_entry_date
    ON ledger_entries (organization_id, entry_date);

-- Rental legs overlapping a period (occupancy, rental days).
CREATE INDEX IF NOT EXISTS idx_rental_vehicles_organization_out
    ON rental_vehicles (organization_id, out_at) WHERE out_at IS NOT NULL;

-- Maintenance done with a cost in a period (direct vehicle cost).
CREATE INDEX IF NOT EXISTS idx_vehicle_maintenance_organization_done_cost
    ON vehicle_maintenance (organization_id, done_date)
    WHERE kind = 'done' AND deleted_at IS NULL AND cost IS NOT NULL;

-- Completed transfers with a cost in a period (direct vehicle cost).
CREATE INDEX IF NOT EXISTS idx_vehicle_transfers_organization_arrival
    ON vehicle_transfers (organization_id, actual_arrival)
    WHERE status = 'completed' AND cost IS NOT NULL;
