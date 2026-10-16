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
| `ledger_entry_links` | Links an entry to an outside document: `document_type` is `invoice`, `statement` or `toll_charge`, plus `document_ref`. Invoicing (TEC-722) hangs off this table. |

Columns of `ledger_entries`:

- **What happened:** `kind` (`accrual`, `collection`, `payment`, `refund`, `offset`, `adjustment`, `reversal`), `direction` (`debit` / `credit`), `purpose` (`rent`, `charge`, `prepayment`, `deposit`, `general`), `amount > 0`, `currency`, `entry_date`, `due_date`.
- **Where it came from:** a generic source (`source_type`, `source_uuid`) plus typed FKs: rental, reservation, rental charge, payment, payment refund and cash movement. The typed FKs are composite `(organization_id, id)`, so no row can point into another organization.
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
| iyzico / branch payment | `collection` (credit), one per payment (`uq_ledger_entries_payment`) | When the payment is applied. The insert is `ON CONFLICT DO NOTHING`, so a webhook retry does nothing. |
| Payment refund | `refund` (debit), one per refund | Posted after the refund. |
| Manual collection / refund | `collection` / `refund`, optionally also posted to a till or bank account | From the panel. |
| Deposit offset | Two `offset` entries sharing an `offset_group`: a deposit debit and a credit to the rent, charge or general debt | From the panel. |

`SyncRental` reconciles one rental inside the caller's transaction. That covers handover, extension, charge decision or override, and toll charging. It compares the desired accruals with the live ones:

- An accrual whose amount or account changed is reversed and accrued again.
- An accrual that is no longer due is reversed. The reason is `system:amount_changed`, `system:no_longer_due`, `system:account_changed`, `system:rental_cancelled` or `system:charge_reversed`.

Running `SyncRental` again changes nothing.

Posting from payments is fail-soft: a failure is logged and the payment still applies. The worker's expire-holds chain runs a catch-up sweep, `PostPending`, which posts missing payment and refund entries. This also back-posts payments that existed before the ledger.

### Single accrual (RT-A23)

A rental's rent, and each charge, is debited once. This is guaranteed by the database, not just by the code:

- **Insert trigger.** It refuses a new accrual while an unreversed accrual with the same `accrual_key` exists. It then sets `accrual_generation` to the number of earlier reversed accruals for that key.
- **Unique index.** `uq_ledger_entries_accrual (organization_id, accrual_key, accrual_generation) WHERE kind = 'accrual'` stops two concurrent inserts from both winning.

Covered by `TestRT_A23_RentDebitedOnce` (finance) and `TestTollChargesReachTheLedger` (tolls).

## Balances and statuses

The balance of an account is debits minus credits, computed over the entries inside the caller's branch scope.

Due states are calculated as follows:

- Credits are applied to debits oldest-first. Offsets are excluded, because they settle the deposit.
- `overdue` is the part of the charges that were due before today and are still unpaid.
- `due_soon` is the part due within 7 days.

An account's status, in order of precedence:

1. `overdue`
2. `due_soon`
3. `receivable` (balance > 0)
4. `payable` (balance < 0)
5. `closed`

The summary (`/finance/summary`) gives organization totals, a breakdown by branch, and the open amount by due date (overdue 30+, overdue, within 7 days, later). `/finance/rental-balances` gives the balance per rental.

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
  - `{uuid}/reverse`
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
- **TEC-722:** Invoices link to entries through `ledger_entry_links`. There is no invoice table or FK yet.
- **TEC-718 / TEC-719:** Supplier accounts exist only as `party_kind = 'supplier'` with a nullable `supplier_id`. The supplier FK arrives with the supplier table.
- The pay link sent with a statement covers one reservation's open balance. A pay link for the whole account has to be added in payments.
- iyzico collections credit the customer account but are not deposited automatically into a bank account in the cash book (see `cash-accounts.md`).
