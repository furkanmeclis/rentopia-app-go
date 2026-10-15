-- Field tasks (TEC-724, RT-A29): operation_tasks become the field task
-- model instead of a parallel field_tasks table. A delivery to an address
-- or an airport is the handover task of the rental at another place; the
-- collection is its return task. Keeping one table means the rental's own
-- handover / return closes the same row the field staff work on, and the
-- overview, the rental page and the mobile inspection forms keep reading
-- one source (docs/design/field-tasks.md §1).
--
-- 1. operation_tasks: the field statuses (planned, assigned, en_route,
--    completed, issue_reported, cancelled; open -> planned / assigned,
--    in_progress -> en_route, done -> completed), the reservation link
--    (delivery tasks exist before the rental), the place (branch, service
--    point or free address with optional coordinates), the contact, the
--    flight number and meeting note, the planned duration, the transfer
--    destination and its fleet transfer, the issue note, and the new types
--    transfer and other.
-- 2. operation_task_events: the timeline. A trigger writes it on insert and
--    on every status, assignee, due time or place change, whichever code
--    path made the change (rentals, inspections, field tasks).
-- 3. service_points: delivery settings (delivery on / off, the delivery
--    extra whose price is the fee, the served districts, a centre and
--    radius, the minimum notice and the conditions text).
-- 4. quotes / reservations: the delivery details of the pick-up and the
--    return (address, coordinates, contact, flight, meeting note) and the
--    manual delivery fee override of a quote.
-- 5. Permissions tenant.field_tasks.read / assign / execute.

-- 1. operation_tasks ---------------------------------------------------

ALTER TABLE operation_tasks DROP CONSTRAINT operation_tasks_status_check;
ALTER TABLE operation_tasks DROP CONSTRAINT chk_operation_tasks_completed;
DROP INDEX uq_operation_tasks_live;

UPDATE operation_tasks SET status = CASE
    WHEN status = 'open' AND assignee_id IS NOT NULL THEN 'assigned'
    WHEN status = 'open' THEN 'planned'
    WHEN status = 'in_progress' THEN 'en_route'
    WHEN status = 'done' THEN 'completed'
    ELSE status
END;

ALTER TABLE operation_tasks ALTER COLUMN status SET DEFAULT 'planned';
ALTER TABLE operation_tasks ADD CONSTRAINT operation_tasks_status_check
    CHECK (status IN ('planned', 'assigned', 'en_route', 'completed', 'issue_reported', 'cancelled'));
ALTER TABLE operation_tasks ADD CONSTRAINT chk_operation_tasks_completed
    CHECK ((status IN ('completed', 'cancelled')) = (completed_at IS NOT NULL));

ALTER TABLE operation_tasks DROP CONSTRAINT operation_tasks_task_type_check;
ALTER TABLE operation_tasks ADD CONSTRAINT operation_tasks_task_type_check
    CHECK (task_type IN ('preparation', 'handover', 'return', 'cleaning', 'maintenance', 'deposit', 'transfer', 'other'));

-- A fleet transfer is referenced by (organization_id, id).
ALTER TABLE vehicle_transfers
    ADD CONSTRAINT uq_vehicle_transfers_organization_id UNIQUE (organization_id, id);

ALTER TABLE operation_tasks
    ADD COLUMN reservation_id BIGINT NULL,
    -- Who opened it: the rental flow, the reservation (delivery /
    -- collection before the rental exists), staff, an inspection.
    ADD COLUMN origin VARCHAR(16) NOT NULL DEFAULT 'rental'
        CHECK (origin IN ('rental', 'reservation', 'manual', 'inspection')),
    -- branch: at the task's branch; point: a service point; address: a
    -- free address (point_id is then the zone that served it, if any).
    ADD COLUMN location_kind VARCHAR(16) NOT NULL DEFAULT 'branch'
        CHECK (location_kind IN ('branch', 'point', 'address')),
    ADD COLUMN point_id BIGINT NULL,
    ADD COLUMN address TEXT NOT NULL DEFAULT '',
    ADD COLUMN district VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN city VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN latitude DOUBLE PRECISION NULL CHECK (latitude BETWEEN -90 AND 90),
    ADD COLUMN longitude DOUBLE PRECISION NULL CHECK (longitude BETWEEN -180 AND 180),
    ADD COLUMN contact_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN contact_phone VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN flight_number VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN meeting_note TEXT NOT NULL DEFAULT '',
    -- Planned duration from due_at (the day timeline draws due_at ..
    -- due_at + planned_minutes). Actual times: started_at, completed_at.
    ADD COLUMN planned_minutes INTEGER NOT NULL DEFAULT 30 CHECK (planned_minutes BETWEEN 5 AND 1440),
    -- Transfer tasks: the destination and the fleet transfer that moves
    -- the vehicle's current branch.
    ADD COLUMN to_branch_id BIGINT NULL,
    ADD COLUMN transfer_id BIGINT NULL,
    ADD COLUMN assigned_at TIMESTAMPTZ NULL,
    ADD COLUMN issue_note TEXT NOT NULL DEFAULT '',
    ADD COLUMN issue_reported_at TIMESTAMPTZ NULL,
    -- The member behind the last change (the timeline trigger reads it).
    ADD COLUMN updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT fk_operation_tasks_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_operation_tasks_point
        FOREIGN KEY (organization_id, point_id) REFERENCES service_points (organization_id, id) ON DELETE SET NULL (point_id),
    ADD CONSTRAINT fk_operation_tasks_to_branch
        FOREIGN KEY (organization_id, to_branch_id) REFERENCES branches (organization_id, id),
    ADD CONSTRAINT fk_operation_tasks_transfer
        FOREIGN KEY (organization_id, transfer_id) REFERENCES vehicle_transfers (organization_id, id) ON DELETE SET NULL (transfer_id),
    ADD CONSTRAINT chk_operation_tasks_address
        CHECK (location_kind <> 'address' OR address <> ''),
    ADD CONSTRAINT chk_operation_tasks_transfer
        CHECK (task_type <> 'transfer' OR to_branch_id IS NOT NULL);

