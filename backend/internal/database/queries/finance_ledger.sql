-- Finance ledger (TEC-717): customer / supplier accounts and append-only
-- entries. Balances are sums of entries; the aging rule (overdue / due
-- soon) applies settlements (credits) to the oldest charges (debits) first:
--   charge side: debits that are not reversals, minus reversals of debits
--   settle side: credits that are not reversals, minus reversals of credits
--   overdue  = max(0, charges due before today - settlements)
--   due soon = max(0, charges due before today + N - settlements) - overdue
-- Entries of purpose 'deposit' (deposits held, returned, captured, the
-- deposit leg of an offset) are a liability kept apart: they never enter
-- the receivable balance, the aging or the running balance; deposit_held
-- sums them. The other leg of an offset / a capture is a settlement.

-- name: EnsureCustomerAccount :one
INSERT INTO accounts (organization_id, code, party_kind, customer_id, name, branch_id, currency, created_by)
VALUES (
    sqlc.arg(organization_id),
    'CR-' || lpad((
        SELECT count(*) + 1 FROM accounts x
        WHERE x.organization_id = sqlc.arg(organization_id) AND x.party_kind = 'customer'
    )::text, 6, '0'),
    'customer', sqlc.arg(customer_id), sqlc.arg(name), sqlc.narg(branch_id), sqlc.arg(currency), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, customer_id) WHERE customer_id IS NOT NULL
DO UPDATE SET name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE accounts.name END
RETURNING id, uuid, code, branch_id, currency, payment_term_days;

-- name: LockOrganizationAccountCodes :exec
-- Serialises opening accounts of one organization (per-organization code
-- sequence) for the rest of the transaction.
SELECT pg_advisory_xact_lock(hashtextextended('finance.accounts:' || sqlc.arg(organization_id)::bigint::text, 0));

-- name: GetCustomerDisplay :one
SELECT c.id, c.uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.phone, c.anonymized_at
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id);

-- name: GetCustomerByUUIDForLedger :one
SELECT c.id, c.uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.phone, c.anonymized_at
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: GetAccountByCustomer :one
SELECT a.id, a.uuid
FROM accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.customer_id = sqlc.arg(customer_id);

-- name: GetAccountRow :one
SELECT a.id, a.uuid, a.code, a.party_kind, a.customer_id, a.supplier_id, a.name, a.branch_id, a.currency,
    a.payment_term_days, a.status, a.note, a.created_at, a.updated_at
FROM accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: GetAccountRowByID :one
SELECT a.id, a.uuid, a.code, a.party_kind, a.customer_id, a.supplier_id, a.name, a.branch_id, a.currency,
    a.payment_term_days, a.status, a.note, a.created_at, a.updated_at
FROM accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id);

-- name: UpdateAccount :execrows
UPDATE accounts
SET payment_term_days = sqlc.arg(payment_term_days), note = sqlc.arg(note), status = sqlc.arg(status)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: LockAccount :one
SELECT a.id FROM accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id)
FOR UPDATE;

