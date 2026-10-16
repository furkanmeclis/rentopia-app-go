-- Collections, allocations to open items, refunds (TEC-718,
-- docs/design/collections.md). Allocation sums are allocate - release.

-- name: LockOrganizationCollectionCodes :exec
-- Serialises numbering the collections of one organization for the rest
-- of the transaction.
SELECT pg_advisory_xact_lock(hashtextextended('finance.collections:' || sqlc.arg(organization_id)::bigint::text, 0));

-- name: NextCollectionCode :one
-- The next TH- code (call under LockOrganizationCollectionCodes).
SELECT ('TH-' || lpad((
    SELECT count(*) + 1 FROM collections x WHERE x.organization_id = sqlc.arg(organization_id)
)::text, 6, '0'))::text AS code;

-- name: InsertCollection :one
INSERT INTO collections (
    uuid, organization_id, code, account_id, method, provider, purpose, currency, amount, gross_amount,
    commission_amount, net_amount, collection_date, reference_no, payer_name, note, branch_id, cash_account_id,
    rental_id, reservation_id, payment_id, ledger_entry_id, cash_movement_id, settlement_status, idempotency_key,
    created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(account_id), sqlc.arg(method), sqlc.arg(provider), sqlc.arg(purpose), sqlc.arg(currency),
    sqlc.arg(amount), sqlc.arg(gross_amount), sqlc.arg(commission_amount), sqlc.arg(net_amount),
    sqlc.arg(collection_date), sqlc.arg(reference_no), sqlc.arg(payer_name), sqlc.arg(note), sqlc.narg(branch_id),
    sqlc.narg(cash_account_id), sqlc.narg(rental_id), sqlc.narg(reservation_id), sqlc.narg(payment_id),
    sqlc.arg(ledger_entry_id), sqlc.narg(cash_movement_id), sqlc.arg(settlement_status), sqlc.narg(idempotency_key),
    sqlc.narg(created_by)
)
RETURNING id, uuid, code;

-- name: GetCollectionRow :one
SELECT c.id, c.uuid, c.code, c.account_id, c.method, c.provider, c.purpose, c.status, c.currency, c.amount,
    c.gross_amount, c.commission_amount, c.net_amount, c.refunded_amount, c.collection_date, c.reference_no,
    c.payer_name, c.branch_id, c.cash_account_id, c.rental_id, c.reservation_id, c.payment_id, c.ledger_entry_id,
    c.cash_movement_id, c.settlement_status,
    (SELECT e.uuid FROM ledger_entries e
     WHERE e.organization_id = c.organization_id AND e.id = c.ledger_entry_id)::uuid AS ledger_entry_uuid
FROM collections c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: GetCollectionRowByID :one
SELECT c.id, c.uuid, c.code, c.account_id, c.method, c.provider, c.purpose, c.status, c.currency, c.amount,
    c.gross_amount, c.commission_amount, c.net_amount, c.refunded_amount, c.collection_date, c.reference_no,
    c.payer_name, c.branch_id, c.cash_account_id, c.rental_id, c.reservation_id, c.payment_id, c.ledger_entry_id,
    c.cash_movement_id, c.settlement_status,
    (SELECT e.uuid FROM ledger_entries e
     WHERE e.organization_id = c.organization_id AND e.id = c.ledger_entry_id)::uuid AS ledger_entry_uuid
FROM collections c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id);

-- name: LockCollection :one
SELECT c.id, c.status, c.refunded_amount
FROM collections c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id)
FOR UPDATE;

-- name: GetCollectionIDByPayment :one
SELECT c.id, c.uuid
FROM collections c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.payment_id = sqlc.arg(payment_id);

-- name: GetCollectionIDByEntry :one
SELECT c.id, c.uuid
FROM collections c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.ledger_entry_id = sqlc.arg(ledger_entry_id);

-- name: GetCollectionByIdempotencyKey :one
SELECT c.id, c.uuid, c.account_id
FROM collections c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.idempotency_key = sqlc.arg(idempotency_key);

