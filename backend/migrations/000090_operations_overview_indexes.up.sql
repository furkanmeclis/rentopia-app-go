-- Operations overview (TEC-716, docs/design/operations-overview.md).
--
-- Two partial indexes for the overview queries that no existing index
-- serves. Every other overview query reuses an index of its module
-- (operation tasks by due time, the approval queue, the waiting
-- reservations, active rentals, open resolution tasks, reservations by
-- start). Indexes only: no table, column or data changes.

-- "Bugünkü tahsilat": money confirmed today, by verification time.
CREATE INDEX idx_payments_organization_verified_collected
    ON payments (organization_id, verified_at)
    WHERE status IN ('succeeded', 'partially_refunded', 'refunded');

-- "Başarısız dış işler": jobs that failed or whose outcome is unknown.
CREATE INDEX idx_integration_jobs_organization_attention
    ON integration_jobs (organization_id, updated_at DESC)
    WHERE status IN ('failed', 'needs_verification');
