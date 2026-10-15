# Customer portal

Status: **implemented** (M3: TEC-715). Requirements §8.1 (last paragraph), §14.3 ("a reference alone is never enough") and §15.1 "Müşteri erişimi". It builds on reservations ([reservations.md](./reservations.md)), payments ([payments.md](./payments.md)), documents and remote approval ([contracts.md](./contracts.md)), the SMS / OTP service ([sms.md](./sms.md)) and customers ([customers.md](./customers.md)). The branded site (M5) mounts the same pages, and the portal works for "Yönetim" package organizations too: it needs only reservations, an SMS account (or the platform mailer) and, for online payment, a payment account.

Code:

| Area | Path |
|------|------|
| Use cases (sign-in, session, views, pay, requests, staff list) | `backend/internal/modules/customerportal/usecase` |
| HTTP, session middleware, rate limits | `backend/internal/modules/customerportal/handler`, `routes.go` |
| E-mail one-time codes | `backend/internal/modules/sms/usecase/otp.go` (`IssueEmailOTP`) |
| BFF cookie handling | `frontend/src/lib/server/customer-session.ts` (+ `bff-proxy.ts`) |
| Portal pages | `frontend/src/app/(public)/r/[orgSlug]/…`, `frontend/src/features/customer-portal` |
| Staff list, reservation tab, nav badge | `frontend/src/features/customer-portal/components/customer-requests-*`, `…/nav` |

## 1. Tables

| Migration | Change | Notes |
|-----------|--------|-------|
| 000091_otp_email_channel | `sms_otp_challenges.channel` (`sms` \| `email`) | E-mail codes live in the same challenge store. For `email`, `phone_masked` / `phone_hash` hold the masked address and `BlindIndex("sms.email", org, address)`. Down deletes every e-mail challenge. |
| 000092_customer_portal | `customer_portal_sessions`, `customer_requests` | Tenant tables (`tenant_tables.txt`), composite `(organization_id, …)` FKs to customers, drivers, reservations and sessions. Down drops both tables (every session and request). |

`customer_portal_sessions` keeps the SHA-256 of a 256-bit token, the customer, the driver (driver sign-in), the reservation used to sign in, the channel and masked contact, the verified challenge, IP, user agent, `last_seen_at`, `expires_at` (sliding) and `hard_expires_at`, and the revocation fields. Its only global index is `uq_customer_portal_sessions_token_hash`: the API resolves a bearer token before it knows the organization, like `payment_links.token` and `document_approval_links.token_hash`. It then compares the organization with the one in the URL.

`customer_requests` holds the type (`change` \| `cancel`), the status (`open` → `resolved` \| `rejected`), the message, the requested dates, the cancellation policy text and fee shown, `outcome` (`self_cancelled`), the masked contact and the staff resolution (note, user, time). A partial unique index allows one open request per reservation and type.

## 2. URL and organization