-- Rental tasks know their reservation.
UPDATE operation_tasks t SET reservation_id = r.reservation_id
FROM rentals r
WHERE r.organization_id = t.organization_id AND r.id = t.rental_id AND t.reservation_id IS NULL;

-- At most one live task per rental, type and vehicle (unchanged rule, new
-- live statuses), and one live delivery / collection task per reservation
-- and type before the rental exists (the rental adopts it).
CREATE UNIQUE INDEX uq_operation_tasks_live
    ON operation_tasks (organization_id, rental_id, task_type, COALESCE(vehicle_id, 0))
    WHERE rental_id IS NOT NULL AND status IN ('planned', 'assigned', 'en_route', 'issue_reported');
CREATE UNIQUE INDEX uq_operation_tasks_reservation_live
    ON operation_tasks (organization_id, reservation_id, task_type)
    WHERE reservation_id IS NOT NULL AND rental_id IS NULL
      AND status IN ('planned', 'assigned', 'en_route', 'issue_reported');
CREATE INDEX idx_operation_tasks_organization_reservation
    ON operation_tasks (organization_id, reservation_id) WHERE reservation_id IS NOT NULL;
-- "My tasks": the assignee's live tasks by due time.
CREATE INDEX idx_operation_tasks_organization_assignee_due
    ON operation_tasks (organization_id, assignee_id, due_at) WHERE assignee_id IS NOT NULL;
CREATE INDEX idx_operation_tasks_organization_point
    ON operation_tasks (organization_id, point_id) WHERE point_id IS NOT NULL;

-- 2. timeline ------------------------------------------------------------

