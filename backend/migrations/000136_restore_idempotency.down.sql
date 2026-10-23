-- Data-loss note: drops the recorded orphan notifications, the refund
-- reconciliation items, the render request times and who reconciled which
-- integration job (the jobs keep their reconciled status).
DROP TABLE IF EXISTS payment_reconciliation_items;
DROP TABLE IF EXISTS payment_webhook_orphans;
DROP INDEX IF EXISTS idx_document_versions_pdf_pending;
ALTER TABLE document_versions DROP COLUMN IF EXISTS pdf_requested_at;
ALTER TABLE integration_jobs
    DROP COLUMN IF EXISTS reconcile_external_ref,
    DROP COLUMN IF EXISTS reconcile_note,
    DROP COLUMN IF EXISTS reconciled_by,
    DROP COLUMN IF EXISTS reconciled_at;
