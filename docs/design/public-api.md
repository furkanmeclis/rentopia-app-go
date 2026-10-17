# Public booking API

Status: **implemented** (M5: TEC-728; RT-A27, RT-A28). Requirements §14.3. The branded site (TEC-729), the embeddable widget (TEC-732) and an organization's own frontends or partners all use this one API. It builds on quotes and reservations ([reservations.md](./reservations.md)), the availability engine ([availability-engine.md](./availability-engine.md)), pricing, payments ([payments.md](./payments.md)) and the customer portal's use cases ([customer-portal.md](./customer-portal.md)).

The requirements document writes the paths as `/api/v1/...`. That collides with the BFF namespace, so the Go routes are **`/v1/public/*`**. Callers reach them through the app's same-origin `/api/v1/public/*`.

| Area | Path |
|------|------|
| Use cases | `backend/internal/modules/publicapi/usecase` |
| HTTP (public guard, panel), routes | `backend/internal/modules/publicapi/handler`, `routes.go` |
| Webhook sweep | `backend/internal/queue/public_api.go` (`app:public-api:webhooks`, every minute) |
| BFF passthrough | `frontend/src/lib/server/public-booking.ts` |
| Panel | `frontend/src/features/public-api`, `/t/{slug}/settings/public-api` |
| Tests | `backend/internal/modules/publicapi/publicapi_test.go`, RT-A01 seeds |

## 1. Tables (migrations 000115 – 000116)

| Table | Notes |
|-------|-------|
| `public_api_keys` | `kind` secret \| publishable, `channel` web \| widget \| api, `key_prefix` + `key_last4` for display, `key_hash` (SHA-256, the only global unique index: a key resolves its organization), `publishable_key` (plain, publishable only: it is embedded in pages anyway), `scopes`, `allowed_origins`, `rate_limit_per_minute`, `status`, `expires_at` (rotation grace), `rotated_from_id`, `last_used_at/ip`. |
| `public_api_idempotency` | One row per (organization, key, session, `Idempotency-Key`): the request hash, the state and the answer, **sealed** (purpose `public_api_idempotency`), because the booking answer carries the reservation access token. Kept 24 h. |
| `public_quotes`, `public_reservations` | Which quotes and reservations the API made, with the key and the channel; reservations also keep the SHA-256 of the **reservation access token**. |
| `public_webhook_endpoints`, `public_webhook_deliveries` | The signing secret is sealed (purpose `public_api_webhooks`, aad = organization + endpoint). Deliveries have status `pending → succeeded \| failed`, attempts, `next_attempt_at` (global partial index for the sweep, like `integration_jobs`), the last HTTP code and error. |

Every table is in `tenant_tables.txt`. Composite FKs to quotes, reservations and keys. The down migration drops all six tables (keys stop working).

Permissions (000116): `tenant.public_api.read`, `tenant.public_api.manage`. They are granted to owner, admin and merkez_yoneticisi.

## 2. Credentials and the organization

The organization always comes from the credential, never from the URL or the body.

- **Secret key** (`rtp_sk_` + 256 random bits) is for server-to-server calls, with `Authorization: Bearer`. A request that carries a browser origin (`Origin`, or `X-Forwarded-Origin` from the BFF) is refused with `SECRET_KEY_IN_BROWSER`, so a leaked secret is not usable from a page.
- **Publishable key** (`rtp_pk_`) is for the site and the widget. `POST /v1/public/sessions {publishable_key}` from one of the key's `allowed_origins` returns a **booking session token** (`rtp_ps_…`, 30 min). The token is stateless: a payload of organization, key, session id, origin and expiry, MACed with the field cipher's keyed hash (purpose `public_api.session`). A session only answers its own origin. Removing the origin from the key, or revoking the key, ends its sessions.
- The publishable key is never a bearer, and the long-lived secret never reaches a browser.
- The branded site ([branded-site.md](./branded-site.md)) resolves its organization from the request host and then uses this same session model: the host names a site, the site names its publishable key and origin, and the site server mints sessions for that origin.

**Checks on every call**:
1. the organization is active and inside its access window (`ORGANIZATION_UNAVAILABLE`);
2. the key's **sales channel** is switched on in the organization settings (`CHANNEL_DISABLED`). This is the feature gate for `public_api` / `widget` / `site` until package entitlements exist (TEC-674 plugs into `checkChannel`);
3. the scope of the route (`INSUFFICIENT_SCOPE`).

**Scopes**: `catalog.read`, `quotes.write`, `reservations.write`, `reservations.read`. A new key gets all of them.

**Rotation** issues a successor with the same settings. The old key keeps working for the grace period (default 24 h, 0 = revoked now, at most 168 h). Revocation is immediate.

