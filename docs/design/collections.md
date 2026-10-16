# Collections, allocation to open items and refunds (TEC-718)

Spec §11.2. A collection ("tahsilat") is one receipt of money from a party: cash, bank transfer / EFT or a physical POS slip taken at the branch, or an online payment (iyzico). It is a document on top of the ledger ([ledger.md](./ledger.md)) and the cash book ([cash-accounts.md](./cash-accounts.md)); it never replaces them.

Module: `backend/internal/modules/finance` (`usecase/collections.go`, `usecase/collection_money.go`). Panel: Finans → Tahsilatlar, and "Tahsilat al" on the rental "Cari" tab and the account detail.

## Model (migration 000101)

| Table | Purpose |
|-------|---------|
| `collections` | One document per ledger collection entry (`ledger_entry_id`, unique). Code `TH-000001` per organization (advisory-lock serialised), account, method (`cash`, `bank_transfer`, `card_pos`, `online`, `other`), provider (`manual`, `iyzico`), purpose (`general`, `prepayment`, `deposit`), date, reference no, payer, note, branch, the cash / bank / clearing account it landed in and its movement, rental / reservation / payment links. Money: `amount` (credited to the party), `gross_amount`, `commission_amount`, `net_amount`, `refunded_amount`. State: `status` (`recorded`, `partially_refunded`, `refunded`, `void`), `settlement_status` (`none`, `pending`, `settled`), void reason / actor / time, `idempotency_key`. |
| `collection_allocations` | Append-only `allocate` / `release` rows applying a collection to an open item. Net allocation of a pair = allocate − release. `source` says who decided (`manual`, `auto`, `refund`, `void`, `accrual_reversed`, `released`), `request_key` the client retry key. |
| `ledger_entries.collection_id` | A refund entry names the collection it returns. New collection entries use `source_type = collection` (`source_uuid` = the collection). |
| `cash_accounts` | Kind `clearing` (a provider balance) and `provider`: the bank / clearing account that receives a provider's net settlement, one active per organization and provider. |

Existing collection entries were backfilled as unallocated collections (method from the payment or `other`, status from reversals / refunds). Online payments posted before the migration are not settled retroactively (`settlement_status = none`).

Migration 000102 seeds `tenant.finance.collections.bulk.allocate`. Everything else reuses existing slugs (below).

## Open items and allocation

An **open item** is a live accrual (kind `accrual`, debit, not reversed): the rent of a rental or an approved charge. Its status is computed from the net allocations: `open` (nothing), `partially_paid`, `paid`. `GET /finance/accounts/{uuid}/open-items` lists them (optionally one rental, optionally open only) with the account's **credit**: the unallocated money of its collections.

Allocation modes on create and on `POST /collections/{uuid}/allocations`:

- `auto` (default): oldest first (entry date, due date, id). With a rental the rental's items only.
- `manual`: the items named, the amount defaulting to the item's open amount. An amount above the item's open amount or above what the collection has left is a validation error.
- `none` (create only): keep everything as credit.

What a collection pays beyond its open items is the customer's **credit**; it is allocated later by hand, by the bulk action, or automatically when the rental accrues (below). Deposits (`purpose = deposit`) are held, never allocated; TEC-719 settles them with deduction / offset documents that are allocated (see open items). Supplier accounts take no collections (`supplier_account`); they are settled by supplier payments ([suppliers.md](./suppliers.md)).

### Safety

- Every allocation path locks the account row (`SELECT … FOR UPDATE`) in its transaction.
- The insert trigger `collection_allocations_guard` locks the account row itself and refuses: an item of another account, a reversed / non-accrual item, a void or deposit collection, more than the item's amount, more than the collection's `amount − refunded_amount`, and a release above the pair's net allocation. The sums are read after the lock (fresh snapshot per statement), so two concurrent writers cannot allocate the same money twice; the loser gets `409 allocation_conflict`.
- `collections_refund_guard` refuses a refund / void update that would leave more allocated than the collection still holds.
- Idempotency: `idempotency_key` on create (unique per organization; a retry answers `200` with the first collection, a concurrent duplicate loses on the index and answers the winner), `idempotency_key` on allocate (a key already used on the collection allocates nothing more).

### Rental lifecycle

`SyncRental` (handover, extension, charge decisions) keeps allocations consistent:

- An accrual that is reversed releases its allocations (`source accrual_reversed`).
- The accrual that replaces it (same key: amount changed, account unchanged) gets them back first.
- Then the credit of the rental's own collections (rental or reservation link, e.g. an online prepayment) and of the collections just released pays the rental's open items, oldest first.

## In-branch collections