-- name: ListAccounts :many
-- One page of accounts with their balances over the entries the caller may
-- see (branch scope: accounts opened at, or with entries in, the branches).
WITH agg AS (
    SELECT e.account_id,
        COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit' AND e.purpose <> 'deposit'), 0)::numeric AS debit,
        COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit' AND e.purpose <> 'deposit'), 0)::numeric AS credit,
        COALESCE(SUM(CASE
            WHEN e.purpose = 'deposit' THEN 0
            WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
            WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
            ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(today)::date), 0)::numeric AS charges_overdue,
        COALESCE(SUM(CASE
            WHEN e.purpose = 'deposit' THEN 0
            WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
            WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
            ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(soon)::date), 0)::numeric AS charges_soon,
        COALESCE(SUM(CASE
            WHEN e.purpose = 'deposit' THEN 0
            WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
            WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
            ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(due_before)::date), 0)::numeric AS charges_before,
        COALESCE(SUM(CASE
            WHEN e.purpose = 'deposit' THEN 0
            WHEN e.direction = 'credit' AND e.kind <> 'reversal' THEN e.amount
            WHEN e.direction = 'debit' AND e.kind = 'reversal' THEN -e.amount
            ELSE 0 END), 0)::numeric AS settlements,
        COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END)
            FILTER (WHERE e.purpose = 'deposit'), 0)::numeric AS deposit_held,
        MAX(e.created_at)::timestamptz AS last_entry_at,
        COUNT(*)::bigint AS entry_count
    FROM ledger_entries e
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
    GROUP BY e.account_id
), base AS (
    SELECT a.id, a.uuid, a.code, a.party_kind, a.name, a.currency, a.payment_term_days, a.status, a.note,
        a.created_at, a.updated_at, a.branch_id,
        c.uuid AS customer_uuid, c.customer_type,
        b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
        COALESCE(agg.debit, 0)::numeric AS debit,
        COALESCE(agg.credit, 0)::numeric AS credit,
        (COALESCE(agg.debit, 0) - COALESCE(agg.credit, 0))::numeric AS balance,
        GREATEST(0, COALESCE(agg.charges_overdue, 0) - COALESCE(agg.settlements, 0))::numeric AS overdue,
        (GREATEST(0, COALESCE(agg.charges_soon, 0) - COALESCE(agg.settlements, 0))
            - GREATEST(0, COALESCE(agg.charges_overdue, 0) - COALESCE(agg.settlements, 0)))::numeric AS due_soon,
        GREATEST(0, COALESCE(agg.charges_before, 0) - COALESCE(agg.settlements, 0))::numeric AS open_before,
        COALESCE(agg.deposit_held, 0)::numeric AS deposit_held,
        agg.last_entry_at,
        COALESCE(agg.entry_count, 0)::bigint AS entry_count
    FROM accounts a
    LEFT JOIN agg ON agg.account_id = a.id
    LEFT JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
    LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
    WHERE a.organization_id = sqlc.arg(organization_id)
      AND (
        sqlc.narg(branch_ids)::bigint[] IS NULL
        OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR agg.account_id IS NOT NULL
      )
), shaped AS (
    SELECT base.*,
        (CASE
            WHEN base.balance = 0 THEN 'closed'
            WHEN base.overdue > 0 THEN 'overdue'
            WHEN base.due_soon > 0 THEN 'due_soon'
            WHEN base.balance > 0 THEN 'receivable'
            ELSE 'payable' END)::text AS balance_status
    FROM base
)
SELECT s.id, s.uuid, s.code, s.party_kind, s.name, s.currency, s.payment_term_days, s.status, s.note,
    s.created_at, s.updated_at, s.customer_uuid, s.customer_type, s.branch_uuid, s.branch_code, s.branch_name,
    s.debit, s.credit, s.balance, s.overdue, s.due_soon, s.deposit_held, s.last_entry_at, s.entry_count,
    s.balance_status,
    COUNT(*) OVER ()::bigint AS total
FROM shaped s
WHERE (sqlc.narg(account_ids)::bigint[] IS NULL OR s.id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR s.balance_status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(account_statuses)::text[] IS NULL OR s.status = ANY(sqlc.narg(account_statuses)::text[]))
  AND (sqlc.narg(party_kinds)::text[] IS NULL OR s.party_kind = ANY(sqlc.narg(party_kinds)::text[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(min_balance)::numeric IS NULL OR s.balance >= sqlc.narg(min_balance)::numeric)
  AND (sqlc.narg(max_balance)::numeric IS NULL OR s.balance <= sqlc.narg(max_balance)::numeric)
  AND (NOT sqlc.arg(only_due_before)::boolean OR s.open_before > 0)
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND NOT sqlc.arg(sort_desc)::boolean THEN s.balance END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND sqlc.arg(sort_desc)::boolean THEN s.balance END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'overdue' AND NOT sqlc.arg(sort_desc)::boolean THEN s.overdue END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'overdue' AND sqlc.arg(sort_desc)::boolean THEN s.overdue END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN s.name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN s.name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN s.code END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN s.code END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'last_entry_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.last_entry_at END ASC NULLS FIRST,
    CASE WHEN sqlc.arg(sort_field)::text = 'last_entry_at' AND sqlc.arg(sort_desc)::boolean THEN s.last_entry_at END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN s.created_at END DESC,
    s.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: InsertLedgerEntry :one
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    reason, source_type, source_uuid, rental_id, reservation_id, rental_charge_id, payment_id, payment_refund_id,
    accrual_key, reverses_entry_id, adjusts_entry_id, offset_group, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), sqlc.arg(kind), sqlc.arg(direction), sqlc.arg(purpose),
    sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(due_date), sqlc.arg(description),
    sqlc.arg(reason), sqlc.arg(source_type), sqlc.narg(source_uuid), sqlc.narg(rental_id), sqlc.narg(reservation_id),
    sqlc.narg(rental_charge_id), sqlc.narg(payment_id), sqlc.narg(payment_refund_id), sqlc.narg(accrual_key),
    sqlc.narg(reverses_entry_id), sqlc.narg(adjusts_entry_id), sqlc.narg(offset_group), sqlc.narg(branch_id),
    sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: InsertPaymentCollectionEntry :one
-- Idempotent: one collection per payment (uq_ledger_entries_payment).
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    source_type, source_uuid, reservation_id, payment_id, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), 'collection', 'credit', sqlc.arg(purpose), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(entry_date), sqlc.arg(description), 'payment',
    sqlc.arg(source_uuid), sqlc.narg(reservation_id), sqlc.arg(payment_id), sqlc.narg(branch_id), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, payment_id) WHERE kind = 'collection' AND payment_id IS NOT NULL DO NOTHING
