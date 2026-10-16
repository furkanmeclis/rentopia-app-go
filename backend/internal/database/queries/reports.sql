-- Reports (TEC-738, §15.1). Read-only facts of one
-- organization; the grouping and the ratios are done in Go
-- (internal/modules/reports/usecase). Occupancy, the branch comparison and
-- the vehicle contribution reuse the vehicle profitability engine
-- (profitability.sql). Every query filters on organization_id; money is TRY
-- only.

-- name: ReportBranches :many
-- Every branch of the organization (labels of the grouped rows).
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
ORDER BY b.code, b.id;

-- name: ReportVehicleGroups :many
-- Rental group memberships of the organization's vehicles.
SELECT vrg.vehicle_id, g.uuid AS group_uuid, g.code AS group_code, g.name AS group_name
FROM vehicle_rental_groups vrg
JOIN rental_groups g ON g.organization_id = vrg.organization_id AND g.id = vrg.rental_group_id
WHERE vrg.organization_id = sqlc.arg(organization_id)
  AND g.organization_id = sqlc.arg(organization_id)
ORDER BY vrg.vehicle_id, g.code;

-- name: ReportIncomeFacts :many
-- Accrual income of the period by purpose, charge kind, branch and month
-- (Europe/Istanbul calendar months of the entry date): live debit accruals
-- of customer accounts minus their reversals. A reversal of a periodic rent
-- (TEC-723) counts on the reversed period's date, like the profitability
-- report. branch_ids NULL = every branch and the head office.
SELECT f.purpose, f.charge_kind, f.branch_id, to_char(f.day, 'YYYY-MM')::text AS month,
       SUM(f.signed)::numeric AS amount
FROM (
    SELECT
        COALESCE(o.purpose, e.purpose)::text AS purpose,
        COALESCE(ch.kind, '')::text AS charge_kind,
        COALESCE(e.branch_id, o.branch_id) AS branch_id,
        (CASE WHEN e.kind = 'reversal' AND e.billing_period_id IS NOT NULL AND o.billing_period_id IS NOT NULL
              THEN o.entry_date ELSE e.entry_date END)::date AS day,
        (CASE WHEN e.kind = 'reversal' THEN -e.amount ELSE e.amount END)::numeric AS signed
    FROM ledger_entries e
    JOIN accounts a ON a.organization_id = e.organization_id AND a.id = e.account_id
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = COALESCE(e.rental_charge_id, o.rental_charge_id)
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND a.organization_id = sqlc.arg(organization_id)
      AND a.party_kind = 'customer'
      AND e.currency = 'TRY'
      AND (
          (e.kind = 'accrual' AND e.direction = 'debit')
          OR (e.kind = 'reversal' AND o.kind = 'accrual' AND o.direction = 'debit')
      )
      AND (
          (e.entry_date >= sqlc.arg(date_from)::date AND e.entry_date <= sqlc.arg(date_to)::date)
          OR (e.kind = 'reversal' AND e.billing_period_id IS NOT NULL)
      )
) f
WHERE f.day >= sqlc.arg(date_from)::date AND f.day <= sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR f.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY f.purpose, f.charge_kind, f.branch_id, to_char(f.day, 'YYYY-MM')
ORDER BY month, f.purpose, f.charge_kind;

-- name: ReportExpenseFacts :many
-- Recorded expenses of the period by category, branch and month (expense
-- date; supplier invoices included through their expense row).
SELECT ec.code AS category_code, ec.name AS category_name, e.branch_id,
       to_char(e.expense_date, 'YYYY-MM')::text AS month, SUM(e.total_amount)::numeric AS amount
FROM expenses e
JOIN expense_categories ec ON ec.organization_id = e.organization_id AND ec.id = e.category_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND ec.organization_id = sqlc.arg(organization_id)
  AND e.status = 'recorded' AND e.currency = 'TRY'
  AND e.expense_date >= sqlc.arg(date_from)::date AND e.expense_date <= sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY ec.code, ec.name, e.branch_id, to_char(e.expense_date, 'YYYY-MM')
