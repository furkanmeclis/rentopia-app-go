-- Reservations (TEC-706), their drivers, event history and plate
-- assignment history. Every query filters by organization_id first
-- (docs/design/tenant-core.md §8); lists apply the caller's branch scope
-- (pick-up OR return branch).

-- name: NextReservationNumber :one
-- Per organization and year counter (row lock of the upsert serialises
-- concurrent creates of one organization).
INSERT INTO reservation_sequences (organization_id, year, last_value)
VALUES (sqlc.arg(organization_id), sqlc.arg(year), 1)
ON CONFLICT (organization_id, year) DO UPDATE SET last_value = reservation_sequences.last_value + 1
RETURNING last_value;

-- name: GetReservationOrganizationSlug :one
SELECT o.slug FROM organizations o WHERE o.id = sqlc.arg(organization_id);

-- name: InsertReservation :one
INSERT INTO reservations (
    uuid, organization_id, reference, status, payment_status, capacity_commitment, allocation_id,
    pickup_branch_id, return_branch_id, pickup_point_id, return_point_id, starts_at, ends_at,
    rental_product_id, product_version, sold_rental_product_id, sold_product_version,
    guarantee_type, product_snapshot, sold_snapshot,
    customer_id, invoice_customer_id, channel, external_reference, collecting_party, commission_amount,
    net_receivable, currency, total_amount, prepayment_amount, quote_id, quote_snapshot,
    settings_version, policy_snapshot, review_deadline, payment_deadline, note, eligibility_override,
    confirmed_at, created_by, approval_protocol, pickup_delivery, return_delivery, agreement_id, open_account
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(reference), sqlc.arg(status), 'unpaid',
    sqlc.arg(capacity_commitment), sqlc.narg(allocation_id),
    sqlc.arg(pickup_branch_id), sqlc.arg(return_branch_id), sqlc.narg(pickup_point_id), sqlc.narg(return_point_id),
    sqlc.arg(starts_at), sqlc.arg(ends_at),
    sqlc.arg(rental_product_id), sqlc.arg(product_version), sqlc.arg(rental_product_id), sqlc.arg(product_version),
    sqlc.arg(guarantee_type), sqlc.arg(product_snapshot), sqlc.arg(product_snapshot),
    sqlc.arg(customer_id), sqlc.narg(invoice_customer_id), sqlc.arg(channel), sqlc.narg(external_reference),
    sqlc.arg(collecting_party), sqlc.arg(commission_amount), sqlc.arg(net_receivable), sqlc.arg(currency),
    sqlc.arg(total_amount), sqlc.arg(prepayment_amount), sqlc.narg(quote_id), sqlc.arg(quote_snapshot),
    sqlc.arg(settings_version), sqlc.arg(policy_snapshot), sqlc.narg(review_deadline), sqlc.narg(payment_deadline),
    sqlc.arg(note), sqlc.narg(eligibility_override), sqlc.narg(confirmed_at), sqlc.narg(created_by),
    sqlc.arg(approval_protocol), sqlc.narg(pickup_delivery), sqlc.narg(return_delivery),
    sqlc.narg(agreement_id), sqlc.arg(open_account)
)
RETURNING id;

-- name: GetReservationCore :one
-- The planning facts of one reservation (lifecycle decisions).
SELECT r.id, r.uuid, r.reference, r.status, r.payment_status, r.capacity_commitment, r.allocation_id,
    r.pickup_branch_id, r.return_branch_id, r.pickup_point_id, r.return_point_id, r.starts_at, r.ends_at,
    r.rental_product_id, r.product_version, r.sold_rental_product_id, r.sold_product_version, r.guarantee_type,
    r.customer_id, r.channel, r.total_amount, r.prepayment_amount, r.paid_amount, r.commission_amount,
    r.collecting_party, r.quote_id, r.quote_snapshot, r.settings_version, r.review_deadline, r.payment_deadline,
    r.assigned_user_id, r.currency, r.product_snapshot, r.equipment_allocations, r.approval_protocol,
    r.pickup_delivery, r.return_delivery, r.agreement_id, r.open_account,
    a.uuid AS allocation_uuid, a.status AS allocation_status, a.kind AS allocation_kind,
    a.vehicle_id AS allocation_vehicle_id, a.plate_fixed AS allocation_plate_fixed, a.expires_at AS allocation_expires_at,
    p.uuid AS product_uuid
