-- Operation tasks (TEC-709; consumed by the mobile handover / return flows
-- TEC-712 / TEC-713). Small, typed work items of a branch: prepare a
-- vehicle, hand it over, take it back, clean it. Rentals open them (plate
-- fixed -> preparation + handover; handover -> return; return -> cleaning)
-- and close them when the matching fact is recorded; staff can assign,
-- start, complete or cancel them.
--
-- status: open -> in_progress -> done | cancelled (open -> done allowed).
-- Branch scope: branch_id. Every index starts with organization_id.

CREATE TABLE operation_tasks (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    task_type VARCHAR(16) NOT NULL CHECK (task_type IN ('preparation', 'handover', 'return', 'cleaning')),
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    branch_id BIGINT NOT NULL,
    rental_id BIGINT NULL,
    vehicle_id BIGINT NULL,
    due_at TIMESTAMPTZ NOT NULL,
    assignee_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    completed_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    completion_note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_operation_tasks_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_operation_tasks_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_operation_tasks_completed CHECK ((status IN ('done', 'cancelled')) = (completed_at IS NOT NULL)),
    CONSTRAINT fk_operation_tasks_branch
        FOREIGN KEY (organization_id, branch_id) REFERENCES branches (organization_id, id),
    CONSTRAINT fk_operation_tasks_rental
        FOREIGN KEY (organization_id, rental_id) REFERENCES rentals (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_operation_tasks_vehicle
        FOREIGN KEY (organization_id, vehicle_id) REFERENCES vehicles (organization_id, id) ON DELETE SET NULL (vehicle_id)
);

-- At most one live task per rental, type and vehicle (a swap opens a
-- cleaning task for the old vehicle and keeps the new one separate).
CREATE UNIQUE INDEX uq_operation_tasks_live
    ON operation_tasks (organization_id, rental_id, task_type, COALESCE(vehicle_id, 0))
    WHERE rental_id IS NOT NULL AND status IN ('open', 'in_progress');
CREATE INDEX idx_operation_tasks_organization_due ON operation_tasks (organization_id, due_at);
CREATE INDEX idx_operation_tasks_organization_branch ON operation_tasks (organization_id, branch_id);
CREATE INDEX idx_operation_tasks_organization_status ON operation_tasks (organization_id, status);
CREATE INDEX idx_operation_tasks_organization_rental
    ON operation_tasks (organization_id, rental_id) WHERE rental_id IS NOT NULL;
CREATE INDEX idx_operation_tasks_organization_assignee
    ON operation_tasks (organization_id, assignee_id) WHERE assignee_id IS NOT NULL;

CREATE TRIGGER trg_operation_tasks_updated_at
    BEFORE UPDATE ON operation_tasks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
