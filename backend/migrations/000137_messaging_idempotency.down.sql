-- Data-loss note: drops the idempotency keys of sms_messages and
-- notifications; reconciled SMS messages become "failed" and reconciled
-- dispatches "skipped" (their last_error keeps the reconcile note).

DROP INDEX IF EXISTS idx_notification_dispatches_organization_sending;

UPDATE notification_dispatches SET status = 'skipped' WHERE status = 'reconciled';
ALTER TABLE notification_dispatches DROP CONSTRAINT notification_dispatches_status_check;
ALTER TABLE notification_dispatches ADD CONSTRAINT notification_dispatches_status_check CHECK (
    status IN ('pending', 'sending', 'delivered', 'failed', 'skipped')
);

DROP INDEX IF EXISTS idx_notifications_open_updated;
DROP INDEX IF EXISTS uq_notifications_idempotency;
ALTER TABLE notifications DROP COLUMN IF EXISTS idempotency_key;

UPDATE sms_messages SET status = 'failed' WHERE status = 'reconciled';
ALTER TABLE sms_messages DROP CONSTRAINT sms_messages_status_check;
ALTER TABLE sms_messages ADD CONSTRAINT sms_messages_status_check CHECK (
    status IN ('queued', 'submitted', 'delivered', 'undelivered', 'failed', 'unknown', 'blocked')
);

DROP INDEX IF EXISTS uq_sms_messages_organization_idempotency;
ALTER TABLE sms_messages DROP COLUMN IF EXISTS idempotency_key;
