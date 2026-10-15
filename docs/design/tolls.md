# HGS / OGS toll passes

Status: **implemented** (M4: TEC-725, acceptance RT-A20). The provider is not chosen yet (TEC-683), so the module is provider-agnostic: a development / test provider (`hgs_fake`) and a file import fallback. It builds on rentals ([rentals.md](./rentals.md): `rental_vehicles`, actual handover / return, `rental_charges`), the integration job runner ([integration-jobs.md](./integration-jobs.md)) and the per-organization encrypted provider accounts ([payments.md](./payments.md) §2).

Code:

| Area | Package |
|------|---------|
| Provider interface, fake | `backend/internal/platform/tolls`, `…/tolls/tollfake` |
| Use cases (ingest, matching, charges, review, tags, settings, fetch) | `backend/internal/modules/tolls/usecase` |
| HTTP, meta, routes | `backend/internal/modules/tolls/handler`, `…/tolls/routes.go` |
| io engine (export, file import) | `backend/internal/modules/tolls/adapters` |
| Hourly schedule | `backend/internal/queue/tolls.go` (`app:tolls:fetch`) |
| Panel | `frontend/src/features/tolls` (list, review queue, pass detail, vehicle tab, rental section, nav badge) |

## 1. Tables (migrations 000400 – 000401; renumbered at merge)

| Table | Notes |
|-------|-------|
| `vehicle_toll_tags` | Vehicle ↔ tag (HGS sticker / OGS device): provider code, network `hgs/ogs`, tag number (normalized `tag_key`), `valid_from` / `valid_to`. An exclusion constraint keeps one tag number on one vehicle at any moment; a vehicle may carry several tags over time. |
| `toll_passes` | One provider transaction: `(organization_id, provider, external_id)` unique. Time, plate (`plate_key`), tag, station / exit station, amount (NUMERIC 14,2, `pkg/money`), currency, source `api / file`, account, `first_fetched_at` / `last_fetched_at`, status, `review_reason`, vehicle, rental, `match_method`, `manual`, the live `charge_id` / `fee_charge_id`, `revision`, non-sensitive `raw`. |
| `toll_pass_revisions` | A pass that arrived again with changed facts: changed fields, previous / current facts, amount before / after, source. |
| `toll_pass_events` | Pass timeline: received, revised, matched, review, assigned, unassigned, ignored, rematched, charge_created, charge_reversed; manual actions carry the reason and actor. |
| `toll_settings` | Per organization: processing fee, automatic fetch, look-back days. |
| `rental_charges` | New kinds `toll`, `toll_fee`; source `toll`; `toll_pass_id`; `reversal_of_id` (a reversal has a non-positive amount; `uq_rental_charges_reversal` allows one reversal per charge). |
| `integration_accounts` | Kind `toll`, provider `hgs_fake`. |

Every table follows the tenant rules (organization first in every index, composite FKs, `tenant_tables.txt`). The down migration drops the toll tables, deletes every toll / toll_fee charge (reversals included), the toll accounts and their jobs, and restores the old CHECKs (stated in the file). 000401 seeds the permissions (§6).

## 2. Statuses

| Status | Meaning |
|--------|---------|
| `needs_review` | Automatic matching was ambiguous; `review_reason` says why. |
| `unmatched` | A person removed the assignment (`unassigned`). |
| `matched` | On a rental, nothing to charge (zero amount). |
| `charged` | On a rental with a live toll charge (and a fee line when a fee is set). |
| `ignored` | A person decided it is not billed. |

The review queue is `unmatched + needs_review`. Missing data is always a reason, never "no debt": the rental section and the queue say so.

## 3. Matching

1. Vehicle: the plate (`plate_key`, live vehicles first) or the tag valid at the pass time. Plate and tag naming different vehicles → `vehicle_conflict`; none → `vehicle_unknown`.
2. Rental: the `rental_vehicles` legs of that vehicle whose **actual** `out_at → in_at` contains the pass time (an open leg runs until now; cancelled rentals are skipped). A swap is handled naturally: each leg belongs to its vehicle.
3. Ambiguity → review: a transfer of the vehicle at that time (`in_transfer`, even if a leg matches), several rentals (`overlap`), a rental planned around the time without a recorded handover (`missing_actual_times`), planned around it but outside its actual handover → return (`outside_actual_window`), nothing (`no_rental`).

One rental found → charge (§4). Matching runs on arrival, on every revision, when a tag is added / changed (passes of that tag), hourly for review passes of the last 60 days (`RematchDue`, not for passes a person decided on), and on a person's "rematch".

## 4. Charges, revisions, reversals

