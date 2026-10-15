-- Tenant resource: customers, corporate contacts, marketing consents,
-- drivers and customer documents (TEC-705).
-- Every query filters by organization_id (docs/design/tenant-core.md §8).
-- Customers are organization-wide (no branch scope). Identity / licence
-- numbers are ciphertext (*_enc) with a blind index (*_bidx) and a masked
-- hint (*_hint); the plaintext never reaches SQL.

-- name: CreateCustomer :one
INSERT INTO customers (
    organization_id, customer_type, first_name, last_name, company_title,
    identity_type, identity_enc, identity_bidx, identity_hint, nationality, tax_office,
    phone, email, billing_address, billing_city, billing_district, billing_postal_code,
    billing_country, e_invoice, note, tags, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_type), sqlc.arg(first_name), sqlc.arg(last_name),
    sqlc.arg(company_title), sqlc.narg(identity_type), sqlc.arg(identity_enc), sqlc.arg(identity_bidx),
    sqlc.arg(identity_hint), sqlc.narg(nationality), sqlc.arg(tax_office), sqlc.arg(phone), sqlc.arg(email),
    sqlc.arg(billing_address), sqlc.arg(billing_city), sqlc.arg(billing_district), sqlc.arg(billing_postal_code),
    sqlc.narg(billing_country), sqlc.arg(e_invoice), sqlc.arg(note), sqlc.arg(tags)::text[], sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateCustomer :one
UPDATE customers SET
    customer_type = sqlc.arg(customer_type),
    first_name = sqlc.arg(first_name),
    last_name = sqlc.arg(last_name),
    company_title = sqlc.arg(company_title),
    identity_type = sqlc.narg(identity_type),
    identity_enc = sqlc.arg(identity_enc),
    identity_bidx = sqlc.arg(identity_bidx),
    identity_hint = sqlc.arg(identity_hint),
    nationality = sqlc.narg(nationality),
    tax_office = sqlc.arg(tax_office),
    phone = sqlc.arg(phone),
    email = sqlc.arg(email),
    billing_address = sqlc.arg(billing_address),
    billing_city = sqlc.arg(billing_city),
    billing_district = sqlc.arg(billing_district),
    billing_postal_code = sqlc.arg(billing_postal_code),
    billing_country = sqlc.narg(billing_country),
    e_invoice = sqlc.arg(e_invoice),
    note = sqlc.arg(note),
    tags = sqlc.arg(tags)::text[]
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
  AND anonymized_at IS NULL
RETURNING *;

-- name: SetCustomerTags :one
UPDATE customers SET tags = sqlc.arg(tags)::text[]
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
  AND anonymized_at IS NULL
RETURNING *;

-- name: GetCustomer :one
SELECT * FROM customers
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetCustomerByIdentityIndex :one
SELECT * FROM customers
WHERE organization_id = sqlc.arg(organization_id)
  AND identity_bidx = sqlc.arg(identity_bidx)
  AND identity_bidx <> ''
  AND deleted_at IS NULL;

