-- Security deposits and pre-authorizations (TEC-719, docs/design/deposits.md).
-- Every query is scoped by organization_id; lists also take the caller's
-- branch scope (the branch the deposit was taken at).

-- name: GetRentalForDeposit :one
-- What a deposit of a rental depends on: the party (invoice customer, or
-- the customer), the branch, the product's deposit amount.
SELECT r.id, r.uuid, r.reference, r.status, r.reservation_id, r.actual_pickup_branch_id, r.actual_return_branch_id,
    res.uuid AS reservation_uuid, res.customer_id, res.invoice_customer_id, res.pickup_branch_id, res.return_branch_id,
    res.currency, res.product_snapshot,
    c.uuid AS customer_uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.email, c.phone,
    c.anonymized_at
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = COALESCE(res.invoice_customer_id, res.customer_id)
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: InsertSecurityDeposit :one
INSERT INTO security_deposits (
    organization_id, rental_id, reservation_id, customer_id, account_id, branch_id, amount, currency, method, status,
    cash_account_id, pos_reference, provider, integration_account_id, authorization_expires_at, note, held_at,
    created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(reservation_id), sqlc.arg(customer_id),
    sqlc.narg(account_id), sqlc.arg(branch_id), sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(method),
    sqlc.arg(status), sqlc.narg(cash_account_id), sqlc.arg(pos_reference), sqlc.arg(provider),
    sqlc.narg(integration_account_id), sqlc.narg(authorization_expires_at), sqlc.arg(note), sqlc.narg(held_at),
    sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetSecurityDepositRow :one
SELECT d.id, d.uuid, d.rental_id, d.reservation_id, d.customer_id, d.account_id, d.branch_id, d.amount, d.currency,
    d.method, d.status, d.cash_account_id, d.pos_reference, d.provider, d.integration_account_id, d.provider_token,
    d.provider_payment_id, d.payment_page_url, d.captured_amount, d.returned_amount, d.held_at, d.resolved_at,
    d.resolution_key, d.created_at,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    ca.uuid AS cash_account_uuid
FROM security_deposits d
JOIN rentals r ON r.organization_id = d.organization_id AND r.id = d.rental_id
LEFT JOIN cash_accounts ca ON ca.organization_id = d.organization_id AND ca.id = d.cash_account_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid);

-- name: LockSecurityDeposit :one
-- Serialises every change of one deposit (take, verify, resolve, cancel).
SELECT d.id, d.status, d.resolution_key, d.captured_amount, d.returned_amount, d.account_id, d.provider_payment_id
FROM security_deposits d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.id = sqlc.arg(id)
FOR UPDATE;

