-- Tenant resource: suppliers (TEC-721, docs/design/suppliers.md). Every
-- query filters by organization_id. The balance of a supplier is its
-- ledger account's (debits minus credits; negative = we owe), over the
-- entries in the caller's branch scope; the due figures come from its open
-- invoices (also branch scoped).

-- name: LockOrganizationSupplierCodes :exec
-- Serialises generated supplier codes of one organization for the rest of
-- the transaction.
SELECT pg_advisory_xact_lock(hashtextextended('suppliers.code:' || sqlc.arg(organization_id)::bigint::text, 0));

-- name: NextSupplierCodeNumber :one
SELECT (COALESCE(MAX(substring(s.code FROM '^TED-([0-9]{1,9})$')::bigint), 0) + 1)::bigint
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id);

-- name: InsertSupplier :one
INSERT INTO suppliers (
    organization_id, code, title, supplier_type, tax_number, tax_office, phone, email, website, address, city,
    billing_address, billing_email, e_invoice, bank_name, iban, payment_term_days, contacts, status, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(title), sqlc.arg(supplier_type), sqlc.arg(tax_number),
    sqlc.arg(tax_office), sqlc.arg(phone), sqlc.arg(email), sqlc.arg(website), sqlc.arg(address), sqlc.arg(city),
    sqlc.arg(billing_address), sqlc.arg(billing_email), sqlc.arg(e_invoice), sqlc.arg(bank_name), sqlc.arg(iban),
    sqlc.arg(payment_term_days), sqlc.arg(contacts), sqlc.arg(status), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateSupplier :one
UPDATE suppliers
SET code = sqlc.arg(code), title = sqlc.arg(title), supplier_type = sqlc.arg(supplier_type),
    tax_number = sqlc.arg(tax_number), tax_office = sqlc.arg(tax_office), phone = sqlc.arg(phone),
    email = sqlc.arg(email), website = sqlc.arg(website), address = sqlc.arg(address), city = sqlc.arg(city),
    billing_address = sqlc.arg(billing_address), billing_email = sqlc.arg(billing_email),
    e_invoice = sqlc.arg(e_invoice), bank_name = sqlc.arg(bank_name), iban = sqlc.arg(iban),
    payment_term_days = sqlc.arg(payment_term_days), contacts = sqlc.arg(contacts), status = sqlc.arg(status),
    note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: SetSupplierStatus :one
UPDATE suppliers AS s
SET status = sqlc.arg(status)
FROM suppliers AS prev
WHERE s.organization_id = sqlc.arg(organization_id)
  AND prev.organization_id = sqlc.arg(organization_id)
  AND s.id = sqlc.arg(id)
  AND prev.id = s.id
RETURNING prev.status AS previous;

-- name: DeleteUnreferencedSupplier :execrows
-- Import rollback of a created card: removed only while nothing refers to
-- it (no ledger account, vehicle or expense); otherwise it is archived.
DELETE FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.id = sqlc.arg(id)
  AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.organization_id = s.organization_id AND a.supplier_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM vehicles v WHERE v.organization_id = s.organization_id AND v.supplier_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM expenses x WHERE x.organization_id = s.organization_id AND x.supplier_id = s.id);

-- name: GetSupplierByUUID :one
SELECT * FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid);

-- name: GetSupplierByCode :one
SELECT * FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND lower(code) = lower(sqlc.arg(code)::text);

-- name: GetSupplierByTaxNumber :one
SELECT * FROM suppliers
WHERE organization_id = sqlc.arg(organization_id)
  AND tax_number = sqlc.arg(tax_number)
  AND tax_number <> ''
ORDER BY id
LIMIT 1;

-- name: SupplierTaxNumberTaken :one
SELECT EXISTS (
    SELECT 1 FROM suppliers
    WHERE organization_id = sqlc.arg(organization_id)
      AND tax_number = sqlc.arg(tax_number)
      AND tax_number <> ''
      AND status = 'active'
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id)::bigint)
)::boolean;

