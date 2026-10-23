-- Outbound messaging idempotency after a restore (TEC-962,
-- docs/operations/backup-restore.md RT-A32 (c)). Providers (Netgsm, SMTP)
-- take no key that lets them drop a repeat, so every outbound row carries
-- a deterministic key of its own and a unique index refuses a second row
-- for the same event, channel and recipient:
--
-- 1. sms_messages.idempotency_key: the notification engine's SMS for one
--    delivery attempt is "notify:<notification_deliveries.uuid>" (the
--    delivery is unique per dispatch and channel, the dispatch per event
--    and recipient). A replayed send returns the existing message instead
--    of queueing a second one. NULL for sends without a caller key (OTP,
--    test sends).
-- 2. notifications.idempotency_key: event-driven notifications
--    ("event:<event id>|<kind>|user:<id>|<channel>"); a replayed outbox
--    event inserts nothing.
-- 3. "reconciled": a message that was waiting in the restored data but was
--    sent after the backup, marked by the operator (cmd/messaging-reconcile)
--    so it is never sent again. Notifications use their existing
--    "cancelled" status for this.
-- 4. Sweep indexes: dispatches stuck in "sending" (reclaim) and queued /
--    processing notifications (re-delivery sweep).

ALTER TABLE sms_messages
    ADD COLUMN idempotency_key VARCHAR(200) NULL;

-- Existing engine messages get their key; the earliest message of a
-- delivery wins should a delivery ever have two.
UPDATE sms_messages m
SET idempotency_key = 'notify:' || m.ref_uuid::text
WHERE m.id IN (
    SELECT DISTINCT ON (s.organization_id, s.ref_uuid) s.id
    FROM sms_messages s
    WHERE s.ref_type = 'notification_delivery'
      AND s.ref_uuid IS NOT NULL
    ORDER BY s.organization_id, s.ref_uuid, s.id
);

CREATE UNIQUE INDEX uq_sms_messages_organization_idempotency
    ON sms_messages (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE sms_messages DROP CONSTRAINT sms_messages_status_check;
ALTER TABLE sms_messages ADD CONSTRAINT sms_messages_status_check CHECK (
    status IN ('queued', 'submitted', 'delivered', 'undelivered', 'failed', 'unknown', 'blocked', 'reconciled')
);

ALTER TABLE notifications
    ADD COLUMN idempotency_key VARCHAR(255) NULL;

CREATE UNIQUE INDEX uq_notifications_idempotency
    ON notifications (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX idx_notifications_open_updated
    ON notifications (updated_at)
    WHERE status IN ('queued', 'processing');

ALTER TABLE notification_dispatches DROP CONSTRAINT notification_dispatches_status_check;
ALTER TABLE notification_dispatches ADD CONSTRAINT notification_dispatches_status_check CHECK (
    status IN ('pending', 'sending', 'delivered', 'failed', 'skipped', 'reconciled')
);

CREATE INDEX idx_notification_dispatches_organization_sending
    ON notification_dispatches (organization_id, updated_at)
    WHERE status = 'sending';
