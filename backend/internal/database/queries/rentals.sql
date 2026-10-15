-- Rentals (TEC-709). Every query filters by organization_id; branch scope
-- (planned pick-up OR planned return OR actual return branch) is applied by
-- the list queries and checked by the use case on single rows.

-- name: InsertRental :one
INSERT INTO rentals (
    organization_id, reference, reservation_id, status, vehicle_id, allocation_id, settings_version,
    policy_snapshot, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(reference), sqlc.arg(reservation_id), 'preparation', sqlc.arg(vehicle_id),
    sqlc.narg(allocation_id), sqlc.arg(settings_version), sqlc.arg(policy_snapshot), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: CopyReservationDriversToRental :exec
INSERT INTO rental_drivers (organization_id, rental_id, driver_id, role, snapshot)
SELECT rd.organization_id, sqlc.arg(rental_id), rd.driver_id, rd.role, rd.snapshot
FROM reservation_drivers rd
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND rd.reservation_id = sqlc.arg(reservation_id)
ORDER BY CASE rd.role WHEN 'primary' THEN 0 ELSE 1 END, rd.id;

-- name: DeleteRentalDrivers :exec
DELETE FROM rental_drivers
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id);

-- name: GetRentalReservationFacts :one
-- The reservation side a rental is created from / acts on.
SELECT res.id, res.uuid, res.reference, res.status, res.starts_at, res.ends_at, res.pickup_branch_id,
    res.return_branch_id, res.pickup_point_id, res.return_point_id, res.settings_version, res.policy_snapshot,
    res.allocation_id, res.customer_id,
    a.uuid AS allocation_uuid, a.kind AS allocation_kind, a.status AS allocation_status,
    a.vehicle_id AS allocation_vehicle_id, a.plate_fixed AS allocation_plate_fixed
FROM reservations res
LEFT JOIN vehicle_allocations a ON a.organization_id = res.organization_id AND a.id = res.allocation_id
WHERE res.organization_id = sqlc.arg(organization_id)
  AND res.uuid = sqlc.arg(uuid);

-- name: GetRentalCore :one
-- The lifecycle facts of one rental (decisions; no display joins).
SELECT r.id, r.uuid, r.reference, r.status, r.reservation_id, r.vehicle_id, r.allocation_id, r.actual_pickup_at,
    r.actual_return_at, r.actual_return_branch_id, r.overdue_at, r.late_minutes, r.settings_version,
    r.policy_snapshot, r.created_at,
    res.uuid AS reservation_uuid, res.status AS reservation_status, res.starts_at AS planned_pickup_at,
    res.ends_at AS planned_return_at, res.pickup_branch_id AS planned_pickup_branch_id,
    res.return_branch_id AS planned_return_branch_id, res.pickup_point_id AS planned_pickup_point_id,
    res.allocation_id AS reservation_allocation_id, res.currency,
    a.uuid AS allocation_uuid, a.kind AS allocation_kind, a.status AS allocation_status,
    a.vehicle_id AS allocation_vehicle_id, a.ends_at AS allocation_ends_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate, v.odometer_km AS vehicle_odometer_km,
    v.status AS vehicle_status
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN vehicles v ON v.organization_id = r.organization_id AND v.id = r.vehicle_id
LEFT JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: GetLiveRentalUUIDByReservation :one
SELECT r.uuid
FROM rentals r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.reservation_id = sqlc.arg(reservation_id)
  AND r.status <> 'cancelled';

