-- SMS send log and OTP challenges (TEC-734). Both tables are
-- organization-owned: every query filters by organization_id.

-- name: InsertSMSMessage :one
INSERT INTO sms_messages (
    uuid, organization_id, integration_account_id, class, purpose, provider, phone_masked, phone_hash, body_redacted,
    body_hash, payload_ciphertext, customer_id, status, error, ref_type, ref_uuid, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.narg(integration_account_id), sqlc.arg(class), sqlc.arg(purpose),
    sqlc.arg(provider), sqlc.arg(phone_masked), sqlc.arg(phone_hash), sqlc.arg(body_redacted), sqlc.arg(body_hash),
    sqlc.arg(payload_ciphertext), sqlc.narg(customer_id), sqlc.arg(status), sqlc.arg(error), sqlc.arg(ref_type), sqlc.narg(ref_uuid),
    sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: SetSMSMessageJob :exec
UPDATE sms_messages
SET integration_job_id = sqlc.arg(integration_job_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: UpdateSMSMessageStatus :exec
-- The runner's outcome for the message's send job (submit / delivery
-- report); clear_payload drops the sealed number and text once final.
UPDATE sms_messages
SET status = sqlc.arg(status),
    integration_job_id = COALESCE(sqlc.narg(integration_job_id), integration_job_id),
    provider_message_id = CASE WHEN sqlc.arg(provider_message_id)::text = '' THEN provider_message_id
        ELSE sqlc.arg(provider_message_id)::text END,
    dlr_status = CASE WHEN sqlc.arg(dlr_status)::text = '' THEN dlr_status ELSE sqlc.arg(dlr_status)::text END,
    dlr_at = COALESCE(sqlc.narg(dlr_at), dlr_at),
    error = sqlc.arg(error),
    payload_ciphertext = CASE WHEN sqlc.arg(clear_payload)::boolean THEN '' ELSE payload_ciphertext END
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid);

-- name: GetSMSMessageForSend :one
-- The sealed payload of a send job's message.
SELECT m.id, m.uuid, m.class, m.purpose, m.status, m.payload_ciphertext
FROM sms_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.uuid = sqlc.arg(uuid);

-- name: ClearSMSPayload :exec
UPDATE sms_messages
SET payload_ciphertext = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetSMSMessage :one
SELECT m.id, m.uuid, m.organization_id, m.class, m.purpose, m.provider, m.phone_masked, m.body_redacted, m.status,
    m.provider_message_id, m.dlr_status, m.dlr_at, m.error, m.ref_type, m.ref_uuid, m.created_at, m.updated_at,
    j.uuid AS job_uuid
FROM sms_messages m
LEFT JOIN integration_jobs j ON j.organization_id = m.organization_id AND j.id = m.integration_job_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.id = sqlc.arg(id);

-- name: CountSMSMessagesSince :one
-- Sends of the organization (optionally to one number) since a time.
SELECT COUNT(*)::bigint
FROM sms_messages m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(phone_hash)::text IS NULL OR m.phone_hash = sqlc.narg(phone_hash)::text)
  AND m.status <> 'blocked'
  AND m.created_at >= sqlc.arg(since);

-- name: CustomerHasSMSConsent :one
-- An open İYS SMS consent of the customer (informational sends).
SELECT EXISTS (
    SELECT 1 FROM customer_marketing_consents c
    WHERE c.organization_id = sqlc.arg(organization_id)
      AND c.customer_id = sqlc.arg(customer_id)
      AND c.channel = 'sms'
      AND c.withdrawn_at IS NULL
)::boolean AS has_consent;

-- name: GetCustomerIDByUUID :one
SELECT c.id FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: InsertOTPChallenge :one
INSERT INTO sms_otp_challenges (
    uuid, organization_id, purpose, subject_ref, phone_masked, phone_hash, code_hash, max_attempts, expires_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(purpose), sqlc.arg(subject_ref), sqlc.arg(phone_masked),
    sqlc.arg(phone_hash), sqlc.arg(code_hash), sqlc.arg(max_attempts), sqlc.arg(expires_at)
)
RETURNING id;

-- name: SetOTPChallengeMessage :exec
UPDATE sms_otp_challenges
SET sms_message_id = sqlc.arg(sms_message_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SupersedeOpenOTPChallenges :exec
UPDATE sms_otp_challenges
SET status = 'superseded'
WHERE organization_id = sqlc.arg(organization_id)
  AND purpose = sqlc.arg(purpose)
  AND subject_ref = sqlc.arg(subject_ref)
  AND status = 'pending';

-- name: LatestOTPChallenge :one
-- The newest challenge of a purpose and subject (resend cooldown).
SELECT o.id, o.uuid, o.status, o.created_at
FROM sms_otp_challenges o
WHERE o.organization_id = sqlc.arg(organization_id)
  AND o.purpose = sqlc.arg(purpose)
  AND o.subject_ref = sqlc.arg(subject_ref)
ORDER BY o.created_at DESC, o.id DESC
LIMIT 1;

-- name: CountOTPChallengesSince :one
SELECT COUNT(*)::bigint
FROM sms_otp_challenges o
WHERE o.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(phone_hash)::text IS NULL OR o.phone_hash = sqlc.narg(phone_hash)::text)
  AND o.created_at >= sqlc.arg(since);

-- name: GetOpenOTPChallengeForUpdate :one
SELECT o.id, o.uuid, o.purpose, o.subject_ref, o.phone_masked, o.phone_hash, o.code_hash, o.status, o.attempts,
    o.max_attempts, o.expires_at, o.sms_message_id
FROM sms_otp_challenges o
WHERE o.organization_id = sqlc.arg(organization_id)
  AND o.purpose = sqlc.arg(purpose)
  AND o.subject_ref = sqlc.arg(subject_ref)
  AND o.status = 'pending'
FOR UPDATE;

-- name: RecordOTPAttempt :exec
UPDATE sms_otp_challenges
SET attempts = attempts + 1,
    status = sqlc.arg(status)::text,
    verified_at = CASE WHEN sqlc.arg(status)::text = 'verified' THEN NOW() ELSE verified_at END
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ExpireOTPChallenge :exec
UPDATE sms_otp_challenges
SET status = 'expired'
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';
