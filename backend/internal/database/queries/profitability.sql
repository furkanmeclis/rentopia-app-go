-- Vehicle profitability report (TEC-727, docs/design/vehicle-profitability.md).
-- Read-only facts of one organization for a period; the attribution
-- (swap split, commissions, overhead allocation, occupancy) is done in Go
-- (internal/modules/profitability/usecase). Every query filters on
-- organization_id; money is TRY only.

-- name: ProfitVehicles :many
-- Every vehicle of the organization (deleted ones too: they may have
-- income or costs in a past period). The service window and the home
-- branch drive occupancy and the overhead pools.
SELECT
    v.id, v.uuid, v.plate,
    COALESCE(NULLIF(btrim(d.name || ' ' || COALESCE(m.name, '')), ''), btrim(COALESCE(v.manual_brand, '') || ' ' || COALESCE(v.manual_model, '')))::text AS label,
    v.home_branch_id, hb.uuid AS home_branch_uuid, hb.code AS home_branch_code, hb.name AS home_branch_name,
    v.current_branch_id, v.in_service_date, v.out_of_service_date, v.created_at, v.deleted_at
FROM vehicles v
JOIN branches hb ON hb.organization_id = v.organization_id AND hb.id = v.home_branch_id
LEFT JOIN catalog_types ct ON ct.id = v.catalog_type_id
LEFT JOIN catalog_models m ON m.id = ct.model_id
LEFT JOIN catalog_brands tb ON tb.id = ct.brand_id
LEFT JOIN catalog_display_brands d ON d.id = tb.display_brand_id
WHERE v.organization_id = sqlc.arg(organization_id)
ORDER BY v.plate, v.id;

-- name: ProfitBranchesByUUID :many
SELECT b.id, b.uuid FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id) AND b.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: ProfitRentalGroupsByUUID :many
SELECT g.id, g.uuid FROM rental_groups g
WHERE g.organization_id = sqlc.arg(organization_id) AND g.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: ProfitVehicleIDsInGroups :many
SELECT DISTINCT vrg.vehicle_id FROM vehicle_rental_groups vrg
WHERE vrg.organization_id = sqlc.arg(organization_id) AND vrg.rental_group_id = ANY(sqlc.arg(group_ids)::bigint[]);

-- name: ProfitLegsOverlapping :many
-- Legs (handed over) of live rentals that overlap [from_ts, to_ts).
SELECT rv.rental_id, rv.vehicle_id, rv.out_at, rv.in_at
FROM rental_vehicles rv
JOIN rentals r ON r.organization_id = rv.organization_id AND r.id = rv.rental_id
WHERE rv.organization_id = sqlc.arg(organization_id)
  AND r.status <> 'cancelled'
  AND rv.out_at IS NOT NULL
  AND rv.out_at < sqlc.arg(to_ts)::timestamptz
  AND (rv.in_at IS NULL OR rv.in_at > sqlc.arg(from_ts)::timestamptz)
ORDER BY rv.rental_id, rv.id;

-- name: ProfitLegsForRentals :many
SELECT rv.rental_id, rv.vehicle_id, rv.out_at, rv.in_at
FROM rental_vehicles rv
WHERE rv.organization_id = sqlc.arg(organization_id) AND rv.rental_id = ANY(sqlc.arg(rental_ids)::bigint[])
ORDER BY rv.rental_id, rv.id;

-- name: ProfitRentals :many
-- Rentals with their sale facts: the planned end (open legs), the channel
-- commission and the share of the sold price that is extras / fees
-- (reservation quote snapshot: extra, one-way and delivery lines plus the
-- extras of every extension).
SELECT
    r.id, r.vehicle_id, res.ends_at, res.collecting_party, res.commission_amount, res.total_amount,
    (
        COALESCE((
            SELECT SUM((l->>'amount')::numeric)
            FROM jsonb_array_elements(CASE WHEN jsonb_typeof(res.quote_snapshot->'lines') = 'array' THEN res.quote_snapshot->'lines' ELSE '[]'::jsonb END) l
            WHERE l->>'kind' IN ('extra', 'one_way', 'delivery')
        ), 0)
        + COALESCE((
            SELECT SUM((x->>'extras_amount')::numeric)
            FROM jsonb_array_elements(CASE WHEN jsonb_typeof(res.quote_snapshot->'extensions') = 'array' THEN res.quote_snapshot->'extensions' ELSE '[]'::jsonb END) x
            WHERE x ? 'extras_amount'
        ), 0)
    )::numeric AS extras_amount
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = ANY(sqlc.arg(rental_ids)::bigint[]);

