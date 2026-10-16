# Customer ledger ("cari", TEC-717)

Organization-scoped receivables and payables. Every money fact about a customer is a
row in an append-only ledger. Balances, due states and statements are all computed
from those rows.

Module: `backend/internal/modules/finance`. Panel: Finans → Cari hesaplar.

## Tables (migration 000096)

| Table | Purpose |
|-------|---------|
| `accounts` | One account per party. `party_kind` is `customer` (with `customer_id`) or `supplier` (with `supplier_id`, nullable, no FK yet). Also holds the code `CR-000001` (a per-organization sequence serialised by an advisory lock), the branch, the currency, `payment_term_days`, and `status` (`active` / `archived`). |
| `ledger_entries` | The ledger itself (see the column list below). |
| `ledger_entry_links` | Links an entry to an outside document: `document_type` is `invoice`, `statement` or `toll_charge`, plus `document_ref`. Invoicing (TEC-722) hangs off this table: `invoice_id`, `amount`, `released_at` / `release_reason` ([einvoice.md](./einvoice.md)). |

Columns of `ledger_entries`:

- **What happened:** `kind` (`accrual`, `collection`, `payment`, `refund`, `offset`, `adjustment`, `reversal`, and since TEC-719 `deposit_hold`, `deposit_return`, `deposit_capture`), `direction` (`debit` / `credit`), `purpose` (`rent`, `charge`, `prepayment`, `deposit`, `general`), `amount > 0`, `currency`, `entry_date`, `due_date`.
- **Where it came from:** a generic source (`source_type`, `source_uuid`) plus typed FKs: rental, reservation, rental charge, payment, payment refund, cash movement, and (TEC-719, migration 000103) security deposit and deposit deduction. The typed FKs are composite `(organization_id, id)`, so no row can point into another organization.
- **Who and why:** `branch_id`, `created_by`, `reason`.
- **Bookkeeping:**
  - `accrual_key` and `accrual_generation`
  - `reverses_entry_id`
  - `offset_group`

### Append-only

The trigger `ledger_entries_append_only` rejects every UPDATE and every DELETE. The only exception is a DELETE cascading from an organization delete (`pg_trigger_depth() > 1`).

A correction is always a new row:

- **Reversal.** The exact opposite of the original entry: same account, amount and purpose, opposite direction. The insert trigger enforces this. An entry can be reversed at most once (`uq_ledger_entries_reverses`).
- **Adjustment.** A new debit or credit line with a reason of at least 3 characters.

Both require `tenant.finance.ledger.adjust`, except automatic reversals, which carry a `system:*` reason.

## When things are posted

| Fact | Entry | Trigger |
|------|-------|---------|
| Rent | `accrual` / `rent`, key `rental:<id>:rent` | When the rental is handed over (status `active`, `return_check` or `returned`). Only when `collecting_party = 'us'`. The amount is the reservation's `total_amount`, posted to the invoice customer (or the customer if there is none). The due date is the handover day plus `payment_term_days`. An extension that changes the total reverses the accrual and accrues the new amount. |
| Rental charge (damage, fuel, km, late fee, toll, toll fee …) | `accrual` / `charge`, key `rental_charge:<id>` | When the charge is approved. Proposed, waived and disputed-but-unapproved charges are never accrued. A charge with a reversal row is not due either. |
| Toll charge reversal (TEC-725) | `reversal` of the charge's accrual, reason `system:charge_reversed` | The reversal row itself (negative amount, `reversal_of_id`) is never accrued. |
| iyzico / branch payment | `collection` (credit), one per payment (`uq_ledger_entries_payment`), plus its collection document | When the payment is applied. The insert is `ON CONFLICT DO NOTHING`, so a webhook retry does nothing. See [collections.md](./collections.md). |
| Payment refund | `refund` (debit), one per refund, naming the collection (`collection_id`) | Posted only after the provider confirmed the refund. |
| Collection (TEC-718) / manual refund | `collection` (`source_type collection`) / `refund`, with the cash movement into / out of a till or bank account | From the panel: [collections.md](./collections.md). |
| Deposit offset | Two `offset` entries sharing an `offset_group`: a deposit debit and a credit to the rent, charge or general debt | From the panel. |
| Supplier invoice (TEC-721) | `accrual` / `charge`, **credit** on the supplier account, key `supplier_invoice:<id>`, source `supplier_invoice` | When the invoice is recorded. A due date, amount or account change re-accrues (`system:due_changed`, `system:amount_changed`, `system:account_changed`). A void reverses it (`system:invoice_voided`). See `suppliers.md`. |
| Supplier payment (TEC-721) | `payment` (debit), one per payment (`uq_ledger_entries_supplier_payment`), source `supplier_payment` | Together with the cash movement. A void reverses it. |
| Security deposit (TEC-719) | `deposit_hold` (credit) when cash / a transfer is taken, `deposit_capture` (debit) + `collection` (credit, applied to the charge) per deduction, `deposit_return` (debit) for the rest paid back; all deposit legs have purpose `deposit`. A pre-auth posts only the captured collections. Unique per deposit / deduction. | From the deposit flow, see [deposits.md](./deposits.md). |

`SyncRental` reconciles one rental inside the caller's transaction. That covers handover, extension, charge decision or override, and toll charging. It compares the desired accruals with the live ones:

- An accrual whose amount or account changed is reversed and accrued again.
- An accrual that is no longer due is reversed. The reason is `system:amount_changed`, `system:no_longer_due`, `system:account_changed`, `system:rental_cancelled` or `system:charge_reversed`.

Running `SyncRental` again changes nothing.

Collections allocated to an accrual that is reversed are released and carried to the accrual that replaces it; the rental's own collections then pay its open items oldest first ([collections.md](./collections.md)).