RETURNING id, uuid;

-- name: InsertPaymentRefundEntry :one
-- Idempotent: one refund entry per payment refund.
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    source_type, source_uuid, reservation_id, payment_id, payment_refund_id, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), 'refund', 'debit', sqlc.arg(purpose), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(entry_date), sqlc.arg(description), 'payment_refund',
    sqlc.arg(source_uuid), sqlc.narg(reservation_id), sqlc.narg(payment_id), sqlc.arg(payment_refund_id),
    sqlc.narg(branch_id), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, payment_refund_id) WHERE kind = 'refund' AND payment_refund_id IS NOT NULL DO NOTHING
RETURNING id, uuid;

-- name: GetLedgerEntryRow :one
SELECT e.id, e.uuid, e.account_id, e.kind, e.direction, e.purpose, e.amount, e.currency, e.entry_date, e.due_date,
    e.description, e.reason, e.source_type, e.source_uuid, e.rental_id, e.reservation_id, e.rental_charge_id,
    e.payment_id, e.payment_refund_id, e.accrual_key, e.branch_id, e.offset_group,
    EXISTS (
        SELECT 1 FROM ledger_entries r
        WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id
    )::boolean AS reversed
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid);

-- name: ListLiveRentalAccruals :many
-- Accruals of one rental (rent and charges) that are not reversed.
SELECT e.id, e.uuid, e.account_id, e.accrual_key, e.amount, e.currency, e.purpose, e.rental_charge_id, e.branch_id,
    e.due_date, e.reservation_id
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.rental_id = sqlc.arg(rental_id)
  AND e.kind = 'accrual'
  AND NOT EXISTS (
    SELECT 1 FROM ledger_entries r
    WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id
  )
ORDER BY e.id;

-- name: GetRentalLedgerFacts :one
-- What the rent accrual of a rental depends on.
SELECT r.id, r.uuid, r.reference, r.status, r.reservation_id, r.actual_pickup_at, r.actual_pickup_branch_id,
    res.uuid AS reservation_uuid, res.customer_id, res.invoice_customer_id, res.total_amount, res.currency,
    res.collecting_party, res.pickup_branch_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id);

-- name: ListRentalChargesForLedger :many
SELECT ch.id, ch.uuid, ch.kind, ch.status, ch.amount, ch.currency, ch.disputed, ch.decided_at,
    ch.reversal_of_id,
    EXISTS (
        SELECT 1 FROM rental_charges rv
        WHERE rv.organization_id = ch.organization_id AND rv.reversal_of_id = ch.id
    ) AS reversed
FROM rental_charges ch
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.rental_id = sqlc.arg(rental_id)
ORDER BY ch.id;

-- name: GetPaymentForLedger :one
SELECT p.id, p.uuid, p.status, p.amount, p.currency, p.provider, p.method, p.reservation_id, p.verified_at,
    p.created_at, p.created_by,
    res.reference AS reservation_reference, res.customer_id, res.invoice_customer_id, res.pickup_branch_id,
    l.purpose AS link_purpose
FROM payments p
JOIN reservations res ON res.organization_id = p.organization_id AND res.id = p.reservation_id
LEFT JOIN payment_links l ON l.organization_id = p.organization_id AND l.id = p.payment_link_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND p.id = sqlc.arg(id);

-- name: GetRefundForLedger :one
SELECT f.id, f.uuid, f.status, f.amount, f.currency, f.created_at, f.created_by, f.payment_id,
    p.reservation_id, res.reference AS reservation_reference, res.customer_id, res.invoice_customer_id,
    res.pickup_branch_id
FROM payment_refunds f
JOIN payments p ON p.organization_id = f.organization_id AND p.id = f.payment_id
JOIN reservations res ON res.organization_id = p.organization_id AND res.id = p.reservation_id
WHERE f.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND f.id = sqlc.arg(id);

-- name: ListUnpostedPayments :many
-- Catch-up sweep: confirmed payments without their collection entry.
SELECT p.organization_id, p.id
FROM payments p
WHERE p.status IN ('succeeded', 'partially_refunded', 'refunded')
  AND p.reservation_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM ledger_entries e
    WHERE e.organization_id = p.organization_id AND e.payment_id = p.id AND e.kind = 'collection'
  )
