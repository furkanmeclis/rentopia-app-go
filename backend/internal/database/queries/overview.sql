-- Operations overview of the tenant panel home page (TEC-716,
-- docs/design/operations-overview.md). Read-only aggregates over other
-- modules' tables. Every query takes the organization and the effective
-- branch filter (nil = every branch; the caller's branch scope, narrowed
-- to one branch when the page filters by branch) and applies the
-- visibility rule of the module that owns the table.

-- name: OverviewBranches :many
-- Live branches of the organization in the caller's branch scope (the
-- page's branch filter).
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY b.code, b.id
LIMIT 500;

-- name: OverviewBranchByUUID :one
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: OverviewTodayOperations :many
-- Handover and return tasks due in [day_start, day_end) of the task's
-- branch (operation task rule), with the rental, plate and customer.
SELECT t.uuid, t.task_type, t.status, t.due_at, t.completed_at,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    res.uuid AS reservation_uuid,
    COALESCE(tv.plate, rv.plate, '')::text AS vehicle_plate,
    c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    COUNT(*) OVER ()::bigint AS total
FROM operation_tasks t
JOIN branches b ON b.organization_id = t.organization_id AND b.id = t.branch_id
JOIN rentals r ON r.organization_id = t.organization_id AND r.id = t.rental_id
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
LEFT JOIN vehicles tv ON tv.organization_id = t.organization_id AND tv.id = t.vehicle_id
LEFT JOIN vehicles rv ON rv.organization_id = r.organization_id AND rv.id = r.vehicle_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND t.task_type IN ('handover', 'return')
  AND t.status <> 'cancelled'
  AND t.due_at >= sqlc.arg(day_start)::timestamptz
  AND t.due_at < sqlc.arg(day_end)::timestamptz
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY t.due_at, t.id
LIMIT sqlc.arg(limit_count);

-- name: OverviewTodayOperationCounts :many
-- (task_type, status, count) of the same window.
SELECT t.task_type, t.status, COUNT(*)::bigint AS count
FROM operation_tasks t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.task_type IN ('handover', 'return')
  AND t.status <> 'cancelled'
  AND t.rental_id IS NOT NULL
  AND t.due_at >= sqlc.arg(day_start)::timestamptz
  AND t.due_at < sqlc.arg(day_end)::timestamptz
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY t.task_type, t.status;

-- name: OverviewOverdueRentals :many
-- Active rentals whose planned return has passed (or that the overdue
-- sweep marked), oldest first. Rental rule: planned pick-up, planned
-- return or actual return branch in the filter.
SELECT r.uuid, r.reference, r.overdue_at,
    res.uuid AS reservation_uuid, res.ends_at AS planned_return_at,
    rb.uuid AS return_branch_uuid, rb.code AS return_branch_code, rb.name AS return_branch_name,
    v.plate AS vehicle_plate,
    c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    COUNT(*) OVER ()::bigint AS total
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN branches rb ON rb.organization_id = res.organization_id AND rb.id = res.return_branch_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
JOIN vehicles v ON v.organization_id = r.organization_id AND v.id = r.vehicle_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND rb.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND r.status = 'active'
  AND (r.overdue_at IS NOT NULL OR res.ends_at < sqlc.arg(now)::timestamptz)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY res.ends_at, r.id
LIMIT sqlc.arg(limit_count);

-- name: OverviewWaitingReservations :many
-- Reservations in the given statuses (approval queue: request,
-- awaiting_review; waiting for payment: awaiting_payment), soonest
-- deadline first. Reservation rule: pick-up or return branch in the
-- filter. overdue = deadline passed.
SELECT r.uuid, r.reference, r.status, r.starts_at, r.ends_at, r.channel, r.review_deadline, r.payment_deadline,
    r.currency, r.total_amount, r.prepayment_amount, r.paid_amount,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    COUNT(*) OVER ()::bigint AS total,
    (COUNT(*) FILTER (
        WHERE COALESCE(CASE WHEN r.status = 'awaiting_payment' THEN r.payment_deadline ELSE r.review_deadline END,
            'infinity'::timestamptz) < sqlc.arg(now)::timestamptz
    ) OVER ())::bigint AS overdue
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND r.status = ANY(sqlc.arg(statuses)::text[])
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY CASE WHEN r.status = 'awaiting_payment' THEN r.payment_deadline ELSE r.review_deadline END ASC NULLS LAST,
    r.starts_at, r.id
