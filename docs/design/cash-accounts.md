# Cash and bank accounts, expenses (TEC-720)

The organization's own money: tills and bank accounts, the movements on them, and
expenses with their receipts. It lives in the same module as the customer ledger
(`backend/internal/modules/finance`, see `ledger.md`).

Panel: Finans → Kasa ve banka, Kasa / banka hareketleri, Giderler, Gider kategorileri, Finans raporları.

## Tables (migration 000097)

| Table | Purpose |
|-------|---------|
| `cash_accounts` | A till (`kind = cash`) or a bank account (`bank`). It belongs to the head office (`branch_id` null) or to a branch, and holds `currency`, `opening_balance`, `opening_date`, the bank name, IBAN, note and `status` (`active` / `archived`). The code is unique per organization. A till cannot open with a negative balance. |
| `cash_movements` | The cash book (see the column list below). Append-only: the trigger allows an UPDATE of `category_id` only, and DELETE only through an organization cascade. Corrections are reversal movements. |
| `expense_categories` | Expense categories, customisable per organization. Each has an immutable `code` (imports match on it), a name, `is_active`, `sort_order` and `is_system`. The 8 defaults are seeded for every existing organization by the migration, and for new ones by an organization hook: `maintenance`, `insurance`, `repair`, `fuel`, `cleaning`, `vehicle_procurement`, `operations`, `other`. |
| `expenses` | One row per expense: date, category, description, supplier, document no, `net_amount`, `vat_rate`, `vat_amount`, `total_amount`, an optional branch and vehicle, and an optional paying account. `status` is `recorded` or `void`. The attachment is a private object (see below). |

Columns of `cash_movements`:

- **What happened:** `kind` (`collection`, `expense`, `refund`, `supplier_payment`, `transfer_in`, `transfer_out`, `adjustment`, `reversal`), `direction` (`in` / `out`), `amount > 0`, `movement_date`.
- **What it belongs to:** an optional category, plus links to a ledger entry (customer collections and refunds), an expense, the counter movement of a transfer (`transfer_group`), and the movement it reverses.
- **Who and why:** branch, `created_by`, `reason`.

Balance = opening balance + money in − money out.

## Rules

- **Transfers are atomic.** One transaction locks both accounts with a single `SELECT … FOR UPDATE ORDER BY id` (so concurrent transfers in opposite directions cannot deadlock), then writes `transfer_out` on the source and `transfer_in` on the target with a shared `transfer_group`. Both accounts must be active and in the same currency.
- **A till cannot go negative.** A movement, transfer or expense payment that would take a till below zero is refused (`insufficient_funds`). Bank accounts may go negative: overdraft facilities are the bank's business.
- **A customer collection or refund can name a cash account.** The ledger entry and the cash movement are then written in the same transaction and linked.
- **Expenses.**
  - **Amounts:** give either the total including VAT or the net amount together with a VAT rate; the other figures are derived.
  - **Payment:** naming a paying account writes an `expense` out-movement.
  - **Corrections:** amounts, VAT and the paying account are fixed once recorded. Voiding an expense reverses its movement; to change any of these, void the expense and record it again. Category, date, texts, branch and vehicle stay editable.
- **Attachments.** `PUT|GET|DELETE /finance/expenses/{uuid}/file` stores PDF, JPEG, PNG or WEBP files up to 10 MB under `ExpenseAttachmentObjectKey`, in the private bucket. The API streams the file back, so the browser never sees an S3 key.
- **Branch scope.** A member limited to some branches sees and writes only their branches' accounts, movements and expenses. Head-office accounts need access to all branches.

## API (`/v1/tenant/finance/*`)

- `cash-accounts`:
  - meta, list, create, export, import, sample
  - `{uuid}` get / patch
  - `{uuid}/movements`: a manual collection, refund, supplier payment or adjustment
- `transfers`
- `movements`:
  - meta, list (with the running balance when one account is chosen), export
  - import (bank statement lines), sample
  - bulk (`set_category`)
  - `{uuid}`
  - `{uuid}/reverse`
- `expense-categories`: list, create, patch
- `expenses`:
  - meta, list, create, export, import, sample
  - bulk (`set_category`)
  - `{uuid}` get / patch
  - `{uuid}/void`
  - `{uuid}/file`
- `reports/expenses?group_by=category|branch|vehicle|month`: net, VAT and total amounts.
- `reports/cash-flow?group_by=account|branch|month`: money in and out, plus collections, expenses, refunds and supplier payments. The period defaults to the current month and is capped at one year.

Permissions (migration 000098):

- `tenant.finance.cash.read|write|export|import`, `tenant.finance.cash.bulk.set_category`
- `tenant.finance.expenses.read|write|export|import`, `tenant.finance.expenses.bulk.set_category`
- `tenant.finance.reports.read`

Role grants:

- owner, admin, merkez_yoneticisi and muhasebe get every finance slug.
- sube_yoneticisi gets read, write and export, but not import and not ledger adjustments.

Realtime: `tenant.cash_accounts`, `tenant.cash_movements`, `tenant.expenses` and `tenant.expense_categories`.

## UI

Every list is a DataTable with I/O toolbar export and import:

- **Kasa ve banka:** accounts with balances.
- **Account detail:** the account's movements with a running balance.
- **Kasa / banka hareketleri:** filters for account, kind and category, a date range, and the bulk "Kategori ata" action.
- **Giderler:** filters for category, branch, status and date. The expense dialog has the receipt upload. The bulk "Kategori ata" action is available here too.
- **Gider kategorileri**
- **Finans raporları:** the two reports, with a group selector and a date range.

## Tests

`internal/modules/finance/integration_test.go` covers the cash book:

- **`TestAtomicTransfer`:** concurrent opposite transfers keep the total, a till cannot go negative, and a failed transfer leaves nothing behind.
- **`TestBranchScope`**
- **`TestCrossOrganization`**

The RT-A01 isolation cases in `httpserver/isolation_test.go` cover every route.

## Open items

- iyzico payments credit the customer ledger but are not deposited into a bank account automatically. A settlement import (provider payouts → bank movement) would close that loop.
- **TEC-718 / TEC-719:** Supplier payments are movements with free-text supplier names until supplier accounts exist. They should then post to the supplier's ledger account as well.
- **TEC-722:** Expense VAT is recorded but not yet reported for VAT returns or e-invoice matching.
