# KABİS rental notifications

Status: **implemented** (M4: TEC-726, acceptance RT-A21). KABİS is the police vehicle rental notification system. Its official connection method is **not verified** (TEC-682), so no endpoint or automation is assumed: the official adapter slot reports "waiting on dependency" and a fake adapter serves development and tests. It builds on rentals ([rentals.md](./rentals.md): actual handover / return), inspections ([inspections.md](./inspections.md)), customers ([customers.md](./customers.md): sealed identity / licence numbers), the integration job runner ([integration-jobs.md](./integration-jobs.md)) and the per-organization encrypted accounts ([payments.md](./payments.md) §2).

Code:

| Area | Package |
|------|---------|
| Provider interface, official slot, fake | `backend/internal/platform/kabis`, `…/kabis/kabisfake` |
| Payload builder, recording hook, runner operation, retry, resolutions, release sweep | `backend/internal/modules/kabis/usecase` |
| HTTP, meta, routes | `backend/internal/modules/kabis/handler`, `…/kabis/routes.go` |
| Rentals hook (`KabisRecorder`) | `backend/internal/modules/rentals/usecase` (handover, return, swap, driver change) |
| Panel | `frontend/src/features/kabis` (screen, detail, resolve dialog, rental chip, nav badge) |

## 1. Rules

- A notification is built **only from an actual event**: the handover (`rental_start`) and the return (`rental_end`) recorded on the rental, plus a `correction` when the person / vehicle data of a reported start changes. A reservation, its confirmation or a rental in preparation is never reported.
- The payload holds the actual time and place (branch, service point, city, address), the vehicle (plate, brand / model), the renter (individual: names, TCKN / passport, nationality; corporate: title, VKN) and every driver (names, identity number, nationality, birth date, licence number / class / country) from the current customer and driver records.
- Required fields are validated; anything missing **blocks** the job (`blocked`) with the field paths (`drivers.0.identity_number`, `place.city`, `vehicle.plate`, `renter.identity_number`, …). Nothing is guessed and nothing is sent.
- Only `succeeded`, `reported_manually` and `reconciled` mean the authority has the record (`official = true` in the API). Every other state is a local record, and the UI wording says so ("Gönderim bekliyor", "Resmi bildirim değil").

## 2. Tables (migrations 000099 – 000100)

| Table / change | Notes |
|-------|-------|
| `kabis_notifications` | One row per rental, event and `sequence` (`uq_kabis_notifications_rental_event`: start and end are sequence 1, corrections count up). The event branch (`branch_id`) and the rental's own branch (`rental_branch_id`, the planned pick-up), `cause` (handover, return, drivers_changed, vehicle_swap), `corrects_id`, the integration job (`integration_job_id`, unique), `occurred_at` (the actual time), `plate`, `payload_enc` (sealed JSON), `payload_hash` (SHA-256 of the reported facts), `summary` (masked facts), `missing_fields`, `hold_reason`, `last_verified_at`, the staff resolution (`resolution`, reason, reference, actor, time) and its attachment (key, name, type, size). Tenant rules: organization first in every index, composite FKs, `tenant_tables.txt`. |
| `integration_jobs.status` | + `blocked`, `waiting_dependency`, `reported_manually`, `reconciled` (full list rewritten; see [integration-jobs.md](./integration-jobs.md) §3). `idx_integration_jobs_waiting` serves the release sweep. |
| `integration_accounts` | Kind `kabis`; providers `kabis` (official slot) and `kabis_fake` (full lists rewritten after 000093). |
| 000100 | Permissions (§7). |

The down migration drops the notifications, deletes the `kabis.*` jobs and the KABİS accounts, rewrites other held / resolved jobs (none exist) and restores the old CHECKs (stated in the file).

**Sealing.** `payload_enc` uses the field cipher purpose `kabis.payload` (its own HKDF key) with associated data `kabis_notifications.payload|org:<id>|notification:<uuid>`, so a copied ciphertext opens nowhere else. The builder opens customer / driver values through `customersusecase.OpenSensitive` (same column AAD as the customers module); plaintext numbers are never logged, never in `summary`, `integration_jobs.payload`, attempt summaries or API answers (the summary holds `pii.MaskIdentity` hints).

## 3. Recording (hooks)

`rentals usecase.KabisRecorder.RecordRentalEvent(ctx, q, org, rental, cause)` runs **inside the rental's transaction** (the engine hook of handover / return / swap, the drivers-changed transaction) with the transaction-bound queries, so the notification and its job commit or roll back with the event. It returns an after-commit step: the live list event and the first runner step (`Runner.Kick`: the worker queue, or one inline step). The job row is created `pending` with `next_attempt_at = now`, so the minute sweep sends it even when the process stops right after the commit (outbox).

