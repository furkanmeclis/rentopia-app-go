-- E-invoices / e-archive invoices (TEC-722, docs/design/einvoice.md). Every
-- query names the organization; lists / facets / pickers take the branch
-- scope (nil = every branch).

-- name: GetInvoiceSettings :one
SELECT s.organization_id, s.efatura_series, s.earsiv_series, s.supplier_district, s.supplier_city,
    s.supplier_postal_code, s.default_note, s.taxpayer_cache_hours, s.updated_at
FROM invoice_settings s
WHERE s.organization_id = sqlc.arg(organization_id);

-- name: UpsertInvoiceSettings :exec
INSERT INTO invoice_settings (
    organization_id, efatura_series, earsiv_series, supplier_district, supplier_city, supplier_postal_code,
    default_note, taxpayer_cache_hours, updated_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(efatura_series), sqlc.arg(earsiv_series), sqlc.arg(supplier_district),
    sqlc.arg(supplier_city), sqlc.arg(supplier_postal_code), sqlc.arg(default_note), sqlc.arg(taxpayer_cache_hours),
    sqlc.narg(updated_by)
)
ON CONFLICT (organization_id) DO UPDATE
SET efatura_series = EXCLUDED.efatura_series, earsiv_series = EXCLUDED.earsiv_series,
    supplier_district = EXCLUDED.supplier_district, supplier_city = EXCLUDED.supplier_city,
    supplier_postal_code = EXCLUDED.supplier_postal_code, default_note = EXCLUDED.default_note,
    taxpayer_cache_hours = EXCLUDED.taxpayer_cache_hours, updated_by = EXCLUDED.updated_by;

-- name: ListInvoiceTaxMappings :many
SELECT m.line_kind, m.vat_rate, m.exemption_code, m.exemption_reason, m.updated_at
FROM invoice_tax_mappings m
WHERE m.organization_id = sqlc.arg(organization_id)
ORDER BY m.line_kind;

-- name: UpsertInvoiceTaxMapping :exec
INSERT INTO invoice_tax_mappings (organization_id, line_kind, vat_rate, exemption_code, exemption_reason, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(line_kind), sqlc.arg(vat_rate), sqlc.arg(exemption_code),
    sqlc.arg(exemption_reason), sqlc.narg(updated_by))
ON CONFLICT (organization_id, line_kind) DO UPDATE
SET vat_rate = EXCLUDED.vat_rate, exemption_code = EXCLUDED.exemption_code,
    exemption_reason = EXCLUDED.exemption_reason, updated_by = EXCLUDED.updated_by;

-- name: NextInvoiceSequence :one
-- The next number of a series in a year (row lock: serialised per series).
INSERT INTO invoice_sequences (organization_id, series, year, last_value)
VALUES (sqlc.arg(organization_id), sqlc.arg(series), sqlc.arg(year), 1)
ON CONFLICT (organization_id, series, year) DO UPDATE SET last_value = invoice_sequences.last_value + 1
RETURNING last_value;

-- name: GetEInvoiceTaxpayer :one
SELECT t.is_einvoice, t.alias, t.title, t.provider, t.checked_at, t.expires_at
FROM einvoice_taxpayers t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.identity_bidx = sqlc.arg(identity_bidx)
  AND t.expires_at > sqlc.arg(now)::timestamptz;

-- name: UpsertEInvoiceTaxpayer :exec
INSERT INTO einvoice_taxpayers (organization_id, identity_bidx, is_einvoice, alias, title, provider, checked_at, expires_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(identity_bidx), sqlc.arg(is_einvoice), sqlc.arg(alias), sqlc.arg(title),
    sqlc.arg(provider), sqlc.arg(checked_at), sqlc.arg(expires_at))
ON CONFLICT (organization_id, identity_bidx) DO UPDATE
SET is_einvoice = EXCLUDED.is_einvoice, alias = EXCLUDED.alias, title = EXCLUDED.title, provider = EXCLUDED.provider,
    checked_at = EXCLUDED.checked_at, expires_at = EXCLUDED.expires_at;

