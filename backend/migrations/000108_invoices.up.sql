-- E-invoices / e-archive invoices (TEC-722, RT-A22, docs/design/einvoice.md).
--
-- An invoice is a document ON TOP of the ledger (docs/design/ledger.md): it
-- covers live accruals (the rent of a rental, approved charges) through
-- ledger_entry_links and never posts a debt or revenue line of its own
-- (RT-A22 + RT-A23). The UBL-TR XML is produced and validated by go-ubltr
-- and transported by an integrator through the integration job runner
-- (docs/design/integration-jobs.md). The integrator is not chosen yet
-- (TEC-685): the official slot ("einvoice") waits on that dependency and a
-- development / test adapter ("einvoice_fake") exists outside production.
--
-- Tables (tenant rules: organization_id first in every index, composite
-- foreign keys, listed in queries/tenant_tables.txt):
--   invoice_settings      series, supplier address parts, taxpayer cache TTL
--   invoice_tax_mappings  line kind → VAT rate / exemption code
--   invoice_sequences     invoice numbers per organization, series and year
--   einvoice_taxpayers    taxpayer lookup cache (keyed hash of the VKN / TCKN)
--   invoices, invoice_lines
-- ledger_entry_links gains the invoice FK, the linked amount and a release
-- (cancelled / returned invoices free their items for a new invoice); a
-- partial unique index allows one live invoice link per ledger entry.

ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll', 'kabis', 'einvoice'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log', 'hgs_fake', 'kabis', 'kabis_fake', 'einvoice', 'einvoice_fake'));

CREATE TABLE invoice_settings (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    -- Three character GİB series of e-Fatura (TEMEL / TICARI) and e-Arşiv
    -- numbers: SSSYYYYNNNNNNNNN.
    efatura_series CHAR(3) NOT NULL DEFAULT 'EFT' CHECK (efatura_series ~ '^[A-Z0-9]{3}$'),
    earsiv_series CHAR(3) NOT NULL DEFAULT 'EAR' CHECK (earsiv_series ~ '^[A-Z0-9]{3}$'),
    -- The supplier party is the organization's company settings (legal
    -- name, tax number / office, address); UBL also needs these parts.
    supplier_district VARCHAR(128) NOT NULL DEFAULT '',
    supplier_city VARCHAR(128) NOT NULL DEFAULT '',
    supplier_postal_code VARCHAR(16) NOT NULL DEFAULT '',
    default_note TEXT NOT NULL DEFAULT '',
    taxpayer_cache_hours INTEGER NOT NULL DEFAULT 24 CHECK (taxpayer_cache_hours BETWEEN 1 AND 720),
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_invoice_settings_updated_at
    BEFORE UPDATE ON invoice_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE invoice_tax_mappings (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    line_kind VARCHAR(16) NOT NULL CHECK (line_kind IN ('rent', 'extra', 'one_way', 'charge', 'toll')),
    vat_rate NUMERIC(5, 2) NOT NULL CHECK (vat_rate >= 0 AND vat_rate <= 100),
    -- GİB TaxExemptionReasonCode / text; required by the module for 0 %.
    exemption_code VARCHAR(8) NOT NULL DEFAULT '',
    exemption_reason VARCHAR(255) NOT NULL DEFAULT '',
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_invoice_tax_mappings PRIMARY KEY (organization_id, line_kind)
);

CREATE TRIGGER trg_invoice_tax_mappings_updated_at
    BEFORE UPDATE ON invoice_tax_mappings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE invoice_sequences (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    series CHAR(3) NOT NULL,
    year INTEGER NOT NULL,
    last_value BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_invoice_sequences PRIMARY KEY (organization_id, series, year)
);

CREATE TABLE einvoice_taxpayers (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Keyed hash (field cipher blind index, purpose invoices.taxpayer) of
    -- the VKN / TCKN; the number itself is never stored here.
    identity_bidx VARCHAR(64) NOT NULL,
    is_einvoice BOOLEAN NOT NULL,
    alias VARCHAR(255) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL DEFAULT '',
    provider VARCHAR(32) NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_einvoice_taxpayers PRIMARY KEY (organization_id, identity_bidx)
);

