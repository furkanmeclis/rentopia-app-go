-- Vehicle inspections (TEC-712 / TEC-713 / TEC-714): templates, handover
-- and return forms, damages, photos (evidence) and signatures. Every query
-- filters on the organization.

-- ------------------------------------------------------------- templates

-- name: GetActiveInspectionTemplate :one
SELECT t.id, t.uuid, t.name, t.version, t.status, t.photo_slots, t.checklist, t.created_at, t.updated_at
FROM inspection_templates t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.status = 'active';

-- name: GetInspectionTemplate :one
SELECT t.id, t.uuid, t.name, t.version, t.status, t.photo_slots, t.checklist, t.created_at, t.updated_at
FROM inspection_templates t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: ListInspectionTemplates :many
SELECT t.id, t.uuid, t.name, t.version, t.status, t.photo_slots, t.checklist, t.created_at, t.updated_at,
    (SELECT COUNT(*) FROM inspections i
        WHERE i.organization_id = t.organization_id AND i.template_id = t.id)::bigint AS inspections,
    COUNT(*) OVER ()::bigint AS total
FROM inspection_templates t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR t.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR t.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'version' AND NOT sqlc.arg(sort_desc)::boolean THEN t.version END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'version' AND sqlc.arg(sort_desc)::boolean THEN t.version END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
    t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: NextInspectionTemplateVersion :one
SELECT (COALESCE(MAX(t.version), 0) + 1)::int AS version
FROM inspection_templates t
WHERE t.organization_id = sqlc.arg(organization_id);

-- name: ArchiveActiveInspectionTemplate :exec
UPDATE inspection_templates
SET status = 'archived'
WHERE organization_id = sqlc.arg(organization_id)
  AND status = 'active';

