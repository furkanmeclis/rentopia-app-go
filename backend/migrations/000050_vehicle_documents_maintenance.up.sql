-- Vehicle documents, maintenance and expiry reminders (TEC-693). Both are
-- sub-resources of a vehicle (tenant.vehicles.read / write); every index
-- starts with organization_id and children point at their vehicle through a
-- composite foreign key, so a row can never reference another organization's
-- vehicle.
--
-- Expiry tracking is computed on read, never copied into vehicles: the
-- "current" document of a type is the live one with the latest valid_until.
-- Reminder horizons are constants in the application (30 days for
-- documents, 15 days / 1000 km for maintenance) until organization settings
-- grow a field for them.

CREATE TABLE vehicle_documents (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    doc_type VARCHAR(24) NOT NULL CHECK (doc_type IN (
        'ruhsat', 'trafik_sigortasi', 'kasko', 'muayene', 'egzoz', 'diger'
    )),
    document_number VARCHAR(128) NOT NULL DEFAULT '',
    issuer VARCHAR(255) NOT NULL DEFAULT '',
    valid_from DATE NULL,
    valid_until DATE NULL,
    -- Scanned file in private object storage. The key is never sent to the
    -- browser; downloads are streamed by the API.
    file_object_key TEXT NULL,
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    file_content_type VARCHAR(64) NOT NULL DEFAULT '',
    file_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (file_size_bytes >= 0),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_vehicle_documents_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_vehicle_documents_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vehicle_documents_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_vehicle_documents_validity CHECK (
        valid_from IS NULL OR valid_until IS NULL OR valid_until >= valid_from
    ),
    CONSTRAINT chk_vehicle_documents_file CHECK (
        (file_object_key IS NULL AND file_size_bytes = 0)
        OR (file_object_key IS NOT NULL AND file_content_type <> '')
    )
);

-- Documents of a vehicle and its current document per type.
CREATE INDEX idx_vehicle_documents_organization_vehicle
    ON vehicle_documents (organization_id, vehicle_id, doc_type, valid_until DESC)
    WHERE deleted_at IS NULL;

-- Expiry sweep of the daily reminder job and the alerts view.
CREATE INDEX idx_vehicle_documents_organization_valid_until
    ON vehicle_documents (organization_id, valid_until)
    WHERE deleted_at IS NULL AND valid_until IS NOT NULL;

CREATE TRIGGER trg_vehicle_documents_updated_at
    BEFORE UPDATE ON vehicle_documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Planned and done maintenance. Completing a planned item turns the same row
-- into done (kind = 'done' with the done date / km). Cost is TRY only; the
-- expense linkage (expense_id with a composite foreign key) arrives with the
-- expenses table in M4.
CREATE TABLE vehicle_maintenance (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('planned', 'done')),
    maintenance_type VARCHAR(16) NOT NULL CHECK (maintenance_type IN (
        'periodic', 'tire', 'brake', 'oil', 'repair', 'other'
    )),
    planned_date DATE NULL,
    planned_km INTEGER NULL CHECK (planned_km BETWEEN 0 AND 5000000),
    done_date DATE NULL,
    done_km INTEGER NULL CHECK (done_km BETWEEN 0 AND 5000000),
    service_provider VARCHAR(255) NOT NULL DEFAULT '',
    cost NUMERIC(14, 2) NULL CHECK (cost >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY' CHECK (currency = 'TRY'),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    completed_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_vehicle_maintenance_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_vehicle_maintenance_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vehicle_maintenance_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    -- A plan needs a date or a km target; a done item needs its date.
    CONSTRAINT chk_vehicle_maintenance_planned CHECK (
        kind <> 'planned' OR planned_date IS NOT NULL OR planned_km IS NOT NULL
    ),
    CONSTRAINT chk_vehicle_maintenance_done CHECK (kind <> 'done' OR done_date IS NOT NULL)
);

CREATE INDEX idx_vehicle_maintenance_organization_vehicle
    ON vehicle_maintenance (organization_id, vehicle_id, kind)
    WHERE deleted_at IS NULL;

-- Due sweep of open plans (reminder job, alerts view, next maintenance).
CREATE INDEX idx_vehicle_maintenance_organization_planned
    ON vehicle_maintenance (organization_id, planned_date)
    WHERE deleted_at IS NULL AND kind = 'planned';

CREATE TRIGGER trg_vehicle_maintenance_updated_at
    BEFORE UPDATE ON vehicle_maintenance
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Idempotency log of the daily reminder job: one row per organization,
-- subject (document or maintenance item) and day. A rerun on the same day
-- finds the row and sends nothing.
CREATE TABLE vehicle_reminder_deliveries (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    subject_kind VARCHAR(16) NOT NULL CHECK (subject_kind IN ('document', 'maintenance')),
    subject_id BIGINT NOT NULL,
    reminder_date DATE NOT NULL,
    alert_state VARCHAR(16) NOT NULL CHECK (alert_state IN ('expiring', 'overdue')),
    recipients INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_vehicle_reminder_deliveries
        PRIMARY KEY (organization_id, subject_kind, subject_id, reminder_date),
    CONSTRAINT fk_vehicle_reminder_deliveries_vehicle
        FOREIGN KEY (organization_id, vehicle_id)
        REFERENCES vehicles (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_vehicle_reminder_deliveries_organization_vehicle
    ON vehicle_reminder_deliveries (organization_id, vehicle_id);
