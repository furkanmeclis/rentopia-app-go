-- Corporate agreements (TEC-723, docs/design/corporate-agreements.md).
-- Branch scope: an agreement with a managing branch is visible inside that
-- branch's scope; a head office agreement (branch NULL) to everyone.
-- Validity state (computed against :today, Istanbul calendar):
--   upcoming  valid_from > today
--   expired   valid_to < today
--   current   otherwise

-- name: LockAgreementCodes :exec
SELECT pg_advisory_xact_lock(hashtextextended('agreements.codes:' || sqlc.arg(organization_id)::bigint::text, 0));

-- name: NextAgreementNumber :one
SELECT (COUNT(*) + 1)::bigint FROM corporate_agreements a WHERE a.organization_id = sqlc.arg(organization_id);

-- name: InsertAgreement :one
INSERT INTO corporate_agreements (
    organization_id, code, name, customer_id, branch_id, status, valid_from, valid_to, invoice_period,
    payment_term_days, open_account, credit_limit, coupon_eligible, monthly_km_allowance,
    maintenance_interval_days, maintenance_interval_km, currency, notes, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(customer_id), sqlc.narg(branch_id),
    sqlc.arg(status), sqlc.arg(valid_from), sqlc.narg(valid_to), sqlc.arg(invoice_period),
    sqlc.arg(payment_term_days), sqlc.arg(open_account), sqlc.narg(credit_limit), sqlc.arg(coupon_eligible),
    sqlc.narg(monthly_km_allowance), sqlc.narg(maintenance_interval_days), sqlc.narg(maintenance_interval_km),
    sqlc.arg(currency), sqlc.arg(notes), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateAgreement :execrows
UPDATE corporate_agreements
SET name = sqlc.arg(name), branch_id = sqlc.narg(branch_id), valid_from = sqlc.arg(valid_from),
    valid_to = sqlc.narg(valid_to), invoice_period = sqlc.arg(invoice_period),
    payment_term_days = sqlc.arg(payment_term_days), open_account = sqlc.arg(open_account),
    credit_limit = sqlc.narg(credit_limit), coupon_eligible = sqlc.arg(coupon_eligible),
    monthly_km_allowance = sqlc.narg(monthly_km_allowance),
    maintenance_interval_days = sqlc.narg(maintenance_interval_days),
    maintenance_interval_km = sqlc.narg(maintenance_interval_km), notes = sqlc.arg(notes)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: SetAgreementStatus :execrows
UPDATE corporate_agreements
SET status = sqlc.arg(status), terminated_at = sqlc.narg(terminated_at),
    termination_reason = sqlc.arg(termination_reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: SoftDeleteAgreement :execrows
UPDATE corporate_agreements SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: LockAgreement :one
SELECT a.id FROM corporate_agreements a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id)
  AND a.deleted_at IS NULL
FOR UPDATE;

-- name: GetAgreement :one
SELECT a.id, a.uuid, a.code, a.name, a.customer_id, a.branch_id, a.status, a.valid_from, a.valid_to,
    a.invoice_period, a.payment_term_days, a.open_account, a.credit_limit, a.coupon_eligible,
    a.monthly_km_allowance, a.maintenance_interval_days, a.maintenance_interval_km, a.currency, a.notes,
    a.terminated_at, a.termination_reason, a.created_at, a.updated_at,
    c.uuid AS customer_uuid, c.customer_type, c.company_title AS customer_company_title,
    c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.identity_hint AS customer_identity_hint,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    acc.uuid AS account_uuid, acc.code AS account_code, acc.payment_term_days AS account_payment_term_days,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM corporate_agreements a
JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
LEFT JOIN accounts acc ON acc.organization_id = a.organization_id AND acc.customer_id = a.customer_id
LEFT JOIN users cu ON cu.id = a.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid)
  AND a.deleted_at IS NULL;