-- name: InsertInspectionTemplate :one
INSERT INTO inspection_templates (organization_id, name, version, status, photo_slots, checklist, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(version), 'active', sqlc.arg(photo_slots),
    sqlc.arg(checklist), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- ------------------------------------------------------------ inspections

-- name: GetInspectionRentalFacts :one
-- The rental side an inspection works on (planned values, money, the
-- product conditions and the policy snapshot used by return charges).
SELECT r.id, r.uuid, r.reference, r.status, r.vehicle_id, r.actual_pickup_at, r.actual_return_at,
    r.actual_return_branch_id, r.policy_snapshot,
    res.id AS reservation_id, res.uuid AS reservation_uuid, res.starts_at AS planned_pickup_at,
    res.ends_at AS planned_return_at, res.pickup_branch_id, res.return_branch_id, res.product_snapshot,
    res.currency, res.total_amount, res.paid_amount, res.payment_status,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN vehicles v ON v.organization_id = r.organization_id AND v.id = r.vehicle_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: InsertInspection :one
INSERT INTO inspections (
    organization_id, rental_id, vehicle_id, kind, version, previous_id, revision_reason, template_id,
    template_snapshot, occurred_at, branch_id, point_id, km, fuel, equipment, verification, note,
    customer_statement, charge_overrides, vehicle_status_after, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(vehicle_id), sqlc.arg(kind), sqlc.arg(version),
    sqlc.narg(previous_id), sqlc.arg(revision_reason), sqlc.arg(template_id), sqlc.arg(template_snapshot),
    sqlc.narg(occurred_at), sqlc.narg(branch_id), sqlc.narg(point_id), sqlc.narg(km), sqlc.narg(fuel),
    sqlc.arg(equipment), sqlc.arg(verification), sqlc.arg(note), sqlc.arg(customer_statement),
    sqlc.arg(charge_overrides), sqlc.narg(vehicle_status_after), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: NextInspectionVersion :one
SELECT (COALESCE(MAX(i.version), 0) + 1)::int AS version
FROM inspections i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.rental_id = sqlc.arg(rental_id)
  AND i.kind = sqlc.arg(kind);

-- name: GetLiveInspection :one
SELECT i.id, i.uuid, i.status
FROM inspections i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.rental_id = sqlc.arg(rental_id)
  AND i.kind = sqlc.arg(kind)
  AND i.status <> 'superseded';

-- name: GetInspection :one
-- One inspection with its display joins.
SELECT i.id, i.uuid, i.rental_id, i.vehicle_id, i.rental_vehicle_id, i.kind, i.version, i.status, i.previous_id,
    i.revision_reason, i.template_id, i.template_snapshot, i.occurred_at, i.branch_id, i.point_id, i.km, i.fuel,
    i.equipment, i.verification, i.note, i.customer_statement, i.customer_reviewed_at, i.charge_overrides,
    i.vehicle_status_after, i.signature_status, i.signature_note, i.disputed, i.dispute_note, i.review_status,
    i.reviewed_at, i.review_note, i.finalized_at, i.created_at, i.updated_at,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    t.uuid AS template_uuid,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    sp.uuid AS point_uuid, sp.name AS point_name,
    prev.uuid AS previous_uuid,
    ru.uuid AS reviewed_by_uuid, ru.name AS reviewed_by_name, ru.surname AS reviewed_by_surname, ru.email AS reviewed_by_email,
    fu.uuid AS finalized_by_uuid, fu.name AS finalized_by_name, fu.surname AS finalized_by_surname, fu.email AS finalized_by_email,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM inspections i
JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
JOIN vehicles v ON v.organization_id = i.organization_id AND v.id = i.vehicle_id
JOIN inspection_templates t ON t.organization_id = i.organization_id AND t.id = i.template_id
LEFT JOIN branches b ON b.organization_id = i.organization_id AND b.id = i.branch_id
LEFT JOIN service_points sp ON sp.organization_id = i.organization_id AND sp.id = i.point_id
LEFT JOIN inspections prev ON prev.organization_id = i.organization_id AND prev.id = i.previous_id
LEFT JOIN users ru ON ru.id = i.reviewed_by
LEFT JOIN users fu ON fu.id = i.finalized_by
LEFT JOIN users cu ON cu.id = i.created_by
WHERE i.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND t.organization_id = sqlc.arg(organization_id)
  AND i.uuid = sqlc.arg(uuid);

-- name: GetInspectionUUIDByID :one
SELECT i.uuid
FROM inspections i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.id = sqlc.arg(id);

-- name: ListRentalInspections :many
-- Every version of the rental's forms (newest first per kind).
SELECT i.uuid, i.kind, i.version, i.status, i.review_status, i.signature_status, i.disputed, i.finalized_at,
    i.created_at
FROM inspections i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.rental_id = sqlc.arg(rental_id)
ORDER BY i.kind, i.version DESC;

-- name: GetFinalizedHandoverInspection :one
-- The handover record a return is compared with.
SELECT i.uuid
FROM inspections i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.rental_id = sqlc.arg(rental_id)
  AND i.kind = 'handover'
  AND i.status = 'finalized';

-- name: ListInspections :many
-- One page of inspections in the caller's branch scope (the rental's
-- planned pick-up / return or actual return branch).
SELECT i.uuid, i.kind, i.version, i.status, i.signature_status, i.disputed, i.review_status, i.km, i.fuel,
    i.occurred_at, i.finalized_at, i.created_at, i.updated_at,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    (SELECT COUNT(*) FROM inspection_photos ph
        WHERE ph.organization_id = i.organization_id AND ph.inspection_id = i.id AND ph.status = 'completed')::bigint AS photos,
    (SELECT COUNT(*) FROM inspection_damages d
        WHERE d.organization_id = i.organization_id AND d.inspection_id = i.id)::bigint AS damages,
    COUNT(*) OVER ()::bigint AS total
FROM inspections i
JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
JOIN vehicles v ON v.organization_id = i.organization_id AND v.id = i.vehicle_id
WHERE i.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(kinds)::text[] IS NULL OR i.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR i.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(review_statuses)::text[] IS NULL OR i.review_status = ANY(sqlc.narg(review_statuses)::text[]))
  AND (sqlc.narg(rental_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(rental_uuids)::uuid[]))
  AND (NOT sqlc.arg(disputed_only)::boolean OR i.disputed)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN i.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN i.created_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'finalized_at' AND NOT sqlc.arg(sort_desc)::boolean THEN i.finalized_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'finalized_at' AND sqlc.arg(sort_desc)::boolean THEN i.finalized_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN i.kind END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN i.kind END DESC,
    i.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListInspectionFacets :many
