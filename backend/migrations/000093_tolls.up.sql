-- HGS / OGS toll passes (TEC-725, RT-A20, docs/design/tolls.md).
--
-- vehicle_toll_tags links a vehicle to a toll tag (HGS sticker / OGS
-- device) of a provider for a period: a vehicle may change tags over time,
-- and a tag number belongs to at most one vehicle at any moment
-- (exclusion constraint over the validity range).
--
-- toll_passes holds one pass reported by a provider (API fetch) or a file
-- import: the provider's transaction id is unique per organization and
-- provider, so a pass that arrives again updates the same row. Every change
-- of its facts is a toll_pass_revisions row; an amount change never edits
-- the rental charge in place, it reverses the charge and creates a new one.
-- Matching links the pass to the rental whose ACTUAL handover → return
-- interval of the vehicle contains the pass time; anything ambiguous goes
-- to the review queue (status needs_review with a reason), never to "no
-- debt".
--
-- rental_charges gains the kinds toll / toll_fee, the source toll, the pass
-- link and reversal rows (negative amount, reversal_of_id, at most one
-- reversal per charge).
--
-- toll_settings is the organization's processing fee and fetch window.
--
-- integration_accounts accepts the kind toll and the development provider
-- hgs_fake (registered outside production only).

CREATE TABLE vehicle_toll_tags (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    vehicle_id BIGINT NOT NULL,
    -- Provider / issuer code of the tag (hgs_fake, ptt, a bank …).
    provider VARCHAR(32) NOT NULL,
    network VARCHAR(8) NOT NULL DEFAULT 'hgs' CHECK (network IN ('hgs', 'ogs')),
    tag_number VARCHAR(40) NOT NULL,
    tag_key VARCHAR(40) GENERATED ALWAYS AS (upper(regexp_replace(tag_number, '[^A-Za-z0-9]', '', 'g'))) STORED,
    valid_from TIMESTAMPTZ NOT NULL,
    -- NULL: still on the vehicle.
    valid_to TIMESTAMPTZ NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vehicle_toll_tags_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_vehicle_toll_tags_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vehicle_toll_tags_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_vehicle_toll_tags_period CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT chk_vehicle_toll_tags_number CHECK (length(tag_key) >= 4),
    -- A tag number is on one vehicle at a time.
    CONSTRAINT ex_vehicle_toll_tags_period EXCLUDE USING gist (
        organization_id WITH =, tag_key WITH =, tstzrange(valid_from, valid_to, '[)') WITH &&
    )
);

CREATE INDEX idx_vehicle_toll_tags_organization_vehicle ON vehicle_toll_tags (organization_id, vehicle_id, valid_from DESC);
CREATE INDEX idx_vehicle_toll_tags_organization_tag ON vehicle_toll_tags (organization_id, tag_key);

CREATE TRIGGER trg_vehicle_toll_tags_updated_at
    BEFORE UPDATE ON vehicle_toll_tags
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE toll_passes (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Provider code (the integration provider, or the code a file names).
    provider VARCHAR(32) NOT NULL,
    -- The provider's transaction id: unique per organization and provider.
    external_id VARCHAR(120) NOT NULL,
    source VARCHAR(8) NOT NULL CHECK (source IN ('api', 'file')),
    integration_account_id BIGINT NULL,
    passed_at TIMESTAMPTZ NOT NULL,
    plate VARCHAR(20) NOT NULL DEFAULT '',
    plate_key VARCHAR(20) GENERATED ALWAYS AS (upper(regexp_replace(plate, '[^A-Za-z0-9]', '', 'g'))) STORED,
    tag_number VARCHAR(40) NOT NULL DEFAULT '',
    tag_key VARCHAR(40) GENERATED ALWAYS AS (upper(regexp_replace(tag_number, '[^A-Za-z0-9]', '', 'g'))) STORED,
    -- Station (bridge, toll plaza) or the entry of a motorway route.
    station VARCHAR(160) NOT NULL DEFAULT '',
    -- Exit of a motorway route ('' for a single station).
    exit_station VARCHAR(160) NOT NULL DEFAULT '',
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    amount NUMERIC(14, 2) NOT NULL CHECK (amount >= 0),
    -- unmatched: no rental (not matched yet, or unassigned by a person);
    -- needs_review: matching was ambiguous (review_reason says why);
    -- matched: linked to a rental, nothing to charge (zero amount);
    -- charged: linked to a rental with a live toll charge;
    -- ignored: a person decided it is not billed.
    status VARCHAR(16) NOT NULL DEFAULT 'unmatched'
        CHECK (status IN ('unmatched', 'matched', 'needs_review', 'ignored', 'charged')),
    review_reason VARCHAR(32) NOT NULL DEFAULT '',
    vehicle_id BIGINT NULL,
    rental_id BIGINT NULL,
    -- How the vehicle / rental were found: plate, tag, manual.
    match_method VARCHAR(8) NOT NULL DEFAULT '' CHECK (match_method IN ('', 'plate', 'tag', 'manual')),
    -- A person assigned / unassigned the pass: automatic matching keeps it.
    manual BOOLEAN NOT NULL DEFAULT FALSE,
    -- The live charges of the pass (toll amount and processing fee).
    charge_id BIGINT NULL,
    fee_charge_id BIGINT NULL,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    first_fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Non-sensitive provider fields kept for reference.
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    decided_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_toll_passes_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_toll_passes_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_toll_passes_organization_external UNIQUE (organization_id, provider, external_id),
    CONSTRAINT fk_toll_passes_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE SET NULL (integration_account_id),
    CONSTRAINT fk_toll_passes_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id),
    -- No action: vehicles are soft-deleted and rentals are never deleted on
    -- their own; an organization delete removes passes in the same cascade.
    CONSTRAINT fk_toll_passes_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id),
    CONSTRAINT chk_toll_passes_rental CHECK ((status IN ('matched', 'charged')) = (rental_id IS NOT NULL)),
    CONSTRAINT chk_toll_passes_charge CHECK ((status = 'charged') = (charge_id IS NOT NULL))
);