`settle` is the single place that changes charges; it runs inside a transaction holding the pass row `FOR UPDATE`, so concurrent arrivals of the same transaction end with one live charge.

- Toll charges are provider facts: recorded `approved` (system), `source = toll`, with the pass facts in `calculation`. The rentals API cannot approve, waive or override them (they are not proposals).
- The processing fee (`toll_settings.processing_fee`, applied at charge time) is its own `toll_fee` line next to each charged toll, never part of the rent.
- A charge is never edited: when the amount, the currency or the rental changes, the live charge gets a reversal row (negative amount, `reversal_of_id`) and a new charge is created. Reassignment reverses on the old rental and charges the new one; unassign / ignore reverse. Every charge and reversal writes a `rental_events` entry (`toll_charged` / `toll_reversed`) and a pass event.
- The same pass again with the same facts only moves `last_fetched_at`. Changed facts: a revision row, then settle (an amount change → revision + reversal + new charge).

### Ledger hook

The finance ledger (TEC-717) is built in parallel; no ledger table is created here. `model.LedgerPoster.PostTollCharge(ctx, tx, LedgerEntry)` is called for every toll / fee charge and every reversal **inside the charge transaction** (`tx` is the pgx transaction), with the organization, rental, customer, charge, kind, signed amount, currency, the reversed charge and the pass. `usecase.Service.SetLedger` plugs the implementation; until then `model.NoLedger` is used.

## 5. Import paths

### Provider fetch (integration runner)

`tolls.Provider` = `integrations.Adapter` + `FetchPasses(ctx, account, {From, To, Tags, Plates})`. Each provider registers the runner operation `toll.fetch`:

- Submit: tags carried in the window (`ListTollTagNumbersForFetch`) → fetch → ingest each pass. A provider error is a retryable failure (fetching is read only), `tolls.ErrRejected` (bad credentials) is final; an ingest error is retryable. Response summary: received / created / revised / unchanged / invalid.
- CheckStatus answers `not_found`: a fetch leaves nothing at the provider, so the runner may fetch again.
- Hourly (`app:tolls:fetch`, cron `17 * * * *`): one job per active toll account and hour (idempotency key `org:<id>:toll.fetch:<account>:slot:<yyyymmddhh>`), window = look-back days. "Şimdi sorgula" (`POST /v1/tenant/toll-passes/fetch`) queues one per minute. Failed / needs-verification jobs appear in the operations overview automatically (any kind).
- Accounts are connected in the existing integrations page (credentials sealed by the integrations Vault). `hgs_fake` is registered only when `APP_ENV != production`; it generates a deterministic 1–2 passes per tag and day (stable transaction ids) or answers scripted passes / failures in tests.

### File import (io engine, `tenant.toll_passes`)

CSV / XLSX / TSV / JSON, by a member with `tenant.tolls.import`. Column mapping (headers are suggested from the tr / en labels):

| Field | Required | Format |
|-------|----------|--------|
| `external_id` | yes | provider transaction id |
| `passed_at` | yes | RFC 3339, `2006-01-02 15:04[:05]`, `02.01.2006 15:04[:05]` (Istanbul time without a zone) |
| `plate` / `tag_number` | one of them | |
| `station` | yes | station or route entry |
| `exit_station` | no | route exit |
| `amount` | yes | `47.00`, `47,00`, `1.234,56` |
| `currency` | no | default TRY |
| `provider` | no | default `hgs` |

The preview validates every row through the adapter (`RowPreviewer`): each invalid row is reported with its number and field names; valid rows show `create` / `update` with the changed fields. Rollback: a created pass has its charges reversed and is removed; a revision is answered by a revision back to the previous facts.

## 6. Permissions and scope

| Slug | owner, admin, merkez_yoneticisi, muhasebe | sube_yoneticisi | rezervasyon_satis, teslim_iade, member |
|------|-----|-----|-----|
| `tenant.tolls.read` | ✓ | ✓ | ✓ |
| `tenant.tolls.export` | ✓ | ✓ | |
| `tenant.tolls.import` (file import, fetch now) | ✓ | | |
| `tenant.tolls.review` (assign, unassign, ignore, rematch, candidates) | ✓ | ✓ | |

Vehicle tags need `tenant.vehicles.write` to change, the processing fee `tenant.settings.write`.

Branch scope: a pass is visible when its vehicle's home or current branch, or its rental's planned pick-up / return or actual return branch, is in the member's scope; a pass with neither (unknown vehicle) is visible to members of every branch only. Out of scope answers 404; assigning to a rental outside the scope is a validation error on `rental_uuid`. Tags follow the vehicle (home or current branch).