-- name: ListCollections :many
-- One page of collections with their allocation state. The branch scope
-- applies to the collection's branch (head office collections need every
-- branch).
WITH alloc AS (
    SELECT a.collection_id,
        SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated
    FROM collection_allocations a
    WHERE a.organization_id = sqlc.arg(organization_id)
    GROUP BY a.collection_id
), scoped AS (
    SELECT c.*, COALESCE(alloc.allocated, 0)::numeric AS allocated,
        (c.amount - c.refunded_amount - COALESCE(alloc.allocated, 0))::numeric AS unallocated
    FROM collections c
    LEFT JOIN alloc ON alloc.collection_id = c.id
    WHERE c.organization_id = sqlc.arg(organization_id)
)
SELECT c.id, c.uuid, c.code, c.method, c.provider, c.purpose, c.status, c.currency, c.amount, c.gross_amount,
    c.commission_amount, c.net_amount, c.refunded_amount, c.allocated, c.unallocated, c.collection_date,
    c.reference_no, c.payer_name, c.note, c.settlement_status, c.settled_at, c.void_reason, c.voided_at,
    c.created_at, c.updated_at,
    a.uuid AS account_uuid, a.code AS account_code, a.name AS account_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    ca.uuid AS cash_account_uuid, ca.code AS cash_account_code, ca.name AS cash_account_name,
    ca.kind AS cash_account_kind,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference,
    p.uuid AS payment_uuid,
    e.uuid AS ledger_entry_uuid,
    mv.uuid AS cash_movement_uuid,
    ex.uuid AS commission_expense_uuid,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM scoped c
