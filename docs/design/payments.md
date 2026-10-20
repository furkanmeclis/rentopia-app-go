# Integrations, payments and approval flows

Status: **implemented** (M3: TEC-676 integration skeleton, TEC-707 payment provider layer + iyzico, TEC-708 approval flows; RT-A14, RT-A15, RT-A16). Builds on reservations ([reservations.md](./reservations.md): state machine, `PaymentRecorder`, expiry sweep), the availability engine ([availability-engine.md](./availability-engine.md)), money ([pricing.md](./pricing.md): `money.Money`, NUMERIC(14,2), prepayment) and the field cipher ([customers.md](./customers.md)).

Code:

| Area | Package |
|------|---------|
| Adapter registry, credential vault | `backend/internal/platform/integrations` |
| Payment provider interface | `backend/internal/platform/payments` |
| iyzico adapter, test fake | `backend/internal/platform/payments/iyzico`, `…/iyzico/iyzicofake`, `backend/cmd/iyzico-fake` |
| Accounts API (`/v1/tenant/integrations`) | `backend/internal/modules/integrations/accounts` |
| Payments API, public pay page, webhook | `backend/internal/modules/payments` |
| Approval flows, late payments | `backend/internal/modules/reservations/usecase/approval.go` |
| Panel | `frontend/src/features/payments` (integrations, approval queue, reservation payments tab, `/pay/[token]`) |

## 1. Tables (migrations 000077 – 000080; 000076 is reserved)

| Migration | Tables / changes | Notes |
|-----------|------------------|-------|
| 000077_integrations | `integration_accounts`, `integration_jobs` | Accounts are tenant-owned (`tenant_tables.txt`). Jobs are **mixed scope** (`mixed_scope_tables.txt`, `organization_id` NULL = a future platform job). |
| 000078_payments | `payment_links`, `payments`, `payment_refunds` | Tenant-owned, composite `(organization_id, …)` FKs to reservations / accounts / links. |
| 000079_reservation_approval_payments | `reservations.approval_protocol`, approval-queue index, `resolution_tasks.task_type += late_payment` | Existing rows backfilled from `policy_snapshot.approval_mode`. |
| 000080_seed_payments_rbac | 4 permission slugs + template grants | §6. |

Every index starts with `organization_id` except three deliberate global ones, each because the organization is not known yet when it is used: `uq_payment_links_token` (the public page resolves the token to its organization), `uq_integration_jobs_provider_key` (idempotency across deliveries; keys carry `org:<id>:` anyway) and `uq_integration_jobs_uuid`. Downs drop the tables / column / task type and say what is lost.

## 2. Integration skeleton (TEC-676)

- `integrations.Adapter` (`Kind`, `Provider`, `Fields`, `Check`) in an `integrations.Registry` keyed by kind / provider. Later providers (KABİS, HGS, e-invoice, channel managers) register the same way; `integration_jobs` records their outbound calls and inbound notifications (idempotency key unique per provider, attempts, deliveries, status `pending → processing → succeeded | failed | ignored`, references only in `payload`).
- **Credentials** are the organization's own (each organization connects its own iyzico merchant). They are sealed by `integrations.Vault` with `FieldCipher.Sealer("integration.credentials")`: keys `HKDF(master_N, "purpose/integration.credentials/enc/vN")` — separate from the personal-data column keys and the blind-index keys — and associated data `integration_accounts.credentials|org:<id>|account:<uuid>`, so a ciphertext copied to another row / organization does not open. Rotation follows the field cipher (older versions still decrypt).
- The API **never returns secrets**: `credentials_hint` holds masked values (`sand••••0001`). Updates replace only the keys sent; activity entries list the changed field names, never values.
- One **active** account per organization and kind (partial unique index). Disabling stops new links / checkouts; pending payments still verify on the account they were opened with.
- `POST …/test` makes an authenticated provider call that moves no money (iyzico BIN check) and stores `last_check_*`.