-- name: CustomerIdentityTaken :one
SELECT EXISTS (
    SELECT 1 FROM customers
    WHERE organization_id = sqlc.arg(organization_id)
      AND identity_bidx = sqlc.arg(identity_bidx)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: DeleteCustomer :execrows
UPDATE customers SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: AnonymizeCustomer :execrows
-- Data subject request: wipes every personal value and keeps the row
-- (references from reservations / invoices stay valid).
UPDATE customers SET
    first_name = '',
    last_name = '',
    company_title = '',
    identity_type = NULL,
    identity_enc = '',
    identity_bidx = '',
    identity_hint = '',
    nationality = NULL,
    tax_office = '',
    phone = '',
    email = '',
    billing_address = '',
    billing_city = '',
    billing_district = '',
    billing_postal_code = '',
    billing_country = NULL,
    e_invoice = FALSE,
    note = '',
    tags = '{}',
    anonymized_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND anonymized_at IS NULL;

-- name: ListCustomers :many
SELECT
    c.*,
    COALESCE((
        SELECT array_agg(mc.channel ORDER BY mc.channel)
        FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
    ), '{}')::text[] AS consent_channels,
    (
        SELECT COUNT(*) FROM drivers d
        WHERE d.organization_id = c.organization_id AND d.customer_id = c.id AND d.deleted_at IS NULL
    )::bigint AS drivers_count
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(customer_types)::text[] IS NULL OR c.customer_type = ANY(sqlc.narg(customer_types)::text[]))
  AND (sqlc.narg(nationalities)::text[] IS NULL OR c.nationality = ANY(sqlc.narg(nationalities)::text[]))
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(anonymized)::boolean IS NULL OR (c.anonymized_at IS NOT NULL) = sqlc.narg(anonymized)::boolean)
  AND (
    sqlc.narg(consents)::text[] IS NULL
    OR EXISTS (
        SELECT 1 FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
          AND mc.channel = ANY(sqlc.narg(consents)::text[])
    )
    OR ('none' = ANY(sqlc.narg(consents)::text[]) AND NOT EXISTS (
        SELECT 1 FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
    ))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.first_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.last_name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND regexp_replace(c.phone, '[^0-9]', '', 'g') LIKE '%' || sqlc.narg(q_digits)::text || '%')
    OR (sqlc.narg(identity_bidxs)::text[] IS NOT NULL AND c.identity_bidx = ANY(sqlc.narg(identity_bidxs)::text[]))
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(COALESCE(NULLIF(c.company_title, ''), c.first_name || ' ' || c.last_name)) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND sqlc.arg(sort_desc)::boolean THEN lower(COALESCE(NULLIF(c.company_title, ''), c.first_name || ' ' || c.last_name)) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer_type' AND NOT sqlc.arg(sort_desc)::boolean THEN c.customer_type END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer_type' AND sqlc.arg(sort_desc)::boolean THEN c.customer_type END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'nationality' AND NOT sqlc.arg(sort_desc)::boolean THEN c.nationality END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'nationality' AND sqlc.arg(sort_desc)::boolean THEN c.nationality END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN c.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
  c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCustomers :one
SELECT COUNT(*)::bigint
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(customer_types)::text[] IS NULL OR c.customer_type = ANY(sqlc.narg(customer_types)::text[]))
  AND (sqlc.narg(nationalities)::text[] IS NULL OR c.nationality = ANY(sqlc.narg(nationalities)::text[]))
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(anonymized)::boolean IS NULL OR (c.anonymized_at IS NOT NULL) = sqlc.narg(anonymized)::boolean)
  AND (
    sqlc.narg(consents)::text[] IS NULL
    OR EXISTS (
        SELECT 1 FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
          AND mc.channel = ANY(sqlc.narg(consents)::text[])
    )
    OR ('none' = ANY(sqlc.narg(consents)::text[]) AND NOT EXISTS (
        SELECT 1 FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
    ))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.first_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.last_name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND regexp_replace(c.phone, '[^0-9]', '', 'g') LIKE '%' || sqlc.narg(q_digits)::text || '%')
    OR (sqlc.narg(identity_bidxs)::text[] IS NOT NULL AND c.identity_bidx = ANY(sqlc.narg(identity_bidxs)::text[]))
  );