FROM reservations r
JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
LEFT JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: LockReservation :one
-- Row lock of a transition (serialises concurrent actions on one
-- reservation inside the caller's transaction).
SELECT r.id, r.status, r.capacity_commitment, r.allocation_id
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id)
FOR UPDATE;

-- name: GetReservationByAllocation :one
SELECT r.id, r.uuid, r.reference, r.status, r.pickup_branch_id, r.return_branch_id
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.allocation_id = sqlc.arg(allocation_id);

-- name: GetReservationByAllocationUUID :one
SELECT r.id, r.uuid, r.reference, r.status, r.pickup_branch_id, r.return_branch_id
FROM reservations r
JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(allocation_uuid);

-- name: GetReservationView :one
-- One reservation with everything the detail page shows.
SELECT r.id, r.uuid, r.reference, r.status, r.payment_status, r.capacity_commitment,
    r.starts_at, r.ends_at, r.pickup_branch_id, r.return_branch_id,
    r.product_version, r.sold_product_version, r.guarantee_type, r.product_snapshot, r.sold_snapshot,
    r.channel, r.external_reference, r.collecting_party, r.commission_amount, r.net_receivable,
    r.currency, r.total_amount, r.prepayment_amount, r.paid_amount, r.quote_snapshot,
    r.settings_version, r.policy_snapshot, r.review_deadline, r.payment_deadline, r.note, r.eligibility_override,
    r.confirmed_at, r.cancelled_at, r.cancel_reason, r.created_at, r.updated_at, r.approval_protocol,
    r.pickup_delivery, r.return_delivery,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    pp.uuid AS pickup_point_uuid, pp.name AS pickup_point_name,
    rp.uuid AS return_point_uuid, rp.name AS return_point_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    sp.uuid AS sold_product_uuid, sp.code AS sold_product_code, sp.title AS sold_product_title,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title, c.identity_hint AS customer_identity_hint, c.phone AS customer_phone,
    c.email AS customer_email,
    ic.uuid AS invoice_customer_uuid, ic.customer_type AS invoice_customer_type,
    ic.first_name AS invoice_first_name, ic.last_name AS invoice_last_name, ic.company_title AS invoice_company_title,
    q.uuid AS quote_uuid,
    a.uuid AS allocation_uuid, a.kind AS allocation_kind, a.status AS allocation_status,
    a.plate_fixed AS allocation_plate_fixed, a.expires_at AS allocation_expires_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(COALESCE(v.manual_brand, '') || ' ' || COALESCE(v.manual_model, '')))::text AS vehicle_label,
    au.uuid AS assigned_user_uuid, au.name AS assigned_user_name, au.surname AS assigned_user_surname, au.email AS assigned_user_email,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    (SELECT COUNT(*) FROM resolution_tasks t
        WHERE t.organization_id = r.organization_id AND t.reservation_id = r.id AND t.status = 'open')::bigint AS open_tasks,
    r.open_account, ag.uuid AS agreement_uuid, ag.code AS agreement_code, ag.name AS agreement_name
