-- Contract templates, documents, versions and approvals (TEC-710). Every
-- query filters by organization_id; documents follow the branch scope of
-- their reservation (pick-up OR return branch, or the rental's actual
-- return branch), applied by the list query and checked by the use case.

-- name: InsertContractTemplate :one
-- The next version number of the kind is taken inside the insert; a
-- concurrent insert of the same kind hits the unique (kind, version) and
-- retries.
INSERT INTO contract_templates (organization_id, kind, name, version, status, content, body_html, placeholders, created_by)
SELECT sqlc.arg(organization_id)::bigint, sqlc.arg(kind)::text, sqlc.arg(name)::text,
    COALESCE((SELECT MAX(t.version) FROM contract_templates t
        WHERE t.organization_id = sqlc.arg(organization_id)::bigint AND t.kind = sqlc.arg(kind)::text), 0) + 1,
    'draft', sqlc.arg(content)::jsonb, sqlc.arg(body_html)::text, sqlc.arg(placeholders)::text[], sqlc.narg(created_by)::bigint
RETURNING id, uuid, version;

-- name: GetContractTemplate :one
SELECT t.id, t.uuid, t.kind, t.name, t.version, t.status, t.content, t.body_html, t.placeholders, t.activated_at,
    t.archived_at, t.created_at, t.updated_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    au.uuid AS activated_by_uuid, au.name AS activated_by_name, au.surname AS activated_by_surname, au.email AS activated_by_email,
    (SELECT COUNT(*) FROM document_versions dv
        WHERE dv.organization_id = t.organization_id AND dv.template_id = t.id)::bigint AS used_by
FROM contract_templates t
LEFT JOIN users cu ON cu.id = t.created_by
LEFT JOIN users au ON au.id = t.activated_by
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: ListContractTemplates :many
SELECT t.uuid, t.kind, t.name, t.version, t.status, t.placeholders, t.activated_at, t.archived_at, t.created_at, t.updated_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM contract_templates t
LEFT JOIN users cu ON cu.id = t.created_by
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(kinds)::text[] IS NULL OR t.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR t.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN t.name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN t.name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'version' AND NOT sqlc.arg(sort_desc)::boolean THEN t.version END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'version' AND sqlc.arg(sort_desc)::boolean THEN t.version END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
    t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: UpdateContractTemplate :execrows