-- name: ListCustomerUUIDs :many
-- Bulk "all matching" scope (same filters as ListCustomers, no paging).
SELECT c.uuid
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(customer_types)::text[] IS NULL OR c.customer_type = ANY(sqlc.narg(customer_types)::text[]))
  AND (sqlc.narg(nationalities)::text[] IS NULL OR c.nationality = ANY(sqlc.narg(nationalities)::text[]))
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(anonymized)::boolean IS NULL OR (c.anonymized_at IS NOT NULL) = sqlc.narg(anonymized)::boolean)
  AND (
    sqlc.narg(consents)::text[] IS NULL
    OR EXISTS (
        SELECT 1 FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
          AND mc.channel = ANY(sqlc.narg(consents)::text[])
    )
    OR ('none' = ANY(sqlc.narg(consents)::text[]) AND NOT EXISTS (
        SELECT 1 FROM customer_marketing_consents mc
        WHERE mc.organization_id = c.organization_id AND mc.customer_id = c.id AND mc.withdrawn_at IS NULL
    ))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR c.first_name ILIKE '%' || sqlc.narg(q) || '%'
    OR c.last_name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.company_title ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND regexp_replace(c.phone, '[^0-9]', '', 'g') LIKE '%' || sqlc.narg(q_digits)::text || '%')
    OR (sqlc.narg(identity_bidxs)::text[] IS NOT NULL AND c.identity_bidx = ANY(sqlc.narg(identity_bidxs)::text[]))
  )
ORDER BY c.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListCustomerFacets :many
-- Dynamic filter options. facet: customer_type | nationality | tag | consent.
WITH live AS (
    SELECT c.id, c.organization_id, c.customer_type, c.nationality, c.tags
    FROM customers c
    WHERE c.organization_id = sqlc.arg(organization_id)
      AND c.deleted_at IS NULL
),
open_consents AS (
    SELECT mc.customer_id, mc.channel
    FROM customer_marketing_consents mc
    WHERE mc.organization_id = sqlc.arg(organization_id)
      AND mc.withdrawn_at IS NULL
)
SELECT 'customer_type'::text AS facet, l.customer_type::text AS value, COUNT(*)::bigint AS count
FROM live l GROUP BY l.customer_type
UNION ALL
SELECT 'nationality'::text, l.nationality::text, COUNT(*)::bigint
FROM live l WHERE l.nationality IS NOT NULL GROUP BY l.nationality
UNION ALL
SELECT 'tag'::text, t.tag::text, COUNT(*)::bigint
FROM live l CROSS JOIN LATERAL unnest(l.tags) AS t(tag) GROUP BY t.tag
UNION ALL
SELECT 'consent'::text, oc.channel::text, COUNT(DISTINCT oc.customer_id)::bigint
FROM open_consents oc JOIN live l ON l.id = oc.customer_id GROUP BY oc.channel
UNION ALL
SELECT 'consent'::text, 'none'::text, COUNT(*)::bigint
FROM live l WHERE NOT EXISTS (SELECT 1 FROM open_consents oc WHERE oc.customer_id = l.id)
HAVING COUNT(*) > 0
ORDER BY 1, 2;

-- name: GetCustomerSearchDocument :one
SELECT c.id, c.uuid, c.organization_id, c.customer_type, c.first_name, c.last_name, c.company_title,
    c.identity_type, c.identity_hint, c.phone, c.anonymized_at, o.slug AS organization_slug
FROM customers c
JOIN organizations o ON o.id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: ListCustomerSearchDocuments :many
SELECT c.id, c.uuid, c.organization_id, c.customer_type, c.first_name, c.last_name, c.company_title,
    c.identity_type, c.identity_hint, c.phone, c.anonymized_at, o.slug AS organization_slug
FROM customers c
JOIN organizations o ON o.id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
ORDER BY c.id ASC;

-- --- contacts ------------------------------------------------------------------

-- name: ListCustomerContacts :many
SELECT * FROM customer_contacts
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
ORDER BY is_primary DESC, full_name ASC, id ASC;

-- name: GetCustomerContact :one
SELECT * FROM customer_contacts
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid);

-- name: CreateCustomerContact :one
INSERT INTO customer_contacts (organization_id, customer_id, full_name, title, phone, email, is_primary, note)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.arg(full_name), sqlc.arg(title),
    sqlc.arg(phone), sqlc.arg(email), sqlc.arg(is_primary), sqlc.arg(note)
)
RETURNING *;

-- name: UpdateCustomerContact :one
UPDATE customer_contacts SET
    full_name = sqlc.arg(full_name),
    title = sqlc.arg(title),
    phone = sqlc.arg(phone),
    email = sqlc.arg(email),
    is_primary = sqlc.arg(is_primary),
    note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: ClearPrimaryCustomerContact :exec
