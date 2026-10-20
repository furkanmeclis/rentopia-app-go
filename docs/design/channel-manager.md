# Channel manager core, the Yolcu360 and Obilet adapters

Status: **implemented** (M5: TEC-736, acceptance RT-A25; Obilet TEC-737, acceptance RT-A26, plugs into the same core). Builds on reservations ([reservations.md](./reservations.md): quotes, the state machine, `collecting_party`, commission and `external_reference` per channel), the availability engine ([availability-engine.md](./availability-engine.md): channel capacity rules) and the per-organization encrypted integration accounts ([payments.md](./payments.md) §2).

Code:

| Area | Package |
|------|---------|
| Provider interface, Yolcu360 and Obilet adapters, fakes | `backend/internal/platform/channels`, `…/channels/yolcu360`, `…/channels/obilet`, `…/channels/channelfake` |
| Push, pull / processing, mappings, error queue, reconciliation, sweep | `backend/internal/modules/channels/usecase` |
| Reservations / customers port | `backend/internal/modules/channels/adapters` (`Booker`) |
| HTTP | `backend/internal/modules/channels/handler`, `…/channels/routes.go` |
| Panel | `frontend/src/features/channels` (`/t/{slug}/channels`) |

## 1. Provider contract

A channel is an integration account of kind `channel` (credentials sealed by the integrations vault). `channels.Provider`:

- `PushInventory` — a full snapshot of the horizon: per mapped product code, location code and pick-up day the units the channel may sell (after stop-sale and the quota cap), the one-day price, an **offer reference**, plus `valid_until` and the **offline policy** (`close`, or `quota` N). Resending is always safe.
- `PullMessages(cursor)` — booking / modification / cancellation messages; each is a **full snapshot** of the booking with a `version`.
- `Acknowledge` — our result per message (reservation reference, or the refusal).
- `Statement(from, to)` — the channel's own list for the reconciliation report.

Unsupported operations answer `integrations.ErrUnsupported`. One active account per **channel** (`yolcu360` and `yolcu360_fake` count as the same channel; index `uq_integration_accounts_organization_active_channel`), so Yolcu360 and Obilet can both be active.

### Yolcu360

Rentopya needs the supplier (fleet publishing) protocol, not the public Agency API. The adapter implements the contract as documented so far (REST/JSON, `X-Api-Key` + HMAC-SHA256 signature over timestamp, method, path and body; inventory PUT, booking-events GET + ack, statements GET, ping). Hosts default to the sandbox / live hosts in the package and can be overridden per account (`base_url`) during onboarding. Classification: connection refused / DNS → `ErrUnavailable`, 4xx → `ErrRejected`, timeouts and 5xx → unknown (the next push replaces it; pulls are read only).

**To be confirmed in writing with Yolcu360 (TEC-680):** connection direction (supplier or distributor), who generates product codes, model guarantee support, availability format (count / offer / open-closed), where the booking becomes final, retry / timeout / cancellation rules, stop-sale and disconnection behaviour. Until then `yolcu360_fake` (outside production only; `scenario` = ok / outage / reject) is the tested path.

### Obilet

Same interface, no core change: the module only learned a second provider (`routes.go` `Providers`) and the provider list (migration 000121). Credentials: `partner_code`, `api_token` (sealed), optional `base_url`.

What follows Obilet's documented client API (v2): JSON over HTTPS, `Authorization: Basic <api token>`, a device session from `POST /api/client/getsession` cached per account (and per token / host), every request wrapped as `{device-session, date, language: tr-TR, data}`, every answer as `{status, data, message, user-message}`, kebab-case keys, dates in Türkiye local time without an offset, amounts as JSON numbers. An expired session (`InvalidSession`) is renewed once and the call repeated (all operations are snapshots, reads or idempotent).

**Assumed until Obilet hands over the car rental supplier contract** (all in `obilet.go`, constants and wire types only): the operation paths (`/api/carrental/supplier/ping`, `…/inventory/update`, `…/bookings/getchanges`, `…/bookings/acknowledge`, `…/statement`), their field names, the change feed (change id + PNR + version with the full booking), `payment-type` (`Online` → collected by the channel, `PayAtOffice` → at the counter), the page size (200 items per push) and the default hosts. Error classification is the core's (as Yolcu360). Obilet is not reachable from the build environment, so `obilet_fake` (outside production only, same fake implementation as `yolcu360_fake`, simulated PNRs `OBL-…`) is the tested path.

## 2. Tables (migrations 000118 – 000119; 000121 adds the Obilet providers)

| Table | Notes |
|-------|-------|
| `channel_accounts` | Per account: push / pull switches, account-wide stop-sale, horizon (1–90 d), intervals, push validity, offline policy, default collecting party and commission %, sync state (last push / pull, `pushed_valid_until`, cursor, `push_requested_at`, failures, crash-safe `locked_until`). |
| `channel_mappings` | `branch` → location code, `product` (optionally per branch) → product code with priority, stop-sale, quota cap, `extra` → extra code. Internal codes are permanent; several products may share one external code; ACRISS never merges products. |
| `channel_offers` | What each pushed item stood for (product, version, branch, day, price); deterministic reference. |
| `channel_reservations` | One per `(organization, channel, external_ref)` (unique): reservation, customer, applied / last version, codes, gross / commission / net / prepaid, `local_total`, `price_difference`. |
| `channel_messages` | Once per `(organization, account, message id)`; repeats only count `deliveries`. Status `received → applied | duplicate | stale | failed | ignored`, error code, ack state, sealed payload (field cipher purpose `channels.message`), masked summary. |
| `channel_sync_runs` | Sync log: push / pull / reconcile, trigger (event / schedule / manual), counts, error, details. |