- `handover` → `rental_start`, `return` → `rental_end`: created once (a repeated call finds sequence 1 and creates nothing).
- `drivers_changed` / `vehicle_swap` after the handover: the facts are built again; the same `payload_hash` → nothing. If the latest start-side notification has sent nothing yet (blocked, waiting, or pending without an attempt and not claimed by a worker), it takes the new data in place; otherwise a `correction` (sequence n+1, `corrects` = the previous one) is created. Corrections amend the start (persons, vehicle); a change after the return is not re-reported (§9).
- The mobile handover / return forms ([inspections.md](./inspections.md)) go through the same rentals use cases, so both paths report the same way, exactly once.

**Routing** (at creation and on every re-validation): the organization's active `kabis` account → its adapter. No account → `waiting_dependency` (`hold_reason = no_account`, provider `kabis`); the official slot (`Ready()` = `ErrDependencyPending`) → `adapter_unverified`; an adapter missing in this environment (a `kabis_fake` account in production) → `adapter_unavailable`. Missing data wins over the route (`blocked`).

## 4. Sending (runner operation)

One operation per kind (`kabis.rental_start | rental_end | correction`) and ready provider:

| Call | Answer | Outcome |
|------|--------|---------|
| Submit (`Report`) | receipt | `succeeded` + the authority's reference (`provider_ref`, "KABİS referansı") |
| | `ErrUnavailable` (nothing left: connection refused) | `failed`, retryable → `pending` with backoff (visible, with the error) |
| | `ErrRejected` | `failed` (definitive) |
| | anything else (timeout after sending) | `uncertain` → `needs_verification` |
| CheckStatus (`Lookup` by our reference = the notification uuid) | found | `succeeded` (reference stored) |
| | not found | `not_found` → submit again (the only resend) |
| | error | `uncertain` → stays `needs_verification` |

`max_attempts` is 8. The rule of the runner holds: an unknown result is never sent again blindly; only a confirmed "not filed" leads to a second report. The runner listener stamps `last_verified_at` on every status check and publishes the live list.

## 5. Staff actions

- **Retry** (`tenant.kabis.retry`, audited `kabis.retried` with the result):
  - `blocked` / `waiting_dependency`: re-validates (payload from the current data, route again). Still missing → `409 KABIS_DATA_MISSING` (details name the fields); still no adapter → `409 KABIS_DEPENDENCY_PENDING`. Nothing is sent in either case.
  - `pending` / `failed` / `needs_verification` / `submitted`: the runner's retry (status check first; `already_done`, `in_progress`, `resubmitted`; unknown → `409 KABIS_UNVERIFIED`, nothing sent).
- **Mark manual** ("Manuel bildirildi", `tenant.kabis.reconcile`): the notification was filed through the official manual channel (TEC-682 procedure: outage, no adapter, incomplete data filed by hand). From blocked, waiting_dependency, pending, failed, needs_verification.
- **Reconcile** ("Uzlaştır", `tenant.kabis.reconcile`): a person confirmed with the authority that the notification was received. From needs_verification, submitted, failed.
- Both need a reason (3–500) and the official reference (≤ 120); an attachment (PDF / JPEG / PNG / WebP, 10 MiB, sniffed) is optional and stored privately under `organizations/{org}/kabis/{notification}/{file}.{ext}`. The job ends (`completed_at`), is never sent afterwards, and the activity log records `kabis.reported_manually` / `kabis.reconciled` with reason and reference. Attachments are streamed by the API after an `OrganizationOwnsKey` check (`kabis.attachment_downloaded`).
- **Release sweep**: the minute integration sweep (worker) first releases waiting notifications whose organization now has a working adapter (re-validate + route + first step), then runs the due runner steps.

## 6. Settings

The KABİS account is an integration account (Ayarlar → Entegrasyonlar, `tenant.integrations.manage`): enabled = the account is active, adapter = its provider, credentials sealed by the integrations vault (purpose `integration.credentials`), masked hints only.

| Provider | Fields | Notes |
|----------|--------|-------|
| `kabis` (official) | `username`, `password` | Always registered. `Check` / `Ready` answer `ErrDependencyPending`; jobs wait. Fields to be confirmed in TEC-682. |
| `kabis_fake` | `username`, `password`, `scenario` | Outside production only. `scenario`: empty / `ok`, `outage` (nothing sent), `timeout` (filed, no receipt), `reject`. Tests script failures with `FailNextReport` / `FailNextLookup`. |

The KABİS screen shows the connection state (none / official not verified / test adapter) with a link to the integrations page.

## 7. Permissions and scope