-- name: ProfitLedgerFacts :many
-- Ledger entries dated in the period that belong to a rental: accruals,
-- collections, refunds, deposit legs and the reversals of those. The
-- rental comes from the entry, the reversed entry, its collection document
-- or the live rental of its reservation (online prepayments).
SELECT f.id, f.kind, f.direction, f.purpose, f.amount, f.entry_date, f.original_kind, f.original_purpose, f.charge_kind,
       f.rental_id::bigint AS rental_id
FROM (
    SELECT
        e.id, e.kind, e.direction, e.purpose, e.amount, e.entry_date,
        COALESCE(o.kind, '')::text AS original_kind,
        COALESCE(o.purpose, '')::text AS original_purpose,
        COALESCE(ch.kind, '')::text AS charge_kind,
        COALESCE(e.rental_id, o.rental_id, c.rental_id, (
            SELECT r.id FROM rentals r
            WHERE r.organization_id = e.organization_id
              AND r.reservation_id = COALESCE(e.reservation_id, o.reservation_id, c.reservation_id)
              AND r.status <> 'cancelled'
            ORDER BY r.id DESC LIMIT 1
        )) AS rental_id
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = COALESCE(e.rental_charge_id, o.rental_charge_id)
    LEFT JOIN collections c ON c.organization_id = e.organization_id
        AND (c.ledger_entry_id = COALESCE(o.id, e.id) OR c.id = COALESCE(e.collection_id, o.collection_id))
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND e.currency = 'TRY'
      AND e.entry_date >= sqlc.arg(date_from)::date AND e.entry_date <= sqlc.arg(date_to)::date
      AND (
          e.kind IN ('accrual', 'collection', 'refund', 'deposit_hold', 'deposit_return')
          OR (e.kind = 'reversal' AND o.kind IN ('accrual', 'collection', 'refund'))
      )
) f
WHERE f.rental_id IS NOT NULL
ORDER BY f.entry_date, f.id;

-- name: ProfitBilledAccruals :many
-- Live (unreversed) debit accruals of rentals, all dates: what each rental
-- billed, by purpose and charge kind.
SELECT e.rental_id::bigint AS rental_id, e.purpose, COALESCE(ch.kind, '')::text AS charge_kind, SUM(e.amount)::numeric AS amount
FROM ledger_entries e
LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = e.rental_charge_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.rental_id = ANY(sqlc.arg(rental_ids)::bigint[])
  AND e.kind = 'accrual' AND e.direction = 'debit' AND e.currency = 'TRY'
  AND NOT EXISTS (
      SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id
  )
GROUP BY e.rental_id, e.purpose, ch.kind;

-- name: ProfitPaymentCommissions :many
-- Provider commissions of the rentals' live collections (online payments).
SELECT f.rental_id::bigint AS rental_id, SUM(f.commission_amount)::numeric AS amount
FROM (
    SELECT c.commission_amount, COALESCE(c.rental_id, (
        SELECT r.id FROM rentals r
        WHERE r.organization_id = c.organization_id AND r.reservation_id = c.reservation_id AND r.status <> 'cancelled'
        ORDER BY r.id DESC LIMIT 1
    )) AS rental_id
    FROM collections c
    WHERE c.organization_id = sqlc.arg(organization_id)
      AND c.status <> 'void' AND c.commission_amount > 0 AND c.currency = 'TRY'
      AND (c.rental_id IS NOT NULL OR c.reservation_id IS NOT NULL)
) f
WHERE f.rental_id = ANY(sqlc.arg(rental_ids)::bigint[])
GROUP BY f.rental_id;