**Rate limits** use Redis fixed windows and fail open without Redis, like the other public routes:
- 600 / min per IP;
- the key's `rate_limit_per_minute` per credential (per key for secret keys, per browser session for publishable keys);
- per route: quotes 120 / min, bookings 20 / 10 min, change / cancellation requests 10 / h, payment links 20 / 10 min;
- sessions: 30 / min per IP.

Over the limit the answer is `429 RATE_LIMITED` with `Retry-After`.

**CORS**: the API reflects the requesting origin. No cookies are used: auth is a header bearer, and the origin is enforced against the credential after authentication. Preflights answer 204 with the allowed methods and headers. The BFF passes `/api/v1/public/{sessions,locations,vehicle-subgroups,extras,availability,quotes,reservations}` through:
- it forwards the caller's own `Authorization`, `Idempotency-Key`, `X-Reservation-Token` and the origin (as `X-Forwarded-Origin`);
- it never attaches the staff session;
- it skips its same-origin mutation check, which protects cookie sessions only;
- it returns the CORS, rate-limit and replay headers.

## 3. Endpoints

| Route | Scope | Notes |
|-------|-------|-------|
| `POST /v1/public/sessions` | – | publishable key + allowed origin → session |
| `GET /v1/public/locations` | catalog.read | active branches shown online (`show_online`, TEC-730) with online-selectable service points, hours, holidays |
| `GET /v1/public/vehicle-subgroups` | catalog.read | published products shown online (`show_online`, TEC-730), current conditions, API image paths |
| `GET /v1/public/extras` | catalog.read | published extras of the channel (priced in quotes) |
| `GET /v1/public/availability` | catalog.read | `reservations.Search` on the key's channel; `units_left` capped at 10; hidden branches answer `400 pickup_branch_uuid / return_branch_uuid not_found`, hidden subgroups are left out |
| `POST /v1/public/quotes`, `GET …/quotes/{uuid}` | quotes.write | `reservations.CreateQuote`; recorded in `public_quotes`; a hidden subgroup or branch answers `400 … not_found` |
| `POST /v1/public/reservations` | reservations.write | see §4 |
| `GET /v1/public/reservations/{reference}` | reservations.read | status view |
| `POST …/change-requests`, `…/cancellation-requests` | reservations.write | portal `CreateRequest` (customer_requests; staff resolve them in the panel) |
| `POST …/payment-links` | reservations.write | portal `Pay`: prepayment due or balance; an active link of the same amount is reused |

Every POST except `sessions` needs an **`Idempotency-Key`**:
- The same key and body replay the stored answer with `Idempotent-Replayed: true`.
- The same key with another body answers `422 IDEMPOTENCY_KEY_REUSED`.
- While the first request still runs, the answer is `409 IDEMPOTENCY_IN_PROGRESS`. After 2 minutes a stuck claim may be taken over.
- 5xx and 429 answers free the key.

**RT-A27**: availability, quotes and bookings call the panel's own use cases with the key's channel. The engine, the channel capacity rules, the price lists, quote revalidation and the approval protocol are therefore identical for the site, the widget and the API. The same request answers the same price and capacity everywhere (`TestCatalogAndSameAnswerEverywhere` also compares with the panel search). Channel-specific prices and quotas remain possible on purpose: they follow the key's channel.

## 4. Booking

1. **Quote.** The quote must be one the API made with the same channel and purpose `new`. Otherwise the answer is `400 quote_uuid not_found`, so a panel quote or another channel's quote cannot be booked.
2. **Customer.** The renter (`customer`: individual or corporate, phone and e-mail required) becomes a **new customer card**. It is never matched to an existing customer: a public caller must not attach bookings to someone else's record. Staff can merge cards later.
3. **Drivers.** Up to 5 drivers are created on that card: `same_as_customer` for the individual renter, plus birth date, licence number and licence issue date.
4. **Reservation.** `reservations.Create` runs with the quote, the customer and the drivers. It re-validates server-side: expiry, used quote, price (`QUOTE_PRICE_CHANGED` returns the new quote, re-issued as a public quote, in `data`), driver eligibility (`422 DRIVER_NOT_ELIGIBLE`), capacity (`409 CAPACITY_UNAVAILABLE` with alternatives capped like availability) and equipment. The channel's approval protocol decides the status (`confirmed` / `awaiting_payment` / `request`).
5. **Failure.** Any failure deletes the new customer card again (only while nothing refers to it).
6. **Answer.** On success, `public_reservations` stores the hash of a new **access token**. The answer returns the token once, with the public reservation view. `external_reference` is allowed for secret keys only.