Posting from payments is fail-soft: a failure is logged and the payment still applies. The worker's expire-holds chain runs a catch-up sweep, `PostPending`, which posts missing payment and refund entries. This also back-posts payments that existed before the ledger.

### Single accrual (RT-A23)

A rental's rent, and each charge, is debited once. This is guaranteed by the database, not just by the code:

- **Insert trigger.** It refuses a new accrual while an unreversed accrual with the same `accrual_key` exists. It then sets `accrual_generation` to the number of earlier reversed accruals for that key.
- **Unique index.** `uq_ledger_entries_accrual (organization_id, accrual_key, accrual_generation) WHERE kind = 'accrual'` stops two concurrent inserts from both winning.

Covered by `TestRT_A23_RentDebitedOnce` (finance) and `TestTollChargesReachTheLedger` (tolls).

## Balances and statuses

The balance of an account is debits minus credits, computed over the entries inside the caller's branch scope. **Entries of purpose `deposit` are not part of it** (TEC-719): a deposit held is a liability towards the customer, kept apart as `deposit_held` (deposit credits minus deposit debits). It never reduces the receivable. The same rule applies to the statement's running balance, the rental balances, the summary, the branch breakdown and the open reservation balances.

Due states are calculated as follows:

- Credits are applied to debits oldest-first. Deposit-purpose entries are excluded; the other leg of an offset or a deposit capture (a credit to the rent, a charge or the general debt) counts as a settlement.
- `overdue` is the part of the charges that were due before today and are still unpaid.
- `due_soon` is the part due within 7 days.

Aging is party-aware. For supplier accounts, credits are the charges, so the direction is flipped before matching. `overdue` and `due_soon` then describe payables. The aging and account list rows return `party_kind`.

An account's status, in order of precedence:

1. `overdue`
2. `due_soon`
3. `receivable` (balance > 0)
4. `payable` (balance < 0)
5. `closed`

The summary (`/finance/summary`) gives organization totals, a breakdown by branch, and the open amount by due date (overdue 30+, overdue, within 7 days, later). `/finance/rental-balances` gives the balance per rental. Supplier payables are kept out of the receivable figures and reported separately, in `payable_overdue` and `payable_due_soon`.

Supplier invoice and payment entries cannot be reversed through `entries/{uuid}/reverse`. They are voided through their invoice or payment, which writes the reversal.

## API (`/v1/tenant/finance/*`)

- `summary`, `rental-balances`
- `accounts`:
  - meta, list, create, export
  - `{uuid}` get / patch (terms and note)
  - `{uuid}/statement`
  - `{uuid}/collections`, `refunds`, `adjustments`, `offsets`
  - `{uuid}/send-statement`: SMS with the balance. With `pay_link: true` it also creates a payment link for the open reservation balance, and the response includes the link's URL, amount and expiry.
- `entries`:
  - meta, list, export
  - `{uuid}`
  - `{uuid}/reverse` (a collection's entry: voids the collection; a collection refund entry cannot be reversed; accruals, offsets and deposit postings are refused: they follow their rental / pair / deposit)
- `accounts/{uuid}/open-items` and `collections/*`: [collections.md](./collections.md).
- `rentals/{uuid}`, `customers/{uuid}`: the ledger cards on the rental and customer detail pages.

Permissions (migration 000098, `rbac.go`, `permissions.ts`): `tenant.finance.accounts.read|write|export` and `tenant.finance.ledger.adjust`.

- `write` together with `tenant.payments.collect` / `refund` is needed to record a collection or a refund.
- `tenant.payments.create_link` is needed for the pay link.

Realtime: `tenant.finance_accounts` and `tenant.ledger_entries` list events.

## UI

- **Finans → Cari hesaplar** has three tabs:
  - **Accounts:** a DataTable with filters for balance status, branch and "due before".
  - **Rental balances**
  - **Breakdowns:** by branch and by due date.
- **Account detail** shows the header figures and the statement as a DataTable with a running balance. Export goes through the I/O engine as PDF or XLSX. The page also offers collection, refund, adjustment, deposit offset, reverse, edit terms, and "Ekstre gönder" (statement + pay link).
- **Rental detail** has a "Cari" tab. **Customer detail** has a "Cari hesap" tab.

## Tests

`internal/modules/finance/integration_test.go`:

- append-only
- reversal flow
- RT-A23
- idempotent webhook posting
- disputed charge not accrued
- balances and statuses
- atomic transfer
- branch scope
- cross-organization

Toll posting is tested in `internal/modules/tolls/ledger_integration_test.go`. `httpserver/isolation_test.go` covers every finance route (RT-A01).

## Open items

- **TEC-721:** Reservations collected by a channel (`collecting_party <> 'us'`) are not accrued. They belong on the partner's account once partner accounts exist.
- **TEC-722 (done):** Invoices link to entries through `ledger_entry_links`, which now carries `invoice_id`, the linked `amount` and a release (`released_at`, `release_reason`). One live invoice link per entry (`uq_ledger_entry_links_invoiced`). An invoice never posts to the ledger; see [einvoice.md](./einvoice.md).
- **TEC-721 (done):** `accounts.supplier_id` now references `suppliers` (`fk_accounts_supplier`). Supplier invoices and payments post to the supplier account (`suppliers.md`).
- **TEC-719 (done):** security deposits post through the ledger with the deposit purpose kept outside the balance ([deposits.md](./deposits.md)). Before TEC-719 a deposit collection reduced the balance; existing rows follow the new rule as soon as the queries run.
- The pay link sent with a statement covers one reservation's open balance. A pay link for the whole account has to be added in payments.
- Aging still applies credits to debits oldest first; per-item allocations (TEC-718) are not read by it yet.