LIMIT sqlc.arg(limit_count);

-- name: OverviewOpenResolutionTasks :many
-- Open resolution tasks (availability problems), newest first.
-- Reservation rule on the task's reservation.
SELECT t.uuid, t.task_type, t.reason, t.created_at,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference, r.starts_at AS reservation_starts_at,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    COUNT(*) OVER ()::bigint AS total
FROM resolution_tasks t
JOIN reservations r ON r.organization_id = t.organization_id AND r.id = t.reservation_id
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND t.status = 'open'
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(limit_count);

-- name: OverviewResolutionTaskCounts :many
-- Open resolution tasks per type.
SELECT t.task_type, COUNT(*)::bigint AS count
FROM resolution_tasks t
JOIN reservations r ON r.organization_id = t.organization_id AND r.id = t.reservation_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND t.status = 'open'
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
GROUP BY t.task_type;

-- name: OverviewAttentionJobs :many
-- Integration jobs of the organization that failed or need verification
-- (organization-wide: jobs belong to no branch), most recently changed
-- first. Generic over provider and kind (SMS, payments; KABİS and
-- e-documents later).
SELECT j.uuid, j.direction, j.provider, j.kind, j.status, j.attempts, j.last_error, j.ref_type, j.ref_uuid,
    j.updated_at, j.created_at,
    a.uuid AS account_uuid, a.kind AS account_kind, a.display_name AS account_name,
    COUNT(*) OVER ()::bigint AS total
FROM integration_jobs j
LEFT JOIN integration_accounts a ON a.organization_id = j.organization_id AND a.id = j.integration_account_id
WHERE j.organization_id = sqlc.arg(organization_id)
  AND j.status IN ('failed', 'needs_verification')
ORDER BY j.updated_at DESC, j.id DESC
LIMIT sqlc.arg(limit_count);

-- name: OverviewPendingContracts :many
-- Confirmed reservations picked up in [window_start, window_end) whose
-- vehicle is not handed over yet and whose rental contract is not
-- approved (no live document, or one awaiting approval). Reservation rule.
SELECT r.uuid, r.reference, r.starts_at,
    pb.uuid AS pickup_branch_uuid, pb.code AS pickup_branch_code, pb.name AS pickup_branch_name,
    c.customer_type, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.company_title AS customer_company_title,
    rt.uuid AS rental_uuid, rt.reference AS rental_reference,
    d.uuid AS document_uuid, d.status AS document_status,
    COUNT(*) OVER ()::bigint AS total
FROM reservations r
JOIN branches pb ON pb.organization_id = r.organization_id AND pb.id = r.pickup_branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
LEFT JOIN rentals rt ON rt.organization_id = r.organization_id AND rt.reservation_id = r.id AND rt.status <> 'cancelled'
LEFT JOIN documents d ON d.organization_id = r.organization_id AND d.reservation_id = r.id
    AND d.kind = 'rental_contract' AND d.status <> 'void'
WHERE r.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND (rt.id IS NULL OR rt.organization_id = sqlc.arg(organization_id))
  AND (d.id IS NULL OR d.organization_id = sqlc.arg(organization_id))
  AND r.status = 'confirmed'
  AND r.starts_at >= sqlc.arg(window_start)::timestamptz
  AND r.starts_at < sqlc.arg(window_end)::timestamptz
  AND (rt.id IS NULL OR rt.status = 'preparation')
  AND (d.id IS NULL OR d.status <> 'approved')
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY r.starts_at, r.id
LIMIT sqlc.arg(limit_count);

