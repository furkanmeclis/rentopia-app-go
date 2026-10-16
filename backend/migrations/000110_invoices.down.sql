-- Data-loss note: drops every invoice and invoice line (receiver snapshots,
-- totals, validation results, storage references: the XML / PDF objects
-- stay in storage), the invoice settings, tax mappings, number sequences
-- and the taxpayer lookup cache; deletes the invoice links of the ledger
-- (the ledger entries themselves are untouched), every e-invoice
-- integration job (einvoice.* kinds) with its attempts and every e-invoice
-- account (kind einvoice, providers einvoice / einvoice_fake).

DROP INDEX IF EXISTS idx_ledger_entry_links_organization_invoice;
DROP INDEX IF EXISTS uq_ledger_entry_links_invoiced;
DELETE FROM ledger_entry_links WHERE invoice_id IS NOT NULL;
ALTER TABLE ledger_entry_links
    DROP CONSTRAINT IF EXISTS chk_ledger_entry_links_release,
    DROP CONSTRAINT IF EXISTS chk_ledger_entry_links_invoice,
    DROP CONSTRAINT IF EXISTS fk_ledger_entry_links_invoice,
    DROP COLUMN IF EXISTS release_reason,
    DROP COLUMN IF EXISTS released_at,
    DROP COLUMN IF EXISTS amount,
    DROP COLUMN IF EXISTS invoice_id;

DROP TABLE IF EXISTS invoice_lines;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS einvoice_taxpayers;
DROP TABLE IF EXISTS invoice_sequences;
DROP TABLE IF EXISTS invoice_tax_mappings;
DROP TABLE IF EXISTS invoice_settings;

DELETE FROM integration_jobs WHERE kind LIKE 'einvoice.%';
DELETE FROM integration_jobs
WHERE integration_account_id IN (SELECT id FROM integration_accounts WHERE kind = 'einvoice' OR provider IN ('einvoice', 'einvoice_fake'));
DELETE FROM integration_accounts WHERE kind = 'einvoice' OR provider IN ('einvoice', 'einvoice_fake');

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis'));
