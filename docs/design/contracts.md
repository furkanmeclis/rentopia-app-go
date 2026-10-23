# Contract templates, documents and PDFs

Status: **implemented** (M3: TEC-710, remote approval TEC-711; acceptance test RT-A18). Documents belong to a reservation and, once one exists, to its rental ([rentals.md](./rentals.md)).

- Backend code: `backend/internal/modules/contracts`
  - `model`: types and the placeholder catalog
  - `usecase`: templates, render, data snapshot, documents, PDF
  - `handler` and `routes.go`
- PDF runtime: `internal/platform/pdfrender`.
- Queue: `internal/queue/documents.go`.
- Panel: `frontend/src/features/contracts`.

## 1. Tables (migration 000083)

| Table | Notes |
|-------|-------|
| `contract_templates` | One row per **version** with kind `rental_contract`, `handover_form`, `return_form` or `terms`. Status moves draft → active → archived. A partial unique index allows one active version per kind. `content` holds the editor state (Lexical JSON), `body_html` the sanitized HTML, and `placeholders` the tokens used. |
| `documents` | One live document per reservation and kind. `rental_id` is set once the rental exists. Status is draft, approved or void. `current_version_id` is a deferrable FK. |
| `document_versions` | **Immutable**: the `document_versions_immutable` trigger rejects updates to content columns. Each row holds the rendered `html`, `content_hash` (SHA-256 hex of the HTML), the `data_snapshot` JSON, `changed_keys`, the template version, a status (draft → issued → approved / superseded) and PDF fields (`pdf_status`, object key, size, sha256). `uq_document_versions_organization_id_hash` backs the approval FK. |
| `document_approvals` | Each row records the party (customer or staff), method, approver name, note and recorder. It has a **composite FK `(organization_id, version_id, content_hash)`**, so an approval can only point at the exact content it approved. When a newer version supersedes the content, `valid` becomes false and `invalidated_reason` is set. |

Every index starts with `organization_id`. The tables are in `tenant_tables.txt`. The down migration drops all four tables and loses every template, document, version and approval.

## 2. Templates and placeholders

- **Editor.** The panel uses the shared Lexical editor (`components/editor/editor-x.tsx`) with three additions: `initialJson`, `onContentChange({ json, html })`, and an `editorRef.insertText` handle.
- **Placeholder picker.** "Alan ekle" inserts `{{group.key}}` tokens from `GET /v1/tenant/contract-templates/placeholders`. Labels are in tr and en (`contracts.placeholders.*`).
- **Catalog.** `model/placeholders.go` defines 57 keys in 13 groups: company, reservation, renter, invoice_customer, drivers, vehicle, product, dates, points, amounts, extras, policies and document.
  - `drivers.table` and `extras.table` are **HTML placeholders**: server-built tables, inserted as markup.
  - Every other value is HTML-escaped text.
  - Unknown tokens are rejected on save (`unknown_placeholder`), and the preview highlights them.
- **Sanitizing.** HTML is sanitized on save with an explicit bluemonday policy (`usecase/render.go`). The policy allows:
  - headings, paragraphs, lists, tables, inline formatting, links and `style`/`class` for alignment
  - `img` with `data:` URLs only, because PDFs must not load remote resources

  Scripts, iframes, event handlers and remote images are removed.
- **Lifecycle.**
  - Only drafts can be edited (`not_draft`).
  - `activate` (`tenant.contracts.activate`) archives the previous active version of the kind.
  - `new-version` copies an active or archived version into a new draft.
  - A draft can be deleted.
- **Preview.** `POST /v1/tenant/contract-templates/preview { kind, body_html, reservation_uuid? }` renders with sample data, or with a reservation's data. It returns `html`, `unknown` and `empty`. The panel shows the result in an `iframe sandbox=""` via `srcdoc`.

## 3. Documents and immutable versions

