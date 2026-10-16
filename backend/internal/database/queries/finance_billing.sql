-- Periodic billing of rentals (TEC-723, docs/design/corporate-agreements.md):
-- rental_billing_periods is the billing plan of a periodically billed
-- rental; SyncRental posts each due period once through the ledger.

-- name: LockRentalForBilling :one
-- Serialises billing-plan work on one rental (job runs, handover,
-- extension, return) for the rest of the transaction.
SELECT r.id FROM rentals r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id)
FOR UPDATE;

-- name: ListRentalBillingPeriods :many
SELECT bp.id, bp.uuid, bp.rental_id, bp.agreement_id, bp.period_no, bp.revision, bp.day_from, bp.day_to,
    bp.starts_on, bp.ends_on, bp.days, bp.partial, bp.amount, bp.currency, bp.km_allowance, bp.accrual_key,
    bp.status, bp.ledger_entry_id, bp.posted_at, bp.closed_at, bp.created_at
FROM rental_billing_periods bp
WHERE bp.organization_id = sqlc.arg(organization_id)
  AND bp.rental_id = sqlc.arg(rental_id)
ORDER BY bp.period_no, bp.revision;

-- name: InsertRentalBillingPeriod :one
INSERT INTO rental_billing_periods (
    organization_id, rental_id, agreement_id, period_no, revision, day_from, day_to, starts_on, ends_on, days,
    partial, amount, currency, km_allowance, accrual_key
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.narg(agreement_id), sqlc.arg(period_no), sqlc.arg(revision),
    sqlc.arg(day_from), sqlc.arg(day_to), sqlc.arg(starts_on), sqlc.arg(ends_on), sqlc.arg(days),
    sqlc.arg(partial), sqlc.arg(amount), sqlc.arg(currency), sqlc.narg(km_allowance), sqlc.arg(accrual_key)
)
RETURNING id, uuid;

-- name: DeletePlannedBillingPeriods :execrows
-- Planned periods never reached the ledger: a re-plan replaces them.
DELETE FROM rental_billing_periods
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND status = 'planned';

-- name: MarkBillingPeriodPosted :execrows
UPDATE rental_billing_periods
SET status = 'posted', ledger_entry_id = sqlc.arg(ledger_entry_id), posted_at = sqlc.arg(posted_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'planned';

-- name: CloseBillingPeriod :execrows
-- posted -> superseded (a new revision replaces it) / reversed (no longer due).
UPDATE rental_billing_periods
SET status = sqlc.arg(status), closed_at = sqlc.arg(closed_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'posted';

-- name: ListDuePeriodicRentals :many
-- Worker sweep (every organization): periodically billed rentals under way
-- or returned whose plan is missing or has a period due by today.
SELECT r.organization_id, r.id
FROM rentals r
WHERE r.rent_billing = 'periodic'
  AND r.status IN ('active', 'return_check', 'returned')
  AND (
    NOT EXISTS (
        SELECT 1 FROM rental_billing_periods bp
        WHERE bp.organization_id = r.organization_id AND bp.rental_id = r.id AND bp.status IN ('planned', 'posted')
    )
    OR EXISTS (
        SELECT 1 FROM rental_billing_periods bp
        WHERE bp.organization_id = r.organization_id AND bp.rental_id = r.id
          AND bp.status = 'planned' AND bp.starts_on <= sqlc.arg(today)::date AND bp.amount > 0
    )
  )
ORDER BY r.organization_id, r.id
LIMIT sqlc.arg(limit_count);