ORDER BY p.id
LIMIT sqlc.arg(limit_count);

-- name: ListUnpostedRefunds :many
SELECT f.organization_id, f.id
FROM payment_refunds f
WHERE f.status = 'succeeded'
  AND NOT EXISTS (
    SELECT 1 FROM ledger_entries e
    WHERE e.organization_id = f.organization_id AND e.payment_refund_id = f.id AND e.kind = 'refund'
  )
ORDER BY f.id
LIMIT sqlc.arg(limit_count);

-- name: ListLedgerEntries :many
-- Entries with a running balance per account (statement). The running
-- balance is computed over every entry of the account the caller may see,
-- before the date / kind filters apply.
WITH scoped AS (
    SELECT e.*,
        SUM(CASE WHEN e.purpose = 'deposit' THEN 0 WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END)
            OVER (PARTITION BY e.account_id ORDER BY e.entry_date, e.id)::numeric AS running_balance
    FROM ledger_entries e
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
      AND (sqlc.narg(reservation_id)::bigint IS NULL OR e.reservation_id = sqlc.narg(reservation_id)::bigint)
)
SELECT e.id, e.uuid, e.kind, e.direction, e.purpose, e.amount, e.currency, e.entry_date, e.due_date, e.description,
    e.reason, e.source_type, e.source_uuid, e.accrual_generation, e.offset_group, e.created_at, e.running_balance,
    a.uuid AS account_uuid, a.code AS account_code, a.name AS account_name,
    rt.uuid AS rental_uuid, rt.reference AS rental_reference,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference,
    ch.uuid AS charge_uuid, ch.kind AS charge_kind,
    p.uuid AS payment_uuid,
    rv.uuid AS reverses_uuid,
    rb.uuid AS reversed_by_uuid,
    adj.uuid AS adjusts_uuid,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    COALESCE((
        SELECT string_agg(l.document_type || ':' || l.document_ref, ',' ORDER BY l.id)
        FROM ledger_entry_links l
        WHERE l.organization_id = e.organization_id AND l.entry_id = e.id
    ), '')::text AS links,
    COUNT(*) OVER ()::bigint AS total
FROM scoped e
JOIN accounts a ON a.organization_id = e.organization_id AND a.id = e.account_id
LEFT JOIN rentals rt ON rt.organization_id = e.organization_id AND rt.id = e.rental_id
LEFT JOIN reservations res ON res.organization_id = e.organization_id AND res.id = e.reservation_id
LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = e.rental_charge_id
LEFT JOIN payments p ON p.organization_id = e.organization_id AND p.id = e.payment_id
LEFT JOIN ledger_entries rv ON rv.organization_id = e.organization_id AND rv.id = e.reverses_entry_id
LEFT JOIN ledger_entries rb ON rb.organization_id = e.organization_id AND rb.reverses_entry_id = e.id
LEFT JOIN ledger_entries adj ON adj.organization_id = e.organization_id AND adj.id = e.adjusts_entry_id
LEFT JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
LEFT JOIN users u ON u.id = e.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(entry_uuid)::uuid IS NULL OR e.uuid = sqlc.narg(entry_uuid)::uuid)
  AND (sqlc.narg(kinds)::text[] IS NULL OR e.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(purposes)::text[] IS NULL OR e.purpose = ANY(sqlc.narg(purposes)::text[]))
  AND (sqlc.narg(directions)::text[] IS NULL OR e.direction = ANY(sqlc.narg(directions)::text[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR e.entry_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR e.entry_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.description ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.code ILIKE '%' || sqlc.narg(q) || '%'
    OR res.reference ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN e.entry_date END DESC,
    CASE WHEN sqlc.arg(sort_desc)::boolean THEN e.id END DESC,
    CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN e.entry_date END ASC,
    CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN e.id END ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: SumLedgerEntries :one
-- Totals of the entries the caller may see for one account / reservation.
SELECT COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit' AND e.purpose <> 'deposit'), 0)::numeric AS debit,
    COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit' AND e.purpose <> 'deposit'), 0)::numeric AS credit,
    COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END)
        FILTER (WHERE e.purpose = 'deposit'), 0)::numeric AS deposit_held,
    COALESCE(SUM(CASE WHEN e.purpose = 'deposit' THEN 0 WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END)
        FILTER (WHERE sqlc.narg(before)::date IS NOT NULL AND e.entry_date < sqlc.narg(before)::date), 0)::numeric AS opening,
    COUNT(*)::bigint AS entry_count
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(reservation_id)::bigint IS NULL OR e.reservation_id = sqlc.narg(reservation_id)::bigint);