-- name: ListAgreements :many
SELECT a.id, a.uuid, a.code, a.name, a.status, a.valid_from, a.valid_to, a.invoice_period, a.payment_term_days,
    a.open_account, a.credit_limit, a.coupon_eligible, a.currency, a.created_at, a.updated_at, a.branch_id,
    c.uuid AS customer_uuid, c.company_title AS customer_company_title, c.customer_type,
    c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    b.uuid AS branch_uuid, b.code AS branch_code, b.name AS branch_name,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = a.organization_id AND r.agreement_id = a.id
       AND r.status NOT IN ('cancelled', 'expired', 'no_show'))::bigint AS reservation_count,
    (SELECT COUNT(*) FROM rate_plans rp
     WHERE rp.organization_id = a.organization_id AND rp.agreement_id = a.id AND rp.deleted_at IS NULL)::bigint AS tariff_count,
    COUNT(*) OVER ()::bigint AS total_count
FROM corporate_agreements a
JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND a.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(customer_uuids)::uuid[] IS NULL OR c.uuid = ANY(sqlc.narg(customer_uuids)::uuid[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(invoice_periods)::text[] IS NULL OR a.invoice_period = ANY(sqlc.narg(invoice_periods)::text[]))
  AND (
    sqlc.narg(validity)::text[] IS NULL
    OR (CASE WHEN a.valid_from > sqlc.arg(today)::date THEN 'upcoming'
             WHEN a.valid_to IS NOT NULL AND a.valid_to < sqlc.arg(today)::date THEN 'expired'
             ELSE 'current' END) = ANY(sqlc.narg(validity)::text[])
  )
  AND (sqlc.narg(valid_on_from)::date IS NULL OR a.valid_to IS NULL OR a.valid_to >= sqlc.narg(valid_on_from)::date)
  AND (sqlc.narg(valid_on_to)::date IS NULL OR a.valid_from <= sqlc.narg(valid_on_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR a.code ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN a.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN a.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN a.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN a.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer' AND NOT sqlc.arg(sort_desc)::boolean THEN c.company_title END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer' AND sqlc.arg(sort_desc)::boolean THEN c.company_title END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN a.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN a.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'valid_from' AND NOT sqlc.arg(sort_desc)::boolean THEN a.valid_from END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'valid_from' AND sqlc.arg(sort_desc)::boolean THEN a.valid_from END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'valid_to' AND NOT sqlc.arg(sort_desc)::boolean THEN a.valid_to END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'valid_to' AND sqlc.arg(sort_desc)::boolean THEN a.valid_to END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN a.created_at END DESC,
  a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: ListAgreementFacets :many
-- facet: status | customer | branch | invoice_period | validity, in the caller's branch scope.
WITH scoped AS (
    SELECT a.organization_id, a.status, a.customer_id, a.branch_id, a.invoice_period,
        (CASE WHEN a.valid_from > sqlc.arg(today)::date THEN 'upcoming'
              WHEN a.valid_to IS NOT NULL AND a.valid_to < sqlc.arg(today)::date THEN 'expired'
              ELSE 'current' END)::text AS validity
    FROM corporate_agreements a
    WHERE a.organization_id = sqlc.arg(organization_id)
      AND a.deleted_at IS NULL
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
)
SELECT 'status'::text AS facet, s.status::text AS value, ''::text AS label, COUNT(*)::bigint AS count
FROM scoped s GROUP BY s.status
UNION ALL
SELECT 'customer'::text, c.uuid::text, c.company_title::text, COUNT(*)::bigint
FROM scoped s JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
GROUP BY c.uuid, c.company_title
UNION ALL
SELECT 'branch'::text, b.uuid::text, b.name::text, COUNT(*)::bigint
FROM scoped s JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'invoice_period'::text, s.invoice_period::text, ''::text, COUNT(*)::bigint FROM scoped s GROUP BY s.invoice_period
UNION ALL
SELECT 'validity'::text, s.validity, ''::text, COUNT(*)::bigint FROM scoped s GROUP BY s.validity;

-- name: ListAgreementContacts :many
SELECT ac.id, ac.uuid, ac.role, ac.note, ac.created_at,
    cc.uuid AS contact_uuid, cc.full_name, cc.title, cc.phone, cc.email, cc.is_primary
FROM agreement_contacts ac
JOIN customer_contacts cc ON cc.organization_id = ac.organization_id AND cc.id = ac.customer_contact_id
WHERE ac.organization_id = sqlc.arg(organization_id)
  AND cc.organization_id = sqlc.arg(organization_id)
  AND ac.agreement_id = sqlc.arg(agreement_id)
ORDER BY cc.is_primary DESC, cc.full_name, ac.id;

-- name: GetCustomerContactForAgreement :one
SELECT cc.id, cc.uuid FROM customer_contacts cc
WHERE cc.organization_id = sqlc.arg(organization_id)
  AND cc.uuid = sqlc.arg(uuid)
  AND cc.customer_id = sqlc.arg(customer_id);

-- name: InsertAgreementContact :one
INSERT INTO agreement_contacts (organization_id, agreement_id, customer_contact_id, role, note)
VALUES (sqlc.arg(organization_id), sqlc.arg(agreement_id), sqlc.arg(customer_contact_id), sqlc.arg(role), sqlc.arg(note))
RETURNING id, uuid;

-- name: DeleteAgreementContact :execrows
DELETE FROM agreement_contacts
WHERE organization_id = sqlc.arg(organization_id)
  AND agreement_id = sqlc.arg(agreement_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListAgreementDrivers :many
SELECT ad.id, ad.uuid, ad.note, ad.created_at, ad.driver_id, ad.customer_id,
    d.uuid AS driver_uuid, d.first_name AS driver_first_name, d.last_name AS driver_last_name,
    d.licence_hint AS driver_licence_hint, d.licence_expires_on AS driver_licence_expires_on,
    dc.uuid AS driver_customer_uuid, dc.customer_type AS driver_customer_type,
    dc.company_title AS driver_customer_company_title, dc.first_name AS driver_customer_first_name,
    dc.last_name AS driver_customer_last_name,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.identity_hint AS customer_identity_hint
FROM agreement_drivers ad
LEFT JOIN drivers d ON d.organization_id = ad.organization_id AND d.id = ad.driver_id
LEFT JOIN customers dc ON dc.organization_id = d.organization_id AND dc.id = d.customer_id
LEFT JOIN customers c ON c.organization_id = ad.organization_id AND c.id = ad.customer_id
WHERE ad.organization_id = sqlc.arg(organization_id)
  AND ad.agreement_id = sqlc.arg(agreement_id)
ORDER BY ad.id;

-- name: GetDriverForAgreement :one
SELECT d.id, d.uuid, d.customer_id FROM drivers d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid);

-- name: GetCustomerForAgreement :one
SELECT c.id, c.uuid, c.customer_type, c.company_title, c.first_name, c.last_name, c.anonymized_at
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: InsertAgreementDriver :one
INSERT INTO agreement_drivers (organization_id, agreement_id, driver_id, customer_id, note)
VALUES (sqlc.arg(organization_id), sqlc.arg(agreement_id), sqlc.narg(driver_id), sqlc.narg(customer_id), sqlc.arg(note))
RETURNING id, uuid;

-- name: DeleteAgreementDriver :execrows
DELETE FROM agreement_drivers
WHERE organization_id = sqlc.arg(organization_id)
  AND agreement_id = sqlc.arg(agreement_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListAgreementReservations :many
-- Reservations sold under the agreement with their rental (if any) and
-- what the ledger accrued for their rent.
SELECT r.id, r.uuid, r.reference, r.status, r.starts_at, r.ends_at, r.total_amount, r.currency, r.open_account,
    r.pickup_branch_id, r.return_branch_id,
    p.uuid AS product_uuid, p.code AS product_code, p.title AS product_title,
    c.uuid AS customer_uuid, c.customer_type, c.company_title AS customer_company_title,
    c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    rt.uuid AS rental_uuid, rt.status AS rental_status, rt.rent_billing, rt.actual_pickup_at, rt.actual_return_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    COALESCE((
        SELECT SUM(e.amount) FROM ledger_entries e
        WHERE e.organization_id = r.organization_id AND e.reservation_id = r.id AND e.kind = 'accrual' AND e.purpose = 'rent'
          AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id)
    ), 0)::numeric AS rent_accrued
FROM reservations r
JOIN rental_products p ON p.organization_id = r.organization_id AND p.id = r.rental_product_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
LEFT JOIN rentals rt ON rt.organization_id = r.organization_id AND rt.reservation_id = r.id AND rt.status <> 'cancelled'
LEFT JOIN vehicles v ON v.organization_id = rt.organization_id AND v.id = rt.vehicle_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND p.organization_id = sqlc.arg(organization_id)
  AND r.agreement_id = sqlc.arg(agreement_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY r.starts_at DESC, r.id DESC
LIMIT 500;

-- name: ListAgreementBillingPeriods :many
-- Billing plans of the agreement's rentals, every revision, with the
-- accrual entry of each posted period (invoicing links to it).
SELECT bp.id, bp.uuid, bp.period_no, bp.revision, bp.starts_on, bp.ends_on, bp.days, bp.partial, bp.amount,
    bp.currency, bp.km_allowance, bp.accrual_key, bp.status, bp.posted_at, bp.closed_at,
    rt.uuid AS rental_uuid, rt.reference AS rental_reference,
    e.uuid AS entry_uuid, e.entry_date, e.due_date
FROM rental_billing_periods bp
JOIN rentals rt ON rt.organization_id = bp.organization_id AND rt.id = bp.rental_id
JOIN reservations r ON r.organization_id = rt.organization_id AND r.id = rt.reservation_id
LEFT JOIN ledger_entries e ON e.organization_id = bp.organization_id AND e.id = bp.ledger_entry_id
WHERE bp.organization_id = sqlc.arg(organization_id)
  AND rt.organization_id = sqlc.arg(organization_id)
  AND r.agreement_id = sqlc.arg(agreement_id)
  AND (sqlc.narg(rental_uuid)::uuid IS NULL OR rt.uuid = sqlc.narg(rental_uuid)::uuid)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY rt.id, bp.period_no, bp.revision;

-- name: ListAgreementLedgerEntries :many
-- The agreement's statement: ledger entries of its reservations and
-- rentals (deposits excluded, as in balances), oldest first.
SELECT e.id, e.uuid, e.kind, e.direction, e.purpose, e.amount, e.currency, e.entry_date, e.due_date,
    e.description, e.reason, e.accrual_key, e.billing_period_id, e.reverses_entry_id,
    r.uuid AS reservation_uuid, r.reference AS reservation_reference,
    rt.uuid AS rental_uuid
FROM ledger_entries e
JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
LEFT JOIN rentals rt ON rt.organization_id = e.organization_id AND rt.id = e.rental_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND r.agreement_id = sqlc.arg(agreement_id)
  AND e.purpose <> 'deposit'
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR r.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
ORDER BY e.entry_date, e.id
LIMIT 2000;

-- name: ListAgreementsForExport :many
SELECT a.uuid, a.code, a.name, a.status, a.valid_from, a.valid_to, a.invoice_period, a.payment_term_days,
    a.open_account, a.credit_limit, a.coupon_eligible, a.currency, a.notes, a.created_at,
    c.company_title AS customer_company_title, b.code AS branch_code
FROM corporate_agreements a
JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND a.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(customer_uuids)::uuid[] IS NULL OR c.uuid = ANY(sqlc.narg(customer_uuids)::uuid[]))
  AND (
    sqlc.narg(validity)::text[] IS NULL
    OR (CASE WHEN a.valid_from > sqlc.arg(today)::date THEN 'upcoming'
             WHEN a.valid_to IS NOT NULL AND a.valid_to < sqlc.arg(today)::date THEN 'expired'
             ELSE 'current' END) = ANY(sqlc.narg(validity)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR a.code ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY a.code
LIMIT sqlc.arg(limit_count);

-- ------------------------------------------------------- sales (ports)

-- name: GetAgreementForSale :one
-- What pricing and reservations need of an agreement (by uuid).
SELECT a.id, a.uuid, a.code, a.name, a.status, a.valid_from, a.valid_to, a.customer_id, a.branch_id,
    a.invoice_period, a.payment_term_days, a.open_account, a.credit_limit, a.coupon_eligible,
    a.monthly_km_allowance, a.currency, c.uuid AS customer_uuid
FROM corporate_agreements a
JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid)
  AND a.deleted_at IS NULL;

-- name: GetAgreementForSaleByID :one
SELECT a.id, a.uuid, a.code, a.name, a.status, a.valid_from, a.valid_to, a.customer_id, a.branch_id,
    a.invoice_period, a.payment_term_days, a.open_account, a.credit_limit, a.coupon_eligible,
    a.monthly_km_allowance, a.currency, c.uuid AS customer_uuid
FROM corporate_agreements a
JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id);

-- name: AgreementDriverRules :one
-- Permitted drivers: none listed = anyone; otherwise the driver must be
-- listed, or belong to a listed (individual) customer.
SELECT
    (SELECT COUNT(*) FROM agreement_drivers ad
     WHERE ad.organization_id = sqlc.arg(organization_id) AND ad.agreement_id = sqlc.arg(agreement_id))::bigint AS listed,
    EXISTS (
        SELECT 1 FROM agreement_drivers ad
        JOIN drivers d ON d.organization_id = ad.organization_id AND d.id = sqlc.arg(driver_id)
        WHERE ad.organization_id = sqlc.arg(organization_id) AND ad.agreement_id = sqlc.arg(agreement_id)
          AND (ad.driver_id = d.id OR ad.customer_id = d.customer_id)
    )::boolean AS permitted;

-- name: AgreementCreditExposure :one
-- Open-account exposure of the agreement's customer: its ledger balance
-- (deposits excluded) plus the rent of the agreement's live reservations
-- not accrued yet.
SELECT
    COALESCE((
        SELECT SUM(CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END)
        FROM ledger_entries e
        JOIN accounts ac ON ac.organization_id = e.organization_id AND ac.id = e.account_id
        WHERE e.organization_id = sqlc.arg(organization_id) AND ac.customer_id = sqlc.arg(customer_id)
          AND e.purpose <> 'deposit'
    ), 0)::numeric AS balance,
    COALESCE((
        SELECT SUM(GREATEST(r.total_amount - COALESCE((
            SELECT SUM(e.amount) FROM ledger_entries e
            WHERE e.organization_id = r.organization_id AND e.reservation_id = r.id
              AND e.kind = 'accrual' AND e.purpose = 'rent'
              AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id)
        ), 0), 0))
        FROM reservations r
        WHERE r.organization_id = sqlc.arg(organization_id) AND r.agreement_id = sqlc.arg(agreement_id)
          AND r.status IN ('awaiting_review', 'awaiting_payment', 'confirmed')
          AND (sqlc.narg(exclude_reservation_id)::bigint IS NULL OR r.id <> sqlc.narg(exclude_reservation_id)::bigint)
    ), 0)::numeric AS pending;

-- name: ListCustomerAgreementsForSale :many
-- Active agreements of a customer valid on a day (the wizard offers them).
SELECT a.uuid, a.code, a.name, a.invoice_period, a.open_account, a.credit_limit, a.coupon_eligible,
    a.payment_term_days, a.valid_from, a.valid_to, a.branch_id
FROM corporate_agreements a
JOIN customers c ON c.organization_id = a.organization_id AND c.id = a.customer_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(customer_uuid)
  AND a.deleted_at IS NULL
  AND a.status = 'active'
  AND a.valid_from <= sqlc.arg(on_day)::date
  AND (a.valid_to IS NULL OR a.valid_to >= sqlc.arg(on_day)::date)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY a.code;

-- name: SetAccountTermsForCustomer :execrows
-- The agreement's payment term on its customer's ledger account.
UPDATE accounts
SET payment_term_days = sqlc.arg(payment_term_days)
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id);
