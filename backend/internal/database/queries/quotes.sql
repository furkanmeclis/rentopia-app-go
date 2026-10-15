-- Quotes (TEC-704): priced search answers; consume no capacity. Every
-- query filters by organization_id first; lists apply the caller's branch
-- scope (pick-up OR return branch).

-- name: InsertQuote :one
INSERT INTO quotes (
    organization_id, purpose, reservation_id, customer_id, rental_product_id, product_version,
    pickup_branch_id, return_branch_id, pickup_point_id, return_point_id, starts_at, ends_at, channel, billing_days,
    currency, rental_subtotal, discount_total, rental_total, extras_total, one_way_fee, delivery_fee,
    base_total, vat_total, total, prepayment, prepayment_rule, vat_rate_bp, difference,
    rate_plan_version_uuid, rate_plan_version, settings_version, extras, coupon_code, manual_discount,
    breakdown, vehicle_id, valid_until, created_by, pickup_delivery, return_delivery, delivery_override
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(purpose), sqlc.narg(reservation_id), sqlc.narg(customer_id),
    sqlc.arg(rental_product_id), sqlc.arg(product_version),
    sqlc.arg(pickup_branch_id), sqlc.arg(return_branch_id), sqlc.narg(pickup_point_id), sqlc.narg(return_point_id),
    sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.arg(channel), sqlc.arg(billing_days),
    sqlc.arg(currency), sqlc.arg(rental_subtotal), sqlc.arg(discount_total), sqlc.arg(rental_total),
    sqlc.arg(extras_total), sqlc.arg(one_way_fee), sqlc.arg(delivery_fee),
    sqlc.arg(base_total), sqlc.arg(vat_total), sqlc.arg(total), sqlc.arg(prepayment), sqlc.arg(prepayment_rule),
    sqlc.narg(vat_rate_bp), sqlc.narg(difference),
    sqlc.narg(rate_plan_version_uuid), sqlc.narg(rate_plan_version), sqlc.arg(settings_version),
    sqlc.arg(extras), sqlc.narg(coupon_code), sqlc.narg(manual_discount),
    sqlc.arg(breakdown), sqlc.narg(vehicle_id), sqlc.arg(valid_until), sqlc.narg(created_by),
    sqlc.narg(pickup_delivery), sqlc.narg(return_delivery), sqlc.narg(delivery_override)
)
RETURNING id, uuid;

-- name: InsertQuoteLine :exec
INSERT INTO quote_lines (
    organization_id, quote_id, position, kind, day_index, day_date, date_rule_version_uuid, date_rule_name,
    code, name, quantity, unit_price, adjustment, amount
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(quote_id), sqlc.arg(position), sqlc.arg(kind), sqlc.narg(day_index),
    sqlc.narg(day_date), sqlc.narg(date_rule_version_uuid), sqlc.arg(date_rule_name), sqlc.arg(code), sqlc.arg(name),
    sqlc.arg(quantity), sqlc.arg(unit_price), sqlc.arg(adjustment), sqlc.arg(amount)
);

-- name: GetQuote :one
SELECT q.id, q.uuid, q.purpose, q.status, q.reservation_id, q.customer_id, q.rental_product_id, q.product_version,
    q.pickup_branch_id, q.return_branch_id, q.pickup_point_id, q.return_point_id, q.starts_at, q.ends_at,
    q.channel, q.billing_days, q.currency, q.rental_subtotal, q.discount_total, q.rental_total, q.extras_total,
    q.one_way_fee, q.delivery_fee, q.base_total, q.vat_total, q.total, q.prepayment, q.prepayment_rule,
    q.vat_rate_bp, q.difference, q.rate_plan_version_uuid, q.rate_plan_version, q.settings_version, q.extras,
    q.coupon_code, q.manual_discount, q.breakdown, q.vehicle_id, q.valid_until, q.used_at, q.created_at,
    q.pickup_delivery, q.return_delivery, q.delivery_override,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    pp.uuid AS pickup_point_uuid, pp.name AS pickup_point_name,
    rp.uuid AS return_point_uuid, rp.name AS return_point_name,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM quotes q