-- name: InsertLedgerEntryLink :exec
INSERT INTO ledger_entry_links (organization_id, entry_id, document_type, document_ref, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(entry_id), sqlc.arg(document_type), sqlc.arg(document_ref), sqlc.narg(created_by))
ON CONFLICT (organization_id, entry_id, document_type, document_ref) DO NOTHING;

-- name: LedgerBranchBreakdown :many
SELECT e.branch_id, b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit' AND e.purpose <> 'deposit'), 0)::numeric AS debit,
    COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit' AND e.purpose <> 'deposit'), 0)::numeric AS credit
FROM ledger_entries e
LEFT JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY e.branch_id, b.uuid, b.code, b.name
ORDER BY b.code NULLS FIRST;

-- name: LedgerAccountAging :many
-- Per account: balance and the open charges by due bucket (FIFO), for the
-- organization summary.
SELECT e.account_id,
    COALESCE(SUM(CASE WHEN e.purpose = 'deposit' THEN 0 WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END), 0)::numeric AS balance,
    COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END)
        FILTER (WHERE e.purpose = 'deposit'), 0)::numeric AS deposit_held,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(late)::date), 0)::numeric AS charges_late,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(today)::date), 0)::numeric AS charges_overdue,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END) FILTER (WHERE e.due_date < sqlc.arg(soon)::date), 0)::numeric AS charges_soon,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.direction = 'debit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.direction = 'credit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END), 0)::numeric AS charges_all,
    COALESCE(SUM(CASE
        WHEN e.purpose = 'deposit' THEN 0
        WHEN e.direction = 'credit' AND e.kind <> 'reversal' THEN e.amount
        WHEN e.direction = 'debit' AND e.kind = 'reversal' THEN -e.amount
        ELSE 0 END), 0)::numeric AS settlements
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY e.account_id;

-- name: ListRentalBalances :many
-- Balances per rental (entries of its reservation).
WITH agg AS (
    SELECT e.reservation_id,
        COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit' AND e.purpose <> 'deposit'), 0)::numeric AS debit,
        COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit' AND e.purpose <> 'deposit'), 0)::numeric AS credit,
        COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END)
            FILTER (WHERE e.purpose = 'deposit'), 0)::numeric AS deposit_held,
        MAX(e.created_at)::timestamptz AS last_entry_at
    FROM ledger_entries e
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND e.reservation_id IS NOT NULL
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
    GROUP BY e.reservation_id
)
SELECT rt.uuid AS rental_uuid, rt.reference, rt.status AS rental_status,
    res.uuid AS reservation_uuid,
    a.uuid AS account_uuid, a.code AS account_code, a.name AS account_name,
    agg.debit, agg.credit, (agg.debit - agg.credit)::numeric AS balance, agg.deposit_held, agg.last_entry_at,
    COUNT(*) OVER ()::bigint AS total
FROM agg
JOIN reservations res ON res.organization_id = sqlc.arg(organization_id) AND res.id = agg.reservation_id
LEFT JOIN rentals rt ON rt.organization_id = res.organization_id AND rt.reservation_id = res.id AND rt.status <> 'cancelled'
LEFT JOIN accounts a ON a.organization_id = res.organization_id
    AND a.customer_id = COALESCE(res.invoice_customer_id, res.customer_id)
WHERE (NOT sqlc.arg(open_only)::boolean OR agg.debit <> agg.credit)
  AND (sqlc.narg(q)::text IS NULL OR res.reference ILIKE '%' || sqlc.narg(q) || '%' OR a.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY agg.last_entry_at DESC, res.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: OpenReservationBalances :many
-- Reservations of an account with an open balance (pay link target).
SELECT res.uuid, res.reference, res.status,
    (COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit' AND e.purpose <> 'deposit'), 0)
        - COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit' AND e.purpose <> 'deposit'), 0))::numeric AS balance
FROM ledger_entries e
JOIN reservations res ON res.organization_id = e.organization_id AND res.id = e.reservation_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.account_id = sqlc.arg(account_id)
GROUP BY res.id, res.uuid, res.reference, res.status
HAVING (COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit' AND e.purpose <> 'deposit'), 0)
        - COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit' AND e.purpose <> 'deposit'), 0)) > 0
ORDER BY res.id DESC;

-- name: GetRentalRef :one
SELECT r.id, r.uuid, r.reference, r.reservation_id, r.status, res.pickup_branch_id, res.return_branch_id,
    r.actual_return_branch_id, res.customer_id, res.invoice_customer_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: GetBranchRefByUUID :one
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;
