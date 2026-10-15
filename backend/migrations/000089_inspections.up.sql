-- Vehicle inspections: mobile handover / return forms and their evidence
-- (TEC-712, TEC-713, TEC-714; acceptance RT-A17, RT-A19). The design is
-- docs/design/inspections.md.
--
-- inspection_templates: per organization, the required photo slots
-- (default: ön, arka, sol, sağ, iç, gösterge) and checklist / equipment
-- items. One active version; saving a template archives the previous one
-- and inserts the next version, so a finalized inspection keeps naming the
-- exact template it was filled with (it also stores a snapshot).
--
-- inspections: one form per rental, kind (handover / return) and version.
-- A draft is edited on the tablet; finalizing it records the handover or
-- the return through the rentals use cases. A finalized row is immutable
-- (trigger below): a correction is a new version with a reason, the
-- previous one becomes superseded.
--
-- inspection_damages: a zone of the vehicle diagram, the kind / severity,
-- a description and its photos. inspection_photos: the evidence of one
-- slot (or a damage): object key under organizations/{org}/rentals/
-- {rental}/inspections/..., server-computed SHA-256, server upload time,
-- device EXIF time / GPS kept apart. inspection_signatures: the signature
-- canvas (strokes with pressure + PNG). A contract approval made with that
-- signature points at it (document_approvals.signature_id).
--
-- Every index starts with organization_id; child rows use composite FKs.

CREATE TABLE inspection_templates (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    -- [{key, label, required}] (photo slots) / [{key, label, required}]
    -- (checklist and equipment items).
    photo_slots JSONB NOT NULL DEFAULT '[]'::jsonb,
    checklist JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_inspection_templates_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_inspection_templates_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_inspection_templates_organization_version UNIQUE (organization_id, version),
    CONSTRAINT chk_inspection_templates_slots CHECK (jsonb_typeof(photo_slots) = 'array'),
    CONSTRAINT chk_inspection_templates_checklist CHECK (jsonb_typeof(checklist) = 'array')
);

CREATE UNIQUE INDEX uq_inspection_templates_active
    ON inspection_templates (organization_id) WHERE status = 'active';
CREATE INDEX idx_inspection_templates_organization_created
    ON inspection_templates (organization_id, created_at DESC);

CREATE TRIGGER trg_inspection_templates_updated_at
    BEFORE UPDATE ON inspection_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Inspections reference the rental leg they recorded.
ALTER TABLE rental_vehicles
    ADD CONSTRAINT uq_rental_vehicles_organization_id UNIQUE (organization_id, id);

