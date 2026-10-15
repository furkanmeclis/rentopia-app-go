-- Reverts 000300_field_tasks.
-- Data-loss note: drops the field task timeline (every operation_task_events
-- row), the field details of operation tasks (reservation link, origin,
-- place, address and coordinates, contact, flight number, meeting note,
-- planned duration, transfer destination and fleet transfer link, issue
-- notes), deletes transfer / other tasks and the reservation-only
-- delivery / collection tasks (no rental yet), drops the delivery settings
-- of service points, the delivery details of quotes and reservations, the
-- manual delivery fee override of quotes, the field task permissions with
-- their role grants and the assignment notification templates. Statuses
-- map back: planned / assigned -> open, en_route / issue_reported ->
-- in_progress, completed -> done.

DELETE FROM notification_templates WHERE code = 'field_tasks.assigned';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('tenant.field_tasks.read', 'tenant.field_tasks.assign', 'tenant.field_tasks.execute')
);
DELETE FROM permissions
WHERE slug IN ('tenant.field_tasks.read', 'tenant.field_tasks.assign', 'tenant.field_tasks.execute');

ALTER TABLE reservations
    DROP COLUMN IF EXISTS pickup_delivery,
    DROP COLUMN IF EXISTS return_delivery;

ALTER TABLE quotes
    DROP COLUMN IF EXISTS pickup_delivery,
    DROP COLUMN IF EXISTS return_delivery,
    DROP COLUMN IF EXISTS delivery_override;

ALTER TABLE service_points
    DROP CONSTRAINT IF EXISTS fk_service_points_delivery_extra,
    DROP COLUMN IF EXISTS delivery_enabled,
    DROP COLUMN IF EXISTS delivery_extra_id,
    DROP COLUMN IF EXISTS service_districts,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS service_radius_km,
    DROP COLUMN IF EXISTS min_notice_minutes,
    DROP COLUMN IF EXISTS delivery_conditions;

DROP TRIGGER IF EXISTS trg_operation_tasks_timeline ON operation_tasks;
DROP FUNCTION IF EXISTS operation_task_timeline();
DROP TABLE IF EXISTS operation_task_events;

DELETE FROM operation_tasks WHERE task_type IN ('transfer', 'other') OR rental_id IS NULL AND reservation_id IS NOT NULL;

DROP INDEX IF EXISTS uq_operation_tasks_live;
DROP INDEX IF EXISTS uq_operation_tasks_reservation_live;
DROP INDEX IF EXISTS idx_operation_tasks_organization_reservation;
DROP INDEX IF EXISTS idx_operation_tasks_organization_assignee_due;
DROP INDEX IF EXISTS idx_operation_tasks_organization_point;

ALTER TABLE operation_tasks
    DROP CONSTRAINT IF EXISTS chk_operation_tasks_address,
    DROP CONSTRAINT IF EXISTS chk_operation_tasks_transfer,
    DROP CONSTRAINT IF EXISTS fk_operation_tasks_reservation,
    DROP CONSTRAINT IF EXISTS fk_operation_tasks_point,
    DROP CONSTRAINT IF EXISTS fk_operation_tasks_to_branch,
    DROP CONSTRAINT IF EXISTS fk_operation_tasks_transfer,
    DROP COLUMN IF EXISTS reservation_id,
    DROP COLUMN IF EXISTS origin,
    DROP COLUMN IF EXISTS location_kind,
    DROP COLUMN IF EXISTS point_id,
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS district,
    DROP COLUMN IF EXISTS city,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS contact_name,
    DROP COLUMN IF EXISTS contact_phone,
    DROP COLUMN IF EXISTS flight_number,
    DROP COLUMN IF EXISTS meeting_note,
    DROP COLUMN IF EXISTS planned_minutes,
    DROP COLUMN IF EXISTS to_branch_id,
    DROP COLUMN IF EXISTS transfer_id,
    DROP COLUMN IF EXISTS assigned_at,
    DROP COLUMN IF EXISTS issue_note,
    DROP COLUMN IF EXISTS issue_reported_at,
    DROP COLUMN IF EXISTS updated_by;

ALTER TABLE vehicle_transfers DROP CONSTRAINT IF EXISTS uq_vehicle_transfers_organization_id;

ALTER TABLE operation_tasks DROP CONSTRAINT operation_tasks_task_type_check;
ALTER TABLE operation_tasks ADD CONSTRAINT operation_tasks_task_type_check
    CHECK (task_type IN ('preparation', 'handover', 'return', 'cleaning', 'maintenance', 'deposit'));

ALTER TABLE operation_tasks DROP CONSTRAINT operation_tasks_status_check;
ALTER TABLE operation_tasks DROP CONSTRAINT chk_operation_tasks_completed;

UPDATE operation_tasks SET status = CASE
    WHEN status IN ('planned', 'assigned') THEN 'open'
    WHEN status IN ('en_route', 'issue_reported') THEN 'in_progress'
    WHEN status = 'completed' THEN 'done'
    ELSE status
END;

ALTER TABLE operation_tasks ALTER COLUMN status SET DEFAULT 'open';
ALTER TABLE operation_tasks ADD CONSTRAINT operation_tasks_status_check
    CHECK (status IN ('open', 'in_progress', 'done', 'cancelled'));
ALTER TABLE operation_tasks ADD CONSTRAINT chk_operation_tasks_completed
    CHECK ((status IN ('done', 'cancelled')) = (completed_at IS NOT NULL));

CREATE UNIQUE INDEX uq_operation_tasks_live
    ON operation_tasks (organization_id, rental_id, task_type, COALESCE(vehicle_id, 0))
    WHERE rental_id IS NOT NULL AND status IN ('open', 'in_progress');