-- name: ProfitExpenses :many
-- Recorded expenses dated in the period (supplier invoices included: each
-- invoice has its expense row). Provider commissions are left out: they
-- are attributed through the collections of the rental.
SELECT e.id, e.vehicle_id, e.branch_id, e.total_amount, e.expense_date, e.supplier_invoice_id,
       ec.code AS category_code, ec.name AS category_name
FROM expenses e
JOIN expense_categories ec ON ec.organization_id = e.organization_id AND ec.id = e.category_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.status = 'recorded' AND e.currency = 'TRY'
  AND e.expense_date >= sqlc.arg(date_from)::date AND e.expense_date <= sqlc.arg(date_to)::date
  AND ec.code <> 'payment_commission'
  AND NOT EXISTS (
      SELECT 1 FROM collections c WHERE c.organization_id = e.organization_id AND c.commission_expense_id = e.id
  )
ORDER BY e.expense_date, e.id;

-- name: ProfitSupplierPayments :many
-- Cash basis of supplier invoices: the amounts posted supplier payments
-- dated in the period allocated to an invoice, with the invoice's expense
-- row (category, branch, vehicle).
SELECT ex.vehicle_id, ex.branch_id, a.amount, p.payment_date, ec.code AS category_code, ec.name AS category_name
FROM supplier_payment_allocations a
JOIN supplier_payments p ON p.organization_id = a.organization_id AND p.id = a.payment_id
JOIN supplier_invoices i ON i.organization_id = a.organization_id AND i.id = a.invoice_id
JOIN expenses ex ON ex.organization_id = i.organization_id AND ex.supplier_invoice_id = i.id
JOIN expense_categories ec ON ec.organization_id = ex.organization_id AND ec.id = ex.category_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND p.status = 'posted' AND i.status <> 'void' AND ex.status = 'recorded' AND ex.currency = 'TRY'
  AND p.payment_date >= sqlc.arg(date_from)::date AND p.payment_date <= sqlc.arg(date_to)::date
  AND ec.code <> 'payment_commission'
ORDER BY p.payment_date, a.id;

-- name: ProfitMaintenance :many
-- Maintenance done with a cost in the period (vehicle records).
SELECT vm.vehicle_id, vm.done_date, vm.cost
FROM vehicle_maintenance vm
WHERE vm.organization_id = sqlc.arg(organization_id)
  AND vm.kind = 'done' AND vm.deleted_at IS NULL AND vm.cost IS NOT NULL AND vm.cost > 0
  AND vm.done_date >= sqlc.arg(date_from)::date AND vm.done_date <= sqlc.arg(date_to)::date;

-- name: ProfitTransfers :many
-- Completed transfers with a cost that arrived in [from_ts, to_ts).
SELECT vt.vehicle_id, vt.actual_arrival, vt.cost
FROM vehicle_transfers vt
WHERE vt.organization_id = sqlc.arg(organization_id)
  AND vt.status = 'completed' AND vt.cost IS NOT NULL AND vt.cost > 0
  AND vt.actual_arrival >= sqlc.arg(from_ts)::timestamptz AND vt.actual_arrival < sqlc.arg(to_ts)::timestamptz;

-- name: ProfitBlocks :many
-- Usage blocks (maintenance, damage, cleaning, out of service, other)
-- overlapping [from_ts, to_ts); an open block runs to to_ts.
SELECT vb.vehicle_id, vb.kind, lower(vb.during)::timestamptz AS starts_at,
       COALESCE(upper(vb.during), sqlc.arg(to_ts)::timestamptz)::timestamptz AS ends_at
FROM vehicle_blocks vb
WHERE vb.organization_id = sqlc.arg(organization_id)
  AND vb.deleted_at IS NULL
  AND vb.during && tstzrange(sqlc.arg(from_ts)::timestamptz, sqlc.arg(to_ts)::timestamptz, '[)');