```text
POST /v1/tenant/documents { kind, reservation_uuid }   prepare (create or refresh)
POST /v1/tenant/documents/{uuid}/refresh               re-render with current data
GET  /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}
POST …/versions/{v}/issue                             draft → issued
POST …/versions/{v}/approve { content_hash, party, method: in_person, approver_name, note? }
GET  …/versions/{v}/pdf                                streamed by the API (tenant.document.download)
```

**Rendering.** The data snapshot (`usecase/data.go`) is built from the following sources:
- the reservation and its policy snapshot
- the rental: actual times, km and fuel
- drivers and extras
- organization settings: legal name, tax number, address

Formatting is Turkish (`4.500,00 TRY`, `02.01.2006 15:04`).

**Versioning.** A **new version** is created only when the rendered content would change. That means one of these conditions holds:
- A key the template uses changed. Volatile `document.*` keys are ignored.
- The template version changed.

The new version supersedes the previous one. The previous version's approvals are invalidated with a reason that names the new version and the changed keys. The document returns to draft.

Version timeline entries:
- the reservation gets `document_version`, `document_approved` and `terms_approved`
- the rental gets `reapproval_required` and `document_approved`

**Automatic preparation** happens through the rentals Documents port:
- the rental contract when the rental is created
- the handover form at handover
- the return form at return
- a refresh of every live document on a plate change, vehicle swap, reservation change (extension, upgrade) or driver change
- documents become void when the rental is cancelled

**Approval.** The client must send the `content_hash` of the version it showed:
- If the hash differs from the current version's, or the version was superseded, the answer is `409 CONTENT_CHANGED` with `data.current_version_uuid`, and nothing is recorded.
- This route records `in_person` approvals. Remote approvals (link + SMS code) are §7.
- Recording approvals needs `tenant.contracts.write` or `tenant.rentals.write`.
- The rentals handover reads `ContractState`: required when an active `rental_contract` template exists, and approved when the live document is approved. A remote approval (§7) sets the same status, so it satisfies the handover like an in-person one.

## 4. PDF runtime

**Choice.** PDFs are printed by headless Chromium through **go-ubltr `render/chromium`**, which uses chromedp (`pdfrender.Chromium`):
- The browser starts on the first print and is reused.
- It is restarted after a crash.
- Each print has a 60 s timeout.
- The page is A4 with 15 mm margins.
- Network loads are blocked.
- **Compatibility fallback** (`pdfrender/compat.go`). go-ubltr blocks loads with `Network.setBlockedURLs` `urlPatterns`, which needs Chrome 142+. Chromium 141 (Playwright 1194) answers `Invalid parameters (-32602)` and nothing is printed. The first time that happens, the renderer switches to its own chromedp print with the same A4 page and margins, and blocks loads through the Fetch domain instead. `pdfrender_test.go` prints a Turkish page with a remote image when a Chromium is on PATH, and is skipped in CI.

**When it runs.**
- With a queue, rendering runs in the **worker**: task `app:documents:render-pdf` on the exports queue, enqueued after the version is created.
- Without a queue (tests, single process), it renders inline.
- `pdf_status` moves none → pending → ready or failed. On failure, the error is shown on the version.
- A render is requested once (`SetDocumentVersionPDFPending` from none / failed, stamping `pdf_requested_at`) and queued under the Asynq task id `documents:render-pdf:<version uuid>`. A `pending` version older than 15 minutes (`PDFStaleAfter`; its task was lost, e.g. after a restore with an older Redis) is requested again: by the next download, and by the worker sweep `SweepPendingPDFs` (at start and every minute). The sweep re-stamps the version first (one winner) and enqueues under the same task id, so a task that still exists is never doubled (TEC-962; `rentals/pdf_sweep_test.go`).

**Storage.**
- The PDF is stored **privately** at `organizations/{org}/documents/{document}/{version}.pdf` (`storage.DocumentPDFObjectKey`).
- Downloads are streamed by `GET …/pdf` after an `OrganizationOwnsKey` check.
- Each download writes an activity event (`documents.downloaded`).
- The browser never sees an object key or a storage URL.

