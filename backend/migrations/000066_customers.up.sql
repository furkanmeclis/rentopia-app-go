-- Customers and drivers (TEC-705, spec §11.1 / §16). Organization-owned:
-- customer records are never shared between organizations (KVKK decision
-- 2021/1303), every index starts with organization_id and children point at
-- their customer through a composite foreign key.
--
-- Customers are organization-wide, not branch-bound: every member with
-- tenant.customers.read sees them (the counter of any branch must find a
-- returning customer); revealing sensitive values needs tenant.customers.reveal.
--
-- Column-level encryption: identity numbers (TCKN / VKN / passport) and
-- driving licence numbers are stored as *_enc (AES-256-GCM, versioned
-- "v<N>:" ciphertext, see internal/platform/crypto.FieldCipher) plus a keyed
-- blind index *_bidx (HMAC-SHA256, purpose- and organization-separated) for
-- exact-match lookups and uniqueness, plus *_hint, the masked form the lists
-- and search show (e.g. 123******45). Plaintext identity numbers are never
-- stored. Phone and e-mail stay plain (operational contact, notifications)
-- but the API masks them unless the caller reveals them.

CREATE TABLE customers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_type VARCHAR(16) NOT NULL CHECK (customer_type IN ('individual', 'corporate')),
    -- Individual: first / last name. Corporate: company title (unvan).
    first_name VARCHAR(128) NOT NULL DEFAULT '',
    last_name VARCHAR(128) NOT NULL DEFAULT '',
    company_title VARCHAR(255) NOT NULL DEFAULT '',
    -- Identity: tckn / passport (individual), vkn / tckn (corporate; a sole
    -- proprietorship is taxed under the owner's TCKN).
    identity_type VARCHAR(16) NULL CHECK (identity_type IN ('tckn', 'vkn', 'passport')),
    identity_enc TEXT NOT NULL DEFAULT '',
    identity_bidx VARCHAR(64) NOT NULL DEFAULT '',
    identity_hint VARCHAR(32) NOT NULL DEFAULT '',
    -- ISO 3166-1 alpha-2 nationality (individual) / country (corporate).
    nationality CHAR(2) NULL,
    tax_office VARCHAR(128) NOT NULL DEFAULT '',
    phone VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    -- Billing (invoice) information.
    billing_address TEXT NOT NULL DEFAULT '',
    billing_city VARCHAR(128) NOT NULL DEFAULT '',
    billing_district VARCHAR(128) NOT NULL DEFAULT '',
    billing_postal_code VARCHAR(16) NOT NULL DEFAULT '',
    billing_country CHAR(2) NULL,
    e_invoice BOOLEAN NOT NULL DEFAULT FALSE,
    note TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    -- Data subject rights: an anonymized customer keeps its row (reservations
    -- and invoices reference it) with every personal value wiped.
    anonymized_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_customers_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customers_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_customers_identity CHECK (
        (identity_type IS NULL AND identity_enc = '' AND identity_bidx = '')
        OR (identity_type IS NOT NULL AND identity_enc <> '' AND identity_bidx <> '')
    ),
    CONSTRAINT chk_customers_name CHECK (
        anonymized_at IS NOT NULL
        OR (customer_type = 'individual' AND first_name <> '' AND last_name <> '')
        OR (customer_type = 'corporate' AND company_title <> '')
    )
);

-- One live customer per identity number in an organization.
CREATE UNIQUE INDEX uq_customers_organization_identity
    ON customers (organization_id, identity_bidx)
    WHERE deleted_at IS NULL AND identity_bidx <> '';