CREATE TABLE inspections (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rental_id BIGINT NOT NULL,
    vehicle_id BIGINT NOT NULL,
    -- The leg the finalized form recorded (handover: the new leg; return:
    -- the leg it closed).
    rental_vehicle_id BIGINT NULL,
    kind VARCHAR(12) NOT NULL CHECK (kind IN ('handover', 'return')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    status VARCHAR(12) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'finalized', 'superseded')),
    -- A correction of a finalized form: the version it replaces and why.
    previous_id BIGINT NULL,
    revision_reason TEXT NOT NULL DEFAULT '',
    template_id BIGINT NOT NULL,
    template_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- What physically happened (the rental use case records the same
    -- facts on the rental and its leg when the form is finalized).
    occurred_at TIMESTAMPTZ NULL,
    branch_id BIGINT NULL,
    point_id BIGINT NULL,
    km INTEGER NULL CHECK (km BETWEEN 0 AND 5000000),
    fuel SMALLINT NULL CHECK (fuel BETWEEN 0 AND 100),
    -- {item_key: true/false} for the template's checklist / equipment.
    equipment JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Step 1: {plate_confirmed, vehicle_ok, driver_uuid, licence_checked,
    -- identity_checked}.
    verification JSONB NOT NULL DEFAULT '{}'::jsonb,
    note TEXT NOT NULL DEFAULT '',
    customer_statement TEXT NOT NULL DEFAULT '',
    customer_reviewed_at TIMESTAMPTZ NULL,
    -- Return: staff changes of computed charge amounts while drafting
    -- ([{key, amount, reason}], tenant.price.override).
    charge_overrides JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Return: where the vehicle goes after the check.
    vehicle_status_after VARCHAR(16) NULL
        CHECK (vehicle_status_after IN ('cleaning', 'available', 'maintenance', 'preparing')),
    signature_status VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (signature_status IN ('pending', 'signed', 'refused', 'waived')),
    signature_note TEXT NOT NULL DEFAULT '',
    -- Exception path (RT-A19): the customer disputes the findings or did
    -- not sign; the form lands in the authorized review queue.
    disputed BOOLEAN NOT NULL DEFAULT FALSE,
    dispute_note TEXT NOT NULL DEFAULT '',
    review_status VARCHAR(12) NOT NULL DEFAULT 'none' CHECK (review_status IN ('none', 'pending', 'resolved')),
    reviewed_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ NULL,
    review_note TEXT NOT NULL DEFAULT '',
    finalized_at TIMESTAMPTZ NULL,
    finalized_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_inspections_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_inspections_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_inspections_organization_rental_kind_version UNIQUE (organization_id, rental_id, kind, version),
    CONSTRAINT chk_inspections_finalized CHECK ((status = 'draft') = (finalized_at IS NULL)),
    CONSTRAINT chk_inspections_revision CHECK (previous_id IS NULL OR length(revision_reason) >= 3),
    CONSTRAINT fk_inspections_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_inspections_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id),
    CONSTRAINT fk_inspections_rental_vehicle
        FOREIGN KEY (organization_id, rental_vehicle_id) REFERENCES rental_vehicles (organization_id, id),
    CONSTRAINT fk_inspections_previous
        FOREIGN KEY (organization_id, previous_id) REFERENCES inspections (organization_id, id),
    CONSTRAINT fk_inspections_template
        FOREIGN KEY (organization_id, template_id) REFERENCES inspection_templates (organization_id, id),
    CONSTRAINT fk_inspections_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_inspections_point
        FOREIGN KEY (organization_id, point_id) REFERENCES service_points (organization_id, id)
);

-- One live (draft or finalized) form per rental and kind.
CREATE UNIQUE INDEX uq_inspections_live
    ON inspections (organization_id, rental_id, kind) WHERE status <> 'superseded';
CREATE INDEX idx_inspections_organization_rental ON inspections (organization_id, rental_id, kind, version);
CREATE INDEX idx_inspections_organization_created ON inspections (organization_id, created_at DESC);
CREATE INDEX idx_inspections_organization_review
    ON inspections (organization_id, review_status) WHERE review_status = 'pending';
CREATE INDEX idx_inspections_organization_template ON inspections (organization_id, template_id);

CREATE TRIGGER trg_inspections_updated_at
    BEFORE UPDATE ON inspections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A finalized form is immutable: only its supersession (finalized ->
-- superseded) and the review fields may change.
CREATE FUNCTION inspections_immutable() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'draft' THEN
        IF (to_jsonb(NEW) - ARRAY['status', 'review_status', 'reviewed_by', 'reviewed_at', 'review_note', 'updated_at'])
            IS DISTINCT FROM
           (to_jsonb(OLD) - ARRAY['status', 'review_status', 'reviewed_by', 'reviewed_at', 'review_note', 'updated_at'])
            OR (NEW.status IS DISTINCT FROM OLD.status AND NOT (OLD.status = 'finalized' AND NEW.status = 'superseded')) THEN
            RAISE EXCEPTION 'finalized inspections are immutable (create a new version with a reason)'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'inspections_immutable';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_inspections_immutable
    BEFORE UPDATE ON inspections
    FOR EACH ROW EXECUTE FUNCTION inspections_immutable();

-- Evidence rows of a finalized form cannot be added or changed. (Deletes
-- are not blocked here: organization / rental deletion cascades through
-- them; the API only deletes evidence of drafts.)
CREATE FUNCTION inspection_children_immutable() RETURNS trigger AS $$
DECLARE
    parent_status VARCHAR(12);