-- name: LinkFreeTextToSupplier :one
-- A new supplier card adopts the vehicles and expenses that name it in
-- free text and have no supplier yet. Returns how many rows it linked.
WITH lv AS (
    UPDATE vehicles AS vv
    SET supplier_id = sqlc.arg(supplier_id)::bigint
    WHERE vv.organization_id = sqlc.arg(organization_id)::bigint
      AND vv.supplier_id IS NULL
      AND btrim(vv.supplier_name) <> ''
      AND lower(btrim(vv.supplier_name)) = lower(btrim(sqlc.arg(title)::text))
    RETURNING vv.id
), lx AS (
    UPDATE expenses AS xx
    SET supplier_id = sqlc.arg(supplier_id)::bigint
    WHERE xx.organization_id = sqlc.arg(organization_id)::bigint
      AND xx.supplier_id IS NULL
      AND btrim(xx.supplier_name) <> ''
      AND lower(btrim(xx.supplier_name)) = lower(btrim(sqlc.arg(title)::text))
    RETURNING xx.id
)
SELECT ((SELECT count(*) FROM lv) + (SELECT count(*) FROM lx))::bigint AS linked;

-- name: RenameSupplierFreeText :exec
-- The kept free-text names of linked rows follow a renamed card.
WITH rv AS (
    UPDATE vehicles AS vv
    SET supplier_name = sqlc.arg(title)::text
    WHERE vv.organization_id = sqlc.arg(organization_id)::bigint
      AND vv.supplier_id = sqlc.arg(supplier_id)::bigint
    RETURNING vv.id
)
UPDATE expenses AS xx
SET supplier_name = sqlc.arg(title)::text
WHERE xx.organization_id = sqlc.arg(organization_id)::bigint
  AND xx.supplier_id = sqlc.arg(supplier_id)::bigint
  AND xx.status = 'recorded';

-- name: ListSuppliers :many
WITH bal AS (
    SELECT a.supplier_id,
        (COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'debit'), 0)
            - COALESCE(SUM(e.amount) FILTER (WHERE e.direction = 'credit'), 0))::numeric AS balance
    FROM ledger_entries e
    JOIN accounts a ON a.organization_id = e.organization_id AND a.id = e.account_id
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND a.organization_id = sqlc.arg(organization_id)
      AND a.supplier_id IS NOT NULL
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
    GROUP BY a.supplier_id
), inv AS (
    SELECT i.supplier_id,
        COALESCE(SUM(i.total_amount - i.paid_amount), 0)::numeric AS open_amount,
        COALESCE(SUM(i.total_amount - i.paid_amount) FILTER (WHERE i.due_date < sqlc.arg(today)::date), 0)::numeric AS overdue_amount,
        COALESCE(SUM(i.total_amount - i.paid_amount)
            FILTER (WHERE i.due_date >= sqlc.arg(today)::date AND i.due_date < sqlc.arg(soon)::date), 0)::numeric AS due_soon_amount,
        COUNT(*)::bigint AS open_invoices,
        MIN(i.due_date)::date AS next_due_date
    FROM supplier_invoices i
    WHERE i.organization_id = sqlc.arg(organization_id)
      AND i.status IN ('open', 'partially_paid')
      AND i.total_amount > i.paid_amount
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
    GROUP BY i.supplier_id
), veh AS (
    SELECT v.supplier_id, COUNT(*)::bigint AS vehicle_count
    FROM vehicles v
    WHERE v.organization_id = sqlc.arg(organization_id)
      AND v.supplier_id IS NOT NULL
      AND v.deleted_at IS NULL
    GROUP BY v.supplier_id
), shaped AS (
    SELECT s.id, s.uuid, s.code, s.title, s.supplier_type, s.tax_number, s.tax_office, s.phone, s.email,
        s.website, s.address, s.city, s.billing_address, s.billing_email, s.e_invoice, s.bank_name, s.iban,
        s.payment_term_days, s.contacts, s.status, s.note, s.created_at, s.updated_at,
        a.uuid AS account_uuid, a.code AS account_code,
        COALESCE(bal.balance, 0)::numeric AS balance,
        COALESCE(inv.open_amount, 0)::numeric AS open_amount,
        COALESCE(inv.overdue_amount, 0)::numeric AS overdue_amount,
        COALESCE(inv.due_soon_amount, 0)::numeric AS due_soon_amount,
        COALESCE(inv.open_invoices, 0)::bigint AS open_invoices,
        inv.next_due_date,
        COALESCE(veh.vehicle_count, 0)::bigint AS vehicle_count,
        (CASE
            WHEN COALESCE(inv.overdue_amount, 0) > 0 THEN 'overdue'
            WHEN COALESCE(inv.due_soon_amount, 0) > 0 THEN 'due_soon'
            WHEN COALESCE(inv.open_amount, 0) > 0 THEN 'open'
            ELSE 'settled' END)::text AS due_state
    FROM suppliers s
    LEFT JOIN accounts a ON a.organization_id = s.organization_id AND a.supplier_id = s.id
    LEFT JOIN bal ON bal.supplier_id = s.id
    LEFT JOIN inv ON inv.supplier_id = s.id
    LEFT JOIN veh ON veh.supplier_id = s.id
    WHERE s.organization_id = sqlc.arg(organization_id)
)
SELECT sh.id, sh.uuid, sh.code, sh.title, sh.supplier_type, sh.tax_number, sh.tax_office, sh.phone, sh.email,
    sh.website, sh.address, sh.city, sh.billing_address, sh.billing_email, sh.e_invoice, sh.bank_name, sh.iban,
    sh.payment_term_days, sh.contacts, sh.status, sh.note, sh.created_at, sh.updated_at,
    sh.account_uuid, sh.account_code, sh.balance, sh.open_amount, sh.overdue_amount, sh.due_soon_amount,
    sh.open_invoices, sh.next_due_date, sh.vehicle_count, sh.due_state,
    COUNT(*) OVER ()::bigint AS total
