-- Approval flows and late payments (TEC-708, docs/design/payments.md §5).
--
-- approval_protocol records which approval rule a reservation was created
-- under: auto (confirmed on payment / at once), staff (an employee accepts
-- within the review window) or channel (a sales channel that confirms on
-- its own side; the organization's online approval mode does not apply).
-- The rule comes from the channel's protocol in the settings
-- (sales_channels[].approval_protocol), falling back to the organization's
-- approval mode.
--
-- resolution_tasks gains late_payment: a successful payment that arrives
-- after the hold expired and capacity is gone is never confirmed silently;
-- a task (refund or re-book) is opened instead.

ALTER TABLE reservations
    ADD COLUMN approval_protocol VARCHAR(8) NOT NULL DEFAULT 'auto'
        CHECK (approval_protocol IN ('auto', 'staff', 'channel'));

UPDATE reservations
SET approval_protocol = 'staff'
WHERE policy_snapshot ->> 'approval_mode' = 'staff';

-- Staff approval queue (requests and reviews by SLA deadline).
CREATE INDEX idx_reservations_organization_approval
    ON reservations (organization_id, review_deadline)
    WHERE status IN ('request', 'awaiting_review');

ALTER TABLE resolution_tasks DROP CONSTRAINT IF EXISTS resolution_tasks_task_type_check;
ALTER TABLE resolution_tasks ADD CONSTRAINT resolution_tasks_task_type_check CHECK (task_type IN (
    'block_conflict', 'release_conflict', 'extension_conflict', 'replan_failed', 'late_return', 'late_payment'
));
