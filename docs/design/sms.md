# SMS and SMS one-time codes

Status: **implemented** (M3: TEC-734). Sends go through the integration job runner ([integration-jobs.md](./integration-jobs.md)); accounts and sealed credentials follow [payments.md](./payments.md) §2. TEC-711 (contract approval by a signed link plus an SMS code) calls the OTP service.

Code:

| Area | Package |
|------|---------|
| Provider interface (`sms.Provider`), classes, `Rejected` | `backend/internal/platform/sms` |
| Netgsm adapter / in-process fake / dev server | `backend/internal/platform/sms/netgsm`, `…/netgsm/netgsmfake`, `backend/cmd/netgsm-fake` |
| Development log provider | `backend/internal/platform/sms/logsms` |
| Send, delivery reports, OTP service | `backend/internal/modules/sms/usecase` |
| Test send route | `backend/internal/modules/sms` |
| Panel (connect, test SMS, rotate, disable, jobs) | `frontend/src/features/payments/components/integration-dialogs.tsx`, `integrations-page.tsx` |

`notifications/providers` keeps its `NoopProvider` for the `sms` channel of user notifications: those rows have no organization SMS account and no send class. Organization SMS uses `sms.Provider` in `platform/sms` (an `integrations.Adapter` plus `Send` / `Status`).

## 1. Accounts

An organization connects its own provider account on Settings → Entegrasyonlar (`integration_accounts`, kind `sms`, one active per kind):

| Provider | Credentials (sealed, `integration.credentials` purpose) | Notes |
|----------|------------------------------------------|-------|
| `netgsm` | `usercode`, `password`, `msgheader` | Live mode uses `NETGSM_LIVE_BASE_URL`. Netgsm has no sandbox: sandbox mode uses `NETGSM_SANDBOX_BASE_URL`, empty in production (sandbox accounts then refuse to send), `cmd/netgsm-fake` in development. |
| `log` | `msgheader` | Development only (`SMS_LOG_PROVIDER=true`, ignored when `APP_ENV=production`): writes a masked log line, reports delivered. |