| Slug | owner, admin, merkez_yoneticisi | sube_yoneticisi | teslim_iade, rezervasyon_satis, member |
|------|-----|-----|-----|
| `tenant.kabis.read` (screen, detail, rental chip, attachment) | ✓ | ✓ | ✓ |
| `tenant.kabis.retry` | ✓ | ✓ | |
| `tenant.kabis.reconcile` (mark manual, reconcile) | ✓ | | |

Branch scope: a notification is visible when its event branch or its rental's branch (planned pick-up) is in the member's scope; otherwise `404`. List, facets and every action apply it. The operations overview's failed-jobs card stays organization-wide (as for every integration job) and links KABİS rows to the KABİS screen.

## 8. API

```text
GET  /v1/tenant/kabis-notifications/meta | /facets     (facets.needs_action = nav badge)
GET  /v1/tenant/kabis-notifications                     q (plate, rental reference), status, needs_action, event, rental, from, to
GET  /v1/tenant/kabis-notifications/{uuid}              summary (masked), resolution, actions, attempt_log
GET  /v1/tenant/kabis-notifications/{uuid}/attachment
POST /v1/tenant/kabis-notifications/{uuid}/retry
POST /v1/tenant/kabis-notifications/{uuid}/mark-manual | /reconcile   multipart (reason, reference, file?) or JSON
```

Live list: `tenant.kabis_notifications` (read permission `tenant.kabis.read`), published on creation, every runner state change, retry and resolution.

## 9. Panel

- **KABİS bildirimleri** (`/t/{slug}/kabis`, nav item with the needs-action badge and a per-state info popover): an "only confirmed records are official" note, the connection card, a DataTable with status (blocked, waiting, needs verification, failed, pending, submitted, succeeded, reported manually, reconciled; with counts) and event facets, date range, the official / not official badge next to every status, the first missing field or the hold reason under it, and the row actions (open, retry, mark manual, reconcile).
- **Detail** (`/kabis/{uuid}`): status explanation, missing fields, state (attempts, next attempt, last verification, KABİS reference, adapter, cause, corrected notification, last error), the reported facts, the people (DataTable of masked hints), the staff resolution with the attachment download, and the attempt history (`IntegrationAttemptsTable`, shared with the integration job drawer).
- **Rental detail**: a chip for the start (its latest correction) and the end, linking to the screen filtered to the rental.
- Integration job labels (statuses, `kabis.*` kinds) are shared by the integrations page and the operations overview.

## 10. Tests

`backend/internal/modules/kabis/integration_test.go` (real Postgres, the reservation → rental → handover chain, the fake on the runner without a queue):

- `TestRT_A21_HandoverCreatesOneStartJob`: nothing for the reservation and the rental in preparation; one start job after the handover whose payload has the actual time, place, plate, renter and driver identity / licence; the summary never holds the identity number; the hook again creates nothing; the return adds one end job.
- `TestRT_A21_ReservationAloneCreatesNone`: a cancelled rental never reports.
- `TestRT_A21_MissingIdentityBlocks`: blocked with `drivers.0.identity_number`, nothing sent; retry refuses while missing, sends after the data is completed.
- `TestRT_A21_OutageKeepsPendingRetryable`: a connection loss keeps the job pending (visible in the filter, not official); retry sends it.
- `TestRT_A21_UncertainChecksStatusFirst`: timeout → needs_verification; the sweep's unclear check and an unclear retry send nothing; a found record ends succeeded with one report; a confirmed "not found" is sent exactly once more.
- `TestRT_A21_ManualReconcile`: validation, reconcile (official, final, never sent), mark manual of a waiting notification with a PDF attachment, attachment isolation.
- `TestRT_A21_WaitingDependency`: no account and the official slot keep it waiting (`409` on retry); a working adapter releases it in the sweep.
- `TestKabisCorrections`: unchanged drivers → nothing; a new driver → a correction of the reported start; a vehicle swap → another; an unsent start is updated in place.
- `TestRT_A21_BranchScope`, `TestRT_A21_CrossOrganization`.

`routes_test.go`: permission gates, nil resolver, live resource, providers per environment. RT-A01 (`isolation_test.go`) seeds a notification (with a manual resolution and attachment) for both organizations and covers every route; the list is probed with the other organization's rental.

## 11. Open items

- TEC-682: verify the official method, the supported events, required fields, correction / query operations and the test environment; then implement `kabis.Provider` for the official slot (`Ready` → nil) and adjust the required fields. Until then organizations report manually and mark the notifications.
- A change of the person / vehicle data after the return is not re-reported (only the start chain is corrected); the procedure for corrections after the end comes with TEC-682.
- The operations overview lists blocked / waiting KABİS jobs only on the KABİS screen (its failed-jobs card shows failed and needs_verification, organization-wide).
- Automatic re-validation when a customer / driver record is completed (today: retry, or the release sweep for waiting ones).