JOIN rental_products p ON p.organization_id = q.organization_id AND p.id = q.rental_product_id
JOIN branches pb ON pb.organization_id = q.organization_id AND pb.id = q.pickup_branch_id
JOIN branches rb ON rb.organization_id = q.organization_id AND rb.id = q.return_branch_id
LEFT JOIN service_points pp ON pp.organization_id = q.organization_id AND pp.id = q.pickup_point_id
LEFT JOIN service_points rp ON rp.organization_id = q.organization_id AND rp.id = q.return_point_id
LEFT JOIN customers c ON c.organization_id = q.organization_id AND c.id = q.customer_id
LEFT JOIN reservations r ON r.organization_id = q.organization_id
    AND (r.id = q.reservation_id OR (q.reservation_id IS NULL AND r.quote_id = q.id))
LEFT JOIN vehicles v ON v.organization_id = q.organization_id AND v.id = q.vehicle_id
LEFT JOIN users cu ON cu.id = q.created_by
WHERE q.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND q.uuid = sqlc.arg(uuid);

-- name: ListQuoteLines :many
SELECT ql.position, ql.kind, ql.day_index, ql.day_date, ql.date_rule_version_uuid, ql.date_rule_name,
    ql.code, ql.name, ql.quantity, ql.unit_price, ql.adjustment, ql.amount
FROM quote_lines ql
WHERE ql.organization_id = sqlc.arg(organization_id)
  AND ql.quote_id = sqlc.arg(quote_id)
ORDER BY ql.position;

-- name: ListQuotes :many
-- One page of quotes in the caller's branch scope, newest first.
SELECT q.uuid, q.purpose, q.status, q.starts_at, q.ends_at, q.channel, q.billing_days, q.currency, q.total,
    q.difference, q.valid_until, q.created_at,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    c.uuid AS customer_uuid, c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    COUNT(*) OVER ()::bigint AS total_count
FROM quotes q
JOIN rental_products p ON p.organization_id = q.organization_id AND p.id = q.rental_product_id
JOIN branches pb ON pb.organization_id = q.organization_id AND pb.id = q.pickup_branch_id
JOIN branches rb ON rb.organization_id = q.organization_id AND rb.id = q.return_branch_id
LEFT JOIN customers c ON c.organization_id = q.organization_id AND c.id = q.customer_id
LEFT JOIN reservations r ON r.organization_id = q.organization_id
    AND (r.id = q.reservation_id OR (q.reservation_id IS NULL AND r.quote_id = q.id))
WHERE q.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR q.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR q.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(purposes)::text[] IS NULL OR q.purpose = ANY(sqlc.narg(purposes)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR q.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(product_uuids)::uuid[] IS NULL OR p.uuid = ANY(sqlc.narg(product_uuids)::uuid[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR q.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(reservation_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(reservation_uuids)::uuid[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.code ILIKE '%' || sqlc.narg(q) || '%'
    OR p.title ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'total' AND NOT sqlc.arg(sort_desc)::boolean THEN q.total END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total' AND sqlc.arg(sort_desc)::boolean THEN q.total END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN q.starts_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND sqlc.arg(sort_desc)::boolean THEN q.starts_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'valid_until' AND NOT sqlc.arg(sort_desc)::boolean THEN q.valid_until END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'valid_until' AND sqlc.arg(sort_desc)::boolean THEN q.valid_until END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN q.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN q.created_at END DESC,
    q.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: MarkQuoteUsed :execrows
-- A quote is consumed once (reservation create / change apply).
UPDATE quotes
SET status = 'used', used_at = sqlc.arg(used_at)::timestamptz, reservation_id = COALESCE(reservation_id, sqlc.narg(reservation_id))
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'open';

-- name: ReopenQuote :exec
-- Compensation of a failed create: the quote is usable again.
UPDATE quotes
SET status = 'open', used_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'used';

-- name: MarkQuoteSuperseded :exec
UPDATE quotes
SET status = 'superseded'
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'open';