-- name: OverviewContractRequired :one
-- An active rental contract template makes an approved contract a
-- handover requirement (rentals ContractState).
SELECT EXISTS (
    SELECT 1 FROM contract_templates ct
    WHERE ct.organization_id = sqlc.arg(organization_id)
      AND ct.kind = 'rental_contract'
      AND ct.status = 'active'
)::boolean AS required;

-- name: OverviewFleetUtilisation :one
-- Rentable vehicles today (live, active, in service, not out of service,
-- in maintenance or damaged) and how many of them are out on an active
-- rental. Vehicle rule: home or current branch in the filter.
SELECT COUNT(*)::bigint AS rentable,
    COUNT(*) FILTER (WHERE EXISTS (
        SELECT 1 FROM rentals r
        WHERE r.organization_id = v.organization_id AND r.vehicle_id = v.id AND r.status = 'active'
    ))::bigint AS rented
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.deleted_at IS NULL
  AND v.is_active
  AND v.status NOT IN ('out_of_service', 'maintenance', 'damaged')
  AND (v.in_service_date IS NULL OR v.in_service_date <= sqlc.arg(today)::date)
  AND (v.out_of_service_date IS NULL OR v.out_of_service_date >= sqlc.arg(today)::date)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  );

-- name: OverviewActiveRentals :one
-- Active rentals and how many of them are late. Rental rule.
SELECT COUNT(*)::bigint AS active,
    COUNT(*) FILTER (WHERE r.overdue_at IS NOT NULL OR res.ends_at < sqlc.arg(now)::timestamptz)::bigint AS overdue
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.status = 'active'
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  );

-- name: OverviewCollections :one
-- TRY payments confirmed in [day_start, day_end): provider checkouts and
-- branch collections. Payment rule: the reservation's pick-up or return
-- branch in the filter.
SELECT COUNT(*)::bigint AS count, COALESCE(SUM(p.amount), 0)::numeric(14, 2) AS amount
FROM payments p
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND p.status IN ('succeeded', 'partially_refunded', 'refunded')
  AND p.currency = 'TRY'
  AND p.verified_at >= sqlc.arg(day_start)::timestamptz
  AND p.verified_at < sqlc.arg(day_end)::timestamptz
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  );

-- name: OverviewRefunds :one
-- TRY refunds that succeeded in [day_start, day_end). Payment rule.
SELECT COUNT(*)::bigint AS count, COALESCE(SUM(f.amount), 0)::numeric(14, 2) AS amount
FROM payment_refunds f
JOIN payments p ON p.organization_id = f.organization_id AND p.id = f.payment_id
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
WHERE f.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND f.status = 'succeeded'
  AND f.currency = 'TRY'
  AND f.updated_at >= sqlc.arg(day_start)::timestamptz
  AND f.updated_at < sqlc.arg(day_end)::timestamptz
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  );

-- name: OverviewBadgeCounts :one
-- Sidebar badges: the approval queue (and how many passed their SLA) and
-- open resolution tasks. Reservation rule.
SELECT
    (SELECT COUNT(*) FROM reservations r
        WHERE r.organization_id = sqlc.arg(organization_id)
          AND r.status IN ('request', 'awaiting_review')
          AND (
            sqlc.narg(branch_ids)::bigint[] IS NULL
            OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
            OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
          ))::bigint AS approvals,
    (SELECT COUNT(*) FROM reservations r
        WHERE r.organization_id = sqlc.arg(organization_id)
          AND r.status IN ('request', 'awaiting_review')
          AND r.review_deadline < sqlc.arg(now)::timestamptz
          AND (
            sqlc.narg(branch_ids)::bigint[] IS NULL
            OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
            OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
          ))::bigint AS approvals_overdue,
    (SELECT COUNT(*) FROM resolution_tasks t
        JOIN reservations r ON r.organization_id = t.organization_id AND r.id = t.reservation_id
        WHERE t.organization_id = sqlc.arg(organization_id)
          AND r.organization_id = sqlc.arg(organization_id)
          AND t.status = 'open'
          AND (
            sqlc.narg(branch_ids)::bigint[] IS NULL
            OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
            OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
          ))::bigint AS resolution_tasks;
