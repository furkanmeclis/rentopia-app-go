# Security deposits and pre-authorizations (TEC-719)

Status: **implemented** (M4, §11.3). Online pre-authorization is **pending sandbox verification** (TEC-684, §5).

A deposit is money (or a card hold) the customer leaves as security for a rental. It is never revenue and never a collection while it is held. Deductions are captured from it at the end and become collections applied to the debt they settle; the rest goes back to the customer.

It builds on the finance foundation:

- the customer ledger ([ledger.md](./ledger.md)),
- the cash book ([cash-accounts.md](./cash-accounts.md)),
- the payments layer and the iyzico adapter ([payments.md](./payments.md)),
- the return inspection and its deposit task ([inspections.md](./inspections.md)).

Code:

| Area | Where |
|------|-------|
| Use cases | `backend/internal/modules/finance/usecase/deposits.go`, `deposit_files.go` |
| HTTP | `backend/internal/modules/finance/handler/deposits.go`, routes in `finance/routes.go` |
| Export adapter | `backend/internal/modules/finance/adapters/deposits.go` |
| iyzico pre-auth | `backend/internal/platform/payments/iyzico` (`PreAuth`, `Capture`, `Void`) and its fake |
| Panel | `frontend/src/features/deposits` |

## 1. Where the amount comes from

The deposit amount of a product lives on `rental_product_versions.deposit_amount` (product conditions). A reservation copies it into its `product_snapshot`, so a later product change never moves the deposit of a booked rental. Taking a deposit defaults to that snapshot value; staff may enter another amount. Rental groups have no deposit setting of their own: the product condition is the single source.

## 2. Tables (migrations 000700, 000701)

| Table | Purpose |
|-------|---------|
| `security_deposits` | One deposit of a rental (and its reservation). It holds the party (the invoice customer, or the customer), the ledger account, the branch it was taken at, the amount and currency, the method, the status, the till / bank account that holds cash, the POS provision number, the online provider references (account, token, payment id, page URL while pending, masked provider data), the hold end, the captured and returned totals, the timestamps (`held_at`, `resolved_at`), the resolution key and note, the cancel reason and the customer notice. |
| `deposit_deductions` | A line captured at the resolution: amount, `applies_to` (`charge`, `rent`, `general`), reason (at least 3 characters), an optional rental charge and an optional inspection damage, and an optional document in private storage. |

Methods: `cash`, `bank_transfer`, `pos_preauth` (a physical POS terminal; the provision number is typed in) and `online_preauth` (iyzico).

Statuses:

| Status | Meaning |
|--------|---------|
| `pending` | An online pre-auth whose hosted page the customer has not completed (or the provider has not confirmed) yet. |
| `held` | Money in a till / bank account, or a confirmed card hold. |
| `released` | Closed without deductions; a pre-authorization was released. |
| `refunded` | Closed without deductions; the cash / transfer was paid back. |
| `partially_captured` | Closed with deductions (one or more lines captured; the rest released or paid back). |
| `cancelled` | Never held (abandoned or declined online pre-auth). |

Rules in the database:

- One open (`pending` / `held`) deposit per rental (`uq_security_deposits_open`).
- `captured_amount + returned_amount <= amount`.
- A closed status always has `resolved_at`; a cash / transfer deposit always names its account; a POS pre-auth always has its reference.
- Composite FKs `(organization_id, …)` to the rental, reservation, customer, account, branch, cash account and integration account. A deduction keeps its row when a draft form's damage is deleted (`ON DELETE SET NULL (inspection_damage_id)`).
- Both tables are in `tenant_tables.txt`; every index starts with `organization_id`.

The down migration drops both tables and deletes the deposit postings with them (ledger entries and cash movements, with the append-only triggers switched off for that statement). It says so in its data-loss note.

## 3. Accounting

The customer ledger stays single-sided per account ([ledger.md](./ledger.md)), with a separate liability inside it: **every entry of purpose `deposit` is kept out of the receivable balance, the aging and the running balance**. `deposit_held` is the sum of those entries (credits minus debits).