-- name: ListSecurityDeposits :many
-- One page of deposits with their rental, party, branch and cash account.
-- held_before: only deposits still held since before that time ("held
-- over N days").
SELECT d.id, d.uuid, d.amount, d.currency, d.method, d.status, d.pos_reference, d.provider, d.provider_payment_id,
    d.payment_page_url, d.authorization_expires_at, d.captured_amount, d.returned_amount, d.note, d.held_at,
    d.resolved_at, d.resolution_note, d.cancel_reason, d.customer_notice_status, d.customer_notice, d.created_at,
    d.updated_at,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    res.uuid AS reservation_uuid, res.reference AS reservation_reference,
    c.uuid AS customer_uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.anonymized_at,
    a.uuid AS account_uuid, a.code AS account_code,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    ca.uuid AS cash_account_uuid, ca.code AS cash_account_code, ca.name AS cash_account_name, ca.kind AS cash_account_kind,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    ru.uuid AS resolved_by_uuid, ru.name AS resolved_by_name, ru.surname AS resolved_by_surname, ru.email AS resolved_by_email,
    (SELECT count(*) FROM deposit_deductions x
      WHERE x.organization_id = d.organization_id AND x.deposit_id = d.id)::bigint AS deduction_count,
    COUNT(*) OVER ()::bigint AS total
FROM security_deposits d
JOIN rentals r ON r.organization_id = d.organization_id AND r.id = d.rental_id
JOIN reservations res ON res.organization_id = d.organization_id AND res.id = d.reservation_id
JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
JOIN branches b ON b.organization_id = d.organization_id AND b.id = d.branch_id
LEFT JOIN accounts a ON a.organization_id = d.organization_id AND a.id = d.account_id
LEFT JOIN cash_accounts ca ON ca.organization_id = d.organization_id AND ca.id = d.cash_account_id
LEFT JOIN users u ON u.id = d.created_by
LEFT JOIN users ru ON ru.id = d.resolved_by
WHERE d.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR d.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(ids)::bigint[] IS NULL OR d.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(deposit_uuid)::uuid IS NULL OR d.uuid = sqlc.narg(deposit_uuid)::uuid)
  AND (sqlc.narg(rental_id)::bigint IS NULL OR d.rental_id = sqlc.narg(rental_id)::bigint)
  AND (sqlc.narg(statuses)::text[] IS NULL OR d.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(methods)::text[] IS NULL OR d.method = ANY(sqlc.narg(methods)::text[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR d.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(date_from)::timestamptz IS NULL OR d.created_at >= sqlc.narg(date_from)::timestamptz)
  AND (sqlc.narg(date_to)::timestamptz IS NULL OR d.created_at < sqlc.narg(date_to)::timestamptz)
  AND (sqlc.narg(held_before)::timestamptz IS NULL
       OR (d.status = 'held' AND d.held_at < sqlc.narg(held_before)::timestamptz))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR res.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR d.pos_reference ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.anonymized_at IS NULL AND (
        c.first_name ILIKE '%' || sqlc.narg(q) || '%'
        OR c.last_name ILIKE '%' || sqlc.narg(q) || '%'
        OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    ))
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN d.amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN d.amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'held_at' AND NOT sqlc.arg(sort_desc)::boolean THEN d.held_at END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'held_at' AND sqlc.arg(sort_desc)::boolean THEN d.held_at END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN d.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN d.created_at END DESC,
    d.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: SecurityDepositTotals :many
-- Open deposits by status and method in the caller's branch scope, and
-- how many have been held since before held_before.
SELECT d.status, d.method,
    COUNT(*)::bigint AS deposit_count,
    COALESCE(SUM(d.amount - d.captured_amount - d.returned_amount), 0)::numeric AS open_amount,
    COUNT(*) FILTER (WHERE d.status = 'held' AND d.held_at < sqlc.arg(held_before)::timestamptz)::bigint AS held_over
FROM security_deposits d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.status IN ('pending', 'held')
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR d.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY d.status, d.method;

-- name: SetSecurityDepositAccount :exec
UPDATE security_deposits SET account_id = sqlc.arg(account_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetSecurityDepositCheckout :exec
UPDATE security_deposits
SET provider_token = sqlc.arg(provider_token), payment_page_url = sqlc.arg(payment_page_url),
    authorization_expires_at = sqlc.narg(authorization_expires_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: MarkSecurityDepositHeld :execrows
-- pending → held once the provider confirmed the pre-authorization.
UPDATE security_deposits
SET status = 'held', held_at = sqlc.arg(held_at), provider_payment_id = sqlc.arg(provider_payment_id),
    provider_data = sqlc.arg(provider_data), payment_page_url = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: ResolveSecurityDeposit :execrows
-- held → a closed state with the captured / returned totals.
UPDATE security_deposits
SET status = sqlc.arg(status), captured_amount = sqlc.arg(captured_amount), returned_amount = sqlc.arg(returned_amount),
    resolved_at = sqlc.arg(resolved_at), resolution_key = sqlc.arg(resolution_key),
    resolution_note = sqlc.arg(resolution_note), resolved_by = sqlc.narg(resolved_by),
    customer_notice_status = sqlc.arg(customer_notice_status), customer_notice = sqlc.arg(customer_notice)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'held';

-- name: CancelSecurityDeposit :execrows
UPDATE security_deposits
SET status = 'cancelled', resolved_at = sqlc.arg(resolved_at), cancel_reason = sqlc.arg(cancel_reason),
    resolved_by = sqlc.narg(resolved_by), payment_page_url = ''
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- name: InsertDepositDeduction :one
INSERT INTO deposit_deductions (
    organization_id, deposit_id, amount, applies_to, reason, rental_charge_id, inspection_damage_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(deposit_id), sqlc.arg(amount), sqlc.arg(applies_to), sqlc.arg(reason),
    sqlc.narg(rental_charge_id), sqlc.narg(inspection_damage_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: ListDepositDeductions :many
SELECT x.id, x.uuid, x.amount, x.applies_to, x.reason, x.document_name, x.document_content_type,
    x.document_size_bytes, (x.document_object_key IS NOT NULL)::boolean AS has_document, x.created_at,
    ch.uuid AS charge_uuid, ch.kind AS charge_kind, ch.amount AS charge_amount,
    dm.uuid AS damage_uuid, dm.zone AS damage_zone, dm.damage_type AS damage_type,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email
FROM deposit_deductions x
LEFT JOIN rental_charges ch ON ch.organization_id = x.organization_id AND ch.id = x.rental_charge_id
LEFT JOIN inspection_damages dm ON dm.organization_id = x.organization_id AND dm.id = x.inspection_damage_id
LEFT JOIN users u ON u.id = x.created_by
WHERE x.organization_id = sqlc.arg(organization_id)
  AND x.deposit_id = sqlc.arg(deposit_id)
ORDER BY x.id;

-- name: GetDepositDeductionRow :one
SELECT x.id, x.uuid, x.deposit_id, x.document_object_key, x.document_name, x.document_content_type,
    x.document_size_bytes
FROM deposit_deductions x
WHERE x.organization_id = sqlc.arg(organization_id)
  AND x.deposit_id = sqlc.arg(deposit_id)
  AND x.uuid = sqlc.arg(uuid);

-- name: SetDepositDeductionDocument :execrows
UPDATE deposit_deductions
SET document_object_key = sqlc.narg(document_object_key), document_name = sqlc.arg(document_name),
    document_content_type = sqlc.arg(document_content_type), document_size_bytes = sqlc.arg(document_size_bytes)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetRentalChargeForDeposit :one
-- A charge of the rental with what earlier deductions already captured.
SELECT ch.id, ch.uuid, ch.kind, ch.status, ch.amount, ch.reversal_of_id, ch.inspection_id,
    EXISTS (
        SELECT 1 FROM rental_charges rv
        WHERE rv.organization_id = ch.organization_id AND rv.reversal_of_id = ch.id
    )::boolean AS reversed,
    COALESCE((
        SELECT SUM(x.amount) FROM deposit_deductions x
        WHERE x.organization_id = ch.organization_id AND x.rental_charge_id = ch.id
    ), 0)::numeric AS deducted
FROM rental_charges ch
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.rental_id = sqlc.arg(rental_id)
  AND ch.uuid = sqlc.arg(uuid);

-- name: GetInspectionDamageForDeposit :one
-- A damage recorded on one of the rental's inspection forms.
SELECT dm.id, dm.uuid, dm.inspection_id
FROM inspection_damages dm
JOIN inspections i ON i.organization_id = dm.organization_id AND i.id = dm.inspection_id
WHERE dm.organization_id = sqlc.arg(organization_id)
  AND i.organization_id = sqlc.arg(organization_id)
  AND i.rental_id = sqlc.arg(rental_id)
  AND dm.uuid = sqlc.arg(uuid);

-- name: InsertDepositLedgerEntry :one
-- Deposit postings (hold, return, capture pair). The partial unique
-- indexes make a second hold / return per deposit or a second leg per
-- deduction fail.
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    reason, source_type, source_uuid, rental_id, reservation_id, rental_charge_id, security_deposit_id,
    deposit_deduction_id, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), sqlc.arg(kind), sqlc.arg(direction), sqlc.arg(purpose),
    sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(entry_date), sqlc.arg(description),
    sqlc.arg(reason), 'security_deposit', sqlc.arg(source_uuid), sqlc.arg(rental_id), sqlc.arg(reservation_id),
    sqlc.narg(rental_charge_id), sqlc.arg(security_deposit_id), sqlc.narg(deposit_deduction_id),
    sqlc.narg(branch_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetDepositHoldEntry :one
SELECT e.id, e.uuid FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.security_deposit_id = sqlc.arg(security_deposit_id)
  AND e.kind = 'deposit_hold';

-- name: GetOpenDepositForRental :one
SELECT d.id, d.uuid, d.status
FROM security_deposits d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.rental_id = sqlc.arg(rental_id)
  AND d.status IN ('pending', 'held');

-- name: FinishDepositTasks :many
-- The deposit is resolved: the rental's live deposit operation task(s)
-- are completed with the resolution as note.
UPDATE operation_tasks
SET status = 'completed', completed_at = sqlc.arg(completed_at), completed_by = sqlc.narg(actor_id),
    completion_note = sqlc.arg(completion_note), updated_by = sqlc.narg(actor_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND rental_id = sqlc.arg(rental_id)
  AND task_type = 'deposit'
  AND status IN ('planned', 'assigned', 'en_route', 'issue_reported')
RETURNING uuid;

-- name: ListLiveDepositTasks :many
SELECT t.uuid, t.status, t.note
FROM operation_tasks t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.rental_id = sqlc.arg(rental_id)
  AND t.task_type = 'deposit'
  AND t.status IN ('planned', 'assigned', 'en_route', 'issue_reported');

-- name: GetLatestDepositForRental :one
-- The latest deposit of a rental (the handover wizard shows its state).
SELECT d.uuid, d.status, d.method, d.amount
FROM security_deposits d
JOIN rentals r ON r.organization_id = d.organization_id AND r.id = d.rental_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(rental_uuid)
ORDER BY d.created_at DESC, d.id DESC
LIMIT 1;