ORDER BY month, ec.code;

-- name: ReportChannelFacts :many
-- Reservations by sales channel, status, collecting party and month of the
-- chosen date (booking = created_at, pickup = starts_at; Europe/Istanbul).
-- scope_branch_ids: the caller's branch scope (pick-up OR return branch, the
-- reservations rule); filter_branch_ids: the pick-up branch filter.
SELECT r.channel, r.status, r.collecting_party,
       to_char((CASE WHEN sqlc.arg(by_pickup)::boolean THEN r.starts_at ELSE r.created_at END) AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')::text AS month,
       COUNT(*)::bigint AS reservations,
       COALESCE(SUM(r.total_amount), 0)::numeric AS total_amount,
       COALESCE(SUM(r.commission_amount), 0)::numeric AS commission_amount,
       COALESCE(SUM(EXTRACT(EPOCH FROM (r.ends_at - r.starts_at))), 0)::float8 AS rental_seconds
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.currency = 'TRY'
  AND (CASE WHEN sqlc.arg(by_pickup)::boolean THEN r.starts_at ELSE r.created_at END) >= sqlc.arg(from_ts)::timestamptz
  AND (CASE WHEN sqlc.arg(by_pickup)::boolean THEN r.starts_at ELSE r.created_at END) < sqlc.arg(to_ts)::timestamptz
  AND (
      sqlc.narg(scope_branch_ids)::bigint[] IS NULL
      OR r.pickup_branch_id = ANY(sqlc.narg(scope_branch_ids)::bigint[])
      OR r.return_branch_id = ANY(sqlc.narg(scope_branch_ids)::bigint[])
  )
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.pickup_branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY r.channel, r.status, r.collecting_party, month
ORDER BY r.channel, month;

-- name: ReportAgingAccounts :many
-- Per account of one party kind: the balance and the open charges by due
-- bucket as of a date (entries posted up to it), FIFO: settlements pay the
-- oldest charges first, like the ledger summary (LedgerAccountAging).
-- Supplier accounts age their payables (credits are their charges).
-- Deposit entries stay out. branch_ids filters the entries' branch.
SELECT a.id, a.uuid, a.code, a.name, a.party_kind, a.payment_term_days,
    COALESCE(SUM(CASE WHEN e.purpose = 'deposit' THEN 0 WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END), 0)::numeric AS balance,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.side = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.side = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(d90)::date), 0)::numeric AS charges_90,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.side = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.side = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(d60)::date), 0)::numeric AS charges_60,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.side = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.side = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(d30)::date), 0)::numeric AS charges_30,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.side = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.side = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(as_of)::date), 0)::numeric AS charges_overdue,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.side = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.side = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END), 0)::numeric AS charges_all,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.side = 'credit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.side = 'debit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END), 0)::numeric AS settlements,
    MAX(e.entry_date)::date AS last_entry
FROM (
    SELECT le.*,
        (CASE WHEN pa.party_kind = 'supplier'
            THEN (CASE le.direction WHEN 'debit' THEN 'credit' ELSE 'debit' END)
            ELSE le.direction END)::text AS side
    FROM ledger_entries le
    JOIN accounts pa ON pa.organization_id = le.organization_id AND pa.id = le.account_id
    WHERE le.organization_id = sqlc.arg(organization_id)
      AND pa.organization_id = sqlc.arg(organization_id)
      AND le.currency = 'TRY'
      AND le.entry_date <= sqlc.arg(as_of)::date
) e
JOIN accounts a ON a.organization_id = e.organization_id AND a.id = e.account_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND a.party_kind = sqlc.arg(party_kind)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY a.id, a.uuid, a.code, a.name, a.party_kind, a.payment_term_days
ORDER BY a.code;