FROM reservations r
LEFT JOIN corporate_agreements ag ON ag.organization_id = r.organization_id AND ag.id = r.agreement_id
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
LEFT JOIN service_points pp ON pp.organization_id = r.organization_id AND pp.id = r.pickup_point_id
LEFT JOIN service_points rp ON rp.organization_id = r.organization_id AND rp.id = r.return_point_id
JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
JOIN rental_products sp ON sp.organization_id = r.organization_id AND sp.id = r.sold_rental_product_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
LEFT JOIN customers ic ON ic.organization_id = r.organization_id AND ic.id = r.invoice_customer_id
LEFT JOIN quotes q ON q.organization_id = r.organization_id AND q.id = r.quote_id
LEFT JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
LEFT JOIN vehicles v ON v.organization_id = a.organization_id AND v.id = a.vehicle_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands tb ON tb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN users au ON au.id = r.assigned_user_id
LEFT JOIN users cu ON cu.id = r.created_by
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND sp.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: ListReservations :many
-- One page of reservations in the caller's branch scope (pick-up OR return
-- branch), with the list filters; total is the window count.
SELECT r.uuid, r.reference, r.status, r.payment_status, r.capacity_commitment, r.starts_at, r.ends_at,
    r.channel, r.external_reference, r.currency, r.total_amount, r.paid_amount, r.created_at, r.updated_at,
    r.review_deadline, r.payment_deadline, r.prepayment_amount, r.approval_protocol,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(a.plate_fixed AND a.status = 'active', FALSE)::boolean AS plate_fixed,
    au.uuid AS assigned_user_uuid, au.name AS assigned_user_name, au.surname AS assigned_user_surname,
    (SELECT COUNT(*) FROM resolution_tasks t
        WHERE t.organization_id = r.organization_id AND t.reservation_id = r.id AND t.status = 'open')::bigint AS open_tasks,
    COUNT(*) OVER ()::bigint AS total
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
LEFT JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id AND a.status = 'active'
LEFT JOIN vehicles v ON v.organization_id = a.organization_id AND v.id = a.vehicle_id
LEFT JOIN users au ON au.id = r.assigned_user_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(payment_statuses)::text[] IS NULL OR r.payment_status = ANY(sqlc.narg(payment_statuses)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR r.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR p.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(customer_uuids)::uuid[] IS NULL OR c.uuid = ANY(sqlc.narg(customer_uuids)::uuid[]))
  AND (
    sqlc.narg(branch_uuids)::uuid[] IS NULL
    OR pb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
    OR rb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
  )
  AND (sqlc.narg(assigned_user_uuids)::uuid[] IS NULL OR au.uuid = ANY(sqlc.narg(assigned_user_uuids)::uuid[]))
  AND (sqlc.narg(starts_from)::timestamptz IS NULL OR r.starts_at >= sqlc.narg(starts_from)::timestamptz)
  AND (sqlc.narg(starts_to)::timestamptz IS NULL OR r.starts_at < sqlc.narg(starts_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR r.external_reference ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'reference' AND NOT sqlc.arg(sort_desc)::boolean THEN r.reference END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'reference' AND sqlc.arg(sort_desc)::boolean THEN r.reference END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN r.status END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN r.status END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.starts_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND sqlc.arg(sort_desc)::boolean THEN r.starts_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'ends_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.ends_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'ends_at' AND sqlc.arg(sort_desc)::boolean THEN r.ends_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN r.total_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total_amount' AND sqlc.arg(sort_desc)::boolean THEN r.total_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'review_deadline' AND NOT sqlc.arg(sort_desc)::boolean THEN r.review_deadline END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'review_deadline' AND sqlc.arg(sort_desc)::boolean THEN r.review_deadline END DESC NULLS LAST,
    r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListReservationFacets :many
-- Facet counts over the caller's branch scope: (facet, value, label, count).
SELECT f.facet::text AS facet, f.value::text AS value, f.label::text AS label, COUNT(*)::bigint AS count
FROM (
    SELECT 'status' AS facet, r.status AS value, '' AS label, r.organization_id, r.pickup_branch_id, r.return_branch_id
    FROM reservations r WHERE r.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'payment_status', r.payment_status, '', r.organization_id, r.pickup_branch_id, r.return_branch_id
    FROM reservations r WHERE r.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'channel', r.channel, '', r.organization_id, r.pickup_branch_id, r.return_branch_id
    FROM reservations r WHERE r.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'product', p.uuid::text, p.code || ' · ' || p.title, r.organization_id, r.pickup_branch_id, r.return_branch_id
    FROM reservations r
    JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
    WHERE r.organization_id = sqlc.arg(organization_id) AND p.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'branch', b.uuid::text, b.code || ' · ' || b.name, r.organization_id, r.pickup_branch_id, r.return_branch_id
    FROM reservations r
    JOIN branches b ON b.organization_id = r.organization_id AND b.id IN (r.pickup_branch_id, r.return_branch_id)
    WHERE r.organization_id = sqlc.arg(organization_id) AND b.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'customer', c.uuid::text,
        CASE WHEN c.customer_type = 'corporate' THEN c.company_title ELSE btrim(c.first_name || ' ' || c.last_name) END,
        r.organization_id, r.pickup_branch_id, r.return_branch_id
    FROM reservations r
    JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
    WHERE r.organization_id = sqlc.arg(organization_id) AND c.organization_id = sqlc.arg(organization_id)
) f
WHERE f.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR f.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR f.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
GROUP BY f.facet, f.value, f.label
ORDER BY f.facet, COUNT(*) DESC, f.value
LIMIT 2000;

-- name: TransitionReservation :execrows
-- Moves the status (optimistic: only from the expected status).
UPDATE reservations
SET status = sqlc.arg(to_status),
    capacity_commitment = sqlc.arg(capacity_commitment),
    review_deadline = sqlc.narg(review_deadline),
    payment_deadline = sqlc.narg(payment_deadline),
    confirmed_at = COALESCE(confirmed_at, sqlc.narg(confirmed_at)),
    cancelled_at = COALESCE(sqlc.narg(cancelled_at), cancelled_at),
    cancel_reason = CASE WHEN sqlc.arg(cancel_reason)::text <> '' THEN sqlc.arg(cancel_reason)::text ELSE cancel_reason END,
    allocation_id = COALESCE(sqlc.narg(allocation_id), allocation_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = sqlc.arg(from_status);

-- name: UpdateReservationPlan :execrows
-- Dates / places / product / price after a change, an extension or an
-- upgrade (the sold product columns never change).
UPDATE reservations
SET allocation_id = sqlc.narg(allocation_id),
    starts_at = sqlc.arg(starts_at),
    ends_at = sqlc.arg(ends_at),
    pickup_branch_id = sqlc.arg(pickup_branch_id),
    return_branch_id = sqlc.arg(return_branch_id),
    pickup_point_id = sqlc.narg(pickup_point_id),
    return_point_id = sqlc.narg(return_point_id),
    pickup_delivery = sqlc.narg(pickup_delivery),
    return_delivery = sqlc.narg(return_delivery),
    rental_product_id = sqlc.arg(rental_product_id),
    product_version = sqlc.arg(product_version),
    guarantee_type = sqlc.arg(guarantee_type),
    product_snapshot = sqlc.arg(product_snapshot),
    total_amount = sqlc.arg(total_amount),
    prepayment_amount = sqlc.arg(prepayment_amount),
    net_receivable = sqlc.arg(net_receivable),
    quote_id = sqlc.narg(quote_id),
    quote_snapshot = sqlc.arg(quote_snapshot)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetReservationAssignee :execrows
UPDATE reservations
SET assigned_user_id = sqlc.narg(assigned_user_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetReservationEquipment :exec
UPDATE reservations
SET equipment_allocations = sqlc.arg(equipment_allocations)::uuid[]
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetReservationPayment :execrows
-- Payment progress (M3 payments call it through PaymentStatusSetter).
UPDATE reservations
SET payment_status = sqlc.arg(payment_status), paid_amount = sqlc.arg(paid_amount)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: InsertReservationEvent :exec
INSERT INTO reservation_events (organization_id, reservation_id, action, from_status, to_status, reason, data, actor_id)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(action), sqlc.narg(from_status),
    sqlc.narg(to_status), sqlc.arg(reason), sqlc.arg(data), sqlc.narg(actor_id)
);

-- name: ListReservationEvents :many
SELECT e.uuid, e.action, e.from_status, e.to_status, e.reason, e.data, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM reservation_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.reservation_id = sqlc.arg(reservation_id)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountReservationEvents :one
SELECT COUNT(*)::bigint FROM reservation_events e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.reservation_id = sqlc.arg(reservation_id);

-- name: InsertReservationDriver :exec
INSERT INTO reservation_drivers (
    organization_id, reservation_id, driver_id, role, snapshot, eligible, reasons, override_reason, overridden_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(driver_id), sqlc.arg(role), sqlc.arg(snapshot),
    sqlc.arg(eligible), sqlc.arg(reasons), sqlc.arg(override_reason), sqlc.narg(overridden_by)
);

-- name: ListReservationDrivers :many
SELECT rd.role, rd.snapshot, rd.eligible, rd.reasons, rd.override_reason, rd.created_at,
    d.uuid AS driver_uuid, d.first_name, d.last_name, d.licence_hint, d.licence_class, d.licence_expires_on,
    c.uuid AS customer_uuid
FROM reservation_drivers rd
LEFT JOIN drivers d ON d.organization_id = rd.organization_id AND d.id = rd.driver_id
LEFT JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND rd.reservation_id = sqlc.arg(reservation_id)
ORDER BY CASE rd.role WHEN 'primary' THEN 0 ELSE 1 END, rd.id;

-- name: ListReservationDriverIDs :many
SELECT rd.driver_id::bigint AS driver_id
FROM reservation_drivers rd
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND rd.reservation_id = sqlc.arg(reservation_id)
  AND rd.driver_id IS NOT NULL;

-- name: InsertVehicleAssignment :exec
INSERT INTO reservation_vehicle_assignments (
    organization_id, reservation_id, vehicle_id, rental_product_id, product_version, kind, source, note, actor_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.narg(vehicle_id), sqlc.arg(rental_product_id),
    sqlc.arg(product_version), sqlc.arg(kind), sqlc.arg(source), sqlc.arg(note), sqlc.narg(actor_id)
);

-- name: EndVehicleAssignments :exec
UPDATE reservation_vehicle_assignments
SET ended_at = clock_timestamp()
WHERE organization_id = sqlc.arg(organization_id)
  AND reservation_id = sqlc.arg(reservation_id)
  AND ended_at IS NULL;

-- name: ListVehicleAssignments :many
SELECT va.kind, va.source, va.note, va.started_at, va.ended_at, va.product_version,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM reservation_vehicle_assignments va
JOIN rental_products p ON p.organization_id = va.organization_id AND p.id = va.rental_product_id
LEFT JOIN vehicles v ON v.organization_id = va.organization_id AND v.id = va.vehicle_id
LEFT JOIN users u ON u.id = va.actor_id
WHERE va.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND va.reservation_id = sqlc.arg(reservation_id)
ORDER BY va.started_at DESC, va.id DESC;

-- name: GetReservationVehicleByID :one
SELECT v.id, v.uuid, v.plate
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.id = sqlc.arg(id);

-- name: GetReservationVehicleByUUID :one
SELECT v.id, v.uuid, v.plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(COALESCE(v.manual_brand, '') || ' ' || COALESCE(v.manual_model, '')))::text AS label
FROM vehicles v
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands tb ON tb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: ListReservationTargetUUIDs :many
-- Bulk "all matching": the uuids of the list filter in the branch scope.
SELECT r.uuid
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN branches rb ON rb.organization_id = r.organization_id AND rb.id = r.return_branch_id
JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(payment_statuses)::text[] IS NULL OR r.payment_status = ANY(sqlc.narg(payment_statuses)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR r.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR p.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(customer_uuids)::uuid[] IS NULL OR c.uuid = ANY(sqlc.narg(customer_uuids)::uuid[]))
  AND (
    sqlc.narg(branch_uuids)::uuid[] IS NULL
    OR pb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
    OR rb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
  )
  AND (sqlc.narg(starts_from)::timestamptz IS NULL OR r.starts_at >= sqlc.narg(starts_from)::timestamptz)
  AND (sqlc.narg(starts_to)::timestamptz IS NULL OR r.starts_at < sqlc.narg(starts_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR r.external_reference ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListDueReservationOrganizations :many
-- Organizations with waiting reservations past their deadline (sweep).
SELECT DISTINCT r.organization_id
FROM reservations r
WHERE (r.status = 'awaiting_review' AND r.review_deadline <= sqlc.arg(now_at)::timestamptz)
   OR (r.status = 'awaiting_payment' AND r.payment_deadline <= sqlc.arg(now_at)::timestamptz)
LIMIT 1000;

-- name: ListDueReservations :many
SELECT r.uuid
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (
    (r.status = 'awaiting_review' AND r.review_deadline <= sqlc.arg(now_at)::timestamptz)
    OR (r.status = 'awaiting_payment' AND r.payment_deadline <= sqlc.arg(now_at)::timestamptz)
  )
ORDER BY r.id
LIMIT 500;

-- name: GetReservationProductVersion :one
-- Guarantee and conditions of a product version (reservation snapshot).
SELECT pv.id, pv.uuid, pv.version, pv.guarantee_type, pv.guarantee_label, pv.deposit_amount, pv.min_driver_age,
    pv.min_license_years, pv.included_km_per_day, pv.extra_km_fee, pv.fuel_policy,
    p.id AS product_id, p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    p.status AS product_status, p.is_active AS product_is_active, p.current_version, p.segment_id
FROM rental_product_versions pv
JOIN rental_products p ON p.organization_id = pv.organization_id AND p.id = pv.product_id
WHERE pv.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND pv.product_id = sqlc.arg(product_id)
  AND pv.version = sqlc.arg(version);

-- name: GetReservationProductByUUID :one
SELECT p.id, p.uuid, p.code, p.title, p.status, p.is_active, p.current_version, p.segment_id
FROM rental_products p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: GetReservationBranchByUUID :one
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: GetReservationPointByUUID :one
SELECT sp.id, sp.uuid, sp.name, sp.branch_id
FROM service_points sp
WHERE sp.organization_id = sqlc.arg(organization_id)
  AND sp.uuid = sqlc.arg(uuid);

-- name: GetReservationCustomer :one
-- A live customer of the organization (contract party / invoice addressee).
SELECT c.id, c.uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.identity_type,
    c.identity_hint, c.phone, c.email, c.anonymized_at
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: GetReservationDriver :one
-- A live driver of the organization with the facts of the condition check.
SELECT d.id, d.uuid, d.customer_id, d.first_name, d.last_name, d.birth_date, d.licence_enc, d.licence_hint,
    d.licence_class, d.licence_issued_on, d.licence_expires_on
FROM drivers d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid)
  AND d.deleted_at IS NULL;

-- name: DeleteReservationDrivers :exec
DELETE FROM reservation_drivers
WHERE organization_id = sqlc.arg(organization_id)
  AND reservation_id = sqlc.arg(reservation_id);

-- name: DeleteReservation :execrows
-- Compensation of a create whose follow-up (equipment) failed: the row
-- never became visible as a sale.
DELETE FROM reservations
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetReservationUserInOrganization :one
-- A member of the organization (staff assignment, task assignee).
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.uuid = sqlc.arg(uuid)
  AND u.deleted_at IS NULL;

-- name: ReinstateReservation :execrows
-- A late payment re-secured capacity for an expired reservation (TEC-708):
-- expired → confirmed on the new firm allocation; the cancellation facts
-- of the expiry are cleared (the timeline keeps them).
UPDATE reservations
SET status = 'confirmed',
    capacity_commitment = 'firm',
    allocation_id = sqlc.arg(allocation_id),
    review_deadline = NULL,
    payment_deadline = NULL,
    confirmed_at = sqlc.arg(confirmed_at),
    cancelled_at = NULL,
    cancel_reason = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'expired';

