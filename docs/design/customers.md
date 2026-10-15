# Customers and drivers (TEC-705)

Spec §11.1 / §16. Module: `backend/internal/modules/customers`, UI:
`frontend/src/features/customers`, routes `/v1/tenant/customers/*`,
pages `/t/{slug}/customers`.

## Data model

| Table | Purpose |
|-------|---------|
| `customers` | individual (first / last name) or corporate (company title); identity TCKN / passport (individual) or VKN / owner's TCKN (corporate), nationality, tax office, phone, e-mail, billing, e-invoice, note, tags, `anonymized_at` |
| `customer_contacts` | contacts of a corporate customer (one primary) |
| `customer_marketing_consents` | one row per grant; withdrawal closes it (`withdrawn_at`, source, user); a new grant opens a new row — full history; channel sms / email / call / whatsapp, source, consent text version |
| `drivers` | drivers of a customer (`is_customer_self` = "customer is also driver"), birth date, identity, licence number / class / issue / expiry date, issuing country (anything but TR = foreign licence) |
| `customer_documents` | identity / licence / tax certificate / … scans in private storage (`organizations/{org}/customers/{customer}/documents/…`) |

Customer records are never shared between organizations (KVKK decision
2021/1303): every row carries `organization_id`, every index starts with it,
children use composite foreign keys, and RT-A01 covers every route.

**Branch scope:** customers are organization-wide. A branch-restricted member
sees and searches every customer of the organization (the counter must find a
returning customer); what they see is limited by permissions instead (masked
values unless `tenant.customers.reveal`).

**Reservations (TEC-706):** a reservation stores `customer_uuid` (contract
party), an invoice addressee `customer_uuid` (may differ, e.g. a corporate
customer paying for an individual) and `driver_uuid` (actual driver).
`usecase.Service.Lookup` / `EligibleDriver` resolve them inside the
organization (another organization's uuid → not found).

## Encryption, blind index, masking

- `crypto.FieldCipher` (AES-256-GCM). Keys are derived from
  `APP_ENCRYPTION_KEY` with HKDF-SHA256 and a purpose label (`pii/enc/v<N>`),
  so they never equal the SecretBox key of the integrations. Ciphertexts are
  `v<N>:base64(nonce|sealed)`; the associated data is
  `<column>|org:<id>`, so a ciphertext copied into another column or another
  organization's row does not decrypt. `AddKey` registers older versions for
  rotation (re-encryption job and an env key for the previous key are open).
- Blind index: HMAC-SHA256 with a per-purpose derived key
  (`pii/bidx/customer.identity`, `driver.identity`, `driver.licence`) over
  `org:<id> \0 <kind>:<normalized value>`. Used for exact-match search
  (`q` = a full TCKN / VKN / passport), import matching and uniqueness
  (one live customer per identity per organization; one licence per
  customer). Partial numbers match nothing. Rotating the master key requires
  recomputing the indexes.
- Stored per value: `*_enc`, `*_bidx`, `*_hint` (the masked form). Plaintext
  identity / licence numbers are never stored or logged. Phone and e-mail stay
  plain in the database (operational contact) but every API read masks them.
- Masking rules live in one place, `internal/platform/pii`: TCKN / VKN
  `123******45`, passport `U******78`, licence `****456`, phone
  `+90 5** *** ** 12`, e-mail `a***@d***.com`.

## Reveal, export, documents, search

- `POST /v1/tenant/customers/{uuid}/reveal` (`tenant.customers.reveal`, reason
  3–500 chars) returns the plaintext of the customer, its contacts and drivers
  with `Cache-Control: no-store` and writes a `customers.revealed` activity
  event with the reason. The UI keeps the values in memory for
  `display_seconds` (120) and never in the query cache.
- Export is masked. `query.include_sensitive=true` additionally needs the
  reveal permission (403 otherwise, checked on the route and again in the job
  scope) and records `customers.sensitive_export_requested`.
- Document files: `tenant.document.download` + `tenant.customers.reveal`;
  every download is an activity event.
- Cmd+K documents: name, masked identity hint and its last two digits, last
  four phone digits. No plaintext identifier is indexed.
- Activity payloads carry field names, masked hints and reasons only.

**Step-up:** not required yet. The step-up policy (TTL, methods) is a
platform setting and there is no organization-level policy (TEC-670); the
reveal / anonymize routes can add `middleware.RequireStepUp` once one exists.

## Data subject rights and retention

`POST /v1/tenant/customers/{uuid}/anonymize` (`tenant.customers.anonymize`,
reason, irreversible): wipes names, identity, contact, billing, note and tags,
deletes contacts, wipes and removes drivers, deletes document files and rows,
withdraws open consents (the history stays as proof of consent). The row
stays for references. Retention periods tied to rentals and invoices (legal
storage obligations) arrive with M3 / M4; until then anonymization is manual.
Import source files (which may contain plaintext identities) follow the import
job retention of the io engine.

## Rental condition check

`model.EligibleForConditions(facts, Conditions{MinDriverAge, MinLicenseYears}, at)`
computes age and licence years on Europe/Istanbul calendar days and returns
reasons (`birth_date_missing`, `under_min_age`, `licence_missing`,
`licence_issue_date_missing`, `under_min_licence_years`, `licence_expired`).
`GET …/drivers/{driverUuid}/eligibility` exposes it.

## Permissions

| Slug | owner / admin / merkez_yoneticisi | sube_yoneticisi | rezervasyon_satis | teslim_iade | muhasebe | member |
|------|---|---|---|---|---|---|
| `tenant.customers.read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `tenant.customers.write` | ✓ | ✓ | ✓ | | | |
| `tenant.customers.reveal` | ✓ | ✓ | ✓ | ✓ | | |
| `tenant.customers.export` | ✓ | ✓ | | | ✓ | |
| `tenant.customers.import` | ✓ | | | | | |
| `tenant.customers.anonymize` | ✓ | | | | | |
