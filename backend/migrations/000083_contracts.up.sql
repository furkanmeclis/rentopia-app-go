-- Contract templates, documents and their versions (TEC-710, spec §10,
-- RT-A18).
--
-- contract_templates: one row per template version of a kind (rental
-- contract, handover form, return form, terms). The body is rich text from
-- the panel editor (editor state for editing, sanitized HTML for rendering)
-- with {{placeholder}} tokens. status draft -> active -> archived; one
-- active template per kind; editing an active template creates the next
-- draft version.
--
-- documents: the contract / form / terms document of one reservation (and
-- its rental when there is one), one live document per reservation and
-- kind. document_versions are immutable renders: HTML, its SHA-256
-- (content_hash), the source data snapshot (JSON) and the PDF (private
-- object, streamed by the API only). Any change of the people, the vehicle
-- or the important terms renders a new version; the previous one becomes
-- superseded.
--
-- document_approvals: who approved which content (approval bound to the
-- content hash by a composite FK: an approval can only name the exact
-- content it approved, so a new version always needs a new approval).
-- Signing flows arrive in TEC-711 (methods click / signature).
--
-- Every index starts with organization_id.

CREATE TABLE contract_templates (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind VARCHAR(24) NOT NULL CHECK (kind IN ('rental_contract', 'handover_form', 'return_form', 'terms')),
    name VARCHAR(160) NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    status VARCHAR(12) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    -- Editor state (Lexical JSON) for editing; body_html is the sanitized
    -- HTML the renderer fills.
    content JSONB NOT NULL DEFAULT '{}'::jsonb,
    body_html TEXT NOT NULL DEFAULT '',
    -- Placeholder keys found in body_html (validated against the catalog).
    placeholders TEXT[] NOT NULL DEFAULT '{}',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    activated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    activated_at TIMESTAMPTZ NULL,
    archived_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contract_templates_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_contract_templates_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_contract_templates_organization_kind_version UNIQUE (organization_id, kind, version),
    CONSTRAINT chk_contract_templates_active CHECK (status = 'draft' OR activated_at IS NOT NULL)
);

CREATE UNIQUE INDEX uq_contract_templates_active_kind
    ON contract_templates (organization_id, kind) WHERE status = 'active';
CREATE INDEX idx_contract_templates_organization_created ON contract_templates (organization_id, created_at DESC);
CREATE INDEX idx_contract_templates_organization_status ON contract_templates (organization_id, status);

CREATE TRIGGER trg_contract_templates_updated_at
    BEFORE UPDATE ON contract_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE documents (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind VARCHAR(24) NOT NULL CHECK (kind IN ('rental_contract', 'handover_form', 'return_form', 'terms')),
    reservation_id BIGINT NOT NULL,
    rental_id BIGINT NULL,
    -- draft: the current version awaits approval; approved: the current
    -- version is approved; void: withdrawn (the rental was cancelled).
    status VARCHAR(12) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'void')),
    current_version_id BIGINT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_documents_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_documents_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_documents_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_documents_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_documents_live_kind
    ON documents (organization_id, reservation_id, kind) WHERE status <> 'void';
CREATE INDEX idx_documents_organization_created ON documents (organization_id, created_at DESC);
CREATE INDEX idx_documents_organization_reservation ON documents (organization_id, reservation_id);
CREATE INDEX idx_documents_organization_rental ON documents (organization_id, rental_id) WHERE rental_id IS NOT NULL;

CREATE TRIGGER trg_documents_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE document_versions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    document_id BIGINT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    status VARCHAR(12) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'issued', 'approved', 'superseded')),
    template_id BIGINT NOT NULL,
    template_version INTEGER NOT NULL,
    -- The rendered HTML, its SHA-256 (hex) and the data it was rendered
    -- from. Never updated (trigger below).
    html TEXT NOT NULL,
    content_hash CHAR(64) NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    data_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Snapshot keys that differ from the previous version (why it exists).
    changed_keys TEXT[] NOT NULL DEFAULT '{}',
    -- PDF render (worker, headless Chromium): private object key.
    pdf_status VARCHAR(8) NOT NULL DEFAULT 'none' CHECK (pdf_status IN ('none', 'pending', 'ready', 'failed')),
    pdf_object_key TEXT NULL,
    pdf_size_bytes BIGINT NOT NULL DEFAULT 0,
    pdf_sha256 CHAR(64) NULL,
    pdf_error TEXT NOT NULL DEFAULT '',
    pdf_rendered_at TIMESTAMPTZ NULL,
    issued_at TIMESTAMPTZ NULL,
    issued_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    superseded_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_versions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_document_versions_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_document_versions_organization_document_version UNIQUE (organization_id, document_id, version),
    -- Target of the approval FK (approval bound to the content hash).
    CONSTRAINT uq_document_versions_organization_id_hash UNIQUE (organization_id, id, content_hash),
    CONSTRAINT chk_document_versions_pdf CHECK (pdf_status <> 'ready' OR pdf_object_key IS NOT NULL),
    CONSTRAINT fk_document_versions_document
        FOREIGN KEY (organization_id, document_id) REFERENCES documents (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_document_versions_template
        FOREIGN KEY (organization_id, template_id) REFERENCES contract_templates (organization_id, id)
);

CREATE INDEX idx_document_versions_organization_document ON document_versions (organization_id, document_id, version DESC);
CREATE INDEX idx_document_versions_organization_template ON document_versions (organization_id, template_id);

-- A rendered version is immutable: content, hash and data never change.
CREATE FUNCTION document_versions_immutable() RETURNS trigger AS $$
BEGIN
    IF NEW.html IS DISTINCT FROM OLD.html
        OR NEW.content_hash IS DISTINCT FROM OLD.content_hash
        OR NEW.data_snapshot IS DISTINCT FROM OLD.data_snapshot
        OR NEW.document_id IS DISTINCT FROM OLD.document_id
        OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.template_id IS DISTINCT FROM OLD.template_id THEN
        RAISE EXCEPTION 'document versions are immutable (render a new version)'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'document_versions_immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_document_versions_immutable
    BEFORE UPDATE ON document_versions
    FOR EACH ROW EXECUTE FUNCTION document_versions_immutable();

ALTER TABLE documents
    ADD CONSTRAINT fk_documents_current_version
    FOREIGN KEY (organization_id, current_version_id) REFERENCES document_versions (organization_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE document_approvals (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    document_version_id BIGINT NOT NULL,
    content_hash CHAR(64) NOT NULL,
    -- customer (the renter / driver) or staff (an internal approval)
    party VARCHAR(12) NOT NULL DEFAULT 'customer' CHECK (party IN ('customer', 'staff')),
    -- in_person: recorded by staff at the counter; click / signature: TEC-711.
    method VARCHAR(16) NOT NULL CHECK (method IN ('in_person', 'click', 'signature')),
    approver_name VARCHAR(200) NOT NULL,
    customer_id BIGINT NULL,
    note TEXT NOT NULL DEFAULT '',
    recorded_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Set when the content was superseded by a new version.
    invalidated_at TIMESTAMPTZ NULL,
    invalidated_reason TEXT NOT NULL DEFAULT '',
    CONSTRAINT uq_document_approvals_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_document_approvals_version_hash
        FOREIGN KEY (organization_id, document_version_id, content_hash)
        REFERENCES document_versions (organization_id, id, content_hash) ON DELETE CASCADE,
    CONSTRAINT fk_document_approvals_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id) ON DELETE SET NULL (customer_id)
);

CREATE INDEX idx_document_approvals_organization_version ON document_approvals (organization_id, document_version_id);