UPDATE customer_contacts SET is_primary = FALSE
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND is_primary
  AND uuid <> sqlc.arg(keep_uuid);

-- name: DeleteCustomerContact :execrows
DELETE FROM customer_contacts
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid);

-- name: DeleteCustomerContacts :exec
DELETE FROM customer_contacts
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id);

-- --- marketing consents -----------------------------------------------------------

-- name: ListCustomerConsents :many
SELECT
    mc.*,
    ru.uuid AS recorded_by_uuid, ru.name AS recorded_by_name, ru.surname AS recorded_by_surname,
    wu.uuid AS withdrawn_by_uuid, wu.name AS withdrawn_by_name, wu.surname AS withdrawn_by_surname
FROM customer_marketing_consents mc
LEFT JOIN users ru ON ru.id = mc.recorded_by
LEFT JOIN users wu ON wu.id = mc.withdrawn_by
WHERE mc.organization_id = sqlc.arg(organization_id)
  AND mc.customer_id = sqlc.arg(customer_id)
ORDER BY mc.granted_at DESC, mc.id DESC;

-- name: GrantCustomerConsent :one
INSERT INTO customer_marketing_consents (
    organization_id, customer_id, channel, granted_at, source, text_version, note, recorded_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.arg(channel), sqlc.arg(granted_at),
    sqlc.arg(source), sqlc.arg(text_version), sqlc.arg(note), sqlc.narg(recorded_by)
)
RETURNING *;