-- name: ListInvoiceableEntries :many
-- Live accruals (debit, not reversed) that no standing invoice covers: the
-- rent of a rental and approved charges. Filters: an account, a rental,
-- explicit entries; the branch scope applies to the entry's branch.
SELECT e.id, e.uuid, e.account_id, e.purpose, e.amount, e.currency, e.entry_date, e.description, e.branch_id,
    e.rental_id, e.reservation_id, e.rental_charge_id,
    a.uuid AS account_uuid, a.code AS account_code, a.name AS account_name, a.customer_id AS account_customer_id,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    ch.uuid AS charge_uuid, ch.kind AS charge_kind, ch.reason AS charge_reason,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name
FROM ledger_entries e
JOIN accounts a ON a.organization_id = e.organization_id AND a.id = e.account_id
LEFT JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
LEFT JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = e.rental_charge_id
LEFT JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND a.organization_id = sqlc.arg(organization_id)
  AND e.kind = 'accrual'
  AND e.direction = 'debit'
  AND a.party_kind = 'customer'
  AND NOT EXISTS (
      SELECT 1 FROM ledger_entries rv
      WHERE rv.organization_id = e.organization_id AND rv.reverses_entry_id = e.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM ledger_entry_links l
      WHERE l.organization_id = e.organization_id AND l.entry_id = e.id
        AND l.invoice_id IS NOT NULL AND l.released_at IS NULL
  )
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(account_id)::bigint IS NULL OR e.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(rental_id)::bigint IS NULL OR e.rental_id = sqlc.narg(rental_id)::bigint)
  AND (sqlc.narg(entry_uuids)::uuid[] IS NULL OR e.uuid = ANY(sqlc.narg(entry_uuids)::uuid[]))
ORDER BY e.entry_date, e.id
LIMIT 500;

-- name: ListUninvoicedRentals :many
-- Rentals with at least one invoiceable item (bulk "invoice the selected
-- rentals" picker).
WITH open_items AS (
    SELECT e.rental_id, e.amount, e.entry_date, e.branch_id
    FROM ledger_entries e
    WHERE e.organization_id = sqlc.arg(organization_id)
      AND e.kind = 'accrual'
      AND e.direction = 'debit'
      AND e.rental_id IS NOT NULL
      AND NOT EXISTS (
          SELECT 1 FROM ledger_entries rv
          WHERE rv.organization_id = e.organization_id AND rv.reverses_entry_id = e.id
      )
      AND NOT EXISTS (
          SELECT 1 FROM ledger_entry_links l
          WHERE l.organization_id = e.organization_id AND l.entry_id = e.id
            AND l.invoice_id IS NOT NULL AND l.released_at IS NULL
      )
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
)
SELECT r.id, r.uuid, r.reference, r.status, r.actual_pickup_at, r.actual_return_at,
    res.uuid AS reservation_uuid,
    COALESCE(NULLIF(c.company_title, ''), TRIM(c.first_name || ' ' || c.last_name))::text AS customer_name,
    pb.uuid AS branch_uuid, pb.code AS branch_code, pb.name AS branch_name,
    SUM(o.amount)::numeric AS open_amount, COUNT(*)::bigint AS item_count, MIN(o.entry_date)::date AS first_date,
    COUNT(*) OVER ()::bigint AS total
FROM open_items o
JOIN rentals r ON r.organization_id = sqlc.arg(organization_id) AND r.id = o.rental_id
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = COALESCE(res.invoice_customer_id, res.customer_id)
JOIN branches pb ON pb.organization_id = res.organization_id AND pb.id = res.pickup_branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND pb.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL
       OR r.reference ILIKE '%' || sqlc.narg(q)::text || '%'
       OR c.company_title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(rental_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(rental_uuids)::uuid[]))
GROUP BY r.id, r.uuid, r.reference, r.status, r.actual_pickup_at, r.actual_return_at, res.uuid, c.company_title,
    c.first_name, c.last_name, pb.uuid, pb.code, pb.name
ORDER BY MIN(o.entry_date) DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetInvoiceRentalFacts :one
-- The reservation behind a rental: price snapshot (VAT split lines), the
-- billing days, the invoice customer, the pick-up branch.
SELECT r.id, r.uuid, r.reference, r.reservation_id, res.quote_snapshot, res.starts_at, res.ends_at,
    res.customer_id, res.invoice_customer_id, res.pickup_branch_id, res.total_amount, res.currency
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id);