FROM shaped sh
WHERE (sqlc.narg(ids)::bigint[] IS NULL OR sh.id = ANY(sqlc.narg(ids)::bigint[]))
  AND (sqlc.narg(types)::text[] IS NULL OR sh.supplier_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR sh.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(cities)::text[] IS NULL OR sh.city = ANY(sqlc.narg(cities)::text[]))
  AND (sqlc.narg(due_states)::text[] IS NULL OR sh.due_state = ANY(sqlc.narg(due_states)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR sh.title ILIKE '%' || sqlc.narg(q) || '%'
    OR sh.code ILIKE '%' || sqlc.narg(q) || '%'
    OR sh.tax_number LIKE '%' || sqlc.narg(q) || '%'
    OR sh.email ILIKE '%' || sqlc.narg(q) || '%'
    OR sh.phone LIKE '%' || sqlc.narg(q) || '%'
    OR sh.city ILIKE '%' || sqlc.narg(q) || '%'
    OR sh.contacts::text ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN sh.code END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN sh.code END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'title' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(sh.title) END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'title' AND sqlc.arg(sort_desc)::boolean THEN lower(sh.title) END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND NOT sqlc.arg(sort_desc)::boolean THEN sh.balance END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND sqlc.arg(sort_desc)::boolean THEN sh.balance END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'open_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN sh.open_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'open_amount' AND sqlc.arg(sort_desc)::boolean THEN sh.open_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'overdue_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN sh.overdue_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'overdue_amount' AND sqlc.arg(sort_desc)::boolean THEN sh.overdue_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'next_due_date' AND NOT sqlc.arg(sort_desc)::boolean THEN sh.next_due_date END ASC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'next_due_date' AND sqlc.arg(sort_desc)::boolean THEN sh.next_due_date END DESC NULLS LAST,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN sh.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN sh.created_at END DESC,
    sh.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListSupplierFacets :many
SELECT 'supplier_type'::text AS facet, s.supplier_type AS value, COUNT(*)::bigint AS count
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
GROUP BY s.supplier_type
UNION ALL
SELECT 'status'::text, s.status, COUNT(*)::bigint
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
GROUP BY s.status
UNION ALL
SELECT 'city'::text, s.city, COUNT(*)::bigint
FROM suppliers s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.city <> ''
GROUP BY s.city
ORDER BY 1, 2;

-- name: GetSupplierSearchDocument :one
SELECT s.id, s.uuid, s.code, s.title, s.tax_number, s.city, s.supplier_type, s.status, s.organization_id,
    o.slug AS organization_slug
FROM suppliers s
JOIN organizations o ON o.id = s.organization_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid);

-- name: ListSupplierSearchDocuments :many
SELECT s.id, s.uuid, s.code, s.title, s.tax_number, s.city, s.supplier_type, s.status, s.organization_id,
    o.slug AS organization_slug
FROM suppliers s
JOIN organizations o ON o.id = s.organization_id
WHERE s.organization_id = sqlc.arg(organization_id)
ORDER BY s.id ASC;
