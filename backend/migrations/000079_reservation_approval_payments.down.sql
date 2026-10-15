-- Data-loss note: deletes every late_payment resolution task (open or
-- resolved) and drops reservations.approval_protocol (which approval rule
-- each reservation was created under).
DELETE FROM resolution_tasks WHERE task_type = 'late_payment';
ALTER TABLE resolution_tasks DROP CONSTRAINT IF EXISTS resolution_tasks_task_type_check;
ALTER TABLE resolution_tasks ADD CONSTRAINT resolution_tasks_task_type_check CHECK (task_type IN (
    'block_conflict', 'release_conflict', 'extension_conflict', 'replan_failed', 'late_return'
));

DROP INDEX IF EXISTS idx_reservations_organization_approval;
ALTER TABLE reservations DROP COLUMN IF EXISTS approval_protocol;