CREATE TABLE invoices (
    id BIGSERIAL PRIMARY KEY,
    -- The ETTN: generated when the draft is created, sent unchanged on
    -- every attempt (a repeated send is recognised by the integrator and
    -- by the status query).
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    account_id BIGINT NULL,
    customer_id BIGINT NULL,
    -- The rental the invoice was cut from (one of its items' rentals).
    rental_id BIGINT NULL,
    -- An IADE invoice names the invoice it returns.
    returns_invoice_id BIGINT NULL,
    -- SSSYYYYNNNNNNNNN, assigned when the invoice is finalized.
    series CHAR(3) NULL,
    sequence_year INTEGER NULL,
    sequence_no BIGINT NULL,
    number VARCHAR(16) NULL,
    profile VARCHAR(16) NOT NULL CHECK (profile IN ('TEMELFATURA', 'TICARIFATURA', 'EARSIVFATURA')),
    invoice_type VARCHAR(8) NOT NULL CHECK (invoice_type IN ('SATIS', 'IADE')),
    status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN (
        'draft', 'queued', 'sent', 'awaiting_result', 'succeeded', 'failed', 'cancelled'
    )),
    issue_date DATE NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- Receiver snapshot (taken from the customer when the draft is made,
    -- editable while it is a draft). The VKN / TCKN is sealed with the
    -- field cipher (purpose invoices.receiver); only its hint is clear.
    receiver_type VARCHAR(12) NOT NULL DEFAULT 'individual' CHECK (receiver_type IN ('individual', 'corporate')),
    receiver_identity_type VARCHAR(4) NULL CHECK (receiver_identity_type IN ('vkn', 'tckn')),
    receiver_identity_enc TEXT NOT NULL DEFAULT '',
    receiver_identity_hint VARCHAR(32) NOT NULL DEFAULT '',
    receiver_name VARCHAR(255) NOT NULL DEFAULT '',
    receiver_first_name VARCHAR(128) NOT NULL DEFAULT '',
    receiver_last_name VARCHAR(128) NOT NULL DEFAULT '',
    receiver_tax_office VARCHAR(128) NOT NULL DEFAULT '',
    receiver_address TEXT NOT NULL DEFAULT '',
    receiver_district VARCHAR(128) NOT NULL DEFAULT '',
    receiver_city VARCHAR(128) NOT NULL DEFAULT '',
    receiver_postal_code VARCHAR(16) NOT NULL DEFAULT '',
    receiver_country CHAR(2) NOT NULL DEFAULT 'TR',
    receiver_email VARCHAR(255) NOT NULL DEFAULT '',
    -- Taxpayer lookup: e-Fatura user (TEMEL / TICARI) or not (e-Arşiv),
    -- the receiver's mailbox alias, and where the answer came from
    -- (provider, cache, customer_flag when no lookup was possible).
    receiver_einvoice BOOLEAN NOT NULL DEFAULT FALSE,
    receiver_alias VARCHAR(255) NOT NULL DEFAULT '',
    taxpayer_source VARCHAR(16) NOT NULL DEFAULT '' CHECK (taxpayer_source IN ('', 'provider', 'cache', 'customer_flag')),
    taxpayer_checked_at TIMESTAMPTZ NULL,
    -- Money: the line sums (net, VAT, payable) and the ledger amount the
    -- invoice covers; payable − linked is the VAT rounding left over
    -- (0 unless no net split reaches the gross amount exactly).
    line_extension_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    payable_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    linked_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    -- Last validation (XSD + Schematron + module checks), per field.
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validated_at TIMESTAMPTZ NULL,
    -- Private storage, streamed by the API only:
    -- organizations/{org}/invoices/{invoice}/{file}.
    xml_key TEXT NULL,
    xml_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    pdf_key TEXT NULL,
    -- Sending (integration job) and the integrator's reference.
    integration_job_id BIGINT NULL REFERENCES integration_jobs (id) ON DELETE SET NULL,
    external_ref VARCHAR(128) NOT NULL DEFAULT '',
    hold_reason VARCHAR(32) NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    -- e-Arşiv cancellation (integration job of kind einvoice.cancel).
    cancel_job_id BIGINT NULL REFERENCES integration_jobs (id) ON DELETE SET NULL,
    cancel_reason TEXT NOT NULL DEFAULT '',
    cancelled_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at TIMESTAMPTZ NULL,
    -- A returned (credited) invoice: the IADE invoice succeeded.
    returned_at TIMESTAMPTZ NULL,
    finalized_at TIMESTAMPTZ NULL,
    sent_at TIMESTAMPTZ NULL,
    succeeded_at TIMESTAMPTZ NULL,
    idempotency_key VARCHAR(64) NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_invoices_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_invoices_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_invoices_branch FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_invoices_account FOREIGN KEY (organization_id, account_id) REFERENCES accounts (organization_id, id),
    CONSTRAINT fk_invoices_customer FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id),
    CONSTRAINT fk_invoices_rental FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id),
    CONSTRAINT fk_invoices_returns
        FOREIGN KEY (organization_id, returns_invoice_id) REFERENCES invoices (organization_id, id),
    CONSTRAINT chk_invoices_number CHECK (
        (number IS NULL AND series IS NULL AND sequence_no IS NULL)
        OR (number IS NOT NULL AND series IS NOT NULL AND sequence_no IS NOT NULL AND length(number) = 16)
    ),
    CONSTRAINT chk_invoices_draft_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT chk_invoices_return CHECK (
        (invoice_type = 'IADE' AND returns_invoice_id IS NOT NULL AND profile <> 'TICARIFATURA')
        OR (invoice_type = 'SATIS' AND returns_invoice_id IS NULL)
    )
);