**Image.** The backend image (`backend/Dockerfile`) adds Alpine `chromium` and `font-dejavu` (Turkish glyphs). It also adds a `/usr/local/bin/chromium` launcher with `--no-sandbox --disable-dev-shm-usage --disable-gpu`, because the container runs as non-root and go-ubltr's `New` takes no flags. Without a Chromium on PATH, `RenderPDF` answers `ErrUnavailable` and the version is marked `failed`.

## 5. RT-A18 (acceptance)

`rentals/integration_test.go: TestRT_A18_ContractReapproval` runs these steps:

1. Rental contract template active → the rental is created → v1 is rendered with the plate and amounts.
2. Handover without an approval is refused, and is allowed only with a waiver and `signature.waive`.
3. v1 is approved with its hash → `ContractState.Approved`.
4. **Vehicle swap**: v2 is created, v1 is superseded, the approval is invalid, and the contract is no longer approved. Approving v2 with v1's hash returns `CONTENT_CHANGED`. Approving the superseded v1 is refused.
5. v2 is approved, then an **extension** (amount change) creates v3 and needs a new approval.
6. A refresh with nothing changed creates no new version.
7. `UPDATE document_versions SET html = …` fails because of the trigger.
8. The PDF of the current version is stored under the private key and streamed back by the API.

## 6. Permissions and panel

| Slug | Grants |
|------|--------|
| `tenant.contracts.read` | templates, documents, versions |
| `tenant.contracts.write` | edit templates, prepare / refresh documents, record approvals |
| `tenant.contracts.activate` | activate a template version |

Reused slug: `tenant.document.download` (PDF).

Panel:
- **Settings → "Sözleşme şablonları":** DataTable with kind and status facets, live updates. The editor page has a details form, the editor with the placeholder picker, and the preview tab.
- **Reservation and rental detail → "Belgeler":**
  - documents DataTable
  - "Belge hazırla" per kind
  - document dialog: versions, sandboxed HTML preview, approvals, issue, record approval, PDF download
  - remote approval links (§7): a "Uzaktan onay" column in the documents table, the row action "Uzaktan onay linki oluştur", and in the document dialog a links DataTable (status, person and masked number, expiry, copy, send by SMS, revoke); the approvals table shows the method (in person / remote SMS) with the verified masked number and an approval PDF download

## 7. Remote approval (TEC-711)

The customer approves the current version of a document from their own phone: a time-limited signed link plus an SMS one-time code.

Code: `usecase/remote.go` (links, public flow), `usecase/approvalpdf.go` (approval page and PDF), `handler/remote.go`; panel `components/remote-approval-section.tsx`; public page `components/remote-approval-page.tsx` at `/d/[token]`.

### 7.1 Tables (migration 000088)