-- name: GetRentalView :one
-- One rental with everything the detail page shows.
SELECT r.id, r.uuid, r.reference, r.status, r.actual_pickup_at, r.actual_return_at, r.overdue_at, r.late_minutes,
    r.settings_version, r.policy_snapshot, r.contract_waiver, r.return_note, r.damage_note, r.note, r.cancel_reason,
    r.closed_at, r.checked_at, r.cancelled_at, r.created_at, r.updated_at,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference, res.status AS reservation_status,
    res.starts_at AS planned_pickup_at, res.ends_at AS planned_return_at, res.currency, res.total_amount,
    res.paid_amount, res.payment_status,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    pp.uuid AS pickup_point_uuid, pp.name AS pickup_point_name,
    rp.uuid AS return_point_uuid, rp.name AS return_point_name,
    apb.uuid AS actual_pickup_branch_uuid, apb.code AS actual_pickup_branch_code, apb.name AS actual_pickup_branch_name,
    arb.uuid AS actual_return_branch_uuid, arb.code AS actual_return_branch_code, arb.name AS actual_return_branch_name,
    app.uuid AS actual_pickup_point_uuid, app.name AS actual_pickup_point_name,
    arp.uuid AS actual_return_point_uuid, arp.name AS actual_return_point_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(COALESCE(v.manual_brand, '') || ' ' || COALESCE(v.manual_model, '')))::text AS vehicle_label,
    a.uuid AS allocation_uuid, a.kind AS allocation_kind, a.status AS allocation_status, a.ends_at AS allocation_ends_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    du.uuid AS delivered_by_uuid, du.name AS delivered_by_name, du.surname AS delivered_by_surname, du.email AS delivered_by_email,
    ru.uuid AS returned_by_uuid, ru.name AS returned_by_name, ru.surname AS returned_by_surname, ru.email AS returned_by_email
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN branches pb ON pb.organization_id = res.organization_id AND pb.id = res.pickup_branch_id
JOIN branches rb ON rb.organization_id = res.organization_id AND rb.id = res.return_branch_id
LEFT JOIN service_points pp ON pp.organization_id = res.organization_id AND pp.id = res.pickup_point_id
LEFT JOIN service_points rp ON rp.organization_id = res.organization_id AND rp.id = res.return_point_id
LEFT JOIN branches apb ON apb.organization_id = r.organization_id AND apb.id = r.actual_pickup_branch_id
LEFT JOIN branches arb ON arb.organization_id = r.organization_id AND arb.id = r.actual_return_branch_id
LEFT JOIN service_points app ON app.organization_id = r.organization_id AND app.id = r.actual_pickup_point_id
LEFT JOIN service_points arp ON arp.organization_id = r.organization_id AND arp.id = r.actual_return_point_id
JOIN rental_products p ON p.organization_id = res.organization_id AND p.id = res.rental_product_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
JOIN vehicles v ON v.organization_id = r.organization_id AND v.id = r.vehicle_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands tb ON tb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
LEFT JOIN users cu ON cu.id = r.created_by
LEFT JOIN users du ON du.id = r.delivered_by
LEFT JOIN users ru ON ru.id = r.returned_by
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: GetRentalScope :one
-- Branches that decide the visibility of a rental.
SELECT res.pickup_branch_id, res.return_branch_id, COALESCE(r.actual_return_branch_id, 0)::bigint AS actual_return_branch_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: ListRentals :many
-- One page of rentals in the caller's branch scope with the list filters;
-- total is the window count.
SELECT r.uuid, r.reference, r.status, r.actual_pickup_at, r.actual_return_at, r.overdue_at, r.late_minutes,
    r.created_at, r.updated_at,
    res.uuid AS reservation_uuid, res.starts_at AS planned_pickup_at, res.ends_at AS planned_return_at,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    arb.uuid AS actual_return_branch_uuid, arb.code AS actual_return_branch_code, arb.name AS actual_return_branch_name,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    (SELECT COUNT(*) FROM rental_charges ch
        WHERE ch.organization_id = r.organization_id AND ch.rental_id = r.id AND ch.status = 'proposed')::bigint AS open_charges,
    COUNT(*) OVER ()::bigint AS total
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN branches pb ON pb.organization_id = res.organization_id AND pb.id = res.pickup_branch_id
JOIN branches rb ON rb.organization_id = res.organization_id AND rb.id = res.return_branch_id
LEFT JOIN branches arb ON arb.organization_id = r.organization_id AND arb.id = r.actual_return_branch_id
JOIN rental_products p ON p.organization_id = res.organization_id AND p.id = res.rental_product_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
JOIN vehicles v ON v.organization_id = r.organization_id AND v.id = r.vehicle_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(branch_uuids)::uuid[] IS NULL
    OR pb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
    OR rb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
    OR arb.uuid = ANY(sqlc.narg(branch_uuids)::uuid[])
  )
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR p.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(vehicle_uuids)::uuid[] IS NULL OR v.uuid = ANY(sqlc.narg(vehicle_uuids)::uuid[]))
  AND (sqlc.narg(customer_uuids)::uuid[] IS NULL OR c.uuid = ANY(sqlc.narg(customer_uuids)::uuid[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR res.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (NOT sqlc.arg(overdue_only)::boolean OR r.overdue_at IS NOT NULL)
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'reference' AND NOT sqlc.arg(sort_desc)::boolean THEN r.reference END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'reference' AND sqlc.arg(sort_desc)::boolean THEN r.reference END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN r.status END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN r.status END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'planned_pickup_at' AND NOT sqlc.arg(sort_desc)::boolean THEN res.starts_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'planned_pickup_at' AND sqlc.arg(sort_desc)::boolean THEN res.starts_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'planned_return_at' AND NOT sqlc.arg(sort_desc)::boolean THEN res.ends_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'planned_return_at' AND sqlc.arg(sort_desc)::boolean THEN res.ends_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
    r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListRentalFacets :many
-- Facet counts over the caller's branch scope: (facet, value, label, count).
SELECT f.facet::text AS facet, f.value::text AS value, f.label::text AS label, COUNT(*)::bigint AS count
FROM (
    SELECT 'status' AS facet, r.status AS value, '' AS label, res.pickup_branch_id, res.return_branch_id,
        r.actual_return_branch_id
    FROM rentals r
    JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    WHERE r.organization_id = sqlc.arg(organization_id) AND res.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'branch', b.uuid::text, b.code || ' · ' || b.name, res.pickup_branch_id, res.return_branch_id,
        r.actual_return_branch_id
    FROM rentals r
    JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    JOIN branches b ON b.organization_id = res.organization_id
        AND b.id IN (res.pickup_branch_id, res.return_branch_id, COALESCE(r.actual_return_branch_id, 0))
    WHERE r.organization_id = sqlc.arg(organization_id) AND res.organization_id = sqlc.arg(organization_id)
      AND b.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'product', p.uuid::text, p.code || ' · ' || p.title, res.pickup_branch_id, res.return_branch_id,
        r.actual_return_branch_id
    FROM rentals r
    JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    JOIN rental_products p ON p.organization_id = res.organization_id AND p.id = res.rental_product_id
    WHERE r.organization_id = sqlc.arg(organization_id) AND res.organization_id = sqlc.arg(organization_id)
      AND p.organization_id = sqlc.arg(organization_id)
) f
WHERE (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR f.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR f.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR f.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
)
GROUP BY f.facet, f.value, f.label
ORDER BY f.facet, COUNT(*) DESC, f.value
LIMIT 2000;

-- name: SetRentalHandover :execrows
UPDATE rentals
SET status = 'active', actual_pickup_at = sqlc.arg(actual_pickup_at), actual_pickup_branch_id = sqlc.arg(branch_id),
    actual_pickup_point_id = sqlc.narg(point_id), delivered_by = sqlc.narg(actor_id),
    contract_waiver = sqlc.narg(contract_waiver)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'preparation';

-- name: SetRentalReturn :execrows
UPDATE rentals
SET status = 'return_check', actual_return_at = sqlc.arg(actual_return_at), actual_return_branch_id = sqlc.arg(branch_id),
    actual_return_point_id = sqlc.narg(point_id), late_minutes = sqlc.arg(late_minutes), returned_by = sqlc.narg(actor_id),
    return_note = sqlc.arg(return_note), damage_note = sqlc.arg(damage_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: SetRentalChecked :execrows
UPDATE rentals
SET status = 'returned', checked_by = sqlc.narg(actor_id), checked_at = sqlc.arg(checked_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'return_check';

-- name: CancelRental :execrows
UPDATE rentals
SET status = 'cancelled', cancelled_at = sqlc.arg(cancelled_at), cancel_reason = sqlc.arg(cancel_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'preparation';

-- name: SetRentalVehicle :execrows
-- Current vehicle / allocation (plate change before the handover, swap).
UPDATE rentals
SET vehicle_id = sqlc.arg(vehicle_id), allocation_id = sqlc.narg(allocation_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('preparation', 'active');

-- name: SetRentalOverdue :execrows
UPDATE rentals
SET overdue_at = COALESCE(overdue_at, sqlc.arg(overdue_at)::timestamptz)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: ClearRentalOverdue :execrows
-- An extension past now ends the overdue state.
UPDATE rentals
SET overdue_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'active';

-- name: SetReservationAllocationForRental :execrows
-- Swap: the reservation follows the rental to the new allocation.
UPDATE reservations
SET allocation_id = sqlc.arg(new_allocation_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND allocation_id = sqlc.arg(old_allocation_id);

-- name: InsertRentalEvent :exec
INSERT INTO rental_events (organization_id, rental_id, action, from_status, to_status, reason, data, actor_id)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(action), sqlc.narg(from_status), sqlc.narg(to_status),
    sqlc.arg(reason), sqlc.arg(data), sqlc.narg(actor_id)
);

-- name: ListRentalEvents :many
SELECT e.uuid, e.action, e.from_status, e.to_status, e.reason, e.data, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM rental_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.rental_id = sqlc.arg(rental_id)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRentalEvents :one
SELECT COUNT(*)::bigint FROM rental_events e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.rental_id = sqlc.arg(rental_id);

-- name: InsertRentalVehicle :one
INSERT INTO rental_vehicles (
    organization_id, rental_id, vehicle_id, allocation_id, reason, out_at, out_branch_id, out_km, out_fuel, note
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(vehicle_id), sqlc.narg(allocation_id), sqlc.arg(reason),
    sqlc.arg(out_at), sqlc.arg(out_branch_id), sqlc.arg(out_km), sqlc.narg(out_fuel), sqlc.arg(note)
)
RETURNING id, uuid;

-- name: CloseRentalVehicleLeg :execrows
-- Return of the current leg (the open one of the vehicle).
UPDATE rental_vehicles
SET in_at = sqlc.arg(in_at), in_branch_id = sqlc.arg(in_branch_id), in_km = sqlc.arg(in_km), in_fuel = sqlc.narg(in_fuel)
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND vehicle_id = sqlc.arg(vehicle_id)
  AND in_at IS NULL;

-- name: ListRentalVehicles :many
SELECT rv.uuid, rv.reason, rv.out_at, rv.out_km, rv.out_fuel, rv.in_at, rv.in_km, rv.in_fuel, rv.note, rv.created_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(COALESCE(v.manual_brand, '') || ' ' || COALESCE(v.manual_model, '')))::text AS vehicle_label,
    ob.uuid AS out_branch_uuid, ob.code AS out_branch_code, ob.name AS out_branch_name,
    ib.uuid AS in_branch_uuid, ib.code AS in_branch_code, ib.name AS in_branch_name
FROM rental_vehicles rv
JOIN vehicles v ON v.organization_id = rv.organization_id AND v.id = rv.vehicle_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands tb ON tb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
LEFT JOIN branches ob ON ob.organization_id = rv.organization_id AND ob.id = rv.out_branch_id
LEFT JOIN branches ib ON ib.organization_id = rv.organization_id AND ib.id = rv.in_branch_id
WHERE rv.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND rv.rental_id = sqlc.arg(rental_id)
ORDER BY rv.id;

-- name: ListRentalDrivers :many
SELECT rd.role, rd.snapshot,
    d.uuid AS driver_uuid, d.first_name, d.last_name, d.licence_hint, d.licence_class, d.licence_expires_on
FROM rental_drivers rd
LEFT JOIN drivers d ON d.organization_id = rd.organization_id AND d.id = rd.driver_id
WHERE rd.organization_id = sqlc.arg(organization_id)
  AND rd.rental_id = sqlc.arg(rental_id)
ORDER BY CASE rd.role WHEN 'primary' THEN 0 ELSE 1 END, rd.id;

-- name: InsertRentalCharge :one
INSERT INTO rental_charges (
    organization_id, rental_id, kind, source, currency, amount, settings_version, calculation, reason, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(kind), sqlc.arg(source), sqlc.arg(currency),
    sqlc.arg(amount), sqlc.arg(settings_version), sqlc.arg(calculation), sqlc.arg(reason), sqlc.narg(created_by)
)
RETURNING uuid;

-- name: DeleteProposedPolicyLateCharge :exec
-- A re-computed late fee replaces the proposal (approved fees stay).
DELETE FROM rental_charges
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND kind = 'late_return'
  AND source = 'policy'
  AND status = 'proposed';

-- name: CountLivePolicyLateCharges :one
SELECT COUNT(*)::bigint
FROM rental_charges ch
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.rental_id = sqlc.arg(rental_id)
  AND ch.kind = 'late_return'
  AND ch.source = 'policy'
  AND ch.status <> 'waived';

-- name: ListRentalCharges :many
SELECT ch.uuid, ch.kind, ch.source, ch.status, ch.currency, ch.amount, ch.settings_version, ch.calculation,
    ch.reason, ch.decided_at, ch.decision_note, ch.created_at, ch.disputed, ch.original_amount, ch.override_reason,
    ch.overridden_at,
    ins.uuid AS inspection_uuid,
    du.uuid AS decided_by_uuid, du.name AS decided_by_name, du.surname AS decided_by_surname, du.email AS decided_by_email,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email,
    ou.uuid AS overridden_by_uuid, ou.name AS overridden_by_name, ou.surname AS overridden_by_surname, ou.email AS overridden_by_email
FROM rental_charges ch
LEFT JOIN inspections ins ON ins.organization_id = ch.organization_id AND ins.id = ch.inspection_id
LEFT JOIN users du ON du.id = ch.decided_by
LEFT JOIN users cu ON cu.id = ch.created_by
LEFT JOIN users ou ON ou.id = ch.overridden_by
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.rental_id = sqlc.arg(rental_id)
ORDER BY ch.created_at, ch.id;

-- name: GetRentalCharge :one
SELECT ch.id, ch.uuid, ch.kind, ch.source, ch.status, ch.amount, ch.disputed, ch.original_amount, ch.inspection_id
FROM rental_charges ch
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.rental_id = sqlc.arg(rental_id)
  AND ch.uuid = sqlc.arg(uuid);

-- name: DecideRentalCharge :execrows
UPDATE rental_charges
SET status = sqlc.arg(status), decided_by = sqlc.narg(decided_by), decided_at = sqlc.arg(decided_at),
    decision_note = sqlc.arg(decision_note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'proposed';

-- name: OverrideRentalCharge :execrows
-- A changed amount of a proposed charge; the first override keeps the
-- computed amount in original_amount.
UPDATE rental_charges
SET original_amount = COALESCE(original_amount, amount), amount = sqlc.arg(amount),
    override_reason = sqlc.arg(override_reason), overridden_by = sqlc.narg(overridden_by),
    overridden_at = sqlc.arg(overridden_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'proposed';

-- name: CountPendingInspectionReviews :one
-- Return forms of the rental that wait for an authorized review (dispute
-- or missing signature): the return check cannot finish before.
SELECT COUNT(*)::bigint
FROM inspections i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.rental_id = sqlc.arg(rental_id)
  AND i.review_status = 'pending'
  AND i.status = 'finalized';

-- name: ListOverdueRentalOrganizations :many
-- Organizations with active rentals whose vehicle should be back by now
-- (planned return + late tolerance passed and the engine allocation ended).
SELECT DISTINCT r.organization_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
WHERE r.status = 'active'
  AND a.status = 'active'
  AND a.ends_at <= sqlc.arg(now_at)::timestamptz
  AND res.ends_at + make_interval(mins => COALESCE(NULLIF(r.policy_snapshot -> 'policies' ->> 'late_return_grace_minutes', '')::int, 0))
      <= sqlc.arg(now_at)::timestamptz
LIMIT 1000;

-- name: ListOverdueRentals :many
SELECT r.uuid
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND r.status = 'active'
  AND a.status = 'active'
  AND a.ends_at <= sqlc.arg(now_at)::timestamptz
  AND res.ends_at + make_interval(mins => COALESCE(NULLIF(r.policy_snapshot -> 'policies' ->> 'late_return_grace_minutes', '')::int, 0))
      <= sqlc.arg(now_at)::timestamptz
ORDER BY r.id
LIMIT 500;

-- name: GetRentalVehicleByUUID :one
-- A live vehicle of the organization (swap target).
SELECT v.id, v.uuid, v.plate, v.odometer_km, v.status, v.current_branch_id, v.is_active
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: GetRentalBranch :one
-- A live branch of the organization by uuid (with its preparation time).
SELECT b.id, b.uuid, b.code, b.name, b.prep_minutes
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: GetRentalBranchByID :one
SELECT b.id, b.uuid, b.code, b.name, b.prep_minutes
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.id = sqlc.arg(id);