CREATE UNIQUE INDEX uq_invoices_organization_number ON invoices (organization_id, number) WHERE number IS NOT NULL;
CREATE UNIQUE INDEX uq_invoices_organization_idempotency
    ON invoices (organization_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX uq_invoices_organization_job ON invoices (organization_id, integration_job_id)
    WHERE integration_job_id IS NOT NULL;
-- One live return invoice per invoice (a cancelled / failed one frees it).
CREATE UNIQUE INDEX uq_invoices_organization_returns ON invoices (organization_id, returns_invoice_id)
    WHERE returns_invoice_id IS NOT NULL AND status NOT IN ('cancelled', 'failed');
CREATE INDEX idx_invoices_organization_created ON invoices (organization_id, created_at DESC);
CREATE INDEX idx_invoices_organization_branch ON invoices (organization_id, branch_id);
CREATE INDEX idx_invoices_organization_status ON invoices (organization_id, status);
CREATE INDEX idx_invoices_organization_issue_date ON invoices (organization_id, issue_date DESC);
CREATE INDEX idx_invoices_organization_account ON invoices (organization_id, account_id);
CREATE INDEX idx_invoices_organization_rental ON invoices (organization_id, rental_id);

CREATE TRIGGER trg_invoices_updated_at
    BEFORE UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE invoice_lines (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    invoice_id BIGINT NOT NULL,
    position INTEGER NOT NULL CHECK (position >= 1),
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('rent', 'extra', 'one_way', 'charge', 'toll')),
    name VARCHAR(255) NOT NULL,
    -- Days of a rent line (quantity in days); 0 = one piece.
    days INTEGER NOT NULL DEFAULT 0 CHECK (days >= 0),
    -- The ledger item the line covers (a rent accrual may give several
    -- lines: rent, extras, one-way fee). NULL on a return invoice (it
    -- copies the returned invoice's lines).
    entry_id BIGINT NULL,
    -- gross = the ledger part the line carries; net + vat = the line's
    -- tax-inclusive amount on the invoice.
    gross_amount NUMERIC(14, 2) NOT NULL CHECK (gross_amount >= 0),
    net_amount NUMERIC(14, 2) NOT NULL CHECK (net_amount >= 0),
    vat_rate NUMERIC(5, 2) NOT NULL CHECK (vat_rate >= 0 AND vat_rate <= 100),
    vat_amount NUMERIC(14, 2) NOT NULL CHECK (vat_amount >= 0),
    exemption_code VARCHAR(8) NOT NULL DEFAULT '',
    exemption_reason VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_invoice_lines_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_invoice_lines_invoice
        FOREIGN KEY (organization_id, invoice_id) REFERENCES invoices (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_invoice_lines_entry
        FOREIGN KEY (organization_id, entry_id) REFERENCES ledger_entries (organization_id, id)
);

CREATE UNIQUE INDEX uq_invoice_lines_position ON invoice_lines (organization_id, invoice_id, position);
CREATE INDEX idx_invoice_lines_organization_entry ON invoice_lines (organization_id, entry_id) WHERE entry_id IS NOT NULL;

CREATE TRIGGER trg_invoice_lines_updated_at
    BEFORE UPDATE ON invoice_lines
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Ledger links of invoices (ledger.md: "Invoicing hangs off this table").
-- document_ref keeps the invoice uuid; invoice_id is the FK. A link is
-- released (released_at, release_reason cancelled / returned / removed)
-- when its invoice is cancelled, returned by an IADE invoice, or the item is
-- taken off a draft; the item may then be invoiced again.
ALTER TABLE ledger_entry_links
    ADD COLUMN invoice_id BIGINT NULL,
    ADD COLUMN amount NUMERIC(14, 2) NULL,
    ADD COLUMN released_at TIMESTAMPTZ NULL,
    ADD COLUMN release_reason VARCHAR(16) NOT NULL DEFAULT '',
    ADD CONSTRAINT fk_ledger_entry_links_invoice
        FOREIGN KEY (organization_id, invoice_id) REFERENCES invoices (organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT chk_ledger_entry_links_invoice CHECK (invoice_id IS NULL OR document_type = 'invoice'),
    ADD CONSTRAINT chk_ledger_entry_links_release CHECK (
        (released_at IS NULL AND release_reason = '')
        OR (released_at IS NOT NULL AND release_reason IN ('cancelled', 'returned', 'removed'))
    );

-- An item already invoiced cannot be invoiced again while that invoice
-- stands (RT-A22): one live invoice link per ledger entry.
CREATE UNIQUE INDEX uq_ledger_entry_links_invoiced
    ON ledger_entry_links (organization_id, entry_id)
    WHERE invoice_id IS NOT NULL AND released_at IS NULL;
CREATE INDEX idx_ledger_entry_links_organization_invoice
    ON ledger_entry_links (organization_id, invoice_id) WHERE invoice_id IS NOT NULL;