| Change | Notes |
|--------|-------|
| `document_approval_links` (tenant table) | One row per link: document, version and `content_hash` (composite FK `(organization_id, document_version_id, content_hash)`, like approvals), `token_hash`, `token_ciphertext`, signer (`renter` or `driver`, name, customer / driver FKs with `ON DELETE SET NULL (col)`), the number snapshot (`phone_masked`, `phone_hash` = the SMS service's keyed phone hash, `phone_ciphertext`), `status` `pending → approved / expired / revoked / superseded`, `expires_at`, counters (`codes_sent`, `failed_attempts`, `sms_sent_count`), revocation fields. A partial unique index allows **one pending link per document**. |
| `document_approvals` | Method `remote_sms` added. New columns: `driver_id`, `link_id`, `otp_challenge_uuid`, `verified_phone_masked`, `verified_phone_hash`, `ip_address`, `user_agent`, and the approval PDF (`pdf_object_key`, `pdf_size_bytes`, `pdf_sha256`). A CHECK makes a `remote_sms` approval carry the verified number, the link and the challenge. |

**Token.** 32 bytes from `crypto/rand`, base64url (43 characters). Only its SHA-256 (`token_hash`) is a lookup key, through the global index `uq_document_approval_links_token_hash`: the public page knows only the token, as with `payment_links.token`. The token and the number are also sealed with the field cipher under their own purpose `documents.approval_link` (AAD `document_approval_links.<token|phone>|org:<id>|link:<uuid>`), so staff can copy or re-send an open link and the code goes to the number staff chose, even if the customer record changes later.

The down migration deletes the `remote_sms` approvals and drops the links table (it says so in its data-loss note). In-person approvals are kept.

### 7.2 Staff flow

```text
GET  /v1/tenant/documents/{uuid}/signers                                   renter + drivers, masked numbers, sms_available
GET  /v1/tenant/documents/{uuid}/approval-links                            newest first; url for pending links only
POST /v1/tenant/documents/{uuid}/versions/{v}/approval-links               { content_hash, signer_role, driver_uuid?, expires_in_minutes?, send_sms? }
POST /v1/tenant/documents/{uuid}/approval-links/{linkUuid}/send-sms
POST /v1/tenant/documents/{uuid}/approval-links/{linkUuid}/revoke
GET  /v1/tenant/documents/{uuid}/approvals/{approvalUuid}/pdf              tenant.document.download
```

- Permissions reuse the contract slugs: reading needs `tenant.contracts.read`; create / send / revoke need `tenant.contracts.write` or `tenant.rentals.write` (use case, like recording an approval). No new slug.
- A link names the **current** version: `content_hash` must be the hash shown (409 `CONTENT_CHANGED` otherwise); an approved version answers 409. Expiry is 15 minutes to 7 days (default 24 h). A new link revokes the document's open link (`replaced`).
- The person must be on the reservation (renter or a driver) with a valid mobile number (`pii.ValidPhone`).
- **Invalidation.** A new version of the document closes the open link of the old version (`superseded`) in the same transaction that supersedes the version and invalidates its approvals (§3). An in-person approval revokes the open link (`approved_in_person`). A void document reads `revoked` on the public page.

**SMS of the link.** Staff can copy the link, or send it by SMS when the organization has an active SMS account. The message uses the **transactional** class, not informational. It is a notice about the contract the customer is entering (sms.md §2), so it needs no İYS marketing consent and reaches a customer who never opted in to marketing. The log keeps the text with the token replaced (`/d/••••••`); the SMS service now accepts a `RedactedBody` for any class. The full text lives only in the sealed payload until the send is final. A failed SMS does not fail the link creation (`sms_error` in the answer).

### 7.3 Public flow

```text
GET  /v1/public/documents/{token}            state, organization, kind, reference, version, content_hash, html (pending / approved), masked number (pending)
POST /v1/public/documents/{token}/code       sends the code
POST /v1/public/documents/{token}/approve    { code, content_hash }
GET  /v1/public/documents/{token}/pdf        the approval PDF of this link's approval only
```

- `state`: `pending`, `approved`, `expired` (also applied lazily from `expires_at`), `revoked` (also when the document became void), `superseded` (a newer version exists).
- The page renders the stored HTML in an `iframe sandbox=""` via `srcdoc`, the same `HtmlPreview` as the panel. It is mobile-first (checked at 375 px), `noindex`, with `Referrer-Policy: no-referrer`.
- **Code.** `smsusecase.Service.IssueOTP(ctx, org, "contract_approval", "document_version:<uuid>", phone)`: 6 digits, stored HMAC-only, 5 min TTL, 5 attempts, 60 s resend cooldown, codes per number and per organization per hour (sms.md §5). The code is never logged or stored in plain text; activity entries carry the challenge uuid and the masked number only.
- **Approve.** In order:
  1. The link must be pending, with `failed_attempts` below the per-link cap.
  2. `content_hash` must equal the link's hash, or the answer is 409 `CONTENT_CHANGED` without using the code.
  3. `VerifyOTP` runs. Any failure (wrong, expired, locked, none sent) answers one generic 400 `OTP_INVALID` and counts a link failure.
  4. The verified challenge must belong to the link's number (`phone_hash`) and be newer than the link.
  5. One transaction locks the document and the link (`FOR UPDATE`) and checks that the version is still current and the link still pending. A version superseded between page load and verify answers 409 `CONTENT_CHANGED`. Then:
     - the version is approved (conditional on the hash);
     - the `remote_sms` approval is inserted: verified number, person, version and hash, time, IP from `X-Forwarded-For`, user agent;
     - the link becomes `approved` and the document `approved`;
     - the reservation and rental timelines get `document_approved` (or `terms_approved`).
  6. Activity `documents.remote_approved` is written, with the IP and user agent in the event columns.
- **Generic errors.** Unknown or malformed tokens answer 404. Nothing reveals whether a number exists or was reachable: a send failure answers 503 `CODE_UNAVAILABLE`.
- **Limits.** Per-link DB counters hold without Redis: 10 codes per link and 15 failed codes per link. The handler adds Redis windows, which fail open like the other public routes:

  | Route | Per IP | Per token |
  |-------|--------|-----------|
  | page | 60 / min | – |
  | code | 10 / 10 min | 5 / 10 min |
  | approve | 30 / 10 min | 10 / 10 min |
  | pdf | 30 / 10 min | 10 / 10 min |

  The per-token key is a hash prefix, never the token itself.
- **BFF.** `/api/v1/public/documents/*` sends no session token upstream, forwards `User-Agent` only on `code` and `approve`, and streams the PDF as binary.

### 7.4 Statement and approval PDF

The page and every rendered document footer state, in Turkish and English, that **SMS and tablet approval is not a qualified electronic signature** ("SMS ve tablet onayı nitelikli elektronik imza değildir"). The footer statement is part of the hashed HTML of versions rendered from now on. Older versions keep their content; the approval page covers them.

The approval PDF is the approved version's HTML with an **approval page** appended (`break-before: page`). The page lists:
- the document, version and content hash
- the approver and method
- the verified number (masked)
- the time (Europe/Istanbul), IP address and user agent
- the approval id and the statement
- a note when a newer version invalidated the approval

The PDF is rendered on its first download (public or staff) and stored privately under `organizations/{org}/documents/{document}/{version}-approval-{approval}.pdf`. Later downloads are streamed by the API. A token reaches only its own link's approval.

### 7.5 Tests

`rentals/remote_approval_test.go` runs against the DB with the Netgsm fake. The code is read from what the fake received.

| Test | Covers |
|------|--------|
| happy path | signers; link + transactional SMS with the token redacted in the log; page; code; wrong hash → `CONTENT_CHANGED`; approval with the verified number, person, IP and user agent; the code verifies once; activity without the code; `ContractState.Approved` and handover allowed; approval PDF with the approval page and statement, stored after the first render; staff approval PDF |
| wrong code and lock | 5 wrong codes lock the challenge; the right code no longer works; a new code after the cooldown approves; failure counter and activity |
| expired / revoked | expired link (page, code refused, no SMS); out-of-range expiry; revoked link (page, approve refused, revoke / send twice refused); a new link replaces the open one |
| new version | a link open while the rental is extended → approve answers `CONTENT_CHANGED` and the page reads `superseded`; no link for the old version; the next version invalidates a remote approval and the contract needs approval again |
| organization isolation | a token resolves only its own organization; B's staff cannot list, revoke, send or download A's links and approvals; forged and malformed tokens → not found |
| rate limits | resend cooldown; per-link code and failure caps; per-token and per-IP windows of the public routes (miniredis) |

RT-A01 (`httpserver/isolation_test.go`) seeds a revoked link and an in-person approval per organization. It covers the new tenant routes through their path parameters (`approval-links/{linkUuid}`, `approvals/{approvalUuid}`). The public routes are outside `/v1/tenant`, like `/v1/public/pay`; the isolation test above covers them.

### 7.6 Open items

- The click and signature-pad methods and a qualified e-signature (NES) integration are not built.
- Notification templates could send the link automatically; today staff send it.
- The recorded IP is the first `X-Forwarded-For` hop, which the BFF sets. Behind other proxies, configure them to pass the client address.