-- name: GetInvoiceRentalByUUID :one
SELECT r.id, r.uuid, r.reference, res.pickup_branch_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: GetInvoiceAccountByUUID :one
SELECT a.id, a.uuid, a.code, a.name, a.party_kind, a.customer_id, a.branch_id
FROM accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: GetInvoiceCustomer :one
-- The receiver's customer record (identity sealed: opened by the service).
SELECT c.id, c.uuid, c.customer_type, c.first_name, c.last_name, c.company_title, c.identity_type, c.identity_enc,
    c.identity_hint, c.nationality, c.tax_office, c.email, c.billing_address, c.billing_city, c.billing_district,
    c.billing_postal_code, c.billing_country, c.e_invoice
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id);

-- name: GetInvoiceBranch :one
SELECT b.id, b.uuid, b.code, b.name, b.city, b.address
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.id = sqlc.arg(id);

-- name: GetInvoiceBranchByUUID :one
SELECT b.id, b.uuid, b.code, b.name, b.city, b.address
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid);

-- name: InsertInvoice :one
INSERT INTO invoices (
    uuid, organization_id, branch_id, account_id, customer_id, rental_id, returns_invoice_id, profile, invoice_type,
    issue_date, currency, receiver_type, receiver_identity_type, receiver_identity_enc, receiver_identity_hint,
    receiver_name, receiver_first_name, receiver_last_name, receiver_tax_office, receiver_address, receiver_district,
    receiver_city, receiver_postal_code, receiver_country, receiver_email, receiver_einvoice, receiver_alias,
    taxpayer_source, taxpayer_checked_at, line_extension_amount, tax_amount, payable_amount, linked_amount, note,
    idempotency_key, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.narg(account_id), sqlc.narg(customer_id),
    sqlc.narg(rental_id), sqlc.narg(returns_invoice_id), sqlc.arg(profile), sqlc.arg(invoice_type), sqlc.arg(issue_date),
    sqlc.arg(currency), sqlc.arg(receiver_type), sqlc.narg(receiver_identity_type), sqlc.arg(receiver_identity_enc),
    sqlc.arg(receiver_identity_hint), sqlc.arg(receiver_name), sqlc.arg(receiver_first_name), sqlc.arg(receiver_last_name),
    sqlc.arg(receiver_tax_office), sqlc.arg(receiver_address), sqlc.arg(receiver_district), sqlc.arg(receiver_city),
    sqlc.arg(receiver_postal_code), sqlc.arg(receiver_country), sqlc.arg(receiver_email), sqlc.arg(receiver_einvoice),
    sqlc.arg(receiver_alias), sqlc.arg(taxpayer_source), sqlc.narg(taxpayer_checked_at), sqlc.arg(line_extension_amount),
    sqlc.arg(tax_amount), sqlc.arg(payable_amount), sqlc.arg(linked_amount), sqlc.arg(note), sqlc.narg(idempotency_key),
    sqlc.narg(created_by)
)
RETURNING id;

-- name: GetInvoiceUUIDByIdempotencyKey :one
SELECT i.uuid FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.idempotency_key = sqlc.arg(idempotency_key);

-- name: InsertInvoiceLine :exec
INSERT INTO invoice_lines (
    organization_id, invoice_id, position, kind, name, days, entry_id, gross_amount, net_amount, vat_rate, vat_amount,
    exemption_code, exemption_reason
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(invoice_id), sqlc.arg(position), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(days),
    sqlc.narg(entry_id), sqlc.arg(gross_amount), sqlc.arg(net_amount), sqlc.arg(vat_rate), sqlc.arg(vat_amount),
    sqlc.arg(exemption_code), sqlc.arg(exemption_reason)
);

-- name: InsertInvoiceLink :exec
-- The invoice covers a ledger item. uq_ledger_entry_links_invoiced refuses
-- an item a standing invoice already covers (RT-A22: no double invoicing).
INSERT INTO ledger_entry_links (organization_id, entry_id, document_type, document_ref, invoice_id, amount, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(entry_id), 'invoice', sqlc.arg(document_ref), sqlc.arg(invoice_id),
    sqlc.arg(amount), sqlc.narg(created_by));