`POST /v1/tenant/finance/collections` (`tenant.finance.accounts.read` + `tenant.payment.collect`): account (`account_uuid`, or the rental's invoice customer / customer from `rental_uuid`, opened on first use), amount, method, date, branch (default: rental pick-up / account branch; a branch-scoped member must stay in scope), the cash account, reference, payer, note, allocation. One transaction writes the ledger credit, the cash movement into the account (cash → a till, transfer / POS → a bank or clearing account; `other` may have none), the document and its allocations.

`POST /finance/accounts/{uuid}/collections` (the account route of TEC-717) now goes through the same code: it writes a document too, keeps the money as credit and still answers the entry.

## Online collections (iyzico)

`PostPayment` (payments apply, staff verify, the webhook path, the `PostPending` sweep) writes, in one transaction and at most once per payment (entry: `uq_ledger_entries_payment`, document: `uq_collections_organization_payment`, serialised by the account lock):

- the collection entry (as before) and an `online` collection with `amount` (owed, credited), `gross_amount` (paid on the provider), `commission_amount` and `net_amount` from the verified retrieve;
- auto allocation to the reservation's rental items when the rental exists;
- the **settlement** when the provider's settlement account is known.

**Decision: the provider commission is an expense, not a contra entry.** The customer paid the full amount and owes nothing more, so the customer ledger is credited with the full amount; the commission is the organization's cost of collecting it. Rentopia has no revenue ledger to net a contra entry against, and an expense (category `payment_commission`, seeded for every organization) shows up where costs are reported: the expense list and report and the cash-flow report. Settlement writes, in the settlement account: a `collection` movement in for the gross (linked to the collection entry), an expense of the commission paid from that account with its `expense` movement out, and marks the collection `settled`. The account then holds the net; the payout to the bank is a normal transfer. An installment surcharge (gross above the amount owed) stays in the clearing account and is visible on the collection (gross vs amount).

Without an active settlement account the collection stays `pending`; the `PostPending` sweep settles pending ones once a bank / clearing account is marked with the provider. The sweep also writes missing documents of posted payments.

## Refunds

`POST /collections/{uuid}/refund` (`tenant.payment.refund`, reason 3–500 characters, amount ≤ `amount − refunded`):

- **Online / payment-backed collections** go through the existing payments refund (`payments.Refund`: payment_refunds row, provider call, integration job and attempt). The provider result decides: a refusal leaves the refund row `failed`, posts nothing and answers `502`; on success payments calls `PostRefund`, which posts the refund entry naming the collection, raises `refunded_amount` (status `partially_refunded` / `refunded`) and, when the collection was settled, takes the money out of the settlement account (`refund` movement). The collection detail lists the payment's refunds with their status (`pending` / `succeeded` / `failed`). Refunds made from the payments page follow the same path.
- **Manual collections** post the refund entry and a `refund` movement out of the till / bank account (the collection's by default; a till cannot go negative) in one transaction.

Either way the collection's allocations shrink newest first so that what stays allocated never exceeds what is left; the items they paid open again.

## Void

`POST /collections/{uuid}/void` (`tenant.finance.ledger.adjust`, reason): a manual collection without refunds — allocations released, the entry reversed, its movement reversed, status `void`. Reversing a collection's entry from the statement (`entries/{uuid}/reverse`) is the same void. Online collections are refunded, never voided; a collection refund entry is final.

## API (`/v1/tenant/finance/*`)

- `collections`: `meta`, list (q, method, status, allocation_status, branch, cash_account, account, rental, from, to; sort collection_date / amount / code / created_at), create, `export`, `bulk` (`allocate`, reversible: the rollback releases what the run allocated)
- `collections/{uuid}`: detail with allocations and refunds; `allocations`, `allocations/release`, `refund`, `void`
- `accounts/{uuid}/open-items`

Permissions: read `tenant.finance.accounts.read`, record `tenant.payment.collect`, allocate / release `tenant.finance.accounts.write`, refund `tenant.payment.refund`, void `tenant.finance.ledger.adjust`, export `tenant.finance.accounts.export`, bulk `tenant.finance.collections.bulk.allocate` (owner, admin, merkez_yoneticisi, muhasebe, sube_yoneticisi).

Branch scope: a collection belongs to its branch (head office collections need every branch); lists, detail and every action answer 404 outside the scope; the cash account must be in scope; open items follow the entries' branches. Activity: `finance.collection_recorded`, `_allocated`, `_allocation_released`, `_refunded`, `_voided` on `tenant.finance_collections`; realtime list `tenant.finance_collections`.

## UI

- **Tahsilat al** (rental "Cari" tab, account detail): method toggle, amount (defaults to the open total), date, till / bank account filtered by method, method-specific reference label, payer, branch, note, and the open items DataTable with the auto preview or an amount per item and the credit that remains. One idempotency key per opened dialog.
- **Finans → Tahsilatlar**: DataTable with method, branch, cash account, status and allocation facets, date range, export, bulk allocate, live updates.
- **Collection detail**: figures (gross / commission / net for online), facts, allocations (release), refunds (status, failure reason); allocate credit, refund and void dialogs.
- **Account detail**: open items with their status and the customer credit, the account's collections.
- **Cash accounts**: kind "Sağlayıcı (ara hesap)" and the online settlement provider.

## Tests

`internal/modules/finance/collections_integration_test.go` (real Postgres): partial and multi-item allocation, overpayment as credit allocated later (rental credit automatically, account credit by hand, idempotent key, release), concurrent double allocation refused (and the same request twice applied once; the database refuses over-allocation directly), online payment with the commission split (pending → settled by the sweep, expense category, idempotent sweep, prepayment paying the rent at the handover), online refund refused vs succeeded, cash refund (till funds, newest allocation first), void and entry reversal, allocations following an extension, branch scope and cross-organization isolation. RT-A01 covers every new route (`httpserver/isolation_test.go`).

## Open items

- Branch collections recorded in finance do not change the reservation's `paid_amount` / payment status; those still come from the payments module (online payments and reservation collections). The rental's money state is the ledger.
- Aging (overdue / due soon on accounts and the summary) still applies credits to debits oldest first over balances; it does not read allocations yet.
- Deposit settlements (TEC-719): a deposit deduction and the settling leg of an offset each get a collection document (method of the deposit, or `other` for an offset) allocated to their charge / the rental's items, so the item shows as paid. These documents are not refundable or voidable (`deposit_managed`); deposit holds and returns never get a document and are never credit ([deposits.md](./deposits.md)).
- Payment-backed branch collections (reservation collections) have no till: a refund goes through payments without a cash movement.
- Provider payouts (clearing → bank) are manual transfers; a settlement report import would close that loop.
