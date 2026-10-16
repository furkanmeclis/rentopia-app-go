-- Supplier invoices and payments (TEC-721, docs/design/suppliers.md): the
-- payables side of the finance ledger. Every query filters by
-- organization_id; branch scope: invoices / payments of the caller's
-- branches (head office rows, branch_id NULL, need every branch).
--
-- Invoice due state (computed, Europe/Istanbul calendar day "today"):
--   settled   status paid / void, nothing open
--   overdue   open amount > 0 and due_date < today
--   due_soon  open amount > 0 and due_date < today + window
--   later     open amount > 0, due later

-- name: GetSupplierForFinance :one
SELECT s.id, s.uuid, s.code, s.title, s.payment_term_days, s.status
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid);

-- name: GetSupplierForFinanceByID :one
SELECT s.id, s.uuid, s.code, s.title, s.payment_term_days, s.status
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.id = sqlc.arg(id);

-- name: FindSupplierIDsByTitle :many
-- Free-text supplier names: live suppliers with this title (case and
-- surrounding spaces ignored).
SELECT s.id, s.uuid, s.title
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.status = 'active'
  AND lower(s.title) = lower(btrim(sqlc.arg(title)::text))
ORDER BY s.id
LIMIT 2;

-- name: EnsureSupplierAccount :one
-- The ledger account of a supplier, opened on its first invoice / payment.
-- The caller holds LockOrganizationAccountCodes (per-organization code).
INSERT INTO accounts (organization_id, code, party_kind, supplier_id, name, branch_id, currency, payment_term_days, created_by)
VALUES (
    sqlc.arg(organization_id),
    'TD-' || lpad((
        SELECT count(*) + 1 FROM accounts x
        WHERE x.organization_id = sqlc.arg(organization_id) AND x.party_kind = 'supplier'
    )::text, 6, '0'),
    'supplier', sqlc.arg(supplier_id), sqlc.arg(name), sqlc.narg(branch_id), sqlc.arg(currency),
    sqlc.arg(payment_term_days), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, supplier_id) WHERE supplier_id IS NOT NULL
DO UPDATE SET name = EXCLUDED.name, payment_term_days = EXCLUDED.payment_term_days
RETURNING id, uuid, code, currency, payment_term_days;

-- name: GetAccountBySupplier :one
SELECT a.id, a.uuid, a.code
FROM accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.supplier_id = sqlc.arg(supplier_id);

-- name: SyncSupplierAccount :exec
-- Keeps the account's display name and terms in step with the card.
UPDATE accounts
SET name = sqlc.arg(name), payment_term_days = sqlc.arg(payment_term_days)
WHERE organization_id = sqlc.arg(organization_id)
  AND supplier_id = sqlc.arg(supplier_id);

-- ------------------------------------------------------------- invoices