CREATE TABLE operation_task_events (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    task_id BIGINT NOT NULL,
    -- created, status_changed, assignee_changed, rescheduled, relocated
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(16) NULL,
    to_status VARCHAR(16) NULL,
    note TEXT NOT NULL DEFAULT '',
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_operation_task_events_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_operation_task_events_task
        FOREIGN KEY (organization_id, task_id) REFERENCES operation_tasks (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_operation_task_events_organization_task
    ON operation_task_events (organization_id, task_id, created_at, id);

CREATE FUNCTION operation_task_timeline() RETURNS trigger AS $$
DECLARE
    actor BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO operation_task_events (organization_id, task_id, action, to_status, data, actor_id)
        VALUES (NEW.organization_id, NEW.id, 'created', NEW.status,
                jsonb_build_object('task_type', NEW.task_type, 'due_at', NEW.due_at, 'origin', NEW.origin),
                COALESCE(NEW.updated_by, NEW.created_by));
        RETURN NEW;
    END IF;
    actor := NEW.updated_by;
    IF NEW.status IS DISTINCT FROM OLD.status AND NEW.status IN ('completed', 'cancelled') THEN
        actor := COALESCE(NEW.completed_by, NEW.updated_by);
    END IF;
    IF NEW.assignee_id IS DISTINCT FROM OLD.assignee_id THEN
        INSERT INTO operation_task_events (organization_id, task_id, action, data, actor_id)
        VALUES (NEW.organization_id, NEW.id, 'assignee_changed', jsonb_build_object(
            'from_assignee_uuid', (SELECT u.uuid FROM users u WHERE u.id = OLD.assignee_id),
            'to_assignee_uuid', (SELECT u.uuid FROM users u WHERE u.id = NEW.assignee_id)), actor);
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO operation_task_events (organization_id, task_id, action, from_status, to_status, note, actor_id)
        VALUES (NEW.organization_id, NEW.id, 'status_changed', OLD.status, NEW.status,
                CASE
                    WHEN NEW.status = 'issue_reported' THEN NEW.issue_note
                    WHEN NEW.status IN ('completed', 'cancelled') THEN NEW.completion_note
                    ELSE ''
                END, actor);
    END IF;
    IF NEW.due_at IS DISTINCT FROM OLD.due_at THEN
        INSERT INTO operation_task_events (organization_id, task_id, action, data, actor_id)
        VALUES (NEW.organization_id, NEW.id, 'rescheduled',
                jsonb_build_object('from_due_at', OLD.due_at, 'to_due_at', NEW.due_at), actor);
    END IF;
    IF NEW.location_kind IS DISTINCT FROM OLD.location_kind OR NEW.point_id IS DISTINCT FROM OLD.point_id
       OR NEW.address IS DISTINCT FROM OLD.address OR NEW.branch_id IS DISTINCT FROM OLD.branch_id THEN
        INSERT INTO operation_task_events (organization_id, task_id, action, data, actor_id)
        VALUES (NEW.organization_id, NEW.id, 'relocated',
                jsonb_build_object('location_kind', NEW.location_kind, 'address', NEW.address), actor);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_operation_tasks_timeline
    AFTER INSERT OR UPDATE ON operation_tasks
    FOR EACH ROW EXECUTE FUNCTION operation_task_timeline();

-- 3. service points: delivery settings -----------------------------------

ALTER TABLE service_points
    -- Staff bring / collect the vehicle here (airport meeting, hotel,
    -- address in a zone) instead of the customer coming to a desk.
    ADD COLUMN delivery_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    -- The delivery extra whose price (product row, else default row) is
    -- the fee; NULL = extra_fee.
    ADD COLUMN delivery_extra_id BIGINT NULL,
    -- Districts served by an address zone (case-insensitive); empty = any
    -- district (the radius may still apply).
    ADD COLUMN service_districts TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN latitude DOUBLE PRECISION NULL CHECK (latitude BETWEEN -90 AND 90),
    ADD COLUMN longitude DOUBLE PRECISION NULL CHECK (longitude BETWEEN -180 AND 180),
    ADD COLUMN service_radius_km NUMERIC(7, 2) NULL CHECK (service_radius_km > 0),
    ADD COLUMN min_notice_minutes INTEGER NOT NULL DEFAULT 0 CHECK (min_notice_minutes BETWEEN 0 AND 10080),
    ADD COLUMN delivery_conditions TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT fk_service_points_delivery_extra
        FOREIGN KEY (organization_id, delivery_extra_id)
        REFERENCES extras (organization_id, id) ON DELETE SET NULL (delivery_extra_id);

UPDATE service_points SET delivery_enabled = TRUE WHERE type IN ('airport', 'address_zone', 'hotel');

-- 4. delivery details of quotes and reservations --------------------------
-- {address, district, city, latitude, longitude, contact_name,
--  contact_phone, flight_number, meeting_note}

ALTER TABLE quotes
    ADD COLUMN pickup_delivery JSONB NULL,
    ADD COLUMN return_delivery JSONB NULL,
    -- A staff-entered delivery fee (tenant.price.override) replacing the
    -- computed one; NULL = computed from the service points.
    ADD COLUMN delivery_override NUMERIC(14, 2) NULL CHECK (delivery_override >= 0);

ALTER TABLE reservations
    ADD COLUMN pickup_delivery JSONB NULL,
    ADD COLUMN return_delivery JSONB NULL;

-- 5. permissions ----------------------------------------------------------
-- read: the assignment board and every task in the branch scope.
-- assign: create, reschedule, assign (bulk), cancel tasks.
-- execute: "my tasks" (assigned to the caller): start, report an issue,
--          complete.

INSERT INTO permissions (name, slug) VALUES
    ('Read field tasks', 'tenant.field_tasks.read'),
    ('Plan and assign field tasks', 'tenant.field_tasks.assign'),
    ('Work assigned field tasks', 'tenant.field_tasks.execute')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.field_tasks.read', 'tenant.field_tasks.assign', 'tenant.field_tasks.execute')
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi', 'sube_yoneticisi')
  AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Delivery / return staff are executors: their own tasks only.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.field_tasks.execute'
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Sales desk: see the board (delivery planning questions).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.field_tasks.read'
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- In-app notification of a new assignment (web push mirrors in-app rows).
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('field_tasks.assigned', 'inapp', 'tr', 'Yeni saha görevi', '{{reference}} · {{due}} · {{place}}', TRUE),
    ('field_tasks.assigned', 'inapp', 'en', 'New field task', '{{reference}} · {{due}} · {{place}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