-- name: GetInvoice :one
SELECT i.id, i.uuid, i.organization_id, i.branch_id, i.account_id, i.customer_id, i.rental_id, i.returns_invoice_id,
    i.series, i.sequence_year, i.sequence_no, i.number, i.profile, i.invoice_type, i.status, i.issue_date, i.currency,
    i.receiver_type, i.receiver_identity_type, i.receiver_identity_enc, i.receiver_identity_hint, i.receiver_name,
    i.receiver_first_name, i.receiver_last_name, i.receiver_tax_office, i.receiver_address, i.receiver_district,
    i.receiver_city, i.receiver_postal_code, i.receiver_country, i.receiver_email, i.receiver_einvoice, i.receiver_alias,
    i.taxpayer_source, i.taxpayer_checked_at, i.line_extension_amount, i.tax_amount, i.payable_amount, i.linked_amount,
    i.note, i.validation_errors, i.validated_at, i.xml_key, i.xml_sha256, i.pdf_key, i.integration_job_id,
    i.external_ref, i.hold_reason, i.last_error, i.cancel_job_id, i.cancel_reason, i.cancelled_at, i.returned_at,
    i.finalized_at, i.sent_at, i.succeeded_at, i.created_at, i.updated_at,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    a.uuid AS account_uuid, a.code AS account_code, a.name AS account_name,
    c.uuid AS customer_uuid,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    ri.uuid AS returns_uuid, ri.number AS returns_number, ri.issue_date AS returns_issue_date,
    rb.uuid AS returned_by_uuid, rb.number AS returned_by_number, rb.status AS returned_by_status,
    j.uuid AS job_uuid, j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
    j.last_error AS job_last_error, j.provider AS job_provider, j.provider_ref AS job_provider_ref,
    j.next_attempt_at AS job_next_attempt_at, j.locked_until AS job_locked_until, j.integration_account_id AS job_account_id,
    cj.uuid AS cancel_job_uuid, cj.status AS cancel_job_status, cj.last_error AS cancel_job_last_error,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname, u.email AS created_by_email,
    cu.uuid AS cancelled_by_uuid, cu.name AS cancelled_by_name, cu.surname AS cancelled_by_surname
FROM invoices i
JOIN branches b ON b.organization_id = i.organization_id AND b.id = i.branch_id
LEFT JOIN accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id
LEFT JOIN customers c ON c.organization_id = i.organization_id AND c.id = i.customer_id
LEFT JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
LEFT JOIN invoices ri ON ri.organization_id = i.organization_id AND ri.id = i.returns_invoice_id
LEFT JOIN invoices rb ON rb.organization_id = i.organization_id AND rb.returns_invoice_id = i.id
    AND rb.status NOT IN ('cancelled', 'failed')
LEFT JOIN integration_jobs j ON j.organization_id = i.organization_id AND j.id = i.integration_job_id
LEFT JOIN integration_jobs cj ON cj.organization_id = i.organization_id AND cj.id = i.cancel_job_id
LEFT JOIN users u ON u.id = i.created_by
LEFT JOIN users cu ON cu.id = i.cancelled_by
WHERE i.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (a.id IS NULL OR a.organization_id = sqlc.arg(organization_id))
  AND (c.id IS NULL OR c.organization_id = sqlc.arg(organization_id))
  AND (r.id IS NULL OR r.organization_id = sqlc.arg(organization_id))
  AND (ri.id IS NULL OR ri.organization_id = sqlc.arg(organization_id))
  AND (rb.id IS NULL OR rb.organization_id = sqlc.arg(organization_id))
  AND (j.id IS NULL OR j.organization_id = sqlc.arg(organization_id))
  AND (cj.id IS NULL OR cj.organization_id = sqlc.arg(organization_id))
  AND i.uuid = sqlc.arg(uuid);

-- name: GetInvoiceRef :one
-- An invoice by id (job listener, operations).
SELECT i.id, i.uuid, i.branch_id, i.status, i.profile, i.invoice_type, i.number, i.returns_invoice_id,
    i.integration_job_id, i.cancel_job_id, i.xml_key
FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.id = sqlc.arg(id);

