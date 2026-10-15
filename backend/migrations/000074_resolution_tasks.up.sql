-- Resolution tasks (TEC-698, spec §5, §8.3, RT-A31). When an operational
-- change (a block over an allocation, a release that strands a later
-- one-way sale, a refused extension, M3: a late return) leaves a
-- reservation without a valid plate plan and the engine cannot re-plan it,
-- a manager task is opened instead of cancelling the reservation. The task
-- names the reservation, the reason and the source record; it is resolved
-- by a person (after re-planning, a plate change or a cancellation).
--
-- At most one open task per reservation, type and source (the hook may see
-- the same block twice: create + patch). Branch scope follows the
-- reservation (pick-up OR return branch). Every index starts with
-- organization_id.

CREATE TABLE resolution_tasks (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    task_type VARCHAR(24) NOT NULL CHECK (task_type IN (
        'block_conflict', 'release_conflict', 'extension_conflict', 'replan_failed', 'late_return'
    )),
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    reason TEXT NOT NULL DEFAULT '',
    -- What caused it (block / transfer / reservation / allocation uuid).
    source_type VARCHAR(24) NOT NULL DEFAULT '',
    source_uuid UUID NULL,
    -- Suggestions and facts at creation ({vehicle, plate, starts_at, …}).
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    assignee_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolution_note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_resolution_tasks_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_resolution_tasks_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_resolution_tasks_resolved CHECK ((status = 'resolved') = (resolved_at IS NOT NULL)),
    CONSTRAINT fk_resolution_tasks_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_resolution_tasks_open_source
    ON resolution_tasks (organization_id, reservation_id, task_type, COALESCE(source_uuid, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status = 'open';
CREATE INDEX idx_resolution_tasks_organization_created ON resolution_tasks (organization_id, created_at DESC);
CREATE INDEX idx_resolution_tasks_organization_reservation ON resolution_tasks (organization_id, reservation_id);
CREATE INDEX idx_resolution_tasks_organization_status ON resolution_tasks (organization_id, status);

CREATE TRIGGER trg_resolution_tasks_updated_at
    BEFORE UPDATE ON resolution_tasks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
