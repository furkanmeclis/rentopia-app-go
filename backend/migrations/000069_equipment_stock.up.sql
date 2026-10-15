-- Physical equipment stock (TEC-702, RT-A13): how many units of a physical
-- extra (child seat, GPS …) each branch owns, and the timed allocations that
-- consume them. docs/design/extras.md.
--
-- extra_branch_stock  one row per (extra, branch): the owned count. Writers
--                     of an extra at a branch serialise on this row
--                     (SELECT … FOR UPDATE) before checking the overlap,
--                     so two concurrent sales of the last unit cannot both
--                     pass the check (the counter-based pattern; compare the
--                     component lock of vehicle_allocations, 000063).
-- equipment_allocations
--     kind    hold | reservation | rental
--     status  active | released | expired (leaves "active" exactly once)
--     period  [starts_at, ends_at): the units are busy in that range. The
--             peak of the active rows overlapping a range (sweep line over
--             the starts / ends) may never exceed the branch stock.
--     A hold consumes stock until expires_at (lazily: a lapsed hold stops
--     counting even before the sweep marks it expired).
--     vehicle_allocation_id ties the equipment to an availability hold /
--     reservation (vehicle_allocations): the trigger below releases,
--     expires, confirms and extends the equipment together with it.
--
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE extra_branch_stock (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    extra_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 0 CHECK (quantity BETWEEN 0 AND 100000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_extra_branch_stock_organization_extra_branch UNIQUE (organization_id, extra_id, branch_id),
    CONSTRAINT fk_extra_branch_stock_extra
        FOREIGN KEY (organization_id, extra_id)
        REFERENCES extras (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_extra_branch_stock_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_extra_branch_stock_organization_branch
    ON extra_branch_stock (organization_id, branch_id);

CREATE TRIGGER trg_extra_branch_stock_updated_at
    BEFORE UPDATE ON extra_branch_stock
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE equipment_allocations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    extra_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('hold', 'reservation', 'rental')),
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'released', 'expired')),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    period TSTZRANGE GENERATED ALWAYS AS (tstzrange(starts_at, ends_at, '[)')) STORED,
    -- Holds only: the option lapses at expires_at.
    expires_at TIMESTAMPTZ NULL,
    -- Availability allocation (hold / reservation) the equipment belongs to.
    vehicle_allocation_id BIGINT NULL,
    -- Related record (TEC-706 reservation, M3 rental).
    ref_type VARCHAR(32) NULL,
    ref_uuid UUID NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    released_at TIMESTAMPTZ NULL,
    released_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    release_reason VARCHAR(48) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_equipment_allocations_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_equipment_allocations_extra
        FOREIGN KEY (organization_id, extra_id)
        REFERENCES extras (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_equipment_allocations_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_equipment_allocations_vehicle_allocation
        FOREIGN KEY (organization_id, vehicle_allocation_id)
        REFERENCES vehicle_allocations (organization_id, id) ON DELETE SET NULL (vehicle_allocation_id),
    CONSTRAINT chk_equipment_allocations_range CHECK (ends_at > starts_at),
    CONSTRAINT chk_equipment_allocations_hold_expiry CHECK (kind <> 'hold' OR expires_at IS NOT NULL),
    CONSTRAINT chk_equipment_allocations_released CHECK ((status = 'active') = (released_at IS NULL))
);

-- Overlap reads of one extra at one branch (the check under the stock lock).
CREATE INDEX idx_equipment_allocations_organization_stock
    ON equipment_allocations USING gist (organization_id, extra_id, branch_id, period)
    WHERE status = 'active';

CREATE INDEX idx_equipment_allocations_organization_created
    ON equipment_allocations (organization_id, created_at DESC);

CREATE INDEX idx_equipment_allocations_organization_vehicle_allocation
    ON equipment_allocations (organization_id, vehicle_allocation_id)
    WHERE vehicle_allocation_id IS NOT NULL;

-- Expiry sweep.
CREATE INDEX idx_equipment_allocations_organization_due
    ON equipment_allocations (organization_id, expires_at)
    WHERE status = 'active' AND kind = 'hold';

CREATE TRIGGER trg_equipment_allocations_updated_at
    BEFORE UPDATE ON equipment_allocations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The equipment of an availability allocation follows it:
--   * released / expired  -> the active equipment rows leave "active" with
--                            the same status (reason vehicle_allocation_*);
--   * hold -> reservation -> the equipment holds become reservations
--                            (expires_at cleared, ref copied);
--   * hold expiry moved   -> the equipment holds get the same expiry.
-- ReserveEquipment locks the vehicle allocation row FOR SHARE while it
-- links to it, so a concurrent release either waits for the new equipment
-- row (and releases it too) or is seen by the reserve (refused).
CREATE FUNCTION vehicle_allocations_cascade_equipment() RETURNS trigger AS $$
BEGIN
    IF OLD.status = 'active' AND NEW.status <> 'active' THEN
        UPDATE equipment_allocations
        SET status = NEW.status,
            released_at = COALESCE(NEW.released_at, NOW()),
            released_by = NEW.released_by,
            release_reason = 'vehicle_allocation_' || NEW.status
        WHERE organization_id = NEW.organization_id
          AND vehicle_allocation_id = NEW.id
          AND status = 'active';
        RETURN NEW;
    END IF;
    IF NEW.status = 'active' AND OLD.kind = 'hold' AND NEW.kind <> 'hold' THEN
        UPDATE equipment_allocations
        SET kind = NEW.kind, expires_at = NULL,
            ref_type = COALESCE(NEW.ref_type, ref_type), ref_uuid = COALESCE(NEW.ref_uuid, ref_uuid)
        WHERE organization_id = NEW.organization_id
          AND vehicle_allocation_id = NEW.id
          AND status = 'active'
          AND kind = 'hold';
    ELSIF NEW.status = 'active' AND NEW.kind = 'hold' AND NEW.expires_at IS DISTINCT FROM OLD.expires_at THEN
        UPDATE equipment_allocations
        SET expires_at = NEW.expires_at
        WHERE organization_id = NEW.organization_id
          AND vehicle_allocation_id = NEW.id
          AND status = 'active'
          AND kind = 'hold';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vehicle_allocations_cascade_equipment
    AFTER UPDATE OF status, kind, expires_at ON vehicle_allocations
    FOR EACH ROW EXECUTE FUNCTION vehicle_allocations_cascade_equipment();