-- name: GetInvoiceForJob :one
-- The invoice a send or cancel job belongs to.
SELECT i.id, i.uuid, i.status, i.profile, i.invoice_type, i.number, i.xml_key, i.receiver_identity_type,
    i.receiver_identity_enc, i.receiver_alias, i.returns_invoice_id, i.integration_job_id, i.cancel_job_id,
    i.cancel_reason, i.external_ref
FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND (i.integration_job_id = sqlc.arg(job_id) OR i.cancel_job_id = sqlc.arg(job_id));

-- name: ListInvoiceLines :many
SELECT l.id, l.uuid, l.position, l.kind, l.name, l.days, l.entry_id, l.gross_amount, l.net_amount, l.vat_rate,
    l.vat_amount, l.exemption_code, l.exemption_reason,
    e.uuid AS entry_uuid, e.purpose AS entry_purpose, e.amount AS entry_amount
FROM invoice_lines l
LEFT JOIN ledger_entries e ON e.organization_id = l.organization_id AND e.id = l.entry_id
WHERE l.organization_id = sqlc.arg(organization_id)
  AND (e.id IS NULL OR e.organization_id = sqlc.arg(organization_id))
  AND l.invoice_id = sqlc.arg(invoice_id)
ORDER BY l.position;

-- name: ListInvoiceLinks :many
SELECT l.id, l.entry_id, l.amount, l.released_at, l.release_reason, l.created_at,
    e.uuid AS entry_uuid, e.kind AS entry_kind, e.purpose AS entry_purpose, e.amount AS entry_amount,
    e.entry_date, e.description AS entry_description,
    r.uuid AS rental_uuid, r.reference AS rental_reference
FROM ledger_entry_links l
JOIN ledger_entries e ON e.organization_id = l.organization_id AND e.id = l.entry_id
LEFT JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
WHERE l.organization_id = sqlc.arg(organization_id)
  AND e.organization_id = sqlc.arg(organization_id)
  AND (r.id IS NULL OR r.organization_id = sqlc.arg(organization_id))
  AND l.invoice_id = sqlc.arg(invoice_id)::bigint
ORDER BY l.id;

-- name: ListInvoices :many
SELECT i.id, i.uuid, i.number, i.profile, i.invoice_type, i.status, i.issue_date, i.currency, i.receiver_type,
    i.receiver_name, i.receiver_identity_hint, i.receiver_einvoice, i.line_extension_amount, i.tax_amount,
    i.payable_amount, i.linked_amount, i.external_ref, i.hold_reason, i.last_error, i.returned_at, i.cancelled_at,
    i.validation_errors, i.created_at, i.updated_at,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    a.uuid AS account_uuid, a.code AS account_code, a.name AS account_name,
    r.uuid AS rental_uuid, r.reference AS rental_reference,
    ri.uuid AS returns_uuid, ri.number AS returns_number,
    j.uuid AS job_uuid, j.status AS job_status, j.attempts AS job_attempts,
    COUNT(*) OVER ()::bigint AS total