| Step | Ledger (customer account) | Cash book |
|------|---------------------------|-----------|
| Take cash / transfer | `deposit_hold`, credit, purpose `deposit` | `deposit_in` into the chosen till / bank account, linked to the entry |
| Take POS / online pre-auth | nothing (no money moved) | nothing |
| Capture a deduction, cash / transfer deposit | `deposit_capture` debit, purpose `deposit` **and** `collection` credit, purpose `charge` / `rent` / `general`, both naming the deduction; the collection names the charge | nothing (the money is already in the account; it just stops being owed back) |
| Capture a deduction, POS pre-auth | `collection` credit applied to the charge | optional `collection` movement into the bank account the POS settles to |
| Capture, online pre-auth | `collection` credit applied to the charge (after the iyzico post-auth) | none (iyzico payouts are not booked automatically, as for every iyzico payment) |
| Pay the rest back (cash / transfer) | `deposit_return`, debit, purpose `deposit` | `deposit_out` from the till / bank account (a till cannot go negative) |
| Release the rest (pre-auth) | nothing | nothing |

So:

- A cash deposit held never reduces what the customer owes. The rent and the approved charges stay due; the account shows the deposit apart as `deposit_held`.
- A pre-auth never closes the customer's balance and never posts revenue.
- A capture converts a deduction into a collection applied to the related charge, through the ledger, with the matching liability leg when money was held.
- A release or refund posts the cash movement back.
- Collections reports (kind `collection`) see a deduction only once it is captured. `deposit_in` / `deposit_out` movements count in the cash flow totals (money in / out) but not as collections or refunds.

Idempotency in the database: one `deposit_hold` and one `deposit_return` per deposit, one leg of each kind per deduction (partial unique indexes on `ledger_entries`). Deposit postings are not reversed by hand: ledger entries answer `deposit_managed`, cash movements `deposit_movement`. A correction is an adjustment.

### Change to the ledger (TEC-717)

Before TEC-719 a manual collection with purpose `deposit` reduced the balance and offsets were left out of the aging. Now purpose `deposit` is outside the balance everywhere (accounts list, statement, rental balances, summary, branch breakdown, aging, open reservation balances), and the other leg of an offset counts as a settlement. The manual "deposit" collection and the offset (mahsup) still work and follow the same rule. `TestRT_A23_RentDebitedOnce` was updated accordingly.

## 4. Flow

1. **Taking.** At the handover, the wizard's amounts step shows the deposit card with "Depozito al"; the rental page has a Depozito tab with the same card. Allowed while the rental is in preparation, active or in return check. The card lists the tills / bank accounts of the caller's scope, so counter staff need no cash book permission.
2. **Online pre-auth.** The deposit is created `pending` and iyzico's hosted pre-auth page opens (URL on the card, copy / open). The provider return lands on `/deposit-preauth/{uuid}/callback`, which only redirects to a neutral page. Staff press "Doğrula": a provider retrieve that matches our reference, amount and currency moves it to `held`; a declined one is `cancelled`. The browser return is never proof.
3. **Return.** The return opens the `deposit` operation task (existing behaviour). Its field task page links to `Finans → Depozitolar?rental=<uuid>`, which opens the rental's open deposit with the resolution.
4. **Resolution** (`POST …/resolve`, one transaction under a row lock):
   - **Release in full:** pre-auth released (online: iyzico cancel), cash / transfer paid back.
   - **Deduct:** lines with amount and reason, linked to an approved charge (never a proposed or disputed one: decide it first) and / or a damage of the return form. The total of the lines on a charge never exceeds the charge; the total never exceeds the deposit. Online: one iyzico post-auth of the total; the rest of the hold is released by the provider.
   - The rental's live deposit task is completed (`completion_note = deposit:<status>`).
   - **Idempotent:** the request carries an idempotency key (the dialog makes one per opening; the `Idempotency-Key` header works too). A replay answers the resolved deposit without posting or calling the provider again; any other call on a closed deposit answers 409 `already_resolved`. Concurrent calls wait on the row lock.