-- name: InsertSupplierInvoice :one
INSERT INTO supplier_invoices (
    organization_id, supplier_id, account_id, invoice_no, invoice_date, due_date, currency, category_id, branch_id,
    vehicle_id, description, net_amount, vat_amount, total_amount, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(supplier_id), sqlc.arg(account_id), sqlc.arg(invoice_no), sqlc.arg(invoice_date),
    sqlc.arg(due_date), sqlc.arg(currency), sqlc.arg(category_id), sqlc.narg(branch_id), sqlc.narg(vehicle_id),
    sqlc.arg(description), sqlc.arg(net_amount), sqlc.arg(vat_amount), sqlc.arg(total_amount), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: InsertSupplierInvoiceLine :exec
INSERT INTO supplier_invoice_lines (
    organization_id, invoice_id, line_no, description, net_amount, vat_rate, vat_amount, total_amount
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(invoice_id), sqlc.arg(line_no), sqlc.arg(description), sqlc.arg(net_amount),
    sqlc.arg(vat_rate), sqlc.arg(vat_amount), sqlc.arg(total_amount)
);

-- name: ListSupplierInvoiceLines :many
SELECT l.line_no, l.description, l.net_amount, l.vat_rate, l.vat_amount, l.total_amount
FROM supplier_invoice_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.invoice_id = sqlc.arg(invoice_id)
ORDER BY l.line_no;

-- name: GetSupplierInvoiceRow :one
SELECT i.id, i.uuid, i.supplier_id, i.account_id, i.invoice_no, i.invoice_date, i.due_date, i.currency,
    i.category_id, i.branch_id, i.vehicle_id, i.description, i.net_amount, i.vat_amount, i.total_amount,
    i.paid_amount, i.status, i.attachment_object_key, i.attachment_name, i.attachment_content_type,
    i.attachment_size_bytes
FROM supplier_invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.uuid = sqlc.arg(uuid);

-- name: GetSupplierInvoiceRowByID :one
SELECT i.id, i.uuid, i.supplier_id, i.account_id, i.invoice_no, i.invoice_date, i.due_date, i.currency,
    i.category_id, i.branch_id, i.vehicle_id, i.description, i.net_amount, i.vat_amount, i.total_amount,
    i.paid_amount, i.status
FROM supplier_invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.id = sqlc.arg(id);

-- name: LockSupplierInvoices :many
SELECT i.id, i.uuid, i.supplier_id, i.currency, i.total_amount, i.paid_amount, i.status, i.branch_id
FROM supplier_invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY i.id
FOR UPDATE;

-- name: ListOpenSupplierInvoices :many
-- Open invoices of a supplier, oldest due date first (FIFO allocation).
SELECT i.id, i.uuid, i.invoice_no, i.currency, i.total_amount, i.paid_amount, i.due_date, i.branch_id
FROM supplier_invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.supplier_id = sqlc.arg(supplier_id)
  AND i.status IN ('open', 'partially_paid')
  AND i.currency = sqlc.arg(currency)
ORDER BY i.due_date, i.id;

-- name: UpdateSupplierInvoiceDetails :execrows
UPDATE supplier_invoices
SET invoice_no = sqlc.arg(invoice_no), due_date = sqlc.arg(due_date), description = sqlc.arg(description),
    category_id = sqlc.arg(category_id), branch_id = sqlc.narg(branch_id), vehicle_id = sqlc.narg(vehicle_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'void';

-- name: SetSupplierInvoicePaid :execrows
-- paid_amount is the sum of the invoice's live allocations; the status
-- follows it.
UPDATE supplier_invoices i
SET paid_amount = sub.paid,
    status = (CASE
        WHEN sub.paid >= i.total_amount THEN 'paid'
        WHEN sub.paid > 0 THEN 'partially_paid'
        ELSE 'open' END)
FROM (
    SELECT COALESCE(SUM(al.amount), 0)::numeric AS paid
    FROM supplier_payment_allocations al
    JOIN supplier_payments p ON p.organization_id = al.organization_id AND p.id = al.payment_id
    WHERE al.organization_id = sqlc.arg(organization_id)
      AND p.organization_id = sqlc.arg(organization_id)
      AND al.invoice_id = sqlc.arg(id)
      AND p.status = 'posted'
) sub
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.id = sqlc.arg(id)
  AND i.status <> 'void';

-- name: VoidSupplierInvoice :execrows
UPDATE supplier_invoices
SET status = 'void', void_reason = sqlc.arg(void_reason), voided_by = sqlc.narg(voided_by), voided_at = sqlc.arg(voided_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status <> 'void';

-- name: SetSupplierInvoiceAttachment :execrows
UPDATE supplier_invoices
SET attachment_object_key = sqlc.narg(attachment_object_key), attachment_name = sqlc.arg(attachment_name),
    attachment_content_type = sqlc.arg(attachment_content_type), attachment_size_bytes = sqlc.arg(attachment_size_bytes)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: CountLiveInvoiceAllocations :one
SELECT COUNT(*)::bigint
FROM supplier_payment_allocations al
JOIN supplier_payments p ON p.organization_id = al.organization_id AND p.id = al.payment_id
WHERE al.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND al.invoice_id = sqlc.arg(invoice_id)
  AND p.status = 'posted';

-- name: ListLiveSupplierInvoiceAccruals :many
-- The invoice's accrual that is not reversed (at most one: RT-A23).
SELECT e.id, e.uuid, e.account_id, e.amount, e.currency, e.purpose, e.due_date, e.branch_id, e.description
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.supplier_invoice_id = sqlc.arg(supplier_invoice_id)
  AND e.kind = 'accrual'
  AND NOT EXISTS (
    SELECT 1 FROM ledger_entries r
    WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id
  )
ORDER BY e.id;

-- name: GetInvoiceExpense :one
SELECT x.id, x.uuid, x.status
FROM expenses x
WHERE x.organization_id = sqlc.arg(organization_id)
  AND x.supplier_invoice_id = sqlc.arg(supplier_invoice_id);

-- name: InsertInvoiceExpense :one
-- The expense row an invoice is the source of (unpaid: no cash account).
INSERT INTO expenses (
    organization_id, category_id, expense_date, description, supplier_name, document_no, currency, net_amount,
    vat_rate, vat_amount, total_amount, branch_id, vehicle_id, supplier_id, supplier_invoice_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(category_id), sqlc.arg(expense_date), sqlc.arg(description),
    sqlc.arg(supplier_name), sqlc.arg(document_no), sqlc.arg(currency), sqlc.arg(net_amount), sqlc.arg(vat_rate),
    sqlc.arg(vat_amount), sqlc.arg(total_amount), sqlc.narg(branch_id), sqlc.narg(vehicle_id), sqlc.arg(supplier_id),
    sqlc.arg(supplier_invoice_id), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, supplier_invoice_id) WHERE supplier_invoice_id IS NOT NULL DO NOTHING
RETURNING id, uuid;

-- name: SyncInvoiceExpense :execrows
-- The invoice's classification follows onto its expense row.
UPDATE expenses
SET category_id = sqlc.arg(category_id), description = sqlc.arg(description), document_no = sqlc.arg(document_no),
    branch_id = sqlc.narg(branch_id), vehicle_id = sqlc.narg(vehicle_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND supplier_invoice_id = sqlc.arg(supplier_invoice_id)
  AND status = 'recorded';

-- name: ListSupplierInvoices :many
SELECT i.id, i.uuid, i.invoice_no, i.invoice_date, i.due_date, i.currency, i.description, i.net_amount,
    i.vat_amount, i.total_amount, i.paid_amount, i.status, i.void_reason, i.voided_at, i.created_at, i.updated_at,
    i.attachment_name, i.attachment_content_type, i.attachment_size_bytes,
    (i.attachment_object_key IS NOT NULL)::boolean AS has_attachment,
    (CASE WHEN i.status = 'void' THEN 0 ELSE i.total_amount - i.paid_amount END)::numeric AS open_amount,
    (CASE
        WHEN i.status IN ('paid', 'void') OR i.total_amount <= i.paid_amount THEN 'settled'
        WHEN i.due_date < sqlc.arg(today)::date THEN 'overdue'
        WHEN i.due_date < sqlc.arg(soon)::date THEN 'due_soon'
        ELSE 'later' END)::text AS due_state,
    s.uuid AS supplier_uuid, s.code AS supplier_code, s.title AS supplier_title,
    a.uuid AS account_uuid, a.code AS account_code,
    cat.uuid AS category_uuid, cat.code AS category_code, cat.name AS category_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    x.uuid AS expense_uuid,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM supplier_invoices i
JOIN suppliers s ON s.organization_id = i.organization_id AND s.id = i.supplier_id
JOIN accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id
JOIN expense_categories cat ON cat.organization_id = i.organization_id AND cat.id = i.category_id
LEFT JOIN branches b ON b.organization_id = i.organization_id AND b.id = i.branch_id
LEFT JOIN vehicles v ON v.organization_id = i.organization_id AND v.id = i.vehicle_id
LEFT JOIN expenses x ON x.organization_id = i.organization_id AND x.supplier_invoice_id = i.id
LEFT JOIN users u ON u.id = i.created_by
WHERE i.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(ids)::bigint[] IS NULL OR i.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(supplier_ids)::bigint[] IS NULL OR i.supplier_id = ANY(sqlc.narg(supplier_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR i.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(category_ids)::bigint[] IS NULL OR i.category_id = ANY(sqlc.narg(category_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR i.invoice_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR i.invoice_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(due_to)::date IS NULL OR i.due_date <= sqlc.narg(due_to)::date)
  AND (sqlc.narg(due_states)::text[] IS NULL OR (CASE
        WHEN i.status IN ('paid', 'void') OR i.total_amount <= i.paid_amount THEN 'settled'
        WHEN i.due_date < sqlc.arg(today)::date THEN 'overdue'
        WHEN i.due_date < sqlc.arg(soon)::date THEN 'due_soon'
        ELSE 'later' END) = ANY(sqlc.narg(due_states)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR i.invoice_no ILIKE '%' || sqlc.narg(q) || '%'
    OR i.description ILIKE '%' || sqlc.narg(q) || '%'
    OR s.title ILIKE '%' || sqlc.narg(q) || '%'
    OR s.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'invoice_date' AND NOT sqlc.arg(sort_desc)::boolean THEN i.invoice_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'invoice_date' AND sqlc.arg(sort_desc)::boolean THEN i.invoice_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'due_date' AND NOT sqlc.arg(sort_desc)::boolean THEN i.due_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'due_date' AND sqlc.arg(sort_desc)::boolean THEN i.due_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN i.total_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'total_amount' AND sqlc.arg(sort_desc)::boolean THEN i.total_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'open_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN i.total_amount - i.paid_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'open_amount' AND sqlc.arg(sort_desc)::boolean THEN i.total_amount - i.paid_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN i.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN i.created_at END DESC,
    i.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: PayablesSummary :one
-- Open supplier invoices by due state (caller's branch scope).
SELECT
    COALESCE(SUM(i.total_amount - i.paid_amount) FILTER (WHERE i.due_date < sqlc.arg(today)::date), 0)::numeric AS overdue_amount,
    COUNT(*) FILTER (WHERE i.due_date < sqlc.arg(today)::date)::bigint AS overdue_count,
    COALESCE(SUM(i.total_amount - i.paid_amount)
        FILTER (WHERE i.due_date >= sqlc.arg(today)::date AND i.due_date < sqlc.arg(soon)::date), 0)::numeric AS due_soon_amount,
    COUNT(*) FILTER (WHERE i.due_date >= sqlc.arg(today)::date AND i.due_date < sqlc.arg(soon)::date)::bigint AS due_soon_count,
    COALESCE(SUM(i.total_amount - i.paid_amount), 0)::numeric AS open_amount,
    COUNT(*)::bigint AS open_count
FROM supplier_invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.status IN ('open', 'partially_paid')
  AND i.total_amount > i.paid_amount
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]));

-- ------------------------------------------------------------- payments

-- name: InsertSupplierPayment :one
INSERT INTO supplier_payments (
    organization_id, supplier_id, account_id, cash_account_id, amount, currency, payment_date, method, reference,
    description, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(supplier_id), sqlc.arg(account_id), sqlc.arg(cash_account_id), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(payment_date), sqlc.arg(method), sqlc.arg(reference), sqlc.arg(description),
    sqlc.narg(branch_id), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: SetSupplierPaymentPosting :exec
UPDATE supplier_payments
SET ledger_entry_id = sqlc.arg(ledger_entry_id), cash_movement_id = sqlc.arg(cash_movement_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: InsertSupplierPaymentAllocation :exec
INSERT INTO supplier_payment_allocations (organization_id, payment_id, invoice_id, amount)
VALUES (sqlc.arg(organization_id), sqlc.arg(payment_id), sqlc.arg(invoice_id), sqlc.arg(amount));

-- name: InsertSupplierPaymentEntry :one
-- Idempotent: one payment entry per supplier payment.
INSERT INTO ledger_entries (
    organization_id, account_id, kind, direction, purpose, amount, currency, entry_date, due_date, description,
    source_type, source_uuid, supplier_payment_id, branch_id, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(account_id), 'payment', 'debit', 'general', sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(entry_date), sqlc.arg(description), 'supplier_payment',
    sqlc.arg(source_uuid), sqlc.arg(supplier_payment_id), sqlc.narg(branch_id), sqlc.narg(created_by)
)
ON CONFLICT (organization_id, supplier_payment_id) WHERE kind = 'payment' AND supplier_payment_id IS NOT NULL DO NOTHING
RETURNING id, uuid;

-- name: GetSupplierPaymentRow :one
SELECT p.id, p.uuid, p.supplier_id, p.account_id, p.cash_account_id, p.amount, p.currency, p.payment_date,
    p.method, p.reference, p.description, p.branch_id, p.ledger_entry_id, p.cash_movement_id, p.status
FROM supplier_payments p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid);

-- name: VoidSupplierPayment :execrows
UPDATE supplier_payments
SET status = 'void', void_reason = sqlc.arg(void_reason), voided_by = sqlc.narg(voided_by), voided_at = sqlc.arg(voided_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'posted';

-- name: ListPaymentAllocationInvoiceIDs :many
SELECT al.invoice_id
FROM supplier_payment_allocations al
WHERE al.organization_id = sqlc.arg(organization_id)
  AND al.payment_id = sqlc.arg(payment_id)
ORDER BY al.invoice_id;

-- name: ListSupplierPaymentAllocations :many
SELECT al.payment_id, al.amount, i.uuid AS invoice_uuid, i.invoice_no, i.due_date, i.total_amount
FROM supplier_payment_allocations al
JOIN supplier_invoices i ON i.organization_id = al.organization_id AND i.id = al.invoice_id
WHERE al.organization_id = sqlc.arg(organization_id)
  AND al.payment_id = ANY(sqlc.arg(payment_ids)::bigint[])
ORDER BY al.payment_id, i.due_date, i.id;

-- name: ListInvoicePayments :many
-- Payments allocated to one invoice (invoice detail).
SELECT p.uuid, p.payment_date, p.status, p.method, al.amount, ca.code AS cash_account_code, ca.name AS cash_account_name
FROM supplier_payment_allocations al
JOIN supplier_payments p ON p.organization_id = al.organization_id AND p.id = al.payment_id
JOIN cash_accounts ca ON ca.organization_id = p.organization_id AND ca.id = p.cash_account_id
WHERE al.organization_id = sqlc.arg(organization_id)
  AND al.invoice_id = sqlc.arg(invoice_id)
ORDER BY p.payment_date, p.id;

-- name: ListSupplierPayments :many
SELECT p.id, p.uuid, p.amount, p.currency, p.payment_date, p.method, p.reference, p.description, p.status,
    p.void_reason, p.voided_at, p.created_at,
    COALESCE((
        SELECT SUM(al.amount) FROM supplier_payment_allocations al
        WHERE al.organization_id = p.organization_id AND al.payment_id = p.id
    ), 0)::numeric AS allocated_amount,
    s.uuid AS supplier_uuid, s.code AS supplier_code, s.title AS supplier_title,
    a.uuid AS account_uuid, a.code AS account_code,
    ca.uuid AS cash_account_uuid, ca.code AS cash_account_code, ca.name AS cash_account_name, ca.kind AS cash_account_kind,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    e.uuid AS ledger_entry_uuid, m.uuid AS movement_uuid,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    COUNT(*) OVER ()::bigint AS total
FROM supplier_payments p
JOIN suppliers s ON s.organization_id = p.organization_id AND s.id = p.supplier_id
JOIN accounts a ON a.organization_id = p.organization_id AND a.id = p.account_id
JOIN cash_accounts ca ON ca.organization_id = p.organization_id AND ca.id = p.cash_account_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
LEFT JOIN ledger_entries e ON e.organization_id = p.organization_id AND e.id = p.ledger_entry_id
LEFT JOIN cash_movements m ON m.organization_id = p.organization_id AND m.id = p.cash_movement_id
LEFT JOIN users u ON u.id = p.created_by
WHERE p.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(ids)::bigint[] IS NULL OR p.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(supplier_ids)::bigint[] IS NULL OR p.supplier_id = ANY(sqlc.narg(supplier_ids)::bigint[]))
  AND (sqlc.narg(cash_account_ids)::bigint[] IS NULL OR p.cash_account_id = ANY(sqlc.narg(cash_account_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(methods)::text[] IS NULL OR p.method = ANY(sqlc.narg(methods)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR p.payment_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR p.payment_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(invoice_id)::bigint IS NULL OR EXISTS (
    SELECT 1 FROM supplier_payment_allocations al
    WHERE al.organization_id = p.organization_id AND al.payment_id = p.id AND al.invoice_id = sqlc.narg(invoice_id)::bigint
  ))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.reference ILIKE '%' || sqlc.narg(q) || '%'
    OR p.description ILIKE '%' || sqlc.narg(q) || '%'
    OR s.title ILIKE '%' || sqlc.narg(q) || '%'
    OR s.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'payment_date' AND NOT sqlc.arg(sort_desc)::boolean THEN p.payment_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'payment_date' AND sqlc.arg(sort_desc)::boolean THEN p.payment_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN p.amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN p.amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
    p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: IsSupplierPaymentMovement :one
SELECT EXISTS (
    SELECT 1 FROM supplier_payments p
    WHERE p.organization_id = sqlc.arg(organization_id)
      AND p.cash_movement_id = sqlc.arg(cash_movement_id)
)::boolean;
