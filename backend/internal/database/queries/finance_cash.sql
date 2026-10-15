-- Cash / bank accounts, movements, expense categories, expenses (TEC-720).

-- name: InsertCashAccount :one
INSERT INTO cash_accounts (organization_id, code, name, kind, branch_id, currency, opening_balance, opening_date,
    bank_name, iban, note, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(kind), sqlc.narg(branch_id), sqlc.arg(currency),
    sqlc.arg(opening_balance), sqlc.arg(opening_date), sqlc.arg(bank_name), sqlc.arg(iban), sqlc.arg(note), sqlc.narg(created_by))
RETURNING id, uuid;

-- name: UpdateCashAccount :execrows
UPDATE cash_accounts
SET code = sqlc.arg(code), name = sqlc.arg(name), branch_id = sqlc.narg(branch_id), bank_name = sqlc.arg(bank_name),
    iban = sqlc.arg(iban), note = sqlc.arg(note), status = sqlc.arg(status),
    opening_balance = sqlc.arg(opening_balance), opening_date = sqlc.arg(opening_date)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetCashAccountRow :one
SELECT c.id, c.uuid, c.code, c.name, c.kind, c.branch_id, c.currency, c.opening_balance, c.opening_date, c.bank_name,
    c.iban, c.status, c.note, c.created_at, c.updated_at
FROM cash_accounts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: GetCashAccountByCode :one
SELECT c.id, c.uuid
FROM cash_accounts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND lower(c.code) = lower(sqlc.arg(code)::text);

-- name: LockCashAccounts :many
-- Transfers lock both accounts in id order (no deadlock between two
-- opposite transfers).
SELECT c.id, c.kind, c.currency, c.status, c.opening_balance
FROM cash_accounts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY c.id
FOR UPDATE;

-- name: CashAccountBalance :one
SELECT (c.opening_balance
    + COALESCE((SELECT SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END)
        FROM cash_movements m
        WHERE m.organization_id = c.organization_id AND m.cash_account_id = c.id), 0))::numeric AS balance
FROM cash_accounts c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id);

-- name: ListCashAccounts :many
WITH agg AS (
    SELECT m.cash_account_id,
        COALESCE(SUM(m.amount) FILTER (WHERE m.direction = 'in'), 0)::numeric AS total_in,
        COALESCE(SUM(m.amount) FILTER (WHERE m.direction = 'out'), 0)::numeric AS total_out,
        MAX(m.created_at)::timestamptz AS last_movement_at,
        COUNT(*)::bigint AS movement_count
    FROM cash_movements m
    WHERE m.organization_id = sqlc.arg(organization_id)
    GROUP BY m.cash_account_id
)
SELECT c.id, c.uuid, c.code, c.name, c.kind, c.currency, c.opening_balance, c.opening_date, c.bank_name, c.iban,
    c.status, c.note, c.created_at, c.updated_at,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    COALESCE(agg.total_in, 0)::numeric AS total_in, COALESCE(agg.total_out, 0)::numeric AS total_out,
    (c.opening_balance + COALESCE(agg.total_in, 0) - COALESCE(agg.total_out, 0))::numeric AS balance,
    agg.last_movement_at, COALESCE(agg.movement_count, 0)::bigint AS movement_count,
    COUNT(*) OVER ()::bigint AS total
FROM cash_accounts c
LEFT JOIN agg ON agg.cash_account_id = c.id
LEFT JOIN branches b ON b.organization_id = c.organization_id AND b.id = c.branch_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR c.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(ids)::bigint[] IS NULL OR c.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR c.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR c.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (NOT sqlc.arg(head_office)::boolean OR c.branch_id IS NULL)
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.code ILIKE '%' || sqlc.narg(q) || '%'
    OR c.bank_name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN c.code END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN c.code END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN c.name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN c.name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND NOT sqlc.arg(sort_desc)::boolean
        THEN c.opening_balance + COALESCE(agg.total_in, 0) - COALESCE(agg.total_out, 0) END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND sqlc.arg(sort_desc)::boolean
        THEN c.opening_balance + COALESCE(agg.total_in, 0) - COALESCE(agg.total_out, 0) END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
    c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: SetCashAccountStatus :one
