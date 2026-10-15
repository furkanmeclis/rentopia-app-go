-- Drops the two operations overview indexes. No data is lost: the overview
-- queries fall back to broader index scans.
DROP INDEX IF EXISTS idx_integration_jobs_organization_attention;
DROP INDEX IF EXISTS idx_payments_organization_verified_collected;
