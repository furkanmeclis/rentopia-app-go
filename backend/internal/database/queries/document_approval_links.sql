-- Remote approval links of documents (TEC-711, docs/design/contracts.md §7).

-- name: ResolveDocumentApprovalLinkToken :one
-- Public approval page: the token hash names the link and its organization.
SELECT l.organization_id, l.id
FROM document_approval_links l
WHERE l.token_hash = sqlc.arg(token_hash);

-- name: GetDocumentApprovalLinkPublic :one
SELECT l.id, l.uuid, l.document_id, l.document_version_id, l.content_hash, l.signer_role, l.signer_name, l.customer_id,
    l.driver_id, l.phone_masked, l.phone_hash, l.phone_ciphertext, l.status, l.expires_at, l.codes_sent, l.failed_attempts,
    l.approved_at, l.created_at,
    d.uuid AS document_uuid, d.kind, d.status AS document_status, d.current_version_id, d.reservation_id, d.rental_id,
    v.uuid AS version_uuid, v.version AS version_number, v.status AS version_status, v.html,
    res.reference AS reservation_reference, o.name AS organization_name
FROM document_approval_links l
JOIN documents d ON d.organization_id = l.organization_id AND d.id = l.document_id
JOIN document_versions v ON v.organization_id = l.organization_id AND v.id = l.document_version_id
JOIN reservations res ON res.organization_id = d.organization_id AND res.id = d.reservation_id
JOIN organizations o ON o.id = l.organization_id
WHERE l.organization_id = sqlc.arg(organization_id)
  AND d.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND l.id = sqlc.arg(id);

-- name: InsertDocumentApprovalLink :one
INSERT INTO document_approval_links (
    uuid, organization_id, document_id, document_version_id, content_hash, token_hash, token_ciphertext, signer_role,
    signer_name, customer_id, driver_id, phone_masked, phone_hash, phone_ciphertext, expires_at, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(document_id), sqlc.arg(document_version_id), sqlc.arg(content_hash),
    sqlc.arg(token_hash), sqlc.arg(token_ciphertext), sqlc.arg(signer_role), sqlc.arg(signer_name), sqlc.narg(customer_id),
    sqlc.narg(driver_id), sqlc.arg(phone_masked), sqlc.arg(phone_hash), sqlc.arg(phone_ciphertext), sqlc.arg(expires_at),
    sqlc.narg(created_by)
)
RETURNING id;

-- name: ExpireDocumentApprovalLinks :exec
UPDATE document_approval_links
SET status = 'expired'
WHERE organization_id = sqlc.arg(organization_id)
  AND document_id = sqlc.arg(document_id)
  AND status = 'pending'
  AND expires_at <= sqlc.arg(now)::timestamptz;