5. **Cancel** (`POST …/cancel`, with a reason): only a pending deposit. If the provider already confirmed the pre-auth, the cancel is refused (`preauth_completed`) and staff verify and release it instead.

## 5. iyzico (pending sandbox verification, TEC-684)

Only operations iyzico offers are used, behind `payments.PreAuthorizer`:

| Step | Endpoint |
|------|----------|
| Open the hold | `POST /payment/iyzipos/checkoutform/initialize/preauth/ecom`: the Checkout Form's pre-auth variant, so no card data reaches Rentopia. The direct `/payment/preauth` needs card fields on the merchant side and is not used. |
| Confirm | `POST /payment/iyzipos/checkoutform/auth/ecom/detail` (the same retrieve as payments) |
| Capture | `POST /payment/postauth` with `paymentId` and `paidPrice` (at most the held amount, once) |
| Release | `POST /payment/cancel`. iyzico has no other way to drop a pre-auth. |

What the adapter does not do, because iyzico does not offer it:

- **A second capture.** A deposit is resolved in one step.
- **Re-authorization or extension of a hold.** When the bank's hold expires the card is free; take a new deposit.
- **Partial release without capture.** Use a post-auth of the deduction total.

Whatever cannot be done online is done on a POS terminal and recorded with the `pos_preauth` method. To confirm against the İdes sandbox:

1. The pre-auth initialize path and that retrieve works for pre-auth tokens.
2. The post-auth answer signature (`paymentId:currency:basketId:conversationId:paidPrice:price`; the adapter verifies it only when present).
3. That a post-auth below the held amount releases the rest of the hold.
4. Cancel of an uncaptured pre-auth after the day it was opened.
5. The notification webhook for pre-auth checkouts: today it names no payment of ours (404, iyzico retries), so confirmation happens through "Doğrula".

`iyzicofake` serves the same endpoints with the IYZWSv2 checks and signed answers; `TestPreAuthCaptureVoid` and `TestDepositOnlinePreAuth` use it.

## 6. Customer notice of deductions

There is no notification template mechanism yet (the SMS module sends free text; user notifications have no customer channel). A resolution with deductions therefore records a **pending notification hook** on the deposit: `customer_notice_status = pending` and `customer_notice` = `{channel, rental_reference, currency, deposit, captured, returned, deductions: [{amount, reason, applies_to}]}`. The deposit drawer shows it. When templates arrive (or an SMS template for deposits), a sender picks up the pending rows, sends, and sets `sent`.

## 7. API and permissions

`/v1/tenant/finance/deposits`:

- `meta`, `totals` (open deposits by method, "held over N days"), list (filters `status`, `method`, `branch`, `rental`, `from`, `to`, `held_over_days`, `q`, sort `created_at` / `held_at` / `amount`)
- POST (take), `export` (I/O engine)
- `{uuid}` GET, `{uuid}/verify`, `{uuid}/resolve`, `{uuid}/cancel`
- `{uuid}/deductions/{deduction}/file` PUT / GET / DELETE (PDF, JPEG, PNG, WebP up to 10 MiB, sniffed; key `organizations/{org}/deposits/{deposit}/deductions/{deduction}/{file}.{ext}`, streamed by the API, `OrganizationOwnsKey` checked)
- `/v1/tenant/finance/rentals/{uuid}/deposits` (the deposit card: default amount, items, whether one can be taken, online enabled, the live task, the cash accounts)

Permissions (migration 000701, `rbac.go`, `permissions.ts`):

| Slug | owner, admin, merkez_yoneticisi, muhasebe | sube_yoneticisi | teslim_iade | rezervasyon_satis |
|------|-----|-----|-----|-----|
| `tenant.finance.deposits.read` | ✓ | ✓ | ✓ | ✓ |
| `tenant.finance.deposits.write` (take, verify, release, cancel) | ✓ | ✓ | ✓ | |
| `tenant.finance.deposits.deduct` (deductions, documents) | ✓ | ✓ | | |
| `tenant.finance.deposits.export` | ✓ | ✓ | | |