JOIN accounts a ON a.organization_id = c.organization_id AND a.id = c.account_id
JOIN ledger_entries e ON e.organization_id = c.organization_id AND e.id = c.ledger_entry_id
LEFT JOIN branches b ON b.organization_id = c.organization_id AND b.id = c.branch_id
LEFT JOIN cash_accounts ca ON ca.organization_id = c.organization_id AND ca.id = c.cash_account_id
LEFT JOIN rentals r ON r.organization_id = c.organization_id AND r.id = c.rental_id
LEFT JOIN reservations res ON res.organization_id = c.organization_id AND res.id = c.reservation_id
LEFT JOIN payments p ON p.organization_id = c.organization_id AND p.id = c.payment_id
LEFT JOIN cash_movements mv ON mv.organization_id = c.organization_id AND mv.id = c.cash_movement_id
LEFT JOIN expenses ex ON ex.organization_id = c.organization_id AND ex.id = c.commission_expense_id
LEFT JOIN users u ON u.id = c.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR c.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(collection_uuid)::uuid IS NULL OR c.uuid = sqlc.narg(collection_uuid)::uuid)
  AND (sqlc.narg(ids)::bigint[] IS NULL OR c.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(account_id)::bigint IS NULL OR c.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(rental_id)::bigint IS NULL OR c.rental_id = sqlc.narg(rental_id)::bigint)
  AND (sqlc.narg(methods)::text[] IS NULL OR c.method = ANY(sqlc.narg(methods)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR c.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR c.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(cash_account_ids)::bigint[] IS NULL OR c.cash_account_id = ANY(sqlc.narg(cash_account_ids)::bigint[]))
  AND (
    sqlc.narg(allocation_statuses)::text[] IS NULL
    OR (CASE
          WHEN c.status = 'void' OR c.purpose = 'deposit' THEN 'not_allocatable'
          WHEN c.allocated <= 0 AND c.unallocated > 0 THEN 'unallocated'
          WHEN c.unallocated > 0 THEN 'partially_allocated'
          ELSE 'allocated'
        END) = ANY(sqlc.narg(allocation_statuses)::text[])
  )
  AND (sqlc.narg(date_from)::date IS NULL OR c.collection_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR c.collection_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.code ILIKE '%' || sqlc.narg(q) || '%'
    OR c.reference_no ILIKE '%' || sqlc.narg(q) || '%'
    OR c.payer_name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.code ILIKE '%' || sqlc.narg(q) || '%'
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN c.amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN c.amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'collection_date' AND NOT sqlc.arg(sort_desc)::boolean THEN c.collection_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'collection_date' AND sqlc.arg(sort_desc)::boolean THEN c.collection_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN c.code END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN c.code END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
    c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: SetCollectionRefunded :execrows
UPDATE collections
SET refunded_amount = sqlc.arg(refunded_amount), status = sqlc.arg(status)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'void';

-- name: VoidCollection :execrows
UPDATE collections
SET status = 'void', void_reason = sqlc.arg(void_reason), voided_by = sqlc.narg(voided_by), voided_at = sqlc.arg(voided_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'recorded'
  AND refunded_amount = 0;

-- name: SettleCollection :execrows
UPDATE collections
SET settlement_status = 'settled', settled_at = sqlc.arg(settled_at), cash_account_id = sqlc.arg(cash_account_id),
    cash_movement_id = sqlc.arg(cash_movement_id), commission_expense_id = sqlc.narg(commission_expense_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND settlement_status = 'pending';

-- name: ListPendingSettlements :many
-- Sweep: online collections whose settlement account exists now.
SELECT c.organization_id, c.id
FROM collections c
WHERE c.settlement_status = 'pending'
  AND c.status <> 'void'
  AND EXISTS (
    SELECT 1 FROM cash_accounts ca
    WHERE ca.organization_id = c.organization_id AND ca.provider = c.provider AND ca.status = 'active'
      AND ca.currency = c.currency
  )
ORDER BY c.id
LIMIT sqlc.arg(limit_count);

-- name: ListPaymentsWithoutCollection :many
-- Sweep: posted payments whose collection document is missing.
SELECT e.organization_id, e.payment_id
FROM ledger_entries e
WHERE e.kind = 'collection'
  AND e.payment_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM collections c WHERE c.organization_id = e.organization_id AND c.ledger_entry_id = e.id
  )
ORDER BY e.id
LIMIT sqlc.arg(limit_count);

-- name: GetProviderSettlementAccount :one
SELECT ca.id, ca.uuid, ca.branch_id, ca.currency, ca.kind
FROM cash_accounts ca
WHERE ca.organization_id = sqlc.arg(organization_id)
  AND ca.provider = sqlc.arg(provider)
  AND ca.status = 'active'
  AND ca.currency = sqlc.arg(currency)
LIMIT 1;

-- name: SetCashAccountProvider :execrows
UPDATE cash_accounts
SET provider = sqlc.narg(provider)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetCashAccountProvider :one
SELECT ca.provider
FROM cash_accounts ca
WHERE ca.organization_id = sqlc.arg(organization_id)
  AND ca.id = sqlc.arg(id);

-- name: InsertCollectionAllocation :one
INSERT INTO collection_allocations (organization_id, collection_id, entry_id, kind, amount, source, request_key, reason, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(collection_id), sqlc.arg(entry_id), sqlc.arg(kind), sqlc.arg(amount),
    sqlc.arg(source), sqlc.narg(request_key), sqlc.arg(reason), sqlc.narg(created_by))
RETURNING id, uuid;

-- name: CountAllocationRequest :one
SELECT count(*)::bigint
FROM collection_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.collection_id = sqlc.arg(collection_id)
  AND a.request_key = sqlc.arg(request_key);

-- name: ListAllocationsByRequest :many
SELECT a.entry_id, a.amount
FROM collection_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.collection_id = sqlc.arg(collection_id)
  AND a.request_key = sqlc.arg(request_key)
ORDER BY a.id;

-- name: CollectionAllocated :one
SELECT COALESCE(SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END), 0)::numeric AS allocated
FROM collection_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.collection_id = sqlc.arg(collection_id);

-- name: ListCollectionAllocationPairs :many
-- The net allocations of one collection per open item, the most recently
-- allocated first (a refund releases from the newest).
SELECT a.entry_id,
    SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated,
    MAX(a.id) FILTER (WHERE a.kind = 'allocate')::bigint AS last_id,
    MIN(a.created_at)::timestamptz AS first_at
FROM collection_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.collection_id = sqlc.arg(collection_id)
GROUP BY a.entry_id
HAVING SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END) > 0
ORDER BY MAX(a.id) FILTER (WHERE a.kind = 'allocate') DESC;

-- name: ListEntryAllocationPairs :many
-- The collections allocated to one open item (released when the accrual
-- is reversed).
SELECT a.collection_id,
    SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated
FROM collection_allocations a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.entry_id = sqlc.arg(entry_id)
GROUP BY a.collection_id
HAVING SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END) > 0
ORDER BY a.collection_id;