### 2.4 Step-up

The tenant shell has no step-up policy yet (the step-up engine guards platform routes with `RequireStepUp`). When a tenant policy exists, `connect`, `PATCH` (rotation) and `disable` get `middleware.RequireStepUp` in `integrations/accounts/routes.go`; nothing else changes.

## 3. Payments (TEC-707)

### 3.1 Provider layer

`payments.Provider` = `integrations.Adapter` + `CreateCheckout` (hosted 3DS checkout / pay-with-provider page), `Retrieve` (read and verify a checkout result), `Refund` (partial / full), `ParseWebhook` and `VerifyWebhook`. `PreAuthorizer` (`PreAuth`, `Capture`, `Void`) is the optional deposit flow. Since TEC-719 the iyzico adapter implements it with the Checkout Form's pre-auth variant (hosted, no card data), post-auth and cancel; pending sandbox verification ([deposits.md](./deposits.md) §5). **No card field ever exists in Rentopya**: the customer pays on iyzico's Checkout Form page.

### 3.2 iyzico adapter

| Call | Endpoint |
|------|----------|
| Checkout initialize | `POST /payment/iyzipos/checkoutform/initialize/auth/ecom` (`conversationId` = `basketId` = our payment uuid, one VIRTUAL basket item, callback `/pay/{token}/callback`) |
| Checkout retrieve | `POST /payment/iyzipos/checkoutform/auth/ecom/detail` |
| Refund | `POST /v2/payment/refund` (by `paymentId`, partial amounts allowed) |
| Connection test | `POST /payment/bin/check` |

