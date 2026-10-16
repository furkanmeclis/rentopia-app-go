# Handover / return inspections and evidence

Status: **implemented** (M3: TEC-712 mobile handover, TEC-713 mobile return, TEC-714 photo and evidence storage; acceptance tests RT-A17 and RT-A19). It builds on rentals ([rentals.md](./rentals.md)), contracts ([contracts.md](./contracts.md)) and payments ([payments.md](./payments.md)).

- Backend code: `backend/internal/modules/inspections`
  - `model`: types, inputs, the default template
  - `usecase`: templates, forms, damages, photos (upload, completion, media links), signatures, charges, completion, revision, review
  - `handler` and `routes.go`: HTTP
- Rentals side: `rentals/model/fees.go` (late, km and fuel rules), the `Finalize` hook of `HandoverInput` / `ReturnInput`, charge override, return-check statuses.
- Contracts side: `contractsusecase.ApproveWithSignature` (approval with method `signature`).
- Panel: `frontend/src/features/inspections`, `frontend/src/components/signature`.

## 1. Tables (migration 000089)

| Table | Notes |
|-------|-------|
| `inspection_templates` | Per organization: `photo_slots` `[{key, label, required}]` and `checklist` `[{key, label, required, price}]`. One `active` version (partial unique index). Saving archives the active one and inserts the next version. When an organization has none, the default is created on first use: photos ön, arka, sol, sağ, iç, gösterge; equipment stepne, kriko, reflektör, ilk yardım, yangın tüpü, ruhsat, anahtar. |
| `inspections` | One form per rental, kind (`handover` / `return`) and `version`. Status is draft, finalized or superseded. The form also holds: a template snapshot; the leg it recorded (`rental_vehicle_id`); the facts (time, branch / point, km, fuel, equipment, verification); the customer statement and review time; staff charge overrides; the vehicle status after a return; the signature status; the dispute flag and note; and the review fields. `uq_inspections_live` allows one non-superseded form per rental and kind. |
| `inspection_damages` | A zone of the vehicle diagram (`front_bumper`, `hood`, …, `interior`), an optional marker position, the type, severity and description, plus `is_new` (a return finding) and `disputed`. |
| `inspection_photos` | One piece of evidence: the slot (or `damage` with `damage_id`, or `extra`), `client_ref` (the device queue's idempotency key) and status pending → uploaded → completed. Also the object key, content type and size, the server **SHA-256**, the server **`uploaded_at`** and `uploaded_by`, the inspection version and rental link, and the scan status. Device facts are kept apart: `device_captured_at` (device clock), `exif_taken_at` / `exif_taken_local`, `exif_gps_lat` / `exif_gps_lng`, make and model. |
| `inspection_signatures` | Signer, the PNG (object key, SHA-256, size), `strokes` (`{width, height, strokes: [[[x, y, pressure, t_ms]]]}`), point count, server `signed_at` and `device_signed_at`. One per form version. |
| `document_approvals.signature_id` | The contract approval given with a form's signature points at that signature (§5). |
| `rental_charges` | New columns `inspection_id`, `disputed`, `original_amount`, `override_reason`, `overridden_by`, `overridden_at`. New kind `equipment`. |
| `operation_tasks` | New types `maintenance` and `deposit`. |

All of these follow the usual tenant rules:
- `organization_id` comes first in every index.
- FKs are composite `(organization_id, …)`.
- Every table cascades from the organization.
- The tables are listed in `tenant_tables.txt`.

The down migration states what it loses: every template, form, damage, photo and signature record (the objects stay in storage); the approval links; the charge links and override history; and maintenance and deposit tasks. `equipment` charges become `other`.

**Immutability.** Two triggers enforce it in the database:
- `inspections_immutable` lets a finalized form change only its status (finalized → superseded) and its review fields.
- `inspection_children_immutable` refuses inserting or updating damages, photos and signatures of a form that is not a draft.

Deletes are not blocked, because organization and rental deletion cascades through these tables. The API deletes evidence of drafts only.

## 2. Evidence storage (TEC-714)

```text
POST /v1/tenant/inspections/{uuid}/photos {slot, damage_uuid?, client_ref, content_type, size_bytes, device_captured_at?}
     → {photo, upload: {method: PUT, path: /v1/tenant/inspections/{uuid}/photos/{photo}/file, field: file}}
PUT  …/photos/{photo}/file            multipart bytes → status uploaded
POST …/photos/{photo}/complete        verify + hash → status completed
POST /v1/tenant/inspections/{uuid}/media-links → [{uuid, kind, url: /v1/public/media/inspections/{token}, expires_at}]
```

- **Upload.** The API issues the upload address. It is an API path; the browser prefixes the BFF and never assembles keys or storage hosts.
  - Images only: JPEG, PNG, WebP, HEIC / HEIF, at most 15 MiB.
  - The declared type is checked, and the bytes are sniffed again on upload. The sniffed type must match the announced extension.
  - A repeated `client_ref` answers the same photo, so the device queue can retry safely.
- **Keys.** `organizations/{org}/rentals/{rental}/inspections/{inspection}/photos/{photo}.{ext}` and `…/signatures/{signature}.png` (`storage.InspectionPhotoObjectKey`, `InspectionSignatureObjectKey`). The bucket is the private one.
- **Completion.**
  - It checks that the object exists (`Exists`), then streams it.
  - It computes **SHA-256 on the server** and checks that the size matches the upload.
  - It reads the EXIF block of JPEG / WebP (capture time with offset, or the raw local time; GPS).
  - It records **`uploaded_at` = server time**.
  - EXIF time and GPS go into their own columns and are never mixed with server time. HEIC EXIF is not parsed.
- **Access.** Media links are HMAC-signed tokens (key derived from the JWT secret). A token names the organization, the kind and the record, and expires after **10 minutes**.
  - `GET /v1/public/media/inspections/{token}` streams with `Cache-Control: private` after an `OrganizationOwnsKey` check.
  - Links are issued by a POST, so read responses stay deterministic. RT-A01 snapshots the GET responses.
- **Malware scan (optional).** `usecase.Scanner` (`Scan(ctx, r) (clean, err)`) runs on the bytes at completion and is wired with `SetScanner`. A clamd client fits there. An infected file is deleted and the completion is refused. Without a scanner, `scan_status` is `skipped`. Nothing is wired by default.
- **Finalized records** are immutable (§1). A correction is `POST …/revise {reason}` (`tenant.rentals.close`):
  - The finalized version becomes `superseded`.
  - A new draft version copies the facts, damages and completed photos. The copies point at the same objects and keep their hashes; `copied_from_id` records the source.
  - The customer signs again.

## 3. Handover (TEC-712, RT-A17)

- **Task screen.** "Saha görevleri" (`/t/{slug}/field`) is a DataTable of open handover and return operation tasks.
  - A row opens the wizard of its rental.
  - A QR code of the wizard URL is generated in the browser (`qrcode`).
  - The rental page has "Mobil teslim / Mobil iade" buttons too.
- **Wizard** (`/t/{slug}/field/handover/{rental}`):
  1. Vehicle (plate confirmed, condition) and customer / driver (driver of the rental, identity, licence).
  2. Amounts: total, paid, remaining payment, payment status, deposit amount from the product conditions. This is display only, from the reservation's payment fields. Payments keeps no deposit record, so the deposit shows "taken at the branch".
  3. Start km, fuel slider, equipment checklist.
  4. Damage on the SVG diagram (`VehicleDiagram`): pick a zone, then type, severity, description, and the photo.
  5. Required photos from the template, through the device upload queue.
  6. Customer review: summary, customer statement, "Müşteri formu inceledi".
  7. Signature (`components/signature/SignaturePad`): pointer events with pressure, strokes plus a PNG, chrome from shadcn buttons. When the rental contract still needs approval, the contract version is shown, and the same signature records its approval (§5).
  8. Complete.
- **Completion** (`POST …/complete`):
  - **RT-A17.** Refused with `409 INSPECTION_INCOMPLETE` (data = readiness) while any of these holds: a photo row is not uploaded and completed, a required slot has no completed photo, a damage has no completed photo, or km / fuel / required equipment / verification / customer review is missing. The wizard's "Tamamla" button is disabled on the same readiness, and while the device queue still holds photos.
  - Without a signature, the handover needs `no_signature_reason` and `tenant.signature.waive` (`signature_status = waived`).
  - The rentals **Handover** use case runs with the form's time (default: server now), place, km and fuel. Its `Finalize` hook finalizes the form **inside the same engine transaction**: the leg id is linked and a rental event is written. The rental becomes active; the contract rule (approval or waiver) is unchanged.
  - **KABİS (M4, TEC-726).** The rentals Handover use case records the rental start notification in the same transaction (`KabisRecorder`, [kabis.md](./kabis.md)); the return does the same for the rental end. `usecase.HandoverListener.HandedOver` still runs after the commit and is not used for KABİS.

## 4. Return (TEC-713, RT-A19)

The wizard (`/t/{slug}/field/return/{rental}`) has eight steps: vehicle, readings, damages, photos, charges, review, signature and complete.

**Recording.** The form records the actual time, the place, km, fuel, equipment and photos in the same template slots. The finalized handover record is shown side by side: `inspection.baseline`, the readings with their deltas, the photos slot by slot, and the handover damages ringed on the diagram. New findings are damages with `is_new`, missing equipment, and the customer statement.

**Charges** (`GET …/charges`, persisted at the completion). They are computed only from the reservation's snapshots:

| Charge | Rule |
|--------|------|
| Late return | The rentals rule (`rentalmodel.LateFee`): every started hour after the planned return, beyond the policy grace, × `extra_hour_fee`. The rentals Return use case still proposes it; the form links it. |
| Km overage | Driven km − `included_km_per_day` × planned days, × `extra_km_fee` (product snapshot). |
| Fuel | Expected level − returned level, in percent, × `policies.fuel_fee_per_percent` (new org setting). The expected level is the handover level for `same_to_same` and 100 for `full_to_full`; `prepaid` and `full_to_empty` compute nothing. |
| Missing equipment | Present at the handover, missing now, × the template item `price`. |
| Damage | The amount staff enter per new damage (manual source). |

Staff review the lines. Changing a computed amount needs **`tenant.price.override`**:
- in the draft (`charge_overrides`)
- at the completion
- later through `PATCH /v1/tenant/rentals/{uuid}/charges/{chargeUuid}`

The computed amount stays in `original_amount`.

**Normal completion** (signed, no dispute):
- The rentals **Return** use case records the return. Its `Finalize` hook proposes the charges, finalizes the form and opens the **deposit** task, all in one transaction.
- **CompleteReturn** then sends the vehicle to cleaning, maintenance or preparation through an operation task. Finishing that task makes the vehicle available.

**Exception path.** No signature (`no_signature_reason` → `signature_status = refused`), a disputed form, or a disputed damage:
- The return is still recorded with the dispute flag and the signature status.
- Every charge of the form is stored as **disputed**.
- `review_status = pending` puts the form in the review queue (`GET /v1/tenant/inspections?review_status=pending`, panel "Teslim / iade formları").
- The deposit task opens with a hold note.
- CompleteReturn refuses with `review_pending`.
- `POST …/review {note, vehicle_status}` (`tenant.rentals.close`; an unsigned return also needs `tenant.signature.waive`) resolves the review and finishes the return check. It **never decides a charge**.
- Approving a disputed charge needs **`tenant.damage.approve` and a note**, whatever its kind. Nothing turns disputed damage into a final debt or a card charge automatically; no payment is taken anywhere in this flow.

## 5. Signature and contract approval

`POST …/signature {signer_name, png_base64, strokes, contract?: {document_uuid, version_uuid, content_hash}}`:
- The customer review comes first (`review_first`).
- The signature is stored (§1, private PNG, server SHA-256 and time).
- With `contract`, `contracts.ApproveWithSignature` records a **customer approval with method `signature`**, bound to the version's content hash as usual. `document_approvals.signature_id` names the signature, so nothing is signed twice.
- A version rendered since the contract was shown answers `409 CONTENT_CHANGED`. The signature is then removed, so the customer signs the current text again.
- A signature that approved a contract cannot be replaced (`signature_bound`).

The documents dialog shows such approvals with `signature_uuid` / `signature_inspection_uuid`.

## 6. Offline behaviour (panel)

- **Upload queue** (`useUploadQueue`, `lib/offline-store.ts`). A photo is stored on the device first, in IndexedDB together with its bytes. It is then announced (same `client_ref` on every retry), uploaded and completed.
  - Statuses: queued (on the device), uploading, completing (server check), done and failed. "Yeniden dene" retries failed items.
  - The queue resumes on reload and on the `online` event.
  - When IndexedDB is unavailable (private mode, blocked storage), an in-memory map keeps the wizard working without persistence.
- **Draft fields.** A PATCH that fails with a network error is merged into a device draft (IndexedDB, wrapped in try/catch) and resent when the connection returns. The header shows "Çevrimdışı" / "Gönderilmemiş değişiklik" and the pending upload count.
- **Step memory.** The current step is remembered per form in `localStorage` (try/catch).
- **Completion** needs the server, as does RT-A17's check.

## 7. API and permissions

`/v1/tenant/inspection-templates`:
- list, `active`, `{uuid}` (read)
- POST (close)

`/v1/tenant/inspections`:
- `facets`, list (read)
- POST start / resume (write)
- `{uuid}` GET (read), PATCH (write)
- `charges`, `media-links` (read)
- `damages` and `damages/{damageUuid}`; `photos`, `photos/{photoUuid}`, `/file` and `/complete`; `signature`; `complete` (write)
- `revise`, `review` (close)

`/v1/public/media/inspections/{token}` is public, authorized by the signed token.

`PATCH /v1/tenant/rentals/{uuid}/charges/{chargeUuid}` changes a charge amount.

No new permission slugs were added. Reused slugs:
- `tenant.rentals.read`, `tenant.rentals.write`, `tenant.rentals.close`
- `tenant.price.override`
- `tenant.signature.waive`
- `tenant.damage.approve`

Every route is in `openapi.yaml` and RT-A01 (`isolation_test.go`):
- param keys: `inspection`, `inspectionTemplate`, `inspectionDamage`, `inspectionPhoto`
- a body case for POST start with another organization's rental
- own-record cases: foreign branch, driver, damage and photo in bodies
- a list probe
- an allowlist entry for POST template (no foreign references)

## 8. Acceptance tests

`inspections/integration_test.go` (real Postgres, in-memory storage):

**`TestRT_A17_UnuploadedPhotoBlocksFinalize`** checks these points:
- A PDF is refused, both by its declared type and by its bytes. The upload address is an API path.
- A repeated `client_ref` answers the same photo.
- The server SHA-256 equals the hash of the stored bytes, `uploaded_at` is the server time, EXIF time and GPS are parsed apart, and the object key sits under `organizations/{org}/rentals/{rental}/inspections/…`.
- The completion is refused in each of these cases: a photo announced but not uploaded (`INSPECTION_INCOMPLETE`, missing slot `dashboard`, one incomplete photo); a photo uploaded but not completed; a damage without a photo; no signature.
- The rental stays in preparation while refused.
- The tablet signature records the contract approval with method `signature` and the signature id.
- The completion makes the rental active with the form's km and time, and links the leg.
- The finalized form refuses changes through the API (PATCH, new photo) and through SQL (`UPDATE inspections`, `UPDATE inspection_photos`).
- A media link streams the stored photo; a forged token is refused.
- A revision without a reason is refused. With a reason, v2 is a draft that copies seven photos with the same hashes, and v1 is superseded and unchanged.

**`TestRT_A19_DisputedReturnNeverBecomesDebt`** checks these points:
- The template's jack price is charged when the jack is missing.
- The charges are proposed and disputed: late return (2 h × 150 after a 30 min grace), km overage 150 × 5, fuel 20 % × 10, the jack, and damage 2 000.
- Overriding the km charge without `price.override` is forbidden. With it, the charge stores 500 with `original_amount` 750.
- A completion with no signature and no reason is refused. With a refusal reason, the rental is in `return_check`, the form is in the review queue, and the deposit task carries a hold note. No cleaning task opens.
- CompleteReturn is refused.
- Approving any disputed charge without `damage.approve` is forbidden.
- The review needs `signature.waive`. After it, the rental is returned, a maintenance task is open, and every charge is still proposed.
- The damage approver approves the damage charge only with a note.

**`TestRT_A19_SignedReturnCompletes`** covers the normal path. An on-time, full return computes no charge. The rental ends as returned, with cleaning and deposit tasks open and the baseline photos alongside.

Unit tests:
- `usecase/evidence_test.go`: EXIF in JPEG and WebP, image sniffing including HEIC, media token expiry and forgery, stroke validation
- `rentals/model/fees_test.go`: late, km and fuel rules
- `storage/keys_test.go`: the new keys
