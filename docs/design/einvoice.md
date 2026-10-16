# E-invoices and e-archive invoices

Status: **implemented** (M4: TEC-722, acceptance RT-A22). The integrator is **not chosen yet** (TEC-685). The official adapter slot reports "waiting on dependency", and a fake adapter serves development and tests. This feature builds on the ledger ([ledger.md](./ledger.md): accruals and `ledger_entry_links`), the integration job runner ([integration-jobs.md](./integration-jobs.md)), the per-organization encrypted accounts ([payments.md](./payments.md) §2), customers ([customers.md](./customers.md): sealed identity numbers) and the customer portal ([customer-portal.md](./customer-portal.md)).

The UBL-TR documents are built, validated and rendered with **go-ubltr** (`github.com/furkanmeclis/go-ubltr`). The module uses its `builder`, `codelist`, `ValidateXSD`, `ValidateSchematronType`, `render` (HTML) and `render/chromium` (PDF, through `platform/pdfrender`). Nothing here builds UBL by hand, and go-ubltr is not vendored.

Code:

| Area | Package |
|------|---------|
| Provider interface, official slot, fake | `backend/internal/platform/einvoice`, `…/einvoice/einvoicefake` |
| UBL-TR build, VAT split, XSD + Schematron, HTML / PDF render | `backend/internal/modules/invoices/ubl` |
| Drafts, finalize, send / cancel operations, returns, settings, taxpayer cache, portal | `backend/internal/modules/invoices/usecase` |
| HTTP, meta, routes, IO export, bulk actions | `backend/internal/modules/invoices/{handler,adapters,routes.go}` |
| Portal invoice list and PDF | `backend/internal/modules/customerportal` (`Invoices` port) |
| Panel | `frontend/src/features/invoices` (list, detail, dialogs, settings, rental / account card, nav badge), the portal reservation page |

## 1. Rules

- **An invoice is a document on top of the ledger.** It links the live accruals it covers (the rent of a rental, extras, the one-way fee, approved charges and re-invoiced tolls) through `ledger_entry_links`. It never posts a debt or a revenue line of its own (RT-A22 + RT-A23). Sending, failing, cancelling or returning an invoice changes no ledger entry and no balance.
- **No double invoicing.** An accrual can have only one live invoice link (`uq_ledger_entry_links_invoiced`, a partial unique index on `(organization_id, entry_id)` where `invoice_id IS NOT NULL AND released_at IS NULL`). A link is released (`released_at`, `release_reason` = `cancelled` / `returned` / `removed`) in three cases: the invoice is cancelled, an IADE invoice returning it is issued, or the item is taken off a draft. The item can then be invoiced again.
- **The UUID (ETTN) exists before the first send.** It is generated when the draft is created, and it is the document's `UUID` and the integrator's idempotency key. The send job's idempotency key is `<org>:einvoice:send:<uuid>` (`ON CONFLICT (provider, idempotency_key)`), so a repeated finalize or retry never creates a second job.
- **An uncertain result asks Status first.** A timeout or an unclassified answer from `Send` makes the runner call `Status` before any second `Send` (integration-jobs.md). Only "unavailable" (nothing reached the integrator) is retried as a send.
- **The document is validated before it is sent.** Finalize builds the XML and runs XSD + Schematron (`efatura` or `earsiv` rules). Any problem keeps the draft unnumbered and is stored and returned per field (`receiver.identity_number`, `lines.2.vat_rate`, `supplier.tax_office`, …). Nothing is sent.
- **Failures are visible.** A failed or held send shows on the invoices screen (the status chip, the "Needs action" switch, the nav badge) and on the operations overview (integration jobs of kind `einvoice.*` link to the invoices page with `needs_action=true`).

## 2. Tables (migrations 000900 – 000901)