FROM invoices i
JOIN branches b ON b.organization_id = i.organization_id AND b.id = i.branch_id
LEFT JOIN accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id
LEFT JOIN rentals r ON r.organization_id = i.organization_id AND r.id = i.rental_id
LEFT JOIN invoices ri ON ri.organization_id = i.organization_id AND ri.id = i.returns_invoice_id
LEFT JOIN integration_jobs j ON j.organization_id = i.organization_id AND j.id = i.integration_job_id
WHERE i.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (a.id IS NULL OR a.organization_id = sqlc.arg(organization_id))
  AND (r.id IS NULL OR r.organization_id = sqlc.arg(organization_id))
  AND (ri.id IS NULL OR ri.organization_id = sqlc.arg(organization_id))
  AND (j.id IS NULL OR j.organization_id = sqlc.arg(organization_id))
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR i.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(profiles)::text[] IS NULL OR i.profile = ANY(sqlc.narg(profiles)::text[]))
  AND (sqlc.narg(types)::text[] IS NULL OR i.invoice_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(account_id)::bigint IS NULL OR i.account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(rental_id)::bigint IS NULL OR i.rental_id = sqlc.narg(rental_id)::bigint OR EXISTS (
      SELECT 1 FROM ledger_entry_links lk
      JOIN ledger_entries le ON le.organization_id = lk.organization_id AND le.id = lk.entry_id
      WHERE lk.organization_id = i.organization_id AND le.organization_id = i.organization_id
        AND lk.invoice_id = i.id AND le.rental_id = sqlc.narg(rental_id)::bigint
  ))
  AND (sqlc.narg(date_from)::date IS NULL OR i.issue_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR i.issue_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(needs_action)::boolean IS NULL OR (
      sqlc.narg(needs_action)::boolean = (i.status = 'failed' OR j.status IN ('needs_verification', 'waiting_dependency', 'blocked'))
  ))
  AND (sqlc.narg(q)::text IS NULL
       OR i.number ILIKE '%' || sqlc.narg(q)::text || '%'
       OR i.receiver_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR r.reference ILIKE '%' || sqlc.narg(q)::text || '%'
       OR a.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'issue_date' AND NOT sqlc.arg(sort_desc)::boolean THEN i.issue_date END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'issue_date' AND sqlc.arg(sort_desc)::boolean THEN i.issue_date END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'number' AND NOT sqlc.arg(sort_desc)::boolean THEN i.number END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'number' AND sqlc.arg(sort_desc)::boolean THEN i.number END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'payable_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN i.payable_amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'payable_amount' AND sqlc.arg(sort_desc)::boolean THEN i.payable_amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN i.created_at END ASC,
    i.created_at DESC, i.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: InvoiceStatusFacets :many
SELECT i.status, COUNT(*)::bigint AS total
FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY i.status;

-- name: InvoiceProfileFacets :many
SELECT i.profile, COUNT(*)::bigint AS total
FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY i.profile;

-- name: InvoiceBranchFacets :many
SELECT b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name, COUNT(*)::bigint AS total
FROM invoices i
JOIN branches b ON b.organization_id = i.organization_id AND b.id = i.branch_id
WHERE i.organization_id = sqlc.arg(organization_id)
  AND b.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.code, b.name
ORDER BY b.code;

-- name: CountInvoicesNeedingAction :one
-- Failed sends and sends waiting for a person (finance screen badge).
SELECT COUNT(*)::bigint
FROM invoices i
LEFT JOIN integration_jobs j ON j.organization_id = i.organization_id AND j.id = i.integration_job_id
WHERE i.organization_id = sqlc.arg(organization_id)
  AND (j.id IS NULL OR j.organization_id = sqlc.arg(organization_id))
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR i.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (i.status = 'failed' OR j.status IN ('needs_verification', 'waiting_dependency', 'blocked'));

-- name: UpdateInvoiceDraft :execrows
UPDATE invoices
SET profile = sqlc.arg(profile), issue_date = sqlc.arg(issue_date), note = sqlc.arg(note),
    receiver_type = sqlc.arg(receiver_type), receiver_identity_type = sqlc.narg(receiver_identity_type),
    receiver_identity_enc = sqlc.arg(receiver_identity_enc), receiver_identity_hint = sqlc.arg(receiver_identity_hint),
    receiver_name = sqlc.arg(receiver_name), receiver_first_name = sqlc.arg(receiver_first_name),
    receiver_last_name = sqlc.arg(receiver_last_name), receiver_tax_office = sqlc.arg(receiver_tax_office),
    receiver_address = sqlc.arg(receiver_address), receiver_district = sqlc.arg(receiver_district),
    receiver_city = sqlc.arg(receiver_city), receiver_postal_code = sqlc.arg(receiver_postal_code),
    receiver_country = sqlc.arg(receiver_country), receiver_email = sqlc.arg(receiver_email),
    receiver_einvoice = sqlc.arg(receiver_einvoice), receiver_alias = sqlc.arg(receiver_alias),
    taxpayer_source = sqlc.arg(taxpayer_source), taxpayer_checked_at = sqlc.narg(taxpayer_checked_at),
    line_extension_amount = sqlc.arg(line_extension_amount), tax_amount = sqlc.arg(tax_amount),
    payable_amount = sqlc.arg(payable_amount), linked_amount = sqlc.arg(linked_amount),
    validation_errors = '[]'::jsonb, validated_at = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: UpdateInvoiceLine :exec
UPDATE invoice_lines
SET name = sqlc.arg(name), net_amount = sqlc.arg(net_amount), vat_rate = sqlc.arg(vat_rate),
    vat_amount = sqlc.arg(vat_amount), exemption_code = sqlc.arg(exemption_code),
    exemption_reason = sqlc.arg(exemption_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: DeleteInvoiceLinesOfEntries :exec
DELETE FROM invoice_lines
WHERE organization_id = sqlc.arg(organization_id)
  AND invoice_id = sqlc.arg(invoice_id)
  AND entry_id = ANY(sqlc.arg(entry_ids)::bigint[]);

-- name: RenumberInvoiceLines :exec
UPDATE invoice_lines l
SET position = n.rn
FROM (
    SELECT x.id, ROW_NUMBER() OVER (ORDER BY x.position, x.id)::integer AS rn
    FROM invoice_lines x
    WHERE x.organization_id = sqlc.arg(organization_id) AND x.invoice_id = sqlc.arg(invoice_id)
) n
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.id = n.id;

-- name: ReleaseInvoiceLinks :execrows
-- Frees the items of an invoice (cancelled / returned) or the items taken
-- off a draft (removed): they may be invoiced again.
UPDATE ledger_entry_links
SET released_at = sqlc.arg(released_at), release_reason = sqlc.arg(reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND invoice_id = sqlc.arg(invoice_id)::bigint
  AND released_at IS NULL
  AND (sqlc.narg(entry_ids)::bigint[] IS NULL OR entry_id = ANY(sqlc.narg(entry_ids)::bigint[]));

-- name: SetInvoiceValidation :exec
UPDATE invoices
SET validation_errors = sqlc.arg(validation_errors), validated_at = sqlc.arg(validated_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: FinalizeInvoice :execrows
-- draft → queued: the number, the stored XML and the send job.
UPDATE invoices
SET status = 'queued', series = sqlc.arg(series), sequence_year = sqlc.arg(sequence_year),
    sequence_no = sqlc.arg(sequence_no), number = sqlc.arg(number), xml_key = sqlc.arg(xml_key),
    xml_sha256 = sqlc.arg(xml_sha256), integration_job_id = sqlc.arg(integration_job_id),
    hold_reason = sqlc.arg(hold_reason), last_error = sqlc.arg(last_error), finalized_at = sqlc.arg(finalized_at),
    validation_errors = '[]'::jsonb, validated_at = sqlc.arg(finalized_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = 'draft';

-- name: InsertInvoiceJob :one
-- The send / cancel job of an invoice, created in the invoice's
-- transaction (outbox: a pending job is due at once; the sweep picks it up
-- if the process stops before the first step). A waiting job has no next
-- step until an integrator is connected.
INSERT INTO integration_jobs (
    organization_id, integration_account_id, direction, provider, kind, idempotency_key, payload, ref_type, ref_uuid,
    priority, max_attempts, status, next_attempt_at, last_error
) VALUES (
    sqlc.arg(organization_id), sqlc.narg(integration_account_id), 'outbound', sqlc.arg(provider), sqlc.arg(kind),
    sqlc.arg(idempotency_key), sqlc.arg(payload), 'invoice', sqlc.arg(ref_uuid), 'normal',
    sqlc.arg(max_attempts), sqlc.arg(status), sqlc.narg(next_attempt_at), sqlc.arg(last_error)
)
ON CONFLICT (provider, idempotency_key) DO NOTHING
RETURNING id, uuid;

-- name: RouteInvoiceJob :execrows
-- Moves a job that has sent nothing yet (waiting, or pending with no
-- attempt) to a new route. A job a worker holds, or one that made a call,
-- is left alone (0 rows).
UPDATE integration_jobs
SET status = sqlc.arg(status)::text, provider = sqlc.arg(provider), integration_account_id = sqlc.narg(integration_account_id),
    last_error = sqlc.arg(last_error), next_attempt_at = sqlc.narg(next_attempt_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND (status IN ('blocked', 'waiting_dependency') OR (status = 'pending' AND attempts = 0))
  AND (locked_until IS NULL OR locked_until < NOW());

-- name: StopUnsentInvoiceJob :execrows
-- Cancelling an invoice that sent nothing: its job ends as failed (nothing
-- was sent and nothing will be).
UPDATE integration_jobs
SET status = 'failed', last_error = sqlc.arg(last_error), next_attempt_at = NULL, completed_at = NOW(), locked_until = NULL
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND (status IN ('blocked', 'waiting_dependency') OR (status = 'pending' AND attempts = 0))
  AND (locked_until IS NULL OR locked_until < NOW());

-- name: SyncInvoiceSendState :exec
-- Mirrors the send job onto the invoice (runner listener).
UPDATE invoices
SET status = sqlc.arg(status), external_ref = sqlc.arg(external_ref), last_error = sqlc.arg(last_error),
    hold_reason = CASE WHEN sqlc.arg(clear_hold)::boolean THEN '' ELSE hold_reason END,
    sent_at = COALESCE(sent_at, sqlc.narg(sent_at)), succeeded_at = COALESCE(succeeded_at, sqlc.narg(succeeded_at))
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status NOT IN ('draft', 'cancelled');

-- name: SetInvoiceHold :exec
UPDATE invoices
SET hold_reason = sqlc.arg(hold_reason), last_error = sqlc.arg(last_error)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetInvoicePDF :exec
UPDATE invoices
SET pdf_key = sqlc.arg(pdf_key)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: CancelInvoice :execrows
UPDATE invoices
SET status = 'cancelled', cancel_reason = sqlc.arg(reason), cancelled_by = sqlc.narg(cancelled_by),
    cancelled_at = sqlc.arg(cancelled_at)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status = ANY(sqlc.arg(from_statuses)::text[]);

-- name: SetInvoiceCancelJob :exec
UPDATE invoices
SET cancel_job_id = sqlc.arg(cancel_job_id), cancel_reason = sqlc.arg(reason), cancelled_by = sqlc.narg(cancelled_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: MarkInvoiceReturned :exec
UPDATE invoices
SET returned_at = COALESCE(returned_at, sqlc.arg(returned_at))
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListWaitingInvoiceJobs :many
-- The release sweep: invoices waiting on a dependency, across
-- organizations (each row carries its organization_id; the service
-- re-scopes from it).
SELECT i.organization_id, i.id, i.uuid, j.id AS job_id
FROM invoices i
JOIN integration_jobs j ON j.organization_id = i.organization_id AND j.id = i.integration_job_id
WHERE j.status = 'waiting_dependency'
  AND j.kind LIKE 'einvoice.%'
ORDER BY i.organization_id, i.id
LIMIT sqlc.arg(limit_count);

-- name: GetInvoiceOrganization :one
SELECT o.uuid, o.slug FROM organizations o WHERE o.id = sqlc.arg(organization_id);

-- name: ListPortalReservationInvoices :many
-- Issued invoices of a reservation for the customer portal: the invoice of
-- its rental, or one covering one of its ledger items.
SELECT i.uuid, i.number, i.profile, i.invoice_type, i.issue_date, i.payable_amount, i.currency, i.status
FROM invoices i
WHERE i.organization_id = sqlc.arg(organization_id)
  AND i.status = 'succeeded'
  AND i.xml_key IS NOT NULL
  AND (
      EXISTS (
          SELECT 1 FROM rentals r
          WHERE r.organization_id = i.organization_id AND r.id = i.rental_id
            AND r.reservation_id = sqlc.arg(reservation_id)
      )
      OR EXISTS (
          SELECT 1 FROM ledger_entry_links lk
          JOIN ledger_entries le ON le.organization_id = lk.organization_id AND le.id = lk.entry_id
          WHERE lk.organization_id = i.organization_id AND le.organization_id = i.organization_id
            AND lk.invoice_id = i.id AND le.reservation_id = sqlc.arg(reservation_id)
      )
      OR EXISTS (
          SELECT 1 FROM invoices oi
          JOIN rentals orr ON orr.organization_id = oi.organization_id AND orr.id = oi.rental_id
          WHERE oi.organization_id = i.organization_id AND orr.organization_id = i.organization_id
            AND oi.id = i.returns_invoice_id AND orr.reservation_id = sqlc.arg(reservation_id)
      )
  )
ORDER BY i.issue_date, i.id;