CREATE INDEX idx_customers_organization_created
    ON customers (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_customers_organization_name
    ON customers (organization_id, lower(last_name), lower(first_name), lower(company_title))
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_customers_updated_at
    BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Contacts of a corporate customer (fleet manager, accounting, …).
CREATE TABLE customer_contacts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    title VARCHAR(128) NOT NULL DEFAULT '',
    phone VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_contacts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_customer_contacts_customer
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_customer_contacts_organization_customer
    ON customer_contacts (organization_id, customer_id);

CREATE TRIGGER trg_customer_contacts_updated_at
    BEFORE UPDATE ON customer_contacts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Marketing consent (İYS / ETK), a separate record per channel with full
-- history: a grant opens a row, a withdrawal closes it (withdrawn_at); a new
-- grant after a withdrawal opens a new row. The text version names the
-- consent wording the customer accepted.
CREATE TABLE customer_marketing_consents (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL,
    channel VARCHAR(16) NOT NULL CHECK (channel IN ('sms', 'email', 'call', 'whatsapp')),
    granted_at TIMESTAMPTZ NOT NULL,
    withdrawn_at TIMESTAMPTZ NULL,
    source VARCHAR(16) NOT NULL CHECK (source IN ('counter', 'web', 'phone', 'import', 'other')),
    withdrawal_source VARCHAR(16) NULL CHECK (withdrawal_source IN ('counter', 'web', 'phone', 'import', 'other')),
    text_version VARCHAR(64) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    recorded_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    withdrawn_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_marketing_consents_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_customer_marketing_consents_customer
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_customer_marketing_consents_window CHECK (withdrawn_at IS NULL OR withdrawn_at >= granted_at)
);

-- At most one open grant per customer and channel.
CREATE UNIQUE INDEX uq_customer_marketing_consents_open
    ON customer_marketing_consents (organization_id, customer_id, channel)
    WHERE withdrawn_at IS NULL;

CREATE INDEX idx_customer_marketing_consents_organization_customer
    ON customer_marketing_consents (organization_id, customer_id, granted_at DESC);

-- Drivers of a customer. For an individual customer the driver is usually
-- the customer ("customer is also driver" copies the person); a corporate
-- customer has its employees as drivers. Reservations (TEC-706) reference a
-- customer (contract party), an invoice addressee customer and a driver
-- (actual driver) separately.
CREATE TABLE drivers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL,
    is_customer_self BOOLEAN NOT NULL DEFAULT FALSE,
    first_name VARCHAR(128) NOT NULL,
    last_name VARCHAR(128) NOT NULL,
    -- Plain: the age check of the rental conditions reads it.
    birth_date DATE NULL,
    phone VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    identity_type VARCHAR(16) NULL CHECK (identity_type IN ('tckn', 'passport')),
    identity_enc TEXT NOT NULL DEFAULT '',
    identity_bidx VARCHAR(64) NOT NULL DEFAULT '',
    identity_hint VARCHAR(32) NOT NULL DEFAULT '',
    nationality CHAR(2) NULL,
    licence_enc TEXT NOT NULL DEFAULT '',
    licence_bidx VARCHAR(64) NOT NULL DEFAULT '',
    licence_hint VARCHAR(32) NOT NULL DEFAULT '',
    -- Class / dates stay plain: the licence-years check reads them.
    licence_class VARCHAR(32) NOT NULL DEFAULT '',
    licence_issued_on DATE NULL,
    licence_expires_on DATE NULL,
    -- ISO country of the issuing authority; anything but TR is a foreign
    -- licence.
    licence_country CHAR(2) NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_drivers_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_drivers_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_drivers_customer
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_drivers_identity CHECK (
        (identity_type IS NULL AND identity_enc = '' AND identity_bidx = '')
        OR (identity_type IS NOT NULL AND identity_enc <> '' AND identity_bidx <> '')
    ),
    CONSTRAINT chk_drivers_licence CHECK ((licence_enc = '') = (licence_bidx = '')),
    CONSTRAINT chk_drivers_licence_dates CHECK (
        licence_issued_on IS NULL OR licence_expires_on IS NULL OR licence_expires_on >= licence_issued_on
    )
);

-- A licence is registered once per customer (the same person may drive for
-- two customers of the organization, e.g. privately and for an employer).
CREATE UNIQUE INDEX uq_drivers_organization_customer_licence
    ON drivers (organization_id, customer_id, licence_bidx)
    WHERE deleted_at IS NULL AND licence_bidx <> '';

-- Exact-match lookups by licence / identity across the organization.
CREATE INDEX idx_drivers_organization_licence
    ON drivers (organization_id, licence_bidx)
    WHERE deleted_at IS NULL AND licence_bidx <> '';

CREATE INDEX idx_drivers_organization_identity
    ON drivers (organization_id, identity_bidx)
    WHERE deleted_at IS NULL AND identity_bidx <> '';

CREATE INDEX idx_drivers_organization_customer
    ON drivers (organization_id, customer_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_drivers_updated_at
    BEFORE UPDATE ON drivers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Identity / licence scans and other customer documents in private object
-- storage (organizations/{org}/customers/{customer}/documents/…). Downloads
-- are streamed by the API and need tenant.document.download plus
-- tenant.customers.reveal; every download is an activity event.
CREATE TABLE customer_documents (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL,
    driver_id BIGINT NULL,
    doc_type VARCHAR(24) NOT NULL CHECK (doc_type IN (
        'identity', 'passport', 'driving_licence', 'tax_certificate', 'signature_circular', 'contract', 'other'
    )),
    valid_until DATE NULL,
    file_object_key TEXT NULL,
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    file_content_type VARCHAR(64) NOT NULL DEFAULT '',
    file_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (file_size_bytes >= 0),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_customer_documents_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_customer_documents_customer
        FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_documents_driver
        FOREIGN KEY (organization_id, driver_id)
        REFERENCES drivers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_customer_documents_file CHECK (
        (file_object_key IS NULL AND file_size_bytes = 0)
        OR (file_object_key IS NOT NULL AND file_content_type <> '')
    )
);

CREATE INDEX idx_customer_documents_organization_customer
    ON customer_documents (organization_id, customer_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_customer_documents_updated_at
    BEFORE UPDATE ON customer_documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