UPDATE cash_accounts AS c
SET status = sqlc.arg(status)
FROM cash_accounts AS prev
WHERE c.organization_id = sqlc.arg(organization_id)
  AND prev.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id)
  AND prev.id = c.id
RETURNING prev.status AS previous;

-- name: InsertCashMovement :one
INSERT INTO cash_movements (
    organization_id, cash_account_id, kind, direction, amount, currency, movement_date, description, reason,
    branch_id, category_id, ledger_entry_id, expense_id, supplier_account_id, transfer_group, counter_account_id,
    reverses_movement_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(cash_account_id), sqlc.arg(kind), sqlc.arg(direction), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(movement_date), sqlc.arg(description), sqlc.arg(reason), sqlc.narg(branch_id),
    sqlc.narg(category_id), sqlc.narg(ledger_entry_id), sqlc.narg(expense_id), sqlc.narg(supplier_account_id),
    sqlc.narg(transfer_group), sqlc.narg(counter_account_id), sqlc.narg(reverses_movement_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetCashMovementRow :one
SELECT m.id, m.uuid, m.cash_account_id, m.kind, m.direction, m.amount, m.currency, m.movement_date, m.description,
    m.reason, m.branch_id, m.category_id, m.ledger_entry_id, m.expense_id, m.transfer_group, m.counter_account_id,
    EXISTS (
        SELECT 1 FROM cash_movements r
        WHERE r.organization_id = m.organization_id AND r.reverses_movement_id = m.id
    )::boolean AS reversed
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.uuid = sqlc.arg(uuid);

-- name: GetMovementByEntry :one
-- The movement that settled a ledger entry (reversed with it).
SELECT m.id, m.uuid, m.cash_account_id, m.direction, m.amount, m.currency, m.branch_id, m.description
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.ledger_entry_id = sqlc.arg(ledger_entry_id)
  AND m.kind IN ('collection', 'refund')
  AND NOT EXISTS (
    SELECT 1 FROM cash_movements r
    WHERE r.organization_id = m.organization_id AND r.reverses_movement_id = m.id
  );

-- name: ListTransferPair :many
SELECT m.id, m.uuid, m.cash_account_id, m.direction, m.amount, m.currency, m.branch_id
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.transfer_group = sqlc.arg(transfer_group)
  AND m.kind IN ('transfer_in', 'transfer_out')
ORDER BY m.id;

-- name: SetCashMovementCategory :one
UPDATE cash_movements AS m
SET category_id = sqlc.narg(category_id)
FROM cash_movements AS prev
WHERE m.organization_id = sqlc.arg(organization_id)
  AND prev.organization_id = sqlc.arg(organization_id)
  AND m.id = sqlc.arg(id)
  AND prev.id = m.id
RETURNING prev.category_id AS previous;

-- name: ListCashMovements :many
WITH scoped AS (
    SELECT m.*,
        SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END)
            OVER (PARTITION BY m.cash_account_id ORDER BY m.movement_date, m.id)::numeric AS running
    FROM cash_movements m
    WHERE m.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(account_id)::bigint IS NULL OR m.cash_account_id = sqlc.narg(account_id)::bigint)
)
SELECT m.id, m.uuid, m.kind, m.direction, m.amount, m.currency, m.movement_date, m.description, m.reason,
    m.transfer_group, m.created_at,
    (c.opening_balance + m.running)::numeric AS running_balance,
    c.uuid AS account_uuid, c.code AS account_code, c.name AS account_name, c.kind AS account_kind,
    ca.uuid AS counter_account_uuid, ca.code AS counter_account_code, ca.name AS counter_account_name,
    cat.uuid AS category_uuid, cat.code AS category_code, cat.name AS category_name,
    e.uuid AS ledger_entry_uuid,
    ex.uuid AS expense_uuid,
    rv.uuid AS reverses_uuid,
    rb.uuid AS reversed_by_uuid,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM scoped m