- Portal pages are at **`/r/{orgSlug}`** (sign-in, the customer's reservations) and `/r/{orgSlug}/reservations/{uuid}`. `/r` collides with no app route: the panel uses `/t`, `/platform`, the public pages `/pay`, `/d` and `/invite`, and NextAuth's proxy matcher does not cover `/r`. The pages build every link from the slug, so the M5 branded site can serve them under its own domain with a rewrite (`/` → `/r/{slug}`, `/reservations/*` → `/r/{slug}/reservations/*`). The pages need nothing from the staff shell.
- The API is at **`/v1/customer/{orgSlug}/*`**. The organization always comes from the URL. Unknown, deleted or suspended organizations, and organizations outside their access window, answer 404.

## 3. Sign-in

```text
GET  /v1/customer/{orgSlug}                organization name, channels (sms, email)
POST /v1/customer/{orgSlug}/auth/start     { reference, contact }        → 202, generic
POST /v1/customer/{orgSlug}/auth/verify    { reference, contact, code }  → session
POST /v1/customer/{orgSlug}/auth/logout
GET  /v1/customer/{orgSlug}/session
```

1. **Input.** The reference is upper-cased with spaces removed. The contact is an e-mail address when it contains `@` (`pii.Email`), otherwise a phone number (`pii.Phone` + `pii.ValidPhone`; national forms such as `0532 …` work). Malformed input is a plain 400.
2. **Channel.** SMS needs the organization's active SMS account (`smsusecase.HasActiveAccount`). E-mail goes through the platform mailer (`internal/platform/mail`, the SMTP sender), because there is no organization mail account. When the chosen channel cannot send, the answer is 503 `CODE_UNAVAILABLE`. This is checked **before** any lookup, so it says nothing about the reservation.
3. **Match.** The reference selects one reservation of the organization. The contact is compared with the renter's and with every driver's on that reservation through keyed hashes: `PhoneHash` (`sms.phone`) or `EmailHash` (`sms.email`), both HMAC-SHA256 with keys derived from the master key, compared in constant time over every candidate. If both the renter and a driver match, the renter wins. Phone numbers and e-mail addresses stay plaintext in `customers` / `drivers` (customers.md: operational contact values). Because the reference narrows the comparison to a handful of people, no new blind-index column was added and existing rows need no backfill. A column index can be added if a lookup without a reference is ever needed.
4. **Generic answer.** `start` always answers `202 {status: "code_sent_if_matched", channel, resend_after_seconds, code_ttl_seconds}`, whether the reference exists, the contact matched, a cooldown or limit applied, or the send failed. `channel` reflects only what the customer typed. `verify` answers 400 `OTP_INVALID` for every failure: unknown reference, other contact, wrong, expired or locked code, or a code that went to another contact. Only a match sends a code. Known gap: an SMS enqueue takes a little longer than a non-match. Lookup work is spent on unknown references too, but the timing is not equalized further.
5. **Code.** `IssueOTP` / `IssueEmailOTP` use purpose **`customer_portal`** and subject **`reservation:<uuid>`**. The code has 6 digits, is stored HMAC-only, lives 5 minutes, allows 5 attempts and has a 60 s resend cooldown, with per-contact and per-organization hourly limits (sms.md §5). Before `VerifyOTP` runs, the subject's open challenge must belong to the contact typed now (channel + hash). So another contact of the same reservation never uses up or tries someone else's code. After verification, the binding is checked again.
6. **Session.** A 32-byte token (base64url) is created and only its SHA-256 is stored. The session slides a **30 minute idle expiry** inside a **2 hour hard cap**; the sliding update happens at most once a minute. `logout` revokes it.
7. **Audit** (`activity_events`, resource `tenant.customer_portal`, never a code): `customer_portal.code_sent`, `code_not_sent` (reason: cooldown / rate_limited / no_channel / send_failed), `sign_in_unmatched`, `sign_in_failed` (reason), `signed_in`, `signed_out`, `document_downloaded`, `payment_link_created` / `_reused`, `request_created`. The OTP service writes its own `sms.otp.*` entries as well.

### Rate limits

These are Redis windows that fail open like the other public routes. The OTP service's DB counters still hold without Redis.

| Route | Per IP | Per reference | Per contact (keyed hash prefix) |
|-------|--------|---------------|------------------|
| `GET /{orgSlug}` | 60 / min | – | – |
| `auth/start` | 10 / 10 min | 5 / 10 min | 5 / 10 min |
| `auth/verify` | 30 / 10 min | 10 / 10 min | 10 / 10 min |

Signed-in routes have per-session limits: 240 / min overall, `pay` 10 / 10 min, `requests` 10 / h, PDFs 30 / 10 min.

## 4. Session transport (BFF)

- The session is **never the staff JWT.** `/v1/customer/*` uses its own middleware (`handler.RequireSession`), which reads `X-Customer-Session` and ignores `Authorization`. The BFF never forwards the staff Bearer token on `customer/*` paths.
- On a successful `auth/verify`, the BFF takes `session_token` out of the JSON and stores it in the cookie **`customer_session`**: HttpOnly, Secure, SameSite=Lax, `Path=/api/v1/customer/{orgSlug}`, `Max-Age` = time left to the hard cap. The path scoping keeps one session per organization, and the browser sends the cookie only to that organization's API paths. The BFF forwards it upstream as `X-Customer-Session`.
- The cookie is cleared on `logout` and on any 401 from a customer path. Malformed slugs answer 404 in the BFF. PDFs stream as binary. The `User-Agent` is forwarded on `start` / `verify` for the audit trail.
- `Secure` is always set: browsers treat `http://localhost` as a secure context. Plain-HTTP access from another host (a LAN IP) cannot sign in.

## 5. What a session can see

**Decision: a renter session sees every reservation of that customer in the organization, not only the one used to sign in.** The customer proved control of the contact on file, and the requirements want "rezervasyonlarım". A **driver** session (the contact matched a driver of the reservation) sees only the reservations that driver is on, never the renter's other reservations. A corporate renter's drivers therefore see their own trips only. Every view and action goes through the same scope query (`GetPortalReservation`), and anything outside it is 404.

```text
GET  /v1/customer/{orgSlug}/reservations
GET  /v1/customer/{orgSlug}/reservations/{uuid}
GET  /v1/customer/{orgSlug}/reservations/{uuid}/documents/{documentUuid}/pdf
POST /v1/customer/{orgSlug}/reservations/{uuid}/pay
POST /v1/customer/{orgSlug}/reservations/{uuid}/requests
```

The detail shows:
- status, pick-up and return times and places, the product sold
- the plate, only once it is fixed
- drivers (first name + initial)
- extras (snapshot `extra` lines)
- money: total, prepayment, paid, prepayment due, balance
- payment status and payment deadline
- the policy texts of the reservation's **policy snapshot**
- the cancellation block: policy text, fee, self-cancel flag
- allowed actions
- documents
- requests

No staff notes, no assignee, no internal fields.

**Documents:** every non-void document of the reservation with a current version: rental contract, handover and return forms, terms; invoices arrive with M4. The PDF is streamed by the API through `contracts.OpenPDF`, which renders on demand and answers 409 `PDF_NOT_READY` with `Retry-After` while it renders. It checks `storage.OrganizationOwnsKey`. The browser gets an API path (`download_path`), never a storage key or URL, and the document must belong to the reservation in the path.

**Approve a pending contract:** when a document has an open remote approval link (contracts.md §7) for the signed-in person, the detail offers its `/d/{token}` URL. The renter's link goes to a renter session; a driver's link goes to the session of that driver, matched by name because links store the signer's name. Approval then follows the existing SMS-code flow.

## 6. Actions

**Pay remaining.** The amount due now is the prepayment still due while the reservation is `awaiting_payment`, else the balance of a `confirmed` reservation; other states have nothing due. The reservation's active pay-by-link link is **reused** when it is for exactly that amount and valid for at least 10 more minutes. Otherwise `payments.CreateLink` creates one, which replaces the active link. The customer is sent to `/pay/{token}` and from there to iyzico's hosted checkout (payments.md §3). Errors: 409 `NOTHING_DUE`, `PAYMENT_UNAVAILABLE` (no active payment account), `INVALID_STATE`.

**Change / cancel request.**

**Decision: a new `customer_requests` table, not `resolution_tasks`.** Resolution tasks are engine-driven work items about broken vehicle plans. They have a source (block, release, extension, late return), a re-plan action and one open task per source. A customer request is different:
- it is customer-authored text with requested dates
- it carries the policy text and fee the customer was shown
- it belongs to a customer, a driver and a session
- it gets an answer the customer reads back

Mixing them would weaken the task invariants and leak engine semantics into the portal.

How a request is handled:
- One open request per reservation and type (409 `REQUEST_EXISTS`).
- A change needs a description (3+ characters) or dates.
- A cancellation needs the reservation to be cancellable (`CanAct(cancel)`); a change needs `request`, `awaiting_review`, `awaiting_payment` or `confirmed`.
- Every request adds a `customer_request` entry to the reservation timeline and publishes `tenant.customer_requests` and `tenant.reservations` live events.

**Cancellation policy.** The detail and the dialog show the cancellation text of the reservation's policy snapshot. They also show a fee when the snapshot names `cancellation_fee`, at the top level or under `policies`. The reservation is **cancelled at once only when the snapshot explicitly says `self_cancel_allowed: true`** (top level or `policies`). The request is then stored `resolved` with `outcome: self_cancelled`, and the reservation is cancelled through `reservations.Cancel`. Today's settings document has neither key, so every cancellation goes to staff until a policy version adds them.

## 7. Staff side

```text
GET  /v1/tenant/customer-requests          list (status, request_type, reservation, q), branch scope of the reservation
GET  /v1/tenant/customer-requests/stats    { open }  (nav badge)
GET  /v1/tenant/customer-requests/{uuid}
POST /v1/tenant/customer-requests/{uuid}/resolve   { status: resolved | rejected, note }
```

**Permissions: no new slug.** Reading needs `tenant.reservations.read`; resolving needs `tenant.reservations.write` (a note is required to reject). Acting on the reservation, for example cancelling or changing it, stays a separate action with its own permission. A resolution writes a `customer_request_resolved` / `_rejected` timeline entry and an activity entry `customer_requests.resolved|rejected`.

Panel:
- **Nav item.** "Müşteri talepleri" sits under Rezervasyonlar, with an open-count badge. The badge refetches on the `tenant.customer_requests` live channel and every 2 minutes.
- **List page.** `/t/{slug}/reservations/requests` is a DataTable with type and status facets, live updates, and row actions to open the reservation, resolve or reject.
- **Reservation detail.** A "Müşteri talepleri" tab shows the same table limited to the reservation.

## 8. Tests

- **`customerportal/portal_test.go`** runs against a real database: two organizations, the Netgsm fake, a capturing mailer and the iyzico fake. Codes are read from what the fakes received.

  | Test | Covers |
  |------|--------|
  | happy path | sign in with a national number and a lower-case reference; session; list (own two reservations, not Y's); detail (policy text, pay action, documents, no storage keys); PDF download; pay link created then reused (one active link); cancel request (policy text, reservation unchanged), duplicate → `REQUEST_EXISTS`; change request with dates; staff list / stats / B sees nothing; reject needs a note; the customer sees the answer; timeline entries; audit without codes; logout → 401 |
  | e-mail and driver | e-mail code through the mailer (masked address and hash stored); a driver's own number → driver session that sees only the reservation they drive |
  | generic answers | unknown reference / other contact / match answer byte-identical (minus request id) and only the match sends; cooldown answer identical; verify failures all `OTP_INVALID` and the open code survives them; unknown portal 404; no SMS account → `CODE_UNAVAILABLE` |
  | OTP lock | 5 wrong codes lock; the right code then fails; audit entries; a new code after the cooldown signs in |
  | session expiry | idle expiry slides with use; idle 31 min → 401; hard cap 2 h despite activity; malformed / unknown tokens |
  | **scoping** (RT-A01 style) | X's session of A on every customer route with Y's and B's reservation ids, random and malformed ids, and X's reservation with Y's / B's document → 404 and none of Y's / B's identifiers; X's A-token on B's portal (B has the same number) → 401 on every route; A's reference on B's portal sends no code; nothing written for B or Y |
  | self-cancel | a snapshot with `self_cancel_allowed` and a fee cancels at once (`resolved`, `self_cancelled`); pay then answers `NOTHING_DUE` |

- **`modules/sms/usecase`**: the existing OTP tests run unchanged on the refactored issue path.
- **RT-A01** (`httpserver/isolation_test.go`) seeds a customer request per organization. It covers `customer-requests/{uuid}` and `/resolve` through the path parameter, and the list (probes `reservation`, `q`) and `stats` through the leak scan.
- **Browser check** (local, Chromium at 375×812, log SMS provider, iyzico fake, real API): sign in, view a reservation, download the terms PDF (rendered by headless Chromium), pay the balance through the fake's hosted page until the pay page shows it paid, send a cancellation request, and see it in the staff list. The log provider never prints a code, so the check sets a known code hash on the open challenge with the local API key, standing in for reading the SMS.

## 9. Open items

- **Invoices** in the documents list (M4).
- **Policy fields.** `self_cancel_allowed` / `cancellation_fee` (and fee rules by time before pick-up) are not in the settings form yet. The portal reads them from snapshots as soon as a settings version carries them.
- **Mail.** E-mail codes use the platform sender (from address and name of the platform). A per-organization mail account and branded templates would replace it.
- **OTP texts.** The SMS template is the OTP service's generic text; the organization name in the SMS text is an sms.md open item.
- **Staff actions.** Staff resolve requests by hand; applying a requested change from the request (pre-filled change dialog) is a follow-up.
- **Notifications.** A notification to the customer when a request is answered (SMS / e-mail) is not sent; the customer sees the answer in the portal.