-- name: ListCollectionAllocations :many
-- The open items a collection pays (net per item) for the detail page.
WITH pairs AS (
    SELECT a.entry_id,
        SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated,
        MIN(a.created_at)::timestamptz AS first_at,
        MAX(a.created_at)::timestamptz AS last_at
    FROM collection_allocations a
    WHERE a.organization_id = sqlc.arg(organization_id)
      AND a.collection_id = sqlc.arg(collection_id)
    GROUP BY a.entry_id
), entry_alloc AS (
    SELECT a.entry_id, SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated
    FROM collection_allocations a
    WHERE a.organization_id = sqlc.arg(organization_id)
      AND a.entry_id IN (SELECT entry_id FROM pairs)
    GROUP BY a.entry_id
)
SELECT e.uuid AS entry_uuid, e.purpose, e.description, e.entry_date, e.due_date, e.amount AS entry_amount, e.currency,
    pairs.allocated, pairs.first_at, pairs.last_at,
    COALESCE(entry_alloc.allocated, 0)::numeric AS entry_allocated,
    EXISTS (
        SELECT 1 FROM ledger_entries rv WHERE rv.organization_id = e.organization_id AND rv.reverses_entry_id = e.id
    )::boolean AS reversed,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    ch.uuid AS charge_uuid, ch.kind AS charge_kind
FROM pairs
JOIN ledger_entries e ON e.organization_id = sqlc.arg(organization_id) AND e.id = pairs.entry_id
LEFT JOIN entry_alloc ON entry_alloc.entry_id = pairs.entry_id
LEFT JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = e.rental_charge_id
WHERE e.organization_id = sqlc.arg(organization_id)
ORDER BY e.entry_date, e.id;

-- name: ListOpenItems :many
-- Open items of an account: live accruals (rent, charges) with what is
-- allocated to them. Branch scope applies to the entry's branch.
WITH entry_alloc AS (
    SELECT a.entry_id, SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated
    FROM collection_allocations a
    WHERE a.organization_id = sqlc.arg(organization_id)
    GROUP BY a.entry_id
)
SELECT e.id, e.uuid, e.purpose, e.description, e.entry_date, e.due_date, e.amount, e.currency, e.branch_id,
    COALESCE(entry_alloc.allocated, 0)::numeric AS allocated,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    ch.uuid AS charge_uuid, ch.kind AS charge_kind
FROM ledger_entries e
LEFT JOIN entry_alloc ON entry_alloc.entry_id = e.id
LEFT JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = e.rental_charge_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.account_id = sqlc.arg(account_id)
  AND e.kind = 'accrual'
  AND e.direction = 'debit'
  AND NOT EXISTS (
    SELECT 1 FROM ledger_entries rv WHERE rv.organization_id = e.organization_id AND rv.reverses_entry_id = e.id
  )
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(rental_id)::bigint IS NULL OR e.rental_id = sqlc.narg(rental_id)::bigint)
  AND (sqlc.narg(entry_ids)::bigint[] IS NULL OR e.id = ANY(sqlc.narg(entry_ids)::bigint[]))
  AND (NOT sqlc.arg(open_only)::boolean OR e.amount > COALESCE(entry_alloc.allocated, 0))
ORDER BY e.entry_date, e.due_date, e.id;

-- name: ListCreditCollections :many
-- Collections of an account with money left to allocate (the party's
-- credit), oldest first. Deposits are not free credit.
WITH alloc AS (
    SELECT a.collection_id, SUM(CASE WHEN a.kind = 'allocate' THEN a.amount ELSE -a.amount END)::numeric AS allocated
    FROM collection_allocations a
    WHERE a.organization_id = sqlc.arg(organization_id)
    GROUP BY a.collection_id
)
SELECT c.id, c.uuid, c.code, c.collection_date, c.branch_id, c.rental_id, c.reservation_id,
    (c.amount - c.refunded_amount - COALESCE(alloc.allocated, 0))::numeric AS unallocated
FROM collections c
LEFT JOIN alloc ON alloc.collection_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.account_id = sqlc.arg(account_id)
  AND c.status <> 'void'
  AND c.purpose <> 'deposit'
  AND c.amount - c.refunded_amount - COALESCE(alloc.allocated, 0) > 0
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR c.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(ids)::bigint[] IS NULL OR c.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (
    sqlc.narg(rental_id)::bigint IS NULL
    OR c.rental_id = sqlc.narg(rental_id)::bigint
    OR (sqlc.narg(reservation_id)::bigint IS NOT NULL AND c.reservation_id = sqlc.narg(reservation_id)::bigint)
  )