JOIN cash_accounts c ON c.organization_id = m.organization_id AND c.id = m.cash_account_id
LEFT JOIN cash_accounts ca ON ca.organization_id = m.organization_id AND ca.id = m.counter_account_id
LEFT JOIN expense_categories cat ON cat.organization_id = m.organization_id AND cat.id = m.category_id
LEFT JOIN ledger_entries e ON e.organization_id = m.organization_id AND e.id = m.ledger_entry_id
LEFT JOIN expenses ex ON ex.organization_id = m.organization_id AND ex.id = m.expense_id
LEFT JOIN cash_movements rv ON rv.organization_id = m.organization_id AND rv.id = m.reverses_movement_id
LEFT JOIN cash_movements rb ON rb.organization_id = m.organization_id AND rb.reverses_movement_id = m.id
LEFT JOIN branches b ON b.organization_id = m.organization_id AND b.id = m.branch_id
LEFT JOIN users u ON u.id = m.created_by
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(movement_uuid)::uuid IS NULL OR m.uuid = sqlc.narg(movement_uuid)::uuid)
  AND (sqlc.narg(ids)::bigint[] IS NULL OR m.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(directions)::text[] IS NULL OR m.direction = ANY(sqlc.narg(directions)::text[]))
  AND (sqlc.narg(category_ids)::bigint[] IS NULL OR m.category_id = ANY(sqlc.narg(category_ids)::bigint[]))
  AND (sqlc.narg(filter_account_ids)::bigint[] IS NULL OR m.cash_account_id = ANY(sqlc.narg(filter_account_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR m.movement_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR m.movement_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.description ILIKE '%' || sqlc.narg(q) || '%'
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN m.amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN m.amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'movement_date' AND NOT sqlc.arg(sort_desc)::boolean THEN m.movement_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'movement_date' AND sqlc.arg(sort_desc)::boolean THEN m.movement_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN m.created_at END DESC,
    m.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListExpenseCategories :many
SELECT c.id, c.uuid, c.code, c.name, c.is_system, c.is_active, c.sort_order, c.created_at, c.updated_at,
    (SELECT count(*) FROM expenses x WHERE x.organization_id = c.organization_id AND x.category_id = c.id)::bigint AS expense_count
FROM expense_categories c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (NOT sqlc.arg(active_only)::boolean OR c.is_active)
  AND (sqlc.narg(q)::text IS NULL OR c.name ILIKE '%' || sqlc.narg(q) || '%' OR c.code ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY c.sort_order, c.name, c.id;

-- name: GetExpenseCategory :one
SELECT c.id, c.uuid, c.code, c.name, c.is_system, c.is_active, c.sort_order
FROM expense_categories c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: GetExpenseCategoryByCode :one
SELECT c.id, c.uuid, c.code, c.name, c.is_system, c.is_active, c.sort_order
FROM expense_categories c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND lower(c.code) = lower(sqlc.arg(code)::text);

-- name: InsertExpenseCategory :one
INSERT INTO expense_categories (organization_id, code, name, is_system, is_active, sort_order)
VALUES (sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(is_system), TRUE, sqlc.arg(sort_order))
ON CONFLICT (organization_id, code) DO NOTHING
RETURNING id, uuid;

-- name: UpdateExpenseCategory :execrows
UPDATE expense_categories
SET name = sqlc.arg(name), is_active = sqlc.arg(is_active), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: InsertExpense :one
INSERT INTO expenses (
    organization_id, category_id, expense_date, description, supplier_name, document_no, currency, net_amount,
    vat_rate, vat_amount, total_amount, branch_id, vehicle_id, cash_account_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(category_id), sqlc.arg(expense_date), sqlc.arg(description),
    sqlc.arg(supplier_name), sqlc.arg(document_no), sqlc.arg(currency), sqlc.arg(net_amount), sqlc.arg(vat_rate),
    sqlc.arg(vat_amount), sqlc.arg(total_amount), sqlc.narg(branch_id), sqlc.narg(vehicle_id),
    sqlc.narg(cash_account_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateExpenseDetails :execrows
-- Classification and description only: amounts and the paying account are
-- fixed once recorded (void and record again to change them).
UPDATE expenses
SET category_id = sqlc.arg(category_id), expense_date = sqlc.arg(expense_date), description = sqlc.arg(description),
    supplier_name = sqlc.arg(supplier_name), document_no = sqlc.arg(document_no), branch_id = sqlc.narg(branch_id),
    vehicle_id = sqlc.narg(vehicle_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'recorded';

-- name: SetExpenseCategory :one
UPDATE expenses AS x
SET category_id = sqlc.arg(category_id)
FROM expenses AS prev
WHERE x.organization_id = sqlc.arg(organization_id)
  AND prev.organization_id = sqlc.arg(organization_id)
  AND x.id = sqlc.arg(id)
  AND prev.id = x.id
RETURNING prev.category_id AS previous;

-- name: VoidExpense :execrows
UPDATE expenses
SET status = 'void', void_reason = sqlc.arg(void_reason), voided_by = sqlc.narg(voided_by), voided_at = sqlc.arg(voided_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'recorded';

-- name: SetExpenseAttachment :execrows
UPDATE expenses
SET attachment_object_key = sqlc.narg(attachment_object_key), attachment_name = sqlc.arg(attachment_name),
    attachment_content_type = sqlc.arg(attachment_content_type), attachment_size_bytes = sqlc.arg(attachment_size_bytes)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetExpenseRow :one
SELECT x.id, x.uuid, x.category_id, x.expense_date, x.description, x.supplier_name, x.document_no, x.currency,
    x.net_amount, x.vat_rate, x.vat_amount, x.total_amount, x.branch_id, x.vehicle_id, x.cash_account_id,
    x.attachment_object_key, x.attachment_name, x.attachment_content_type, x.attachment_size_bytes, x.status
FROM expenses x
WHERE x.organization_id = sqlc.arg(organization_id)
  AND x.uuid = sqlc.arg(uuid);

-- name: GetExpensePayment :one
SELECT m.id, m.uuid
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.expense_id = sqlc.arg(expense_id)
  AND m.kind = 'expense';

-- name: ListExpenses :many
SELECT x.id, x.uuid, x.expense_date, x.description, x.supplier_name, x.document_no, x.currency, x.net_amount,
    x.vat_rate, x.vat_amount, x.total_amount, x.status, x.void_reason, x.voided_at, x.created_at, x.updated_at,
    x.attachment_name, x.attachment_content_type, x.attachment_size_bytes,
    (x.attachment_object_key IS NOT NULL)::boolean AS has_attachment,
    cat.uuid AS category_uuid, cat.code AS category_code, cat.name AS category_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    ca.uuid AS cash_account_uuid, ca.code AS cash_account_code, ca.name AS cash_account_name,
    m.uuid AS movement_uuid,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM expenses x
JOIN expense_categories cat ON cat.organization_id = x.organization_id AND cat.id = x.category_id
LEFT JOIN branches b ON b.organization_id = x.organization_id AND b.id = x.branch_id
LEFT JOIN vehicles v ON v.organization_id = x.organization_id AND v.id = x.vehicle_id
LEFT JOIN cash_accounts ca ON ca.organization_id = x.organization_id AND ca.id = x.cash_account_id
LEFT JOIN cash_movements m ON m.organization_id = x.organization_id AND m.expense_id = x.id AND m.kind = 'expense'
LEFT JOIN users u ON u.id = x.created_by
WHERE x.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(expense_uuid)::uuid IS NULL OR x.uuid = sqlc.narg(expense_uuid)::uuid)
  AND (sqlc.narg(ids)::bigint[] IS NULL OR x.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(category_ids)::bigint[] IS NULL OR x.category_id = ANY(sqlc.narg(category_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(vehicle_ids)::bigint[] IS NULL OR x.vehicle_id = ANY(sqlc.narg(vehicle_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR x.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR x.expense_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR x.expense_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR x.description ILIKE '%' || sqlc.narg(q) || '%'
    OR x.supplier_name ILIKE '%' || sqlc.narg(q) || '%'
    OR x.document_no ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'expense_date' AND NOT sqlc.arg(sort_desc)::boolean THEN x.expense_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'expense_date' AND sqlc.arg(sort_desc)::boolean THEN x.expense_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN x.total_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total_amount' AND sqlc.arg(sort_desc)::boolean THEN x.total_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN x.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN x.created_at END DESC,
    x.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ExpenseReport :many
-- Recorded expenses grouped by one dimension (category, branch, vehicle,
-- month) inside a period.
SELECT
    (CASE sqlc.arg(group_by)::text
        WHEN 'category' THEN cat.code
        WHEN 'branch' THEN COALESCE(b.code, '')
        WHEN 'vehicle' THEN COALESCE(v.plate, '')
        ELSE to_char(x.expense_date, 'YYYY-MM') END)::text AS group_key,
    (CASE sqlc.arg(group_by)::text
        WHEN 'category' THEN cat.name
        WHEN 'branch' THEN COALESCE(b.name, '')
        WHEN 'vehicle' THEN COALESCE(v.plate, '')
        ELSE to_char(x.expense_date, 'YYYY-MM') END)::text AS group_label,
    x.currency,
    COUNT(*)::bigint AS expense_count,
    SUM(x.net_amount)::numeric AS net_amount,
    SUM(x.vat_amount)::numeric AS vat_amount,
    SUM(x.total_amount)::numeric AS total_amount
FROM expenses x
JOIN expense_categories cat ON cat.organization_id = x.organization_id AND cat.id = x.category_id
LEFT JOIN branches b ON b.organization_id = x.organization_id AND b.id = x.branch_id
LEFT JOIN vehicles v ON v.organization_id = x.organization_id AND v.id = x.vehicle_id
WHERE x.organization_id = sqlc.arg(organization_id)
  AND x.status = 'recorded'
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND x.expense_date >= sqlc.arg(date_from)::date
  AND x.expense_date <= sqlc.arg(date_to)::date
GROUP BY 1, 2, 3
ORDER BY 7 DESC, 1;

-- name: CashFlowReport :many
-- Movements grouped by account, branch or month and kind inside a period.
SELECT
    (CASE sqlc.arg(group_by)::text
        WHEN 'account' THEN c.code
        WHEN 'branch' THEN COALESCE(b.code, '')
        ELSE to_char(m.movement_date, 'YYYY-MM') END)::text AS group_key,
    (CASE sqlc.arg(group_by)::text
        WHEN 'account' THEN c.name
        WHEN 'branch' THEN COALESCE(b.name, '')
        ELSE to_char(m.movement_date, 'YYYY-MM') END)::text AS group_label,
    m.currency,
    COALESCE(SUM(m.amount) FILTER (WHERE m.direction = 'in'), 0)::numeric AS total_in,
    COALESCE(SUM(m.amount) FILTER (WHERE m.direction = 'out'), 0)::numeric AS total_out,
    COALESCE(SUM(m.amount) FILTER (WHERE m.kind = 'collection'), 0)::numeric AS collections,
    COALESCE(SUM(m.amount) FILTER (WHERE m.kind = 'expense'), 0)::numeric AS expenses,
    COALESCE(SUM(m.amount) FILTER (WHERE m.kind = 'refund'), 0)::numeric AS refunds,
    COALESCE(SUM(m.amount) FILTER (WHERE m.kind = 'supplier_payment'), 0)::numeric AS supplier_payments,
    COUNT(*)::bigint AS movement_count
FROM cash_movements m
JOIN cash_accounts c ON c.organization_id = m.organization_id AND c.id = m.cash_account_id
LEFT JOIN branches b ON b.organization_id = m.organization_id AND b.id = m.branch_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND m.movement_date >= sqlc.arg(date_from)::date
  AND m.movement_date <= sqlc.arg(date_to)::date
GROUP BY 1, 2, 3
ORDER BY 1;

-- name: GetVehicleRefByUUID :one
SELECT v.id, v.uuid, v.plate, v.home_branch_id, v.current_branch_id
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid)
  AND v.deleted_at IS NULL;

-- name: GetVehicleRefByPlate :one
SELECT v.id, v.uuid, v.plate, v.home_branch_id, v.current_branch_id
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND upper(replace(v.plate, ' ', '')) = upper(replace(sqlc.arg(plate)::text, ' ', ''))
  AND v.deleted_at IS NULL;

-- name: GetBranchRefByCode :one
SELECT b.id, b.uuid, b.code, b.name
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND lower(b.code) = lower(sqlc.arg(code)::text)
  AND b.deleted_at IS NULL;

-- name: GetLedgerEntryForDeposit :one
SELECT e.id, e.uuid, e.kind, e.direction, e.amount, e.currency, e.branch_id
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid);

-- name: GetOrganizationUUIDForFinance :one
SELECT o.uuid FROM organizations o WHERE o.id = sqlc.arg(id);