| Table / change | Notes |
|-------|-------|
| `invoice_settings` | One row per organization: the e-Fatura and e-Arşiv series (three capital letters or digits, different from each other), the supplier district / city / postal code (the company profile has no such fields), the default note and the taxpayer cache TTL in hours (1–720, default 24). Missing row = the defaults. |
| `invoice_tax_mappings` | `(organization_id, line_kind)` → VAT rate, exemption code and reason. A missing kind uses the Turkish default (§4). |
| `invoice_sequences` | The last number per organization, series and year. Numbers are taken at finalize, inside the finalize transaction. |
| `einvoice_taxpayers` | Taxpayer lookup cache. The key is a keyed blind index of the VKN / TCKN (`invoices.taxpayer` purpose) and never the number. It also holds the e-invoice flag, the alias, the title and `checked_at`. |
| `invoices` | Organization, branch, account, customer, rental, `uuid` (ETTN), `number` (null until finalized), `profile` (`TEMELFATURA`, `TICARIFATURA`, `EARSIVFATURA`), `invoice_type` (`SATIS`, `IADE`), `returns_invoice_id`, issue date, currency, note, the receiver snapshot (type, names, tax office, address, e-mail, e-invoice flag and its source, plus the identity number sealed with AAD `invoices.receiver\|org:<id>\|invoice:<uuid>` and a masked hint), the totals (net, VAT, payable, linked amount, rounding), the status (§3), the hold reason, the validation problems, the integration job, the external reference, `xml_key` / `pdf_key` (private storage), the send / issue / cancel / return timestamps, the cancel reason and job, `last_error` and the idempotency key. The CHECKs enforce three things: a number has the GİB shape (series + year + 9 digits), every non-draft, non-cancelled invoice has a number, and an IADE always names the invoice it returns. Unique indexes cover the number, the idempotency key, the job and one live return per invoice. |
| `invoice_lines` | Position, kind (`rent`, `extra`, `one_way`, `charge`, `toll`), linked entry, name, quantity / unit, the net / VAT / total amounts, the VAT rate and the exemption code and reason. |
| `ledger_entry_links` | Gains `invoice_id` (FK, only with `document_type = 'invoice'`), `amount`, `released_at` and `release_reason`, plus the partial unique index above. `document_ref` still holds the invoice uuid. |
| `integration_accounts` | Kind `einvoice`, providers `einvoice` (official slot) and `einvoice_fake`. The CHECK lists are rewritten in full. |
| 000901 | Permissions (§8). |

The down migrations drop the tables, the new `ledger_entry_links` columns and index, and put back the earlier kind / provider lists. 000901 down removes the permissions and their role grants.

## 3. Statuses and flow

`draft → queued → sent → awaiting_result → succeeded`, with `failed` and `cancelled` at the side.

1. **Draft.** Use "Fatura kes" on a rental or an account, or the bulk "invoice the selected rentals". The draft links the chosen live accruals and splits each gross amount into net + VAT with the tax mapping. The line totals equal the gross sum. A remaining cent goes to `rounding_amount` (±0.01 at most). The rent line is split by the reservation quote snapshot (rental days, extras, one-way fee) when there is one. The receiver comes from the account's invoice customer: identity, names, tax office and address. The taxpayer lookup (§5) chooses the profile: e-Fatura taxpayers get `TEMELFATURA` and the others get `EARSIVFATURA`. Drafts can be edited (date, note, profile, receiver, the line name / VAT / exemption, removing an item). There are no free manual lines: every line comes from an accrual.
2. **Validate** (optional) builds the document without numbering it and stores the problems.
3. **Finalize** takes the next number, builds and validates the XML, stores it privately and queues an `einvoice.send` job, all in one transaction. A document with problems answers `422 INVOICE_DOCUMENT_INVALID` with the problems per field, and the draft stays unnumbered.
4. **Send** (the runner): `Send` → `Status` until the integrator answers succeeded, failed or cancelled. A held job (`waiting_dependency` when there is no account or the official slot is not ready, `blocked` when the adapter is unavailable) leaves the invoice `queued` with a hold reason. The worker sweep `ReleaseWaiting` moves held sends to the account once it is ready.
5. **Retry** (`POST /retry`) checks the current job. A held job is rerouted to the current account. A failed job is resubmitted with the same UUID. A job that is still in flight answers `in_progress`, and a done job answers `already_done`.

## 4. Tax mapping

This is a setting per organization, edited on **Ayarlar → Fatura ayarları**. Every line kind defaults to 20 % VAT (the general KDV rate since 2023). An exemption code must be a valid GİB code (`codelist.ValidExemption` / `ValidExemptionIstisna`). A 0 % rate needs a code and a reason. go-ubltr merges the tax subtotals at document level and drops the exemption there, so `ubl.carryExemptions` puts the exemption back on the merged 0 % subtotal, because the Schematron requires it. Changing a mapping affects new drafts only.

## 5. Taxpayer lookup

