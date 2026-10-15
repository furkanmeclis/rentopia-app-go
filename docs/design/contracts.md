# Contract templates, documents and PDFs

Status: **implemented** (M3: TEC-710; acceptance test RT-A18). Documents belong to a reservation and, once one exists, to its rental ([rentals.md](./rentals.md)).

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
- Only `in_person` is accepted in M3. Remote click and e-signature come with TEC-711.
- Recording approvals needs `tenant.contracts.write` or `tenant.rentals.write`.
- The rentals handover reads `ContractState`: required when an active `rental_contract` template exists, and approved when the live document is approved.

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