CREATE INDEX idx_toll_passes_organization_passed ON toll_passes (organization_id, passed_at DESC, id DESC);
CREATE INDEX idx_toll_passes_organization_status ON toll_passes (organization_id, status, passed_at DESC);
CREATE INDEX idx_toll_passes_organization_vehicle ON toll_passes (organization_id, vehicle_id, passed_at DESC);
CREATE INDEX idx_toll_passes_organization_rental ON toll_passes (organization_id, rental_id) WHERE rental_id IS NOT NULL;
CREATE INDEX idx_toll_passes_organization_plate ON toll_passes (organization_id, plate_key);

CREATE TRIGGER trg_toll_passes_updated_at
    BEFORE UPDATE ON toll_passes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A changed fact of a pass that arrived again (amount, time, plate …).
CREATE TABLE toll_pass_revisions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    toll_pass_id BIGINT NOT NULL,
    -- The revision this row created (the first arrival is revision 1 and
    -- has no row).
    revision INTEGER NOT NULL CHECK (revision >= 2),
    source VARCHAR(8) NOT NULL CHECK (source IN ('api', 'file')),
    changed_fields TEXT[] NOT NULL,
    previous JSONB NOT NULL,
    current JSONB NOT NULL,
    amount_before NUMERIC(14, 2) NOT NULL,
    amount_after NUMERIC(14, 2) NOT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_toll_pass_revisions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_toll_pass_revisions_pass_revision UNIQUE (organization_id, toll_pass_id, revision),
    CONSTRAINT fk_toll_pass_revisions_pass
        FOREIGN KEY (organization_id, toll_pass_id) REFERENCES toll_passes (organization_id, id) ON DELETE CASCADE
);

-- The pass timeline (received, matched, review, assigned, unassigned,
-- ignored, revised, charge created / reversed); manual actions carry the
-- mandatory reason and the actor.
CREATE TABLE toll_pass_events (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    toll_pass_id BIGINT NOT NULL,
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(16) NULL,
    to_status VARCHAR(16) NULL,
    reason TEXT NOT NULL DEFAULT '',
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_toll_pass_events_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_toll_pass_events_pass
        FOREIGN KEY (organization_id, toll_pass_id) REFERENCES toll_passes (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_toll_pass_events_organization_pass ON toll_pass_events (organization_id, toll_pass_id, created_at, id);

CREATE TABLE toll_settings (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    -- Charged to the rental as its own line next to each toll (0 = none).
    processing_fee NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (processing_fee >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    -- Periodic provider fetch and its look-back window (passes are often
    -- reported days late).
    auto_fetch BOOLEAN NOT NULL DEFAULT TRUE,
    lookback_days INTEGER NOT NULL DEFAULT 7 CHECK (lookback_days BETWEEN 1 AND 60),
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_toll_settings_updated_at
    BEFORE UPDATE ON toll_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Rental charges: toll kinds, the toll source, the pass link and reversal
-- rows. A reversal has a non-positive amount and points at the charge it
-- cancels; a charge is reversed at most once.
ALTER TABLE rental_charges ADD CONSTRAINT uq_rental_charges_organization_id UNIQUE (organization_id, id);
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_kind_check;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_kind_check
    CHECK (kind IN ('late_return', 'early_return', 'damage', 'fuel', 'mileage', 'equipment', 'toll', 'toll_fee', 'other'));
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_source_check;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_source_check CHECK (source IN ('policy', 'manual', 'toll'));
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_amount_check;
ALTER TABLE rental_charges
    ADD COLUMN toll_pass_id BIGINT NULL,
    ADD COLUMN reversal_of_id BIGINT NULL,
    ADD CONSTRAINT chk_rental_charges_amount CHECK (
        (reversal_of_id IS NULL AND amount >= 0) OR (reversal_of_id IS NOT NULL AND amount <= 0)
    ),
    ADD CONSTRAINT fk_rental_charges_toll_pass
        FOREIGN KEY (organization_id, toll_pass_id) REFERENCES toll_passes (organization_id, id)
        ON DELETE SET NULL (toll_pass_id),
    ADD CONSTRAINT fk_rental_charges_reversal_of
        FOREIGN KEY (organization_id, reversal_of_id) REFERENCES rental_charges (organization_id, id) ON DELETE CASCADE;
CREATE UNIQUE INDEX uq_rental_charges_reversal
    ON rental_charges (organization_id, reversal_of_id) WHERE reversal_of_id IS NOT NULL;
CREATE INDEX idx_rental_charges_organization_toll_pass
    ON rental_charges (organization_id, toll_pass_id) WHERE toll_pass_id IS NOT NULL;

ALTER TABLE toll_passes
    ADD CONSTRAINT fk_toll_passes_charge
        FOREIGN KEY (organization_id, charge_id) REFERENCES rental_charges (organization_id, id),
    ADD CONSTRAINT fk_toll_passes_fee_charge
        FOREIGN KEY (organization_id, fee_charge_id) REFERENCES rental_charges (organization_id, id);

-- Toll provider accounts (the provider is chosen in TEC-683; hgs_fake is
-- the development / test provider).
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_kind_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_kind_check
    CHECK (kind IN ('payment', 'sms', 'toll'));
ALTER TABLE integration_accounts DROP CONSTRAINT integration_accounts_provider_check;
ALTER TABLE integration_accounts ADD CONSTRAINT integration_accounts_provider_check
    CHECK (provider IN ('iyzico', 'netgsm', 'log', 'hgs_fake'));