`einvoice.Provider.LookupTaxpayer(vkn/tckn)` answers whether the receiver is an e-Fatura taxpayer, with its alias and title. Results are cached in `einvoice_taxpayers` for the configured TTL. A cache hit or a fresh lookup is shown on the invoice as `provider` / `cache`. When the provider is unavailable (no account, official slot pending), the customer record's flag is used (`customer_flag`).

## 6. Cancel and return (IADE)

- A draft, a send that has not started, or a failed send (the integrator confirmed it was not issued) is **cancelled at once**, and its links are released.
- An issued **e-Arşiv** invoice is cancelled through the integrator: an `einvoice.cancel` job runs, and the invoice becomes `cancelled` when the integrator confirms.
- An issued **e-Fatura** cannot be cancelled (`USE_RETURN_INVOICE`). Issue a **return invoice**: `POST /return` creates an IADE draft with the same receiver and lines, a `BillingReference` / `InvoiceDocumentReference` to the returned invoice (`DocumentTypeCode = IADE`) and the profile `TEMELFATURA`, since IADE is not allowed on `TICARIFATURA`. When the IADE is issued, the returned invoice is marked returned and its links are released. The return document is a UBL `Invoice` of type `IADE`, not a `CreditNote`, as the GİB Schematron expects.
- Neither path touches the ledger. Correcting the debt (a reversal or an adjustment) is a separate finance action.

## 7. Provider

`einvoice.Provider` (the brief's EInvoiceProvider) has `Ready`, `Send`, `Status`, `Cancel`, `Download` and `LookupTaxpayer`. The errors are `ErrDependencyPending`, `ErrUnavailable`, `ErrRejected`, `ErrTimeout` and `ErrNotCancellable`. The states are `processing`, `succeeded`, `failed` and `cancelled`.

- `einvoice` (official): `Ready()` answers `ErrDependencyPending` until the integrator of TEC-685 is implemented. Its accounts show "waiting on dependency" on the integrations page.
- `einvoice_fake` (outside production): scenarios `ok`, `slow`, `outage`, `timeout` and `reject`. A 10-digit VKN counts as an e-Fatura taxpayer. It records what was sent, so tests can assert idempotency.

The credentials are stored per organization, encrypted, on the integrations page (`integration_accounts`, kind `einvoice`).

## 8. API and permissions

All routes are under `/v1/tenant/invoices`, organization-scoped and branch-scoped. Tenant isolation is covered by RT-A01:

`GET` list (status, profile, invoice_type, branch, account, rental, from, to, needs_action, q, sort) · `GET meta`, `facets`, `items?account|rental`, `uninvoiced-rentals`, `settings` · `PUT settings` · `POST` (draft) · `POST export`, `bulk` (`create_for_rentals`, reversible; `send`) · `GET` / `PATCH {uuid}` · `POST {uuid}/validate`, `finalize`, `retry`, `cancel`, `return` · `GET {uuid}/preview` (HTML), `xml`, `pdf`.

The portal route is `GET /v1/customer/{orgSlug}/reservations/{uuid}/invoices/{invoiceUuid}/pdf`. Renter sessions see the issued invoices of their reservation on the reservation page (`CustomerReservation.invoices`).

| Permission | Owner, admin, merkez yöneticisi, muhasebe | Şube yöneticisi | Rezervasyon / satış, member |
|------|------|------|------|
| `tenant.invoices.read` | ✓ | ✓ | ✓ |
| `tenant.invoices.write` | ✓ | ✓ | |
| `tenant.invoices.cancel` | ✓ | | |
| `tenant.invoices.export` | ✓ | ✓ | |
| `tenant.invoices.settings` | ✓ | | |
| `tenant.invoices.bulk.create_for_rentals`, `…bulk.send` | ✓ | ✓ | |

The engines are wired in: activity (`invoices.created`, `updated`, `validated`, `finalized`, `retried`, `cancelled`, `return_created`, `downloaded`, `settings_updated`), realtime (live list `tenant.invoices`), IO export, bulk, and the nav badge (failed and held sends).

## 9. Open items

- **TEC-685:** implement the chosen private integrator in the official slot. That covers sending, status, cancel, downloading the signed document and the taxpayer list.
- **Number ownership:** numbers are taken locally at finalize. If the chosen integrator assigns numbers itself, the sequence moves to it. A finalized invoice that is later cancelled before sending leaves a gap in the local sequence.
- **Partial returns:** an IADE returns the whole invoice. Returning only some lines is not supported yet.