UPDATE contract_templates
SET name = sqlc.arg(name), content = sqlc.arg(content), body_html = sqlc.arg(body_html), placeholders = sqlc.arg(placeholders)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: DeleteContractTemplate :execrows
DELETE FROM contract_templates
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: ArchiveActiveContractTemplate :exec
UPDATE contract_templates
SET status = 'archived', archived_at = sqlc.arg(archived_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND kind = sqlc.arg(kind)
  AND status = 'active';

-- name: ActivateContractTemplate :execrows
UPDATE contract_templates
SET status = 'active', activated_at = sqlc.arg(activated_at), activated_by = sqlc.narg(activated_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: GetActiveContractTemplate :one
SELECT t.id, t.uuid, t.kind, t.name, t.version, t.body_html, t.placeholders
FROM contract_templates t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.kind = sqlc.arg(kind)
  AND t.status = 'active';

-- name: GetDocumentOrganization :one
SELECT o.uuid, o.slug, o.name
FROM organizations o
WHERE o.id = sqlc.arg(organization_id);

-- name: GetLiveDocument :one
SELECT d.id, d.uuid, d.kind, d.status, d.current_version_id, d.rental_id
FROM documents d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.reservation_id = sqlc.arg(reservation_id)
  AND d.kind = sqlc.arg(kind)
  AND d.status <> 'void';

-- name: ListLiveDocumentKinds :many
SELECT d.kind
FROM documents d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.reservation_id = sqlc.arg(reservation_id)
  AND d.status <> 'void'
ORDER BY d.kind;

-- name: InsertDocument :one
INSERT INTO documents (organization_id, kind, reservation_id, rental_id, status, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(reservation_id), sqlc.narg(rental_id), 'draft', sqlc.narg(created_by))
RETURNING id, uuid;

-- name: SetDocumentCurrent :execrows
UPDATE documents
SET current_version_id = sqlc.arg(current_version_id), status = sqlc.arg(status),
    rental_id = COALESCE(sqlc.narg(rental_id), rental_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'void';

-- name: SetDocumentStatus :execrows
UPDATE documents
SET status = sqlc.arg(status)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'void';

-- name: VoidRentalDocuments :many
UPDATE documents
SET status = 'void'
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND status <> 'void'
RETURNING uuid;

-- name: GetDocument :one
SELECT d.id, d.uuid, d.kind, d.status, d.current_version_id, d.reservation_id, d.created_at, d.updated_at,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference, res.status AS reservation_status,
    res.pickup_branch_id, res.return_branch_id, res.customer_id,
    r.id AS rental_id, r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    COALESCE(r.actual_return_branch_id, 0)::bigint AS actual_return_branch_id
FROM documents d
JOIN reservations res ON res.organization_id = d.organization_id AND res.id = d.reservation_id
LEFT JOIN rentals r ON r.organization_id = d.organization_id AND r.id = d.rental_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid);

-- name: ListDocuments :many
SELECT d.uuid, d.kind, d.status, d.created_at, d.updated_at,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    v.uuid AS version_uuid, v.version AS version_number, v.status AS version_status, v.content_hash,
    v.pdf_status, v.created_at AS version_created_at, v.template_version,
    t.name AS template_name,
    COALESCE(rl.status, '')::text AS remote_link_status, rl.expires_at AS remote_link_expires_at,
    COUNT(*) OVER ()::bigint AS total
FROM documents d
JOIN reservations res ON res.organization_id = d.organization_id AND res.id = d.reservation_id
LEFT JOIN rentals r ON r.organization_id = d.organization_id AND r.id = d.rental_id
LEFT JOIN document_versions v ON v.organization_id = d.organization_id AND v.id = d.current_version_id
LEFT JOIN contract_templates t ON t.organization_id = v.organization_id AND t.id = v.template_id
LEFT JOIN LATERAL (
    SELECT l.status, l.expires_at
    FROM document_approval_links l
    WHERE l.organization_id = d.organization_id AND l.document_id = d.id
    ORDER BY l.created_at DESC, l.id DESC
    LIMIT 1
) rl ON TRUE
WHERE d.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(kinds)::text[] IS NULL OR d.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR d.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR res.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (sqlc.narg(rental_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(rental_uuids)::uuid[]))
  AND (sqlc.narg(q)::text IS NULL OR res.reference ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: InsertDocumentVersion :one
INSERT INTO document_versions (
    organization_id, document_id, version, status, template_id, template_version, html, content_hash, data_snapshot,
    changed_keys, created_by
)
SELECT sqlc.arg(organization_id)::bigint, sqlc.arg(document_id)::bigint,
    COALESCE((SELECT MAX(x.version) FROM document_versions x
        WHERE x.organization_id = sqlc.arg(organization_id)::bigint AND x.document_id = sqlc.arg(document_id)::bigint), 0) + 1,
    'draft', sqlc.arg(template_id)::bigint, sqlc.arg(template_version)::int, sqlc.arg(html)::text, sqlc.arg(content_hash)::text,
    sqlc.arg(data_snapshot)::jsonb, sqlc.arg(changed_keys)::text[], sqlc.narg(created_by)::bigint
RETURNING id, uuid, version;

-- name: GetDocumentVersionByID :one
SELECT v.id, v.uuid, v.document_id, v.version, v.status, v.template_id, v.template_version, v.html, v.content_hash,
    v.data_snapshot, v.changed_keys, v.pdf_status, v.pdf_object_key, v.pdf_size_bytes, v.pdf_sha256, v.pdf_error,
    v.pdf_rendered_at, v.issued_at, v.superseded_at, v.created_at
FROM document_versions v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.id = sqlc.arg(id);

-- name: GetDocumentVersion :one
SELECT v.id, v.uuid, v.document_id, v.version, v.status, v.template_id, v.template_version, v.html, v.content_hash,
    v.data_snapshot, v.changed_keys, v.pdf_status, v.pdf_object_key, v.pdf_size_bytes, v.pdf_sha256, v.pdf_error,
    v.pdf_rendered_at, v.issued_at, v.superseded_at, v.created_at,
    t.uuid AS template_uuid, t.name AS template_name,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM document_versions v
JOIN contract_templates t ON t.organization_id = v.organization_id AND t.id = v.template_id
LEFT JOIN users cu ON cu.id = v.created_by
WHERE v.organization_id = sqlc.arg(organization_id)
  AND t.organization_id = sqlc.arg(organization_id)
  AND v.document_id = sqlc.arg(document_id)
  AND v.uuid = sqlc.arg(uuid);

-- name: ListDocumentVersions :many
SELECT v.id, v.uuid, v.version, v.status, v.template_version, v.content_hash, v.changed_keys, v.pdf_status, v.pdf_error,
    v.pdf_size_bytes, v.issued_at, v.superseded_at, v.created_at,
    t.uuid AS template_uuid, t.name AS template_name,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM document_versions v
JOIN contract_templates t ON t.organization_id = v.organization_id AND t.id = v.template_id
LEFT JOIN users cu ON cu.id = v.created_by
WHERE v.organization_id = sqlc.arg(organization_id)
  AND t.organization_id = sqlc.arg(organization_id)
  AND v.document_id = sqlc.arg(document_id)
ORDER BY v.version DESC;

-- name: SupersedeDocumentVersion :execrows
UPDATE document_versions
SET status = 'superseded', superseded_at = sqlc.arg(superseded_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'superseded';

-- name: IssueDocumentVersion :execrows
UPDATE document_versions
SET status = 'issued', issued_at = sqlc.arg(issued_at), issued_by = sqlc.narg(issued_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: ApproveDocumentVersion :execrows
UPDATE document_versions
SET status = 'approved', issued_at = COALESCE(issued_at, sqlc.arg(approved_at)::timestamptz),
    issued_by = COALESCE(issued_by, sqlc.narg(actor_id))
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND content_hash = sqlc.arg(content_hash)
  AND status IN ('draft', 'issued');

-- name: SetDocumentVersionPDFPending :execrows
UPDATE document_versions
SET pdf_status = 'pending', pdf_error = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND pdf_status IN ('none', 'failed');

-- name: SetDocumentVersionPDF :execrows
UPDATE document_versions
SET pdf_status = 'ready', pdf_object_key = sqlc.arg(pdf_object_key), pdf_size_bytes = sqlc.arg(pdf_size_bytes),
    pdf_sha256 = sqlc.arg(pdf_sha256), pdf_error = '', pdf_rendered_at = sqlc.arg(pdf_rendered_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetDocumentVersionPDFFailed :execrows
UPDATE document_versions
SET pdf_status = 'failed', pdf_error = sqlc.arg(pdf_error)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND pdf_status <> 'ready';

-- name: InsertDocumentApproval :one
INSERT INTO document_approvals (
    organization_id, document_version_id, content_hash, party, method, approver_name, customer_id, note, recorded_by,
    approved_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(document_version_id), sqlc.arg(content_hash), sqlc.arg(party), sqlc.arg(method),
    sqlc.arg(approver_name), sqlc.narg(customer_id), sqlc.arg(note), sqlc.narg(recorded_by), sqlc.arg(approved_at)
)
RETURNING uuid;

-- name: InvalidateDocumentApprovals :exec
UPDATE document_approvals
SET invalidated_at = sqlc.arg(invalidated_at), invalidated_reason = sqlc.arg(invalidated_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND document_version_id = sqlc.arg(document_version_id)
  AND invalidated_at IS NULL;

-- name: ListDocumentApprovals :many
SELECT a.uuid, a.content_hash, a.party, a.method, a.approver_name, a.note, a.approved_at, a.invalidated_at,
    a.invalidated_reason, a.verified_phone_masked, a.ip_address, a.user_agent,
    v.uuid AS version_uuid, v.version AS version_number,
    l.uuid AS link_uuid, l.signer_role,
    ru.uuid AS recorded_by_uuid, ru.name AS recorded_by_name, ru.surname AS recorded_by_surname, ru.email AS recorded_by_email
FROM document_approvals a
JOIN document_versions v ON v.organization_id = a.organization_id AND v.id = a.document_version_id
LEFT JOIN document_approval_links l ON l.organization_id = a.organization_id AND l.id = a.link_id
LEFT JOIN users ru ON ru.id = a.recorded_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND (l.organization_id IS NULL OR l.organization_id = sqlc.arg(organization_id))
  AND v.document_id = sqlc.arg(document_id)
ORDER BY a.approved_at DESC, a.id DESC;

-- name: GetDocumentRentalFacts :one
-- The live rental of a reservation as documents render it.
SELECT r.id, r.uuid, r.reference, r.status, r.actual_pickup_at, r.actual_return_at, r.late_minutes,
    arb.name AS actual_return_branch_name, apb.name AS actual_pickup_branch_name
FROM rentals r
LEFT JOIN branches arb ON arb.organization_id = r.organization_id AND arb.id = r.actual_return_branch_id
LEFT JOIN branches apb ON apb.organization_id = r.organization_id AND apb.id = r.actual_pickup_branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.reservation_id = sqlc.arg(reservation_id)
  AND r.status <> 'cancelled';

-- name: GetDocumentReservationRef :one
SELECT res.id, res.uuid, res.reference, res.pickup_branch_id, res.return_branch_id
FROM reservations res
WHERE res.organization_id = sqlc.arg(organization_id)
  AND res.uuid = sqlc.arg(uuid);

-- name: GetDocumentReservationByID :one
SELECT res.id, res.uuid, res.reference, res.pickup_branch_id, res.return_branch_id
FROM reservations res
WHERE res.organization_id = sqlc.arg(organization_id)
  AND res.id = sqlc.arg(id);

-- name: GetDocumentUUIDByID :one
SELECT d.uuid
FROM documents d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id);