BEGIN
    SELECT i.status INTO parent_status
    FROM inspections i
    WHERE i.organization_id = NEW.organization_id AND i.id = NEW.inspection_id;
    IF parent_status IS NOT NULL AND parent_status <> 'draft' THEN
        RAISE EXCEPTION 'evidence of a finalized inspection is immutable'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'inspection_children_immutable';
    END IF;
    IF TG_OP = 'UPDATE' AND (NEW.inspection_id IS DISTINCT FROM OLD.inspection_id
        OR NEW.organization_id IS DISTINCT FROM OLD.organization_id) THEN
        RAISE EXCEPTION 'evidence cannot move to another inspection'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'inspection_children_immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE inspection_damages (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    inspection_id BIGINT NOT NULL,
    -- Zone of the vehicle diagram (front, rear, left_front_door, roof, ...)
    -- and an optional marker position inside the diagram (0..1).
    zone VARCHAR(32) NOT NULL,
    pos_x NUMERIC(5, 4) NULL CHECK (pos_x BETWEEN 0 AND 1),
    pos_y NUMERIC(5, 4) NULL CHECK (pos_y BETWEEN 0 AND 1),
    damage_type VARCHAR(16) NOT NULL DEFAULT 'scratch'
        CHECK (damage_type IN ('scratch', 'dent', 'crack', 'broken', 'missing', 'stain', 'other')),
    severity VARCHAR(12) NOT NULL DEFAULT 'minor' CHECK (severity IN ('minor', 'moderate', 'major')),
    description TEXT NOT NULL DEFAULT '',
    -- Return: found now (not on the handover record).
    is_new BOOLEAN NOT NULL DEFAULT TRUE,
    -- Return: the customer disputes this finding.
    disputed BOOLEAN NOT NULL DEFAULT FALSE,
    -- Revisions copy the findings forward.
    copied_from_id BIGINT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_inspection_damages_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_inspection_damages_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_inspection_damages_inspection
        FOREIGN KEY (organization_id, inspection_id) REFERENCES inspections (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_inspection_damages_organization_inspection ON inspection_damages (organization_id, inspection_id, id);

CREATE TRIGGER trg_inspection_damages_updated_at
    BEFORE UPDATE ON inspection_damages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_inspection_damages_immutable
    BEFORE INSERT OR UPDATE ON inspection_damages
    FOR EACH ROW EXECUTE FUNCTION inspection_children_immutable();

CREATE TABLE inspection_photos (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    inspection_id BIGINT NOT NULL,
    -- The rental and the form version the evidence belongs to.
    rental_id BIGINT NOT NULL,
    inspection_version INTEGER NOT NULL,
    -- A template slot key, 'damage' (damage_id set) or 'extra'.
    slot VARCHAR(32) NOT NULL,
    damage_id BIGINT NULL,
    -- Idempotency key of the device's upload queue.
    client_ref VARCHAR(64) NULL,
    -- pending (row + upload address issued) -> uploaded (bytes stored)
    -- -> completed (object verified, hashed on the server).
    status VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'uploaded', 'completed')),
    object_key TEXT NOT NULL,
    content_type VARCHAR(64) NOT NULL DEFAULT '',
    declared_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (declared_size_bytes >= 0),
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    -- Server-side SHA-256 of the stored bytes and the server time of the
    -- completion (never the device clock).
    sha256 CHAR(64) NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    uploaded_at TIMESTAMPTZ NULL,
    uploaded_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    -- Device facts, kept apart from server time: the device clock at
    -- capture and the EXIF date / GPS read from the file.
    device_captured_at TIMESTAMPTZ NULL,
    exif_taken_at TIMESTAMPTZ NULL,
    exif_taken_local VARCHAR(32) NOT NULL DEFAULT '',
    exif_gps_lat NUMERIC(9, 6) NULL CHECK (exif_gps_lat BETWEEN -90 AND 90),
    exif_gps_lng NUMERIC(9, 6) NULL CHECK (exif_gps_lng BETWEEN -180 AND 180),
    exif JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Optional malware scan (ClamAV hook): skipped when no scanner is set.
    scan_status VARCHAR(12) NOT NULL DEFAULT 'skipped' CHECK (scan_status IN ('skipped', 'clean', 'infected')),
    copied_from_id BIGINT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_inspection_photos_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_inspection_photos_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_inspection_photos_completed
        CHECK ((status = 'completed') = (sha256 IS NOT NULL AND uploaded_at IS NOT NULL)),
    CONSTRAINT chk_inspection_photos_damage CHECK ((slot = 'damage') = (damage_id IS NOT NULL)),
    CONSTRAINT fk_inspection_photos_inspection
        FOREIGN KEY (organization_id, inspection_id) REFERENCES inspections (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_inspection_photos_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_inspection_photos_damage
        FOREIGN KEY (organization_id, damage_id) REFERENCES inspection_damages (organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_inspection_photos_client_ref
    ON inspection_photos (organization_id, inspection_id, client_ref) WHERE client_ref IS NOT NULL;
CREATE INDEX idx_inspection_photos_organization_inspection ON inspection_photos (organization_id, inspection_id, slot);
CREATE INDEX idx_inspection_photos_organization_rental ON inspection_photos (organization_id, rental_id);
CREATE INDEX idx_inspection_photos_organization_damage
    ON inspection_photos (organization_id, damage_id) WHERE damage_id IS NOT NULL;

CREATE TRIGGER trg_inspection_photos_updated_at
    BEFORE UPDATE ON inspection_photos
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_inspection_photos_immutable
    BEFORE INSERT OR UPDATE ON inspection_photos
    FOR EACH ROW EXECUTE FUNCTION inspection_children_immutable();

CREATE TABLE inspection_signatures (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    inspection_id BIGINT NOT NULL,
    rental_id BIGINT NOT NULL,
    signer_name VARCHAR(200) NOT NULL,
    signer_role VARCHAR(12) NOT NULL DEFAULT 'customer' CHECK (signer_role IN ('customer', 'driver')),
    -- The PNG (private object) and its server-side SHA-256.
    object_key TEXT NOT NULL,
    sha256 CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    size_bytes BIGINT NOT NULL DEFAULT 0,
    -- {width, height, strokes: [[[x, y, pressure, t_ms], ...], ...]}
    strokes JSONB NOT NULL,
    point_count INTEGER NOT NULL DEFAULT 0 CHECK (point_count >= 0),
    signed_at TIMESTAMPTZ NOT NULL,
    device_signed_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_inspection_signatures_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_inspection_signatures_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_inspection_signatures_inspection UNIQUE (organization_id, inspection_id),
    CONSTRAINT fk_inspection_signatures_inspection
        FOREIGN KEY (organization_id, inspection_id) REFERENCES inspections (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_inspection_signatures_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_inspection_signatures_organization_rental ON inspection_signatures (organization_id, rental_id);

CREATE TRIGGER trg_inspection_signatures_immutable
    BEFORE INSERT OR UPDATE ON inspection_signatures
    FOR EACH ROW EXECUTE FUNCTION inspection_children_immutable();

-- A contract approval given with the tablet signature names it (the
-- approval stays bound to the version's content hash).
ALTER TABLE document_approvals
    ADD COLUMN signature_id BIGINT NULL,
    ADD CONSTRAINT fk_document_approvals_signature
        FOREIGN KEY (organization_id, signature_id) REFERENCES inspection_signatures (organization_id, id)
        ON DELETE SET NULL (signature_id);
CREATE INDEX idx_document_approvals_organization_signature
    ON document_approvals (organization_id, signature_id) WHERE signature_id IS NOT NULL;

-- Charges computed from a return form: the form, the dispute flag and the
-- staff override of a computed amount (tenant.price.override).
ALTER TABLE rental_charges
    ADD COLUMN inspection_id BIGINT NULL,
    ADD COLUMN disputed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN original_amount NUMERIC(14, 2) NULL CHECK (original_amount >= 0),
    ADD COLUMN override_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN overridden_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN overridden_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT fk_rental_charges_inspection
        FOREIGN KEY (organization_id, inspection_id) REFERENCES inspections (organization_id, id)
        ON DELETE SET NULL (inspection_id);
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_kind_check;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_kind_check
    CHECK (kind IN ('late_return', 'early_return', 'damage', 'fuel', 'mileage', 'equipment', 'other'));
CREATE INDEX idx_rental_charges_organization_inspection
    ON rental_charges (organization_id, inspection_id) WHERE inspection_id IS NOT NULL;

-- Operation tasks after the return: maintenance and the deposit
-- resolution.
ALTER TABLE operation_tasks DROP CONSTRAINT operation_tasks_task_type_check;
ALTER TABLE operation_tasks ADD CONSTRAINT operation_tasks_task_type_check
    CHECK (task_type IN ('preparation', 'handover', 'return', 'cleaning', 'maintenance', 'deposit'));