-- Facet counts over the caller's branch scope: (facet, value, count).
SELECT f.facet::text AS facet, f.value::text AS value, COUNT(*)::bigint AS count
FROM (
    SELECT 'kind' AS facet, i.kind AS value, res.pickup_branch_id, res.return_branch_id, r.actual_return_branch_id
    FROM inspections i
    JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
    JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    WHERE i.organization_id = sqlc.arg(organization_id) AND r.organization_id = sqlc.arg(organization_id)
      AND res.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'status', i.status, res.pickup_branch_id, res.return_branch_id, r.actual_return_branch_id
    FROM inspections i
    JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
    JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    WHERE i.organization_id = sqlc.arg(organization_id) AND r.organization_id = sqlc.arg(organization_id)
      AND res.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'review_status', i.review_status, res.pickup_branch_id, res.return_branch_id, r.actual_return_branch_id
    FROM inspections i
    JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
    JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    WHERE i.organization_id = sqlc.arg(organization_id) AND r.organization_id = sqlc.arg(organization_id)
      AND res.organization_id = sqlc.arg(organization_id)
) f
WHERE (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR f.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR f.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR f.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
)
GROUP BY f.facet, f.value
ORDER BY f.facet, f.value;

-- name: UpdateInspectionDraft :execrows
UPDATE inspections
SET occurred_at = sqlc.narg(occurred_at), branch_id = sqlc.narg(branch_id), point_id = sqlc.narg(point_id),
    km = sqlc.narg(km), fuel = sqlc.narg(fuel), equipment = sqlc.arg(equipment), verification = sqlc.arg(verification),
    note = sqlc.arg(note), customer_statement = sqlc.arg(customer_statement),
    customer_reviewed_at = sqlc.narg(customer_reviewed_at), charge_overrides = sqlc.arg(charge_overrides),
    vehicle_status_after = sqlc.narg(vehicle_status_after), disputed = sqlc.arg(disputed),
    dispute_note = sqlc.arg(dispute_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: SetInspectionSignatureStatus :execrows
UPDATE inspections
SET signature_status = sqlc.arg(signature_status), signature_note = sqlc.arg(signature_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: FinalizeInspection :execrows
UPDATE inspections
SET status = 'finalized', finalized_at = sqlc.arg(finalized_at), finalized_by = sqlc.narg(finalized_by),
    occurred_at = sqlc.arg(occurred_at), rental_vehicle_id = sqlc.narg(rental_vehicle_id),
    signature_status = sqlc.arg(signature_status), signature_note = sqlc.arg(signature_note),
    review_status = sqlc.arg(review_status)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: SupersedeInspection :execrows
UPDATE inspections
SET status = 'superseded'
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'finalized';

-- name: ResolveInspectionReview :execrows
UPDATE inspections
SET review_status = 'resolved', reviewed_by = sqlc.narg(reviewed_by), reviewed_at = sqlc.arg(reviewed_at),
    review_note = sqlc.arg(review_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND review_status = 'pending';

-- name: GetRentalLegForInspection :one
-- The newest leg of the rental's vehicle (the one a handover opened or a
-- return closed).
SELECT rv.id
FROM rental_vehicles rv
WHERE rv.organization_id = sqlc.arg(organization_id)
  AND rv.rental_id = sqlc.arg(rental_id)
  AND rv.vehicle_id = sqlc.arg(vehicle_id)
ORDER BY rv.id DESC
LIMIT 1;

-- name: GetRentalLegKm :one
-- Km out / in of the rental's legs (driven km of the whole rental).
SELECT COALESCE(SUM(rv.in_km - rv.out_km) FILTER (WHERE rv.in_km IS NOT NULL AND rv.out_km IS NOT NULL), 0)::bigint AS driven_km,
    COALESCE(MAX(rv.out_km) FILTER (WHERE rv.in_km IS NULL), 0)::int AS open_out_km,
    COALESCE(MAX(rv.out_fuel::int) FILTER (WHERE rv.in_km IS NULL), -1)::int AS open_out_fuel
FROM rental_vehicles rv
WHERE rv.organization_id = sqlc.arg(organization_id)
  AND rv.rental_id = sqlc.arg(rental_id);

-- -------------------------------------------------------------- damages

-- name: InsertInspectionDamage :one
INSERT INTO inspection_damages (
    organization_id, inspection_id, zone, pos_x, pos_y, damage_type, severity, description, is_new, disputed,
    copied_from_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(inspection_id), sqlc.arg(zone), sqlc.narg(pos_x), sqlc.narg(pos_y),
    sqlc.arg(damage_type), sqlc.arg(severity), sqlc.arg(description), sqlc.arg(is_new), sqlc.arg(disputed),
    sqlc.narg(copied_from_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetInspectionDamage :one
SELECT d.id, d.uuid, d.zone, d.pos_x, d.pos_y, d.damage_type, d.severity, d.description, d.is_new, d.disputed,
    d.created_at
FROM inspection_damages d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.inspection_id = sqlc.arg(inspection_id)
  AND d.uuid = sqlc.arg(uuid);

-- name: UpdateInspectionDamage :execrows
UPDATE inspection_damages
SET zone = sqlc.arg(zone), pos_x = sqlc.narg(pos_x), pos_y = sqlc.narg(pos_y), damage_type = sqlc.arg(damage_type),
    severity = sqlc.arg(severity), description = sqlc.arg(description), is_new = sqlc.arg(is_new),
    disputed = sqlc.arg(disputed)
WHERE organization_id = sqlc.arg(organization_id)
  AND inspection_id = sqlc.arg(inspection_id)
  AND id = sqlc.arg(id);

-- name: DeleteInspectionDamage :execrows
DELETE FROM inspection_damages
WHERE organization_id = sqlc.arg(organization_id)
  AND inspection_id = sqlc.arg(inspection_id)
  AND id = sqlc.arg(id);

-- name: ListInspectionDamages :many
SELECT d.id, d.uuid, d.zone, d.pos_x, d.pos_y, d.damage_type, d.severity, d.description, d.is_new, d.disputed,
    d.created_at
FROM inspection_damages d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.inspection_id = sqlc.arg(inspection_id)
ORDER BY d.id;

-- --------------------------------------------------------------- photos

-- name: InsertInspectionPhoto :one
INSERT INTO inspection_photos (
    organization_id, inspection_id, rental_id, inspection_version, slot, damage_id, client_ref, status, object_key,
    content_type, declared_size_bytes, size_bytes, sha256, uploaded_at, uploaded_by, device_captured_at,
    exif_taken_at, exif_taken_local, exif_gps_lat, exif_gps_lng, exif, scan_status, copied_from_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(inspection_id), sqlc.arg(rental_id), sqlc.arg(inspection_version),
    sqlc.arg(slot), sqlc.narg(damage_id), sqlc.narg(client_ref), sqlc.arg(status), sqlc.arg(object_key),
    sqlc.arg(content_type), sqlc.arg(declared_size_bytes), sqlc.arg(size_bytes), sqlc.narg(sha256),
    sqlc.narg(uploaded_at), sqlc.narg(uploaded_by), sqlc.narg(device_captured_at), sqlc.narg(exif_taken_at),
    sqlc.arg(exif_taken_local), sqlc.narg(exif_gps_lat), sqlc.narg(exif_gps_lng), sqlc.arg(exif),
    sqlc.arg(scan_status), sqlc.narg(copied_from_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetInspectionPhoto :one
SELECT ph.id, ph.uuid, ph.inspection_id, ph.rental_id, ph.slot, ph.damage_id, ph.client_ref, ph.status, ph.object_key,
    ph.content_type, ph.declared_size_bytes, ph.size_bytes
FROM inspection_photos ph
WHERE ph.organization_id = sqlc.arg(organization_id)
  AND ph.inspection_id = sqlc.arg(inspection_id)
  AND ph.uuid = sqlc.arg(uuid);

-- name: GetInspectionPhotoByClientRef :one
SELECT ph.uuid
FROM inspection_photos ph
WHERE ph.organization_id = sqlc.arg(organization_id)
  AND ph.inspection_id = sqlc.arg(inspection_id)
  AND ph.client_ref = sqlc.arg(client_ref);

-- name: GetInspectionMediaObject :one
-- A completed photo (or a signature) for a media link: the object key and
-- its content type.
SELECT ph.object_key, ph.content_type, ph.size_bytes
FROM inspection_photos ph
WHERE ph.organization_id = sqlc.arg(organization_id)
  AND ph.uuid = sqlc.arg(uuid)
  AND ph.status = 'completed';

-- name: GetInspectionSignatureObject :one
SELECT s.object_key, s.size_bytes
FROM inspection_signatures s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid);

-- name: SetInspectionPhotoUploaded :execrows
UPDATE inspection_photos
SET status = 'uploaded', content_type = sqlc.arg(content_type), size_bytes = sqlc.arg(size_bytes)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('pending', 'uploaded');

-- name: CompleteInspectionPhoto :execrows
UPDATE inspection_photos
SET status = 'completed', sha256 = sqlc.arg(sha256), size_bytes = sqlc.arg(size_bytes),
    uploaded_at = sqlc.arg(uploaded_at), uploaded_by = sqlc.narg(uploaded_by),
    exif_taken_at = sqlc.narg(exif_taken_at), exif_taken_local = sqlc.arg(exif_taken_local),
    exif_gps_lat = sqlc.narg(exif_gps_lat), exif_gps_lng = sqlc.narg(exif_gps_lng), exif = sqlc.arg(exif),
    scan_status = sqlc.arg(scan_status)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'uploaded';

-- name: DeleteInspectionPhoto :execrows
DELETE FROM inspection_photos
WHERE organization_id = sqlc.arg(organization_id)
  AND inspection_id = sqlc.arg(inspection_id)
  AND id = sqlc.arg(id);

-- name: ListInspectionPhotos :many
SELECT ph.id, ph.uuid, ph.inspection_version, ph.slot, ph.damage_id, ph.client_ref, ph.status, ph.object_key,
    ph.content_type, ph.declared_size_bytes, ph.size_bytes, ph.sha256, ph.uploaded_at, ph.uploaded_by, ph.device_captured_at,
    ph.exif_taken_at, ph.exif_taken_local, ph.exif_gps_lat, ph.exif_gps_lng, ph.exif, ph.scan_status,
    ph.copied_from_id, ph.created_at,
    d.uuid AS damage_uuid,
    uu.uuid AS uploaded_by_uuid, uu.name AS uploaded_by_name, uu.surname AS uploaded_by_surname, uu.email AS uploaded_by_email
FROM inspection_photos ph
LEFT JOIN inspection_damages d ON d.organization_id = ph.organization_id AND d.id = ph.damage_id
LEFT JOIN users uu ON uu.id = ph.uploaded_by
WHERE ph.organization_id = sqlc.arg(organization_id)
  AND ph.inspection_id = sqlc.arg(inspection_id)
ORDER BY ph.id;

-- ----------------------------------------------------------- signatures

-- name: InsertInspectionSignature :one
INSERT INTO inspection_signatures (
    organization_id, inspection_id, rental_id, signer_name, signer_role, object_key, sha256, size_bytes, strokes,
    point_count, signed_at, device_signed_at, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(inspection_id), sqlc.arg(rental_id), sqlc.arg(signer_name),
    sqlc.arg(signer_role), sqlc.arg(object_key), sqlc.arg(sha256), sqlc.arg(size_bytes), sqlc.arg(strokes),
    sqlc.arg(point_count), sqlc.arg(signed_at), sqlc.narg(device_signed_at), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetInspectionSignature :one
SELECT s.id, s.uuid, s.signer_name, s.signer_role, s.object_key, s.sha256, s.size_bytes, s.point_count, s.signed_at,
    s.device_signed_at, s.created_at,
    a.uuid AS approval_uuid, a.content_hash AS approval_content_hash, dv.uuid AS approval_version_uuid,
    doc.uuid AS approval_document_uuid
FROM inspection_signatures s
LEFT JOIN document_approvals a ON a.organization_id = s.organization_id AND a.signature_id = s.id
LEFT JOIN document_versions dv ON dv.organization_id = a.organization_id AND dv.id = a.document_version_id
LEFT JOIN documents doc ON doc.organization_id = dv.organization_id AND doc.id = dv.document_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.inspection_id = sqlc.arg(inspection_id)
ORDER BY a.id DESC NULLS LAST
LIMIT 1;

-- name: DeleteInspectionSignature :execrows
DELETE FROM inspection_signatures
WHERE organization_id = sqlc.arg(organization_id)
  AND inspection_id = sqlc.arg(inspection_id);

-- ------------------------------------------------------------- charges

-- name: InsertInspectionCharge :one
INSERT INTO rental_charges (
    organization_id, rental_id, kind, source, currency, amount, settings_version, calculation, reason, created_by,
    inspection_id, disputed, original_amount, override_reason, overridden_by, overridden_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(kind), sqlc.arg(source), sqlc.arg(currency),
    sqlc.arg(amount), sqlc.arg(settings_version), sqlc.arg(calculation), sqlc.arg(reason), sqlc.narg(created_by),
    sqlc.arg(inspection_id), sqlc.arg(disputed), sqlc.narg(original_amount), sqlc.arg(override_reason),
    sqlc.narg(overridden_by), sqlc.narg(overridden_at)
)
RETURNING uuid;

-- name: DeleteProposedInspectionPolicyCharges :exec
-- A re-computed mileage / fuel / equipment charge replaces the proposal.
DELETE FROM rental_charges
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND kind = ANY(sqlc.arg(kinds)::text[])
  AND source = 'policy'
  AND status = 'proposed';

-- name: LinkLateChargeToInspection :one
-- The policy late fee the return computed belongs to the return form.
UPDATE rental_charges
SET inspection_id = sqlc.arg(inspection_id), disputed = sqlc.arg(disputed)
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND kind = 'late_return'
  AND source = 'policy'
  AND status = 'proposed'
RETURNING id, amount;

-- name: OverrideLateCharge :execrows
UPDATE rental_charges
SET original_amount = amount, amount = sqlc.arg(amount), override_reason = sqlc.arg(override_reason),
    overridden_by = sqlc.narg(overridden_by), overridden_at = sqlc.arg(overridden_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'proposed';

-- name: CountInspectionCharges :one
SELECT COUNT(*)::bigint
FROM rental_charges ch
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.inspection_id = sqlc.arg(inspection_id);

-- name: GetOrganizationUUIDForInspections :one
SELECT o.uuid FROM organizations o WHERE o.id = sqlc.arg(id);
