-- Reports (TEC-738, §15.1): occupancy, income / expense, channel
-- performance, receivable / payable aging and the branch comparison are
-- computed at query time over existing tables (no cache table). These
-- indexes back the period scans no module index served. Every index starts
-- with organization_id (tenant isolation guard rail).

-- Channel performance by booking date and by pick-up date.
CREATE INDEX IF NOT EXISTS idx_reservations_organization_channel_created
    ON reservations (organization_id, channel, created_at);
CREATE INDEX IF NOT EXISTS idx_reservations_organization_channel_starts
    ON reservations (organization_id, channel, starts_at);

-- Income by category / branch / month: debit accruals and their reversals
-- by entry date.
CREATE INDEX IF NOT EXISTS idx_ledger_entries_organization_accrual_date
    ON ledger_entries (organization_id, entry_date)
    WHERE kind IN ('accrual', 'reversal');