## 7. API

```text
GET  /v1/tenant/toll-passes/meta | /facets            (facets.review = queue count)
GET  /v1/tenant/toll-passes                            q, status, review=1, review_reason, source, provider, vehicle, rental, has_rental, from, to
POST /v1/tenant/toll-passes/export                     io engine
POST /v1/tenant/toll-passes/import  GET …/import/sample
POST /v1/tenant/toll-passes/fetch
GET  /v1/tenant/toll-passes/{uuid}                     revisions, events, charges
GET  /v1/tenant/toll-passes/{uuid}/candidates          rentals of the vehicle ±3 days
POST /v1/tenant/toll-passes/{uuid}/assign | /unassign | /ignore | /rematch   (reason 3-500 chars)
GET|PATCH /v1/tenant/toll-settings
GET|POST /v1/tenant/vehicles/{uuid}/toll-tags   PATCH|DELETE …/{tagUuid}
```

Rental charges (`GET /v1/tenant/rentals/{uuid}`) carry `toll_pass_uuid`, `reversal_of_uuid` and `reversed`. Every mutation is recorded in the activity log (`tolls.*`, `vehicles.toll_tag_*`). Live: `tenant.toll_passes` (read permission `tenant.tolls.read`); charge changes also publish `tenant.rentals`.

## 8. Panel

- **HGS/OGS geçişleri** (`/t/{slug}/tolls`): DataTable with date range, status / review reason / rental / source facets, import and export, "Şimdi sorgula", settings dialog (fee, auto fetch, look-back).
- **HGS inceleme kuyruğu** (`/tolls/review`, nav badge = queue count with the reasons in the info popover): oldest first; assign (candidate rentals DataTable, rental search, mandatory reason), unassign / ignore / rematch with the reason dialog.
- **Pass detail** (`/tolls/{uuid}`): facts, match, charges (reversals), revisions, timeline.
- **Vehicle detail** "HGS / OGS" tab: tags (add, end, delete) and the vehicle's passes.
- **Rental detail** "Ücretler" tab: toll charges apart from the other charges, reversals marked, net total.

## 9. Tests

`backend/internal/modules/tolls/integration_test.go` (real Postgres, the full reservation → rental → handover chain):

- `TestRT_A20_PassDuringRentalChargedOnce`: a pass by plate inside the actual interval → one toll + one fee line; the same pass twice more changes nothing; the ledger hook sees each posting in the transaction; the charge cannot be waived through the rental.
- `TestRT_A20_DuplicateImport`: the same file row imported twice (preview + apply) → one pass, one charge.
- `TestRT_A20_AmountRevision`: amount change → revision row, reversal + new charge, the original row untouched, a second reversal refused by the index; import rollback revises back.
- `TestRT_A20_OutsideRentalGoesToReview`: no rental, unknown vehicle, missing handover, transfer → review with reasons, nothing charged, queue count.
- `TestRT_A20_ManualReassignment`: reason required; reassignment reverses on the old rental and charges the new one; rental and pass events; a repeated pass keeps the manual decision; unassign, ignore, rematch; candidates.
- `TestRT_A20_FileImportRowErrors`: per-row errors naming the fields (amount, time, missing id / plate / station, future time); valid rows stored; sample rows valid.
- `TestRT_A20_BranchScope`: a member of another branch sees only that branch's passes (list, facets, export), gets 404 for the others, cannot assign to a rental outside the scope, cannot read tags of out-of-scope vehicles.
- `TestRT_A20_CrossOrganization`: the same transaction id in two organizations is two passes; another organization's pass / rental / vehicle answers not found.
- `TestTollFetchJob`: tag uniqueness over time; fetch through the runner (tag match, unknown tag to review); refetch changes nothing; a provider outage leaves a pending job with its error; the hourly schedule makes one job per slot.

`routes_test.go`: permission gates, nil resolver, live resource, fake only outside production. RT-A01 (`isolation_test.go`) seeds a tag, a `hgs_fake` account and fetched passes for both organizations and covers every new route.

## 10. Open items

- Choose the real provider (TEC-683: PTT / bank API, tag coverage, test account) and implement its `tolls.Provider`; until then production organizations use the file import.
- Plug the finance ledger (TEC-717) through `SetLedger`.
- Plate history: matching uses the vehicle's current plate; a re-plated vehicle's old passes need the tag or a manual assignment.
- Merge note: 000400 replaces the `integration_accounts` kind / provider and `rental_charges` kind CHECKs with full lists; parallel migrations that touch the same CHECKs must be merged into one list.