Branch scope: a deposit belongs to the branch it was taken at. A member limited to some branches sees and resolves only their branches' deposits; others answer 404. Engines: activity entries (`finance.deposit_taken`, `deposit_verified`, `deposit_resolved`, `deposit_cancelled`, `deposit_document_uploaded`, `deposit_document_deleted`), the live list `tenant.security_deposits`, and the export adapter. There is no bulk action: every money step needs a per-deposit decision and a reason. There is no import either, because a deposit is money taken at the counter.

## 8. UI

- **Finans → Depozitolar** shows the open totals (held, cash / transfer, card holds, held over N days, pending), then a DataTable with branch, method and status facets, a date range, a "held over N days" selector, export and live updates. A row opens the deposit drawer.
- **Deposit drawer:**
  - Figures, the hosted page while pending, and "Doğrula".
  - "Tamamını serbest bırak / iade et", "Kesinti yap" and "İptal et".
  - The deductions as a DataTable with document upload, download and removal.
  - The pending customer notice.
- **Resolution dialog:** release in full, or deduction lines (amount, what it settles, approved charge, return damage, reason) with the live total and remainder. Cash deposits also choose the refund account; POS deposits choose the optional settlement bank. The dialog shows a per-method hint, including the manual POS path.
- **Rental page:** the Depozito tab (card plus history). The handover wizard's amounts step shows the same card. The deposit field task has the "Depozitoyu sonuçlandır" link.

## 9. Tests

`internal/modules/finance/deposits_integration_test.go` (real Postgres, iyzico fake):

- **`TestDepositCashHeldAsLiability`:** the receivable stays at the rent. The deposit is held, and `deposit_in` is the only movement. The summary is unchanged, there is one open deposit per rental, and a manual reversal is refused. The running balance ignores the deposit, and taking a deposit needs write.
- **`TestDepositPreAuthPostsNothing`:** no entry and no movement, the totals by method, and a release that posts nothing.
- **`TestDepositPartialCaptureAgainstCharge`:** deductions need the permission, a reason, an approved charge, and must stay within the charge and the deposit. The capture pair is applied to the charge and 1 250 is paid back. The customer then owes the rent only. The document is stored under the organization prefix.
- **`TestDepositReleaseIdempotent`** and **`TestDepositCaptureIdempotent`:** concurrent resolutions with the same key post once. Another key, or a capture after the release, is refused. The database refuses a second return or capture leg.
- **`TestDepositOnlinePreAuth`:** pending, then verify, held, a post-auth of 500 (one collection), a replay without a second post-auth, a full release with iyzico cancel, a declined pre-auth cancelled, and no online path without an account.
- **`TestDepositTaskCompletes`:** the return opens the task, and the resolution completes it.
- **`TestDepositListFilters`:** method, branch, held over 7 days (with a moved clock) and the totals.
- **`TestDepositBranchScopeAndIsolation`:** another branch gets 404 everywhere. Another organization's deposit, rental, till and charge are unknown.

Also: `platform/payments/iyzico` `TestPreAuthCaptureVoid`, the updated `TestRT_A23_RentDebitedOnce`, and RT-A01 (`httpserver/isolation_test.go`) over every deposit route. The RT-A01 cases cover the path parameters `deposits/{uuid}` and `deductions/{deduction}`, bodies naming another organization's rental, till, damage and charge, and the list probes.

## 10. Open items

- **TEC-684:** run §5 against the iyzico sandbox and route pre-auth webhooks to deposits.
- **Customer notice:** send the pending notices once templates exist (§6).
- **Cash flow report:** `deposit_in` / `deposit_out` count in money in / out; a separate "deposits" column would make the report clearer.
- **POS settlement:** a POS capture is booked to a bank account only when one is chosen; a settlement import would close the loop (same open item as iyzico payouts).
- **Rental groups:** no group-level deposit setting; products carry it.