-- name: WithdrawCustomerConsent :execrows
UPDATE customer_marketing_consents SET
    withdrawn_at = sqlc.arg(withdrawn_at),
    withdrawal_source = sqlc.arg(withdrawal_source),
    withdrawn_by = sqlc.narg(withdrawn_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND channel = sqlc.arg(channel)
  AND withdrawn_at IS NULL;

-- name: WithdrawAllCustomerConsents :exec
UPDATE customer_marketing_consents SET
    withdrawn_at = GREATEST(granted_at, sqlc.arg(withdrawn_at)::timestamptz),
    withdrawal_source = 'other'
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND withdrawn_at IS NULL;

-- --- drivers ----------------------------------------------------------------------

-- name: ListCustomerDrivers :many
SELECT * FROM drivers
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND deleted_at IS NULL
ORDER BY is_customer_self DESC, last_name ASC, first_name ASC, id ASC;

-- name: GetCustomerDriver :one
SELECT * FROM drivers
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetDriverByUUID :one
-- Reservations resolve a driver_uuid of the organization (customer joined).
SELECT d.*, c.uuid AS customer_uuid
FROM drivers d
JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.uuid = sqlc.arg(uuid)
  AND d.deleted_at IS NULL
  AND c.deleted_at IS NULL;

-- name: DriverLicenceTaken :one
SELECT EXISTS (
    SELECT 1 FROM drivers
    WHERE organization_id = sqlc.arg(organization_id)
      AND customer_id = sqlc.arg(customer_id)
      AND licence_bidx = sqlc.arg(licence_bidx)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: CreateDriver :one
INSERT INTO drivers (
    organization_id, customer_id, is_customer_self, first_name, last_name, birth_date, phone, email,
    identity_type, identity_enc, identity_bidx, identity_hint, nationality,
    licence_enc, licence_bidx, licence_hint, licence_class, licence_issued_on, licence_expires_on,
    licence_country, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.arg(is_customer_self), sqlc.arg(first_name),
    sqlc.arg(last_name), sqlc.narg(birth_date), sqlc.arg(phone), sqlc.arg(email),
    sqlc.narg(identity_type), sqlc.arg(identity_enc), sqlc.arg(identity_bidx), sqlc.arg(identity_hint),
    sqlc.narg(nationality), sqlc.arg(licence_enc), sqlc.arg(licence_bidx), sqlc.arg(licence_hint),
    sqlc.arg(licence_class), sqlc.narg(licence_issued_on), sqlc.narg(licence_expires_on),
    sqlc.narg(licence_country), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateDriver :one
UPDATE drivers SET
    is_customer_self = sqlc.arg(is_customer_self),
    first_name = sqlc.arg(first_name),
    last_name = sqlc.arg(last_name),
    birth_date = sqlc.narg(birth_date),
    phone = sqlc.arg(phone),
    email = sqlc.arg(email),
    identity_type = sqlc.narg(identity_type),
    identity_enc = sqlc.arg(identity_enc),
    identity_bidx = sqlc.arg(identity_bidx),
    identity_hint = sqlc.arg(identity_hint),
    nationality = sqlc.narg(nationality),
    licence_enc = sqlc.arg(licence_enc),
    licence_bidx = sqlc.arg(licence_bidx),
    licence_hint = sqlc.arg(licence_hint),
    licence_class = sqlc.arg(licence_class),
    licence_issued_on = sqlc.narg(licence_issued_on),
    licence_expires_on = sqlc.narg(licence_expires_on),
    licence_country = sqlc.narg(licence_country),
    note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: DeleteDriver :execrows
UPDATE drivers SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: AnonymizeCustomerDrivers :exec
-- Data subject request: wipes the drivers of a customer and soft-deletes them.
UPDATE drivers SET
    first_name = '',
    last_name = '',
    birth_date = NULL,
    phone = '',
    email = '',
    identity_type = NULL,
    identity_enc = '',
    identity_bidx = '',
    identity_hint = '',
    nationality = NULL,
    licence_enc = '',
    licence_bidx = '',
    licence_hint = '',
    licence_class = '',
    licence_issued_on = NULL,
    licence_expires_on = NULL,
    licence_country = NULL,
    note = '',
    deleted_at = COALESCE(deleted_at, NOW())
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id);

-- --- documents --------------------------------------------------------------------

-- name: ListCustomerDocuments :many
SELECT cd.*, d.uuid AS driver_uuid, d.first_name AS driver_first_name, d.last_name AS driver_last_name
FROM customer_documents cd
LEFT JOIN drivers d ON d.organization_id = cd.organization_id AND d.id = cd.driver_id
WHERE cd.organization_id = sqlc.arg(organization_id)
  AND cd.customer_id = sqlc.arg(customer_id)
  AND cd.deleted_at IS NULL
ORDER BY cd.created_at DESC, cd.id DESC;

-- name: GetCustomerDocument :one
SELECT cd.*, d.uuid AS driver_uuid, d.first_name AS driver_first_name, d.last_name AS driver_last_name
FROM customer_documents cd
LEFT JOIN drivers d ON d.organization_id = cd.organization_id AND d.id = cd.driver_id
WHERE cd.organization_id = sqlc.arg(organization_id)
  AND cd.customer_id = sqlc.arg(customer_id)
  AND cd.uuid = sqlc.arg(uuid)
  AND cd.deleted_at IS NULL;

-- name: CreateCustomerDocument :one
INSERT INTO customer_documents (organization_id, customer_id, driver_id, doc_type, valid_until, note, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.narg(driver_id), sqlc.arg(doc_type),
    sqlc.narg(valid_until), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING *;

-- name: SetCustomerDocumentFile :one
UPDATE customer_documents SET
    file_object_key = sqlc.arg(file_object_key),
    file_name = sqlc.arg(file_name),
    file_content_type = sqlc.arg(file_content_type),
    file_size_bytes = sqlc.arg(file_size_bytes)
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: DeleteCustomerDocument :execrows
UPDATE customer_documents SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: ListCustomerDocumentKeys :many
-- Every stored file of a customer (live or soft-deleted), for anonymization.
SELECT file_object_key::text
FROM customer_documents
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id)
  AND file_object_key IS NOT NULL;

-- name: PurgeCustomerDocuments :exec
DELETE FROM customer_documents
WHERE organization_id = sqlc.arg(organization_id)
  AND customer_id = sqlc.arg(customer_id);