The API answers masked hints only. `POST /v1/tenant/integrations/{uuid}/test` checks the connection (Netgsm: the account's message headers; the configured header must be one of them). `POST /v1/tenant/integrations/{uuid}/test-sms {phone}` sends a short transactional test message through that account and answers the masked number, the message and its job.

Env (schema v3): `NETGSM_LIVE_BASE_URL` (const `https://api.netgsm.com.tr`), `NETGSM_SANDBOX_BASE_URL` (empty), `SMS_LOG_PROVIDER` (const `false`); in `compose.prod.yml` and `.env.example`.

## 2. Send classes

| Class | Queue / priority | Consent | Log body |
|-------|------------------|---------|----------|
| `otp` | `integrations_critical`, priority `critical`; Netgsm OTP endpoint | never subject to İYS marketing consent | redacted (`Doğrulama kodunuz: ••••••…`) |
| `transactional` | `integrations_critical`, `critical`; Netgsm `iysfilter "0"` | not needed (notices about a contract the customer entered) | plain |
| `informational` | `integrations`, `normal`; Netgsm `iysfilter "11"` | **required**: an open `customer_marketing_consents` row with channel `sms` for the customer; without a customer or consent the message is logged `blocked` and `ErrNoConsent` is returned | plain |

Marketing (campaign) SMS is not a class here; it needs İYS brand registration and is out of scope.

## 3. Send log (migration 000087)

`sms_messages` (tenant table): class, purpose, provider, `phone_masked` (`+90 5** *** ** 67`), `phone_hash` (`BlindIndex("sms.phone", org, number)`, for counting / lookups), `body_redacted`, `body_hash` (keyed), `payload_ciphertext` (the number and text the provider needs, sealed with purpose `sms.payload`, AAD `sms_messages.payload|org:<id>|message:<uuid>`), customer, status `queued | submitted | delivered | undelivered | failed | unknown | blocked | reconciled`, `provider_message_id`, `dlr_status` / `dlr_at`, error, reference, `idempotency_key` (optional caller key, unique per organization: the notification engine sends `notify:<delivery uuid>`, and a second send with the key returns the first message without queueing anything, TEC-962). `reconciled`: queued in restored data but sent after the backup, marked by the operator (`cmd/messaging-reconcile`); its send job finishes as succeeded without calling the provider. The sealed payload is emptied as soon as the send is final (delivered / undelivered / failed) and when an OTP is verified, locked or expired. Neither the number nor an OTP code is ever stored or logged in plaintext.

`Service.Send` validates (class, purpose code, number via `pii.Phone`, body ≤ 900 chars), applies the organization's hourly cap (DB counter, default 1000), inserts the message and enqueues an `sms.send` job (`org:<id>:sms:<message uuid>`, max 8 calls). The runner's listener keeps the message in step with its job.

## 4. Delivery reports

The send operation's `CheckStatus` reads the provider's delivery report by the provider message id (Netgsm jobid): delivered → job succeeded / message delivered; still waiting → submitted, polled with backoff by the minute sweep; undelivered → failed / undelivered (`dlr_status` = report code). A missing report for an id the provider accepted is "not yet", never "not sent". A send whose answer was lost has no id: Netgsm cannot look it up, so the job stays `needs_verification` and is **not** resent; a fresh OTP request creates a new message instead. A push DLR webhook is not used (polling only, §8).

## 5. OTP service

```go
IssueOTP(ctx, orgID, purpose, subjectRef, phone) (Challenge, error)
VerifyOTP(ctx, orgID, purpose, subjectRef, code) (Verification, error)
```

Generic by `purpose` (lower-case code, e.g. `contract_approval`) and `subjectRef` (≤ 160 chars, e.g. `document_version:<uuid>` for TEC-711). `sms_otp_challenges` (tenant table) holds one issued code:

- **Code**: 6 digits from `crypto/rand`; stored only as `code_hash = BlindIndex("sms.otp.code", "org:<id>|otp:<challenge uuid>", code)` — HMAC-SHA256 with a key derived by HKDF from the master key under its own purpose, bound to the organization and the challenge.
- **TTL** 5 min; **max attempts** 5 (the 5th wrong code locks the challenge); a new code **supersedes** the subject's open one (partial unique index: one `pending` per purpose and subject).
- **Resend cooldown** 60 s per subject (`CooldownError{RetryAfter}`).
- **Rate limits** (DB counters, so they hold without Redis and never fail open): 5 codes per number per hour, 300 per organization per hour (`ErrOTPRateLimited`), plus the general SMS cap.
- **Verify** hashes whatever was entered and compares with `subtle.ConstantTimeCompare` under `SELECT … FOR UPDATE`; a correct code verifies once; errors: `ErrOTPNotFound`, `ErrOTPExpired`, `ErrOTPLocked`, `*InvalidCodeError{AttemptsLeft}` (`errors.Is(err, ErrOTPInvalid)`).
- **Audit**: `sms.otp.issued | verified | verify_failed | locked | expired | rate_limited | send_failed` activity entries on resource `tenant.sms_otp` with purpose, subject and the masked number — never the code.
- A failed send closes the challenge (no usable code without its SMS).

Limits are `usecase.Limits` (`DefaultLimits()`). TEC-711 calls these two methods from its public routes (`/v1/public/documents/{token}/code` and `/approve`, rate-limited per IP and token; [contracts.md](./contracts.md) §7).

`Send` takes a `RedactedBody` for any class: an OTP always needs one, and a message that carries a bearer link (the TEC-711 approval link, transactional) passes one so the log never holds the token.

## 6. Tests

- `platform/sms/netgsm` (against `netgsmfake`, no network): number format, send + delivery report (pending → delivered → undelivered, unknown jobid, no jobid → unsupported, report 503 → uncertain), OTP endpoint, rejections (unknown header `40`, credentials `30`, retryable `85`), lost answer and 503 → uncertain, missing sandbox → rejected, connection check.
- `modules/sms/usecase` (DB + fake): transactional send with delivery report (payload kept until final, dropped after); undelivered; lost answer stays unknown and is not resent (also on retry); informational without consent → blocked; OTP hash only (the code appears in no row of the challenges, messages, jobs, attempts or activity tables), redacted log body, wrong code / other subject / other purpose, verify once, payload dropped, audit entries; lock after 5; TTL; cooldown, supersede, per-number and per-organization limits and their windows; input validation.
- RT-A01: `POST /v1/tenant/integrations/{uuid}/test-sms` and the job routes.

## 7. Development

```bash
cd backend && go run ./cmd/netgsm-fake -addr 127.0.0.1:18091 -usercode 8500000000 -password sandbox-password -header RENTOPIA
# .env: NETGSM_SANDBOX_BASE_URL=http://127.0.0.1:18091
```

Connect a Netgsm account in sandbox mode with the same values, then "Test SMS gönder". The fake lists received messages (OTP codes included) on its own page `/`; the app never logs them. `SMS_LOG_PROVIDER=true` offers the `log` provider instead.

## 8. Netgsm details to confirm (unverified)

netgsm.com.tr was not reachable from the build environment; the adapter follows Netgsm's public REST v2 request format as published in its documentation and SDKs. Confirm with a real account (sub-user with API access, approved header):

1. Endpoints and method: `POST /sms/rest/v2/send`, `POST /sms/rest/v2/otp`, `POST /sms/rest/v2/report`, `GET /sms/rest/v2/msgheader` on `https://api.netgsm.com.tr`, HTTP Basic `usercode:password`.
2. Send body `{"msgheader","encoding":"TR","iysfilter","messages":[{"msg","no"}]}`; `iysfilter` values `"0"` (transactional, no İYS check) and `"11"` (individual recipients, İYS checked); whether `partnercode` is required.
3. OTP body `{"msgheader","msg","no"}`; OTP SMS restrictions (Turkish characters, length, the header having OTP permission). Our OTP text contains Turkish characters (`Doğrulama`) — may need an ASCII variant.
4. Answer shape `{"code":"00","jobid":"…","description":"…"}` and the error codes / meanings used for classification: `20`, `30`, `40`/`41`, `50`/`51`, `70`, `80`, `85`, `100` (treated as a definitive, retryable rejection — confirm that a `100` answer means nothing was queued).
5. Number format: national 10 digits for Turkish mobiles (`5321234567`), `00` + country code for foreign numbers.
6. Report body `{"bulkIds":["<jobid>"]}`, answer `{"code":"00","jobs":[{"jobid","status","errorCode","deliveredDate"}]}`; status codes (`0` waiting, `1` delivered, others undelivered: `2` timeout, `3` invalid number, `4` not sent to the operator, `11`, `12`, `13`, `14`, `15`, `16`/`17` İYS …); `deliveredDate` format `dd.MM.yyyy HH:mm:ss` (Europe/Istanbul); code `60` = no record (and whether a very recent jobid answers `60`).
7. msgheader listing answer `{"code":"00","msgheaders":[…]}`.
8. HTTP status codes on auth failures (401 vs. 200 with code `30`) and whether an IP allow-list must include the worker hosts.
9. A push delivery report (DLR URL) option, to replace polling.

## 9. Open items

- Confirm §8 on a real Netgsm account, then switch an account to live.
- Templates per purpose (organization name in the OTP text).
- Notification templates sending transactional SMS (reservation / contract notices) through `Service.Send`.
- An SMS log page (the table is ready; the panel shows SMS through the job list today).
- Push DLR webhook instead of polling (§8.9).