Authentication is **IYZWSv2**: `signature = hex(HMAC-SHA256(secretKey, randomKey + uriPath + body))`, header `Authorization: IYZWSv2 base64("apiKey:<k>&randomKey:<r>&signature:<s>")` and `x-iyzi-rnd: <r>`. Prices are sent as iyzico's SDKs format them (`"1500.0"`). Signed answers are verified with the account's secret: initialize `HMAC(conversationId:token)`, retrieve `HMAC(paymentStatus:paymentId:currency:basketId:conversationId:paidPrice:price:token)`. Base URLs come from `IYZICO_SANDBOX_BASE_URL` / `IYZICO_LIVE_BASE_URL` (env schema v2, defaults = iyzico's); the account's `mode` picks one. Retrieve maps `paymentStatus SUCCESS` + `fraudStatus 1` → succeeded, `fraudStatus 0` (review) → still pending, `-1` / `FAILURE` → failed. Gross (`paidPrice`, may include installment interest), commission (`iyziCommissionRateAmount + iyziCommissionFee`) and net are stored apart from the amount owed.

`iyzicofake` serves the same endpoints in-process (checks IYZWSv2, signs answers and webhooks, hosted test page that posts back to the callback and delivers the webhook). Tests never touch the network; `go run ./cmd/iyzico-fake` serves it for local browser checks.

### 3.3 To confirm against the real sandbox (TEC-684)

The formulas follow iyzico's official SDKs (iyzipay-node / iyzipay-php) because docs.iyzico.com was not reachable from the build environment. With İdes sandbox keys, check: (1) the webhook header name and the HPP formula `HMAC-SHA256(secretKey, secretKey + iyziEventType + iyziPaymentId + token + paymentConversationId + status)` (direct format without the token); (2) the numeric text of `price` / `paidPrice` in the retrieve signature (the adapter accepts the raw JSON text and the normalised forms); (3) `tokenExpireTime` and the buyer placeholders (`identityNumber 11111111111`, city / country) the checkout form accepts; (4) the refund-v2 answer field that carries the refund reference. Configure the merchant notification URL to `https://<app>/api/v1/public/payments/iyzico/webhook`.

### 3.4 Records and links

- `payments`: one checkout attempt (provider `iyzico`, method `checkout_form`) or one branch collection (provider `manual`, method `cash | card_pos | bank_transfer`). Status `pending → succeeded → partially_refunded → refunded`, or `failed`. `amount` (owed, NUMERIC(14,2)), `gross_amount`, `commission_amount`, `net_amount`, `refunded_amount`; `provider_payment_id`, `provider_token`, masked `provider_data` (card family, last four, installment, fraud status); `verified_via` (`webhook | retrieve | manual`), `applied_at` + `outcome` (what the reservation did).
- `payment_links`: a bearer `token` (32 random bytes) → `/pay/{token}` on the app origin; amount, currency, purpose (`prepayment | balance | custom`), `expires_at` (≤ the payment window while the reservation waits for payment), status `active | paid | expired | cancelled` (expiry applied lazily). A new link cancels the reservation's active one.
- `payment_refunds`: one row per refund (pending → succeeded / failed, provider reference, reason, actor); each refund is also an outbound `integration_jobs` row.

### 3.5 The confirmation rule

A payment is confirmed **only** by a provider retrieve whose answer matches our row: order reference (`basketId` = payment uuid, `conversationId`), token, currency, `price == amount` and `paidPrice ≥ amount`, a provider payment id. A mismatch fails the payment (`verification_mismatch: …`, logged) and never confirms. The retrieve runs:

1. after a **webhook** whose signature verified with the organization's secret (§3.6);
2. on the **pay page poll** after the customer returns (`GET /v1/public/pay/{token}?verify=1`, rate-limited per token) — the return itself is ignored (`/pay/{token}/callback` only redirects to the page);
3. on staff **verify** (`POST /v1/tenant/payments/{uuid}/verify`);
4. in the **reconciliation sweep** (worker, with the minute hold expiry): checkouts pending for 2 min – 48 h, so a lost webhook never leaves a paid reservation behind.

### 3.6 Webhook and idempotency (RT-A16)

`POST /v1/public/payments/iyzico/webhook` (no auth, 300 / min per IP; the BFF forwards only `X-IYZ-SIGNATURE-V3` on this path):

1. parse the body (`400` when malformed);
2. `paymentConversationId` → our payment → its organization (`404` when unknown / ambiguous);
3. verify `X-IYZ-SIGNATURE-V3` with **that organization's** secret key and check the token (`401`);
4. upsert the `integration_jobs` row `org:<id>:webhook:<paymentId>:<event>:<status>` — a repeated delivery only increments `deliveries`; a key already `succeeded` answers `{duplicate: true}` without calling the provider; a concurrent delivery loses the `pending → processing` claim and answers duplicate too;
5. provider retrieve + match (§3.5); `pending → succeeded` is a conditional update (one winner), `uq_payments_organization_provider_payment` makes one payment per provider payment id, and `ClaimPaymentApply` lets exactly one caller tell the reservation.

Non-2xx answers make iyzico retry; a failed processing leaves the job `failed` for the next delivery. RT-A16 (`payments_flow_test.go`): the same notification three times (two concurrently) → one succeeded payment, one `confirm` event, one provider retrieve, one job with `deliveries = 3`; a forged signature changes nothing.

### 3.7 Refunds and collections

`POST /v1/tenant/payments/{uuid}/refund` (`tenant.payment.refund`): partial or full, never above `amount − refunded`; through the provider (`/v2/payment/refund`), manual payments are refunded as a record. `POST /v1/tenant/reservations/{uuid}/collections` (`tenant.payment.collect`): money taken at the branch, confirmed at once. Both recount the reservation (§4).

## 4. Reservations port

`PaymentRecorder.RecordPayment(ctx, org, reservation, PaymentUpdate{Paid, Refunded, Payment, Amount, Source}) (outcome, error)` — `Paid` is the total received (refunds deducted, `SumReservationPayments`), so repeated calls are harmless. It sets `payment_status` (`unpaid | partially_paid | paid | refunded`) and `paid_amount`, writes `payment_recorded`, then:

| Reservation | New payment covers the prepayment? | Outcome |
|-------------|-----------------------------------|---------|
| `awaiting_payment` | yes | `confirmed`: hold → firm on the **same** allocation (`Confirm` path; both approval modes — in staff mode it only reaches awaiting_payment after the staff accepted) |
| `awaiting_payment`, option lapsed before the sweep | yes | the reservation expires now; continue as late (below) |
| `expired` | yes | **re-validate**: reserve a firm allocation now; found → `reinstated` (`expired → confirmed`, event `reinstate` with the payment); not found → `late_payment_task` |
| `expired` (partial) / `cancelled` / `no_show` | – | `late_payment_task` |
| anything else | – | `recorded` |

A late payment is **never confirmed silently**: either the reinstate is recorded on the timeline with the payment uuid, or a `late_payment` resolution task (source = payment, one per payment) asks staff to refund or re-book; the pay page tells the customer the team will contact them. Reinstate notes: equipment holds are re-taken best effort; a coupon use released on expiry is not re-taken.

The reservation price exposes `prepayment_due` (prepayment − paid) and `balance` (total − paid, the remaining rent collected at the branch) separately next to `payment_status`.

## 5. Approval flows (TEC-708)

### 5.1 Protocol per channel

`sales_channels[].approval_protocol` in the settings document: `""` (follow the organization's `approval_mode`), `auto`, `staff`, or `channel` (the sales channel confirms on its own side — marketplaces such as yolcu360 / obilet — so the reservation is confirmed at once whatever the online approval mode). The protocol applied is stored on the reservation (`approval_protocol`: `auto | staff | channel`). Channel reservations are therefore not bound to the online approval setting.

| Protocol | Channel | Start | Capacity |
|----------|---------|-------|----------|
| auto | any | `awaiting_payment` until `payment_hold_minutes` (prepayment due) else `confirmed` | option / firm |
| staff | online (web, widget, api, marketplaces) | `request` ("değerlendirme bekliyor"), SLA = `staff_review_window_minutes` | **none** |
| staff | branch counter | `awaiting_review` (option for the review window, as in M2) | option |
| channel | any | `confirmed` | firm |

### 5.2 Staff flow

`POST /v1/tenant/reservations/{uuid}/accept` (`tenant.reservations.approve`; `/approve` delegates for requests):

- **request**: re-priced now (the sold coupon is re-evaluated; a manual discount or a kept price stands). A changed total answers `409 QUOTE_PRICE_CHANGED` with a change quote; accepting with its `quote_uuid` applies that price. Capacity is reserved now at the current availability (`409 CAPACITY_UNAVAILABLE` with alternatives otherwise): prepayment due → `awaiting_payment` on an option for the payment window; nothing due → `confirmed` (firm).
- **awaiting_review**: the held option is kept and moved to the payment window (M2 `Approve`).
- `send_payment_link` (+ `tenant.payments.create_link`, active payment account): a link for the prepayment due, expiring with the payment window, returned with the answer.
- `POST …/reject { reason }`: `request | awaiting_review → cancelled` (option released once, coupon returned).

Requests do not expire with the SLA (no stock is held); the queue shows them overdue. Reviews expire with their option (M2 sweep).

**Approval queue** (`/t/{slug}/reservations/approvals`): the reservations list filtered to `request, awaiting_review`, sorted by `review_deadline`, DataTable with an SLA countdown, accept (dialog: payment link switch, new-price step, copyable link) and reject (reason).

### 5.3 State machine additions

`request --approve--> awaiting_payment | confirmed`, `request | awaiting_review --reject--> cancelled`, `expired --reinstate--> confirmed` (system, late payment only). `model/statemachine_test.go` covers the table.

## 6. Permissions

| Slug | owner, admin, merkez_yoneticisi | sube_yoneticisi, rezervasyon_satis | muhasebe | teslim_iade |
|------|-----|-----|-----|-----|
| `tenant.integrations.manage` (new) | ✓ | | | |
| `tenant.payments.read` (new) | ✓ | ✓ | ✓ | ✓ |
| `tenant.payments.create_link` (new; also staff verify) | ✓ | ✓ | | |
| `tenant.reservations.approve` (new; approve / accept / reject) | ✓ | ✓ | | |
| `tenant.payment.refund` (TEC-689, reused) | ✓ (000041) | sube ✓ | ✓ | |
| `tenant.payment.collect` (TEC-689, reused) | ✓ (000041) | ✓ | ✓ | ✓ |

`tenant.payments.refund` was not added: the existing `tenant.payment.refund` / `tenant.payment.collect` slugs already name these actions and roles were designed around them. `/approve` moved from `tenant.reservations.confirm` to `tenant.reservations.approve` (granted to every role that had confirm).

## 7. Routes

Tenant (RequireOrganization; RT-A01 covers each — path cases for `integrations/{uuid}`, `payments/{uuid}`, `payment-links/{uuid}`, reservation sub-routes; leak scans with probes for the lists; `POST /v1/tenant/integrations` is allowlisted: its body names no record):

```text
GET|POST /v1/tenant/integrations            GET /v1/tenant/integrations/meta
GET|PATCH /v1/tenant/integrations/{uuid}    POST …/{uuid}/test | /disable | /enable    GET …/{uuid}/jobs
GET /v1/tenant/payments   GET /v1/tenant/payments/{uuid}   POST …/{uuid}/verify   POST …/{uuid}/refund
GET /v1/tenant/payment-links   GET /v1/tenant/payment-links/{uuid}   POST …/{uuid}/cancel
POST /v1/tenant/reservations/{uuid}/payment-links | /collections | /accept | /reject
```

Public (OpenAPI tag `Payments`): `GET /v1/public/pay/{token}` (organization name, reference, product, dates, amount, verified state — no customer data; 120 / min per IP), `POST /v1/public/pay/{token}/checkout` (20 / 10 min per IP, 10 per token), `POST /v1/public/payments/iyzico/webhook`. Next.js: `/pay/[token]` (public page, polls until verified) and `/pay/[token]/callback` (provider return → 303 to the page).

## 8. Tests

- `platform/payments/iyzico`: IYZWSv2, price format, checkout / retrieve (pending → paid), refund (partial, over-refund refused), webhook signature (valid, other secret, tampered status, missing header), malformed body.
- `platform/crypto`: purpose separation (another purpose / owner / the PII key cannot open), rotation.
- `reservations/usecase/payments_flow_test.go` (DB + fake): **RT-A16** (above, plus refund and branch collection recount), **RT-A14** (online staff request without stock → SLA in the queue → price change → accept at the new price with a payment link → paid → confirmed; reject; branch review keeps the option), **RT-A15** (payment after expiry with the only car sold again → expired stays expired, one `late_payment` task even when the notification repeats, refund → `refunded`), reinstate when capacity is free, lost-webhook reconciliation, verification mismatch, channel protocol.
- `payments/routes_test.go`: permission gates; `reservations/routes_test.go`: approve / accept / reject gates; RT-A01 over every new route.

## 9. Open items

- **Real iyzico sandbox** (TEC-684, İdes merchant access): run §3.3 against the sandbox, set the merchant notification URL, then flip an account to live.
- Pay-by-link delivery (SMS / e-mail to the customer) is manual (copy link); notifications can send it once templates exist.
- Customer-facing booking channels (web / widget / API) that create staff-approved requests themselves are M4; the protocol and the queue are ready.
- Deposits / pre-authorization: implemented in TEC-719 ([deposits.md](./deposits.md)); the pre-auth webhook is not routed to deposits yet (staff verify with a retrieve).
- A tenant step-up policy for integration changes (§2.4).
- Several pending checkouts of one link can each succeed (a customer paying twice): both are recorded, the reservation is confirmed once, the extra is refunded by staff.