## 3. Outbound

The worker's minute integration sweep (`Service.SyncDue`) pushes an account when an event asked for it (a reservation confirmed / changed / terminated — the reservations lifecycle listener — an applied channel message, a mapping or settings change) or its push interval passed, and pulls when the pull interval passed. Per branch and day the engine answers with the **channel's** sellable count (channel capacity rules apply) and the one-day price; among products sharing a code the lowest priority with capacity is offered (never summed: shared vehicles are not counted twice). An item without a price is sent closed. The channel stops selling the pushed capacity after `valid_until` and applies the offline policy.

## 4. Inbound

Each message is stored once, then processed under a transaction advisory lock per external booking:

1. version ≤ applied version → `duplicate` (equal) or `stale` (older): nothing changes;
2. `cancelled` → the reservation is cancelled (capacity released once); without a reservation a tombstone keeps the version, so a late booking is stale;
3. `confirmed` → codes resolved (location by branch mapping; product by the echoed offer, else the single product mapping of the code at the location, else the offer pushed for that day, else `ambiguous_product`); a new booking creates a customer and a quote on the channel, then a reservation confirmed at once with `external_reference`, `collecting_party` and commission; an existing one is changed (dates / places / product / extras, re-priced) and its commission updated.

A refused message goes to the error queue (`unknown_location`, `unknown_product`, `ambiguous_product`, `unknown_extra`, `capacity_unavailable`, `pricing_unavailable`, `reinstate_not_supported`, `cancel_refused`, `change_refused`, `booking_refused`, `customer_failed`, `invalid_message`) and is **not acknowledged**; staff retry it after a fix or dismiss it with a note (the channel is then told it was refused). A reservation created whose link was not stored (crash) is adopted by its external reference on the next attempt.

**Money.** A booking collected by the channel (`collecting_party = channel`) has no prepayment and its balance shows zero: the customer is never sent to a payment, and finance does not accrue it on the customer's account (TEC-721 rule). Gross, commission and net are kept on the channel booking; `price_difference` = channel gross − our total.

## 5. Reconciliation

`GET /channel-accounts/{uuid}/reconciliation?from&to`: the statement (or, without one, the last messages) against the bookings: `matched`, `missing_local`, `missing_channel`, `status_mismatch`, `amount_mismatch`, `commission_mismatch`; totals split what the channel owes (net of bookings it collected) and what the organization owes (commission of bookings collected at the counter).

## 6. API and permissions

```text
GET|PATCH /v1/tenant/channel-accounts[/{uuid}]           read | write
POST      /v1/tenant/channel-accounts/{uuid}/push|pull|simulate   sync
GET       /v1/tenant/channel-accounts/{uuid}/reconciliation       read
GET|POST  /v1/tenant/channel-mappings, /facets, PATCH|DELETE /{uuid}   read | write
GET       /v1/tenant/channel-messages, /facets, /{uuid}; POST /{uuid}/retry|ignore   read | sync
GET       /v1/tenant/channel-sync-runs                     read
```

| Slug | owner, admin, merkez_yoneticisi | rezervasyon_satis | sube_yoneticisi, muhasebe |
|------|-----|-----|-----|
| `tenant.channels.read` | ✓ | ✓ | ✓ |
| `tenant.channels.write` | ✓ | | |
| `tenant.channels.sync` | ✓ | ✓ | |

Writes and sync act for the whole organization (members limited to some branches get 403). The channel account itself is `tenant.integrations.manage`. Live lists: `tenant.channel_accounts`, `tenant.channel_mappings`, `tenant.channel_messages`.

## 7. Tests

`backend/internal/modules/channels/integration_test.go` (Postgres, reservations + customers behind the Booker, the fake): `TestRT_A25_BookingModificationCancellationWithRepeats` (repeats and out-of-order versions → one reservation; capacity, price, commission / net, no payment, acknowledgements), cancellation before booking, error queue retry / dismiss, no capacity, push (quota, stop-sale, offline policy, failure state), shared code resolved by offer, reconciliation, the sweep (event vs schedule), cross-organization. `obilet_test.go` (module): `TestRT_A26_ObiletAvailabilityPriceBookingCancellation` (Obilet next to Yolcu360 in one organization: its own push with availability and price under its codes, a booking → reservation on the `obilet` channel with the PNR as external reference and the channel booking record, acknowledgement, reconciliation, cancellation releasing capacity, Yolcu360 untouched) and one active account per channel. `yolcu360_test.go` covers signing, wire mapping and error classification; `obilet/obilet_test.go` the session envelope and renewal, wire mapping (local times, numbers, payment type), acknowledgement, statement and error classification; `routes_test.go` the permission gates; RT-A01 the routes.

## 8. Open items

- TEC-680: confirm the Yolcu360 supplier contract (§1) and adjust the adapter (paths, fields, availability format, final-booking point); then test in the Yolcu360 sandbox with real credentials.
- TEC-737: obtain Obilet's car rental supplier contract and adjust `obilet.go` (paths, fields, payment types, hosts); then test against Obilet's test host with real credentials. Until then the issue waits on that dependency.
- Inbound webhooks (the provider interface has no push entry yet; pull only).
- A partner ledger account per channel for the net receivable / commission settlement (today the figures live on the channel bookings and the report).
- Customer de-duplication across channel bookings (each booking creates its customer; modifications reuse it).