-- name: CloseOpenDocumentApprovalLinks :many
-- Revokes (status revoked) or supersedes (status superseded) the open link
-- of a document; a link of except_version_id stays open.
UPDATE document_approval_links
SET status = sqlc.arg(status), revoked_at = sqlc.arg(closed_at), revoked_by = sqlc.narg(revoked_by),
    revoked_reason = sqlc.arg(reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND document_id = sqlc.arg(document_id)
  AND status = 'pending'
  AND (sqlc.narg(except_version_id)::bigint IS NULL OR document_version_id <> sqlc.narg(except_version_id)::bigint)
RETURNING uuid;

-- name: RevokeDocumentApprovalLink :execrows
UPDATE document_approval_links
SET status = 'revoked', revoked_at = sqlc.arg(revoked_at), revoked_by = sqlc.narg(revoked_by),
    revoked_reason = sqlc.arg(reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: MarkDocumentApprovalLinkExpired :exec
UPDATE document_approval_links
SET status = 'expired'
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: ListDocumentApprovalLinks :many
SELECT l.id, l.uuid, l.content_hash, l.token_ciphertext, l.signer_role, l.signer_name, l.phone_masked, l.status,
    l.expires_at, l.codes_sent, l.failed_attempts, l.sms_sent_count, l.last_sms_at, l.approved_at, l.revoked_at,
    l.revoked_reason, l.created_at,
    v.uuid AS version_uuid, v.version AS version_number,
    a.uuid AS approval_uuid,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM document_approval_links l
JOIN document_versions v ON v.organization_id = l.organization_id AND v.id = l.document_version_id
LEFT JOIN document_approvals a ON a.organization_id = l.organization_id AND a.link_id = l.id
LEFT JOIN users cu ON cu.id = l.created_by
WHERE l.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND (a.organization_id IS NULL OR a.organization_id = sqlc.arg(organization_id))
  AND l.document_id = sqlc.arg(document_id)
ORDER BY l.created_at DESC, l.id DESC;

-- name: GetDocumentApprovalLink :one
SELECT l.id, l.uuid, l.document_version_id, l.content_hash, l.token_ciphertext, l.signer_name, l.phone_masked,
    l.phone_ciphertext, l.status, l.expires_at
FROM document_approval_links l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.document_id = sqlc.arg(document_id)
  AND l.uuid = sqlc.arg(uuid);

-- name: CountDocumentApprovalLinkCode :execrows
-- Counts a code request; refused (0 rows) at the per-link limit.
UPDATE document_approval_links
SET codes_sent = codes_sent + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending'
  AND codes_sent < sqlc.arg(max_codes)::int;

-- name: RecordDocumentApprovalLinkFailure :exec
UPDATE document_approval_links
SET failed_attempts = failed_attempts + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: MarkDocumentApprovalLinkSMS :exec
UPDATE document_approval_links
SET sms_sent_count = sms_sent_count + 1, last_sms_at = sqlc.arg(sent_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: LockDocumentApprovalLink :one
SELECT l.id, l.status, l.document_version_id, l.content_hash, l.expires_at
FROM document_approval_links l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.id = sqlc.arg(id)
FOR UPDATE;

-- name: LockDocument :one
SELECT d.id, d.status, d.current_version_id
FROM documents d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id)
FOR UPDATE;

-- name: ApproveDocumentApprovalLink :execrows
UPDATE document_approval_links
SET status = 'approved', approved_at = sqlc.arg(approved_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: GetOTPChallengeBinding :one
-- The number and time of a verified OTP challenge (a remote approval binds
-- the code to the link's number).
SELECT c.phone_hash, c.created_at
FROM sms_otp_challenges c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: InsertRemoteDocumentApproval :one
INSERT INTO document_approvals (
    organization_id, document_version_id, content_hash, party, method, approver_name, customer_id, driver_id, note,
    approved_at, link_id, otp_challenge_uuid, verified_phone_masked, verified_phone_hash, ip_address, user_agent
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(document_version_id), sqlc.arg(content_hash), 'customer', 'remote_sms',
    sqlc.arg(approver_name), sqlc.narg(customer_id), sqlc.narg(driver_id), '', sqlc.arg(approved_at), sqlc.arg(link_id),
    sqlc.arg(otp_challenge_uuid), sqlc.arg(verified_phone_masked), sqlc.arg(verified_phone_hash), sqlc.narg(ip_address),
    sqlc.arg(user_agent)
)
RETURNING uuid;

-- name: GetDocumentApprovalForPDF :one
SELECT a.id, a.uuid, a.method, a.party, a.approver_name, a.approved_at, a.content_hash, a.verified_phone_masked,
    a.ip_address, a.user_agent, a.otp_challenge_uuid, a.invalidated_at, a.pdf_object_key, a.pdf_size_bytes,
    v.uuid AS version_uuid, v.version AS version_number, v.html
FROM document_approvals a
JOIN document_versions v ON v.organization_id = a.organization_id AND v.id = a.document_version_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND v.document_id = sqlc.arg(document_id)
  AND (sqlc.narg(uuid)::uuid IS NULL OR a.uuid = sqlc.narg(uuid)::uuid)
  AND (sqlc.narg(link_id)::bigint IS NULL OR a.link_id = sqlc.narg(link_id)::bigint)
ORDER BY a.approved_at DESC
LIMIT 1;

-- name: SetDocumentApprovalPDF :exec
UPDATE document_approvals
SET pdf_object_key = sqlc.arg(pdf_object_key), pdf_size_bytes = sqlc.arg(pdf_size_bytes), pdf_sha256 = sqlc.arg(pdf_sha256)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetDocumentSignerRenter :one
SELECT c.id, c.uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.phone
FROM reservations res
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
WHERE res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND res.id = sqlc.arg(reservation_id);

-- name: ListDocumentSignerDrivers :many
SELECT dr.id, dr.uuid, dr.customer_id, dr.first_name, dr.last_name, dr.phone, rd.role
FROM reservation_drivers rd
JOIN drivers dr ON dr.organization_id = rd.organization_id AND dr.id = rd.driver_id
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND dr.organization_id = sqlc.arg(organization_id)
  AND rd.reservation_id = sqlc.arg(reservation_id)
ORDER BY rd.role DESC, rd.id;