**Customer verification** for status, requests and payment links:
- A booking session always needs `X-Reservation-Token`.
- A secret key needs it unless the reservation was made with that key or a key it was rotated from.
- Every mismatch (no token, wrong token, another organization, another partner's reservation) answers 404.

The view comes from the customer portal's `Reservation`, with documents and invoices left out: they stay behind the portal's OTP sign-in. `customerportal.PrincipalForReference` builds the renter principal without a portal session. Requests then carry the channel label (`Web sitesi`, `Rezervasyon widget'ı`, `API`) as their contact. The lost-token path is the customer portal (`/r/{slug}`, reference + contact + one-time code).

## 5. Webhooks

Endpoints are managed in the panel (public `https` URLs only). Private, loopback and link-local targets are refused when the endpoint is saved and again at dial time after DNS. Development and tests allow them.

**Events** cover reservations **made through the public API**:
- `reservation.created`;
- `reservation.confirmed`, `reservation.changed` (dates / product / drivers), `reservation.cancelled` (any end: cancelled, expired, no-show; the payload carries the status) and `reservation.plate_fixed`, all from the reservations `LifecycleListener` (API server and worker).

The payload holds the reference, the status, payment status, dates, branches, subgroup, channel, money and your `external_reference`. It holds no personal data.

Delivery is a signed `POST` with these headers:
- `Rentopia-Webhook-Id` (the delivery);
- `Rentopia-Webhook-Event`;
- `Rentopia-Webhook-Timestamp`;
- `Rentopia-Webhook-Signature: v1=<hex HMAC-SHA256(secret, "<timestamp>.<body>")>`.

Any 2xx is a success. Other answers back off 1 min, 5 min, 30 min, 2 h, 6 h, 12 h, then 24 h, for 8 attempts, then the delivery fails.

The API server tries a new delivery right away. The minute worker sweep claims due deliveries with a lease (`FOR UPDATE SKIP LOCKED`, so none is sent twice) and purges expired idempotency rows.

Panel actions:
- test ping (sent once, not retried);
- retry of a failed delivery;
- rotate the signing secret (shown once);
- disable or delete an endpoint (deleting also deletes its deliveries).

## 6. Panel

**Ayarlar → Public API** (`tenant.public_api.read`):
- header cards: which channels are on, failed / pending deliveries of the last 7 days, and the API base URL;
- tabs, each a DataTable with facets and live updates (`tenant.public_api_keys`, `tenant.public_webhooks`):
  - **API anahtarları**: create (the secret is shown once; publishable keys show a usage snippet), edit, rotate with a grace period, revoke;
  - **Webhook'lar**: create (the signing secret is shown once), test, edit, rotate the secret, delete, open deliveries;
  - **Teslimatlar**: a drawer with the payload, and retry.

Writes need `tenant.public_api.manage`. Activity: `public_api.key_*`, `public_api.webhook_*`.

## 7. Tests

`publicapi_test.go` runs against a real database: two organizations, the iyzico fake, miniredis for limits and an httptest webhook receiver.

| Test | What it checks |
|------|----------------|
| `TestCredentials` | secret key refused from a browser; sessions only from allowed origins and only their own origin; a tampered token is rejected; preflight; removing an origin ends its sessions; disabled channel; rotation grace and revoke; scopes; B never sees A's keys |
| `TestCatalogAndSameAnswerEverywhere` | locations hide points that are not online-selectable; subgroup conditions; **RT-A27**: API, site and widget answer identically and match the panel search; past pick-up; B's branch is refused |
| `TestQuoteIdempotency` | `Idempotency-Key` required; replay; reuse with another body; per-key scope; B cannot read A's quote; no internal fields |
| `TestBookingFlow` | another channel's quote; an ineligible driver is refused and the customer is removed; browser `external_reference`; booking, replay (no second booking), used quote, channel; token rules (session, secret key, other key, organization B); payment link created then reused; change / cancellation requests (`REQUEST_EXISTS`, policy text, staff label) |
| `TestRateLimit` | the per-key window, `Retry-After`, independent keys, the next window |
| `TestWebhooks` | ping; isolation; `reservation.created` signature and payload (no personal data); backoff, failure after max attempts, manual retry, endpoint health; staff cancel → `reservation.cancelled`; secret rotation; delete cascade; URL rules in production mode |

RT-A01 seeds a key, a webhook and a delivery per organization and covers every `/v1/tenant/public-api/*` route.

## 8. Open items

- **Packages.** The `public_api` / `widget` / `site` entitlements arrive with TEC-674. Until then the sales channel switch is the gate (`checkChannel`).
- **Host-based resolution**: done by the branded site (TEC-729, [branded-site.md](./branded-site.md)); verified custom domains are TEC-731.
- **Customer matching.** Public bookings create a new customer card each time; merging duplicates is a staff task.
- **Webhook scope.** Webhooks cover reservations made through the public API only. Panel and marketplace reservations could be added as an endpoint option.