ORDER BY c.collection_date, c.id;

-- name: ListCollectionRefunds :many
-- Refunds of one collection: provider refunds of its payment (any status,
-- with the entry once it succeeded) and manual refunds (ledger entries
-- naming the collection).
SELECT f.uuid, f.amount, f.currency, f.status, f.reason, f.failure_reason, f.provider_refund_id,
    e.uuid AS entry_uuid, mv.uuid AS movement_uuid, ca.uuid AS cash_account_uuid, ca.code AS cash_account_code,
    ca.name AS cash_account_name, f.created_at,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email
FROM payment_refunds f
JOIN collections c ON c.organization_id = f.organization_id AND c.payment_id = f.payment_id
LEFT JOIN ledger_entries e ON e.organization_id = f.organization_id AND e.payment_refund_id = f.id AND e.kind = 'refund'
LEFT JOIN cash_movements mv ON mv.organization_id = f.organization_id AND mv.ledger_entry_id = e.id AND mv.kind = 'refund'
LEFT JOIN cash_accounts ca ON ca.organization_id = f.organization_id AND ca.id = mv.cash_account_id
LEFT JOIN users u ON u.id = f.created_by
WHERE f.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(collection_id)
UNION ALL
SELECT e.uuid, e.amount, e.currency, 'succeeded'::varchar AS status, e.reason, ''::text AS failure_reason,
    NULL::varchar AS provider_refund_id, e.uuid AS entry_uuid, mv.uuid AS movement_uuid, ca.uuid AS cash_account_uuid,
    ca.code AS cash_account_code, ca.name AS cash_account_name, e.created_at,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email
FROM ledger_entries e
LEFT JOIN cash_movements mv ON mv.organization_id = e.organization_id AND mv.ledger_entry_id = e.id AND mv.kind = 'refund'
LEFT JOIN cash_accounts ca ON ca.organization_id = e.organization_id AND ca.id = mv.cash_account_id
LEFT JOIN users u ON u.id = e.created_by
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.collection_id = sqlc.arg(collection_id)
  AND e.kind = 'refund'
  AND e.payment_refund_id IS NULL
ORDER BY created_at;

-- name: InsertCollectionRefundEntry :one
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    reason, source_type, source_uuid, rental_id, reservation_id, collection_id, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), 'refund', 'debit', sqlc.arg(purpose), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(entry_date), sqlc.arg(description), sqlc.arg(reason),
    'collection', sqlc.arg(source_uuid), sqlc.narg(rental_id), sqlc.narg(reservation_id), sqlc.arg(collection_id),
    sqlc.narg(branch_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: InsertPaymentRefundCollectionEntry :one
-- Idempotent like InsertPaymentRefundEntry; names the collection.
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    source_type, source_uuid, reservation_id, payment_id, payment_refund_id, collection_id, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), 'refund', 'debit', sqlc.arg(purpose), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(entry_date), sqlc.arg(description), 'payment_refund',
    sqlc.arg(source_uuid), sqlc.narg(reservation_id), sqlc.narg(payment_id), sqlc.arg(payment_refund_id),
    sqlc.narg(collection_id), sqlc.narg(branch_id), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, payment_refund_id) WHERE kind = 'refund' AND payment_refund_id IS NOT NULL DO NOTHING
RETURNING id, uuid;

-- name: GetPaymentSettlementFacts :one
SELECT p.id, p.uuid, p.provider, p.method, p.amount, p.gross_amount, p.commission_amount, p.net_amount,
    p.provider_payment_id, p.currency
FROM payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.id = sqlc.arg(id);

-- name: GetReservationRentalID :one
-- The live rental of a reservation (prepayments are allocated to it).
SELECT r.id
FROM rentals r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.reservation_id = sqlc.arg(reservation_id)
  AND r.status <> 'cancelled'
ORDER BY r.id DESC
LIMIT 1;

-- name: GetPaymentCollectionEntry :one
SELECT e.id, e.uuid, e.entry_date, e.purpose, e.branch_id, e.created_by
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.payment_id = sqlc.arg(payment_id)
  AND e.kind = 'collection';

-- name: GetCommissionCategoryID :one
SELECT x.id
FROM expense_categories x
WHERE x.organization_id = sqlc.arg(organization_id)
  AND x.code = 'payment_commission';
