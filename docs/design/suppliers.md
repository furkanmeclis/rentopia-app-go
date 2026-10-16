# Suppliers, supplier invoices and supplier payments (TEC-721)

What the organization owes: supplier cards ("Tedarikçi"), their invoices ("gider faturası") and the payments made to them.

- **Supplier cards** live in their own module: `backend/internal/modules/suppliers`.
- **Invoices and payments** live in the finance module (`backend/internal/modules/finance`, `usecase/suppliers.go`). They write to the same ledger (`ledger.md`) and cash book (`cash-accounts.md`).

Panel: Finans → Tedarikçiler, Gider faturaları, Tedarikçi ödemeleri, Vadesi yaklaşan ödemeler.

## Tables (migrations 000800, 000801)

| Table | Purpose |
|-------|---------|
| `suppliers` | The supplier card. It holds:<ul><li>`code`: `TED-000001`, unique per organization and generated under an advisory lock.</li><li>Identity: `title`, `supplier_type` (`service`, `insurance`, `vehicle_supplier`, `parts`, `fuel`, `other`), `tax_number` (VKN with 10 digits or TCKN with 11, checksum validated by the API, `chk_suppliers_tax_number` checks the shape) and `tax_office`.</li><li>Contact: phone, email, website, address and city, plus `contacts` (a JSONB list of name / role / phone / email).</li><li>Billing: billing address and email, the e-invoice flag, bank name and IBAN (mod-97 validated), `payment_term_days`.</li><li>`note`, and `status`: `active` / `archived`. Cards are archived, never deleted.</li></ul> |
| `supplier_invoices` | One row per invoice:<ul><li>Number, `invoice_date`, `due_date` and the expense category.</li><li>An optional branch and vehicle.</li><li>Totals: `net_amount`, `vat_amount`, `total_amount` and the effective `vat_rate`; `paid_amount`.</li><li>`status`: `open` / `partially_paid` / `paid` / `void`.</li><li>The attachment columns and the void fields.</li></ul>The number is unique per supplier among non-void invoices. |
| `supplier_invoice_lines` | Optional lines (description, net amount, VAT rate, VAT, total). When they are given, the header totals are their sum. |
| `supplier_payments` | Money out of a cash or bank account: amount, date, method, reference, `status` (`posted` / `void`). It links to its ledger entry and cash movement. |
| `supplier_payment_allocations` | The amounts a payment settles on one or more invoices. An unallocated remainder is an advance on the supplier's account. |

Columns added to existing tables (all composite FKs `(organization_id, …)`):

| Column | Notes |
|--------|-------|
| `accounts.supplier_id` | Now has the FK `fk_accounts_supplier`. Each supplier has one ledger account (`party_kind = 'supplier'`, code `TD-…`), created on first use. A rename or a terms change is synced to it. |
| `ledger_entries.supplier_invoice_id`, `supplier_payment_id` | New columns. `source_type` gains `supplier_invoice` and `supplier_payment`; the CHECK lists the full union. `uq_ledger_entries_supplier_payment` allows one payment entry per supplier payment. |
| `expenses.supplier_id`, `supplier_invoice_id` | `supplier_invoice_id` is unique. |
| `vehicles.supplier_id` | FK `fk_vehicles_supplier`. |

All five new tables are in `tenant_tables.txt`. Every index starts with `organization_id`.

### Free-text names → supplier cards

The migration creates one card per distinct, trimmed, case-insensitive `vehicles.supplier_name` / `expenses.supplier_name` in each organization:

- **Type:** `vehicle_supplier` when the name came from a vehicle, otherwise `other`.
- **Linking:** the rows are linked through the new `supplier_id` columns. The text columns stay, for backward compatibility and exports.
- **Rename:** renaming a card rewrites the linked text columns.
- **Matching names later:** a card created later adopts unlinked rows with the same name, so an import of supplier cards also links old rows.

Existing manual cash movements of kind `supplier_payment` carry no supplier name, so they cannot be matched to a supplier. They stay as plain cash movements.

The down migration drops the five tables, the new columns, the FKs and the RBAC rows. It also deletes supplier-sourced ledger entries and cash movements; to do so it disables the append-only triggers for the duration of the delete. This loses data and is noted in the file.

## Expenses: the invoice is the source

The choice was to link expenses to invoices, not to replace them. Each invoice creates exactly one expense row (`expenses.supplier_invoice_id`), with the category, branch, vehicle, net, VAT and total amounts of the invoice. That way:

- expense reports, the expense list and the vehicle cost tab keep covering every cost, without a second write path;
- an expense paid on the spot (a receipt, no supplier account) is still recorded directly as an expense.

Invoice-generated expense rows are read-only. Patch, void and category changes are refused with `managed_by_supplier_invoice`; the invoice is edited instead, and the row is synced. The expense list shows a "Gider faturasından" badge and links to the invoice. A direct expense can name a supplier (`supplier_uuid`), but it does not post to the supplier's account, because it is already paid.

## Posting rules

| Fact | Entry |
|------|-------|
| Invoice created | `accrual` / `charge`, **credit** on the supplier account, key `supplier_invoice:<id>`. The due date is the invoice's due date, or invoice date + `payment_term_days`. The single-accrual trigger (RT-A23) guards it. |
| Invoice due date, amount or account changed | `syncInvoice` reverses the live accrual and accrues again. The reason is `system:due_changed`, `system:amount_changed` or `system:account_changed`. |
| Invoice voided | The accrual is reversed (`system:invoice_voided`) and the expense row is voided. An invoice with live payment allocations is refused with `invoice_has_payments`: void the payments first. |
| Supplier payment | One `payment` entry, **debit** on the supplier account (`ON CONFLICT DO NOTHING` on `uq_ledger_entries_supplier_payment`), plus one `supplier_payment` out-movement on the cash account. Both are written in one transaction, which also locks the cash account. A till cannot go negative (`insufficient_funds`). |
| Payment voided | The entry is reversed, a `reversal` movement is written, and the allocated invoices are recomputed. |

`syncInvoice` works like `SyncRental`: it compares the desired accrual with the live one, and running it again changes nothing.

Supplier entries and movements cannot be reversed from the generic ledger or cash screens (`Reverse` and `ReverseMovement` refuse them); they are voided through their invoice or payment.

### Allocation

- **Explicit:** a payment can list `allocations` (invoice + amount).
- **Automatic:** without a list (and unless `auto_allocate` is `false`), the payment settles the oldest due invoices first (FIFO by due date, then invoice date), within the caller's branch scope.
- **Validation:** each allocation must belong to the same supplier and currency and must not exceed the invoice's open amount, and the total must not exceed the payment.
- **Invoice status:** `paid_amount` and `status` are always recomputed from the posted allocations (`SetSupplierInvoicePaid`), never incremented.

### Due states

Each invoice has a computed due state, using Europe/Istanbul days:

| State | Meaning |
|-------|---------|
| `overdue` | Open amount > 0 and due date before today. |
| `due_soon` | Due within the window. The default is `DueSoonDays = 7`; `window_days` overrides it. |
| `later` | Due after the window. |
| `settled` | Paid or void. |

`/finance/payables/summary` returns the overdue, due-soon and total open amounts with their counts. The same figures appear as a card on the payables and supplier pages, and as the badge on the "Vadesi yaklaşan ödemeler" nav item.

### Ledger side

A supplier account's credits are its charges. Aging and account status are party-aware: for supplier accounts the direction is flipped before the oldest-first matching. So "overdue" on a supplier account means a payable that is past due.

`/finance/summary` reports supplier payables separately, in `payable_overdue` and `payable_due_soon`, so they do not mix with customer receivables.

## Vehicles

`vehicles.supplier_id` is set from the vehicle form's supplier combobox (`supplier_uuid`; `null` clears it). The vehicle detail links the supplier; the supplier detail lists its vehicles. The vehicle list filters on `supplier`.

## Permissions (000801, `rbac.go`, `permissions.ts`)

New slugs: `tenant.suppliers.read|write|export|import|bulk.archive`.

| Role | Slugs |
|------|-------|
| owner, admin, merkez_yoneticisi, muhasebe | all |
| sube_yoneticisi | read, write, export |
| member | read |

Invoices and payments reuse the finance slugs:

| Resource | Read | Write (create, patch, void, attachment) | Export |
|----------|------|------------------------------------------|--------|
| Supplier invoices, payables summary | `tenant.finance.expenses.read` | `tenant.finance.expenses.write` | `tenant.finance.expenses.export` |
| Supplier payments | `tenant.finance.cash.read` | `tenant.finance.cash.write` | `tenant.finance.cash.export` |

Branch scope works as follows:

- Invoices and payments carry a branch. A payment takes the branch of its cash account.
- Head-office rows (`branch_id` null) need access to all branches.
- A branch-limited user sees only their branches' invoices, and allocation skips invoices they cannot see.

## API

Supplier cards (`/v1/tenant/suppliers`):

- `meta`, `facets`, list, create
- `export`, `import`, `import/sample`, `bulk` (archive)
- `{uuid}` get / patch, `{uuid}/activity`

Finance (`/v1/tenant/finance/*`):

- `payables/summary`
- `supplier-invoices`:
  - meta, list, create, export
  - `{uuid}` get / patch (number, due date, category, branch, vehicle, description)
  - `{uuid}/void`
  - `{uuid}/file`: PUT / GET / DELETE. The object key is `SupplierInvoiceAttachmentObjectKey`, in the private bucket, streamed by the API.
- `supplier-payments`:
  - meta, list, create, export
  - `{uuid}`, `{uuid}/void`
- `cash-movements` with `kind = supplier_payment` and a `supplier_uuid` is delegated to the supplier payment flow.
- `expenses` accepts `supplier_uuid` and filters on `supplier`.

All routes are documented in `backend/docs/openapi.yaml` and covered by RT-A01 (`httpserver/isolation_test.go`).

The engines are used throughout:

- **Activity:** suppliers, invoices and payments.
- **Live lists:** `tenant.suppliers`, `tenant.supplier_invoices`, `tenant.supplier_payments`.
- **I/O and bulk:** supplier export and import (matched on code, then tax number), invoice and payment exports, and bulk archive.
- **Search:** a Cmd+K adapter over title, code, tax number and city.

## UI

| Screen | Contents |
|--------|----------|
| **Tedarikçiler** | A DataTable with filters for type, city, due state and status, export and import, bulk archive, the payables card, and the supplier dialog (tabs for contacts and billing). |
| **Supplier detail** | Header figures (payable, overdue, due soon, nearest due date, open invoices) and tabs: Genel, Faturalar, Ödemeler, Cari ekstre, Araçlar, Aktivite. The statement reuses the ledger's `StatementTable`. |
| **Gider faturaları** | A DataTable plus the invoice dialog. The dialog takes either a single total or lines with a VAT breakdown, and uses `AppDatePicker` for the dates. Rows offer pay, edit, void and attachment. |
| **Tedarikçi ödemeleri** | A DataTable plus the payment dialog: cash account, amount, method, and an allocation table with an "otomatik dağıt" option. |
| **Vadesi yaklaşan ödemeler** | The invoice table prefiltered to `overdue` and `due_soon` and sorted by due date, with the summary card. |

Every string is in `suppliers.json` (tr, en).

## Tests

`internal/modules/suppliers/integration_test.go`:

| Test | Covers |
|------|--------|
| `TestInvoicePostsPayableOnceAndVoidReverses` | Posting once, re-sync is a no-op, due-date change re-accrues, void reverses. |
| `TestInvoiceLinesVATBreakdown` | Totals from lines with mixed VAT rates. |
| `TestPartialAndMultiInvoicePayment` | Partial payment, one payment over several invoices, auto allocation, advance, payment void. |
| `TestDueSoonPayables` | Due states and the summary on a fixed clock. |
| `TestSupplierOnVehicle` | Set, list filter and clear. |
| `TestTaxNumberAndCardValidation` | VKN / TCKN checksums, IBAN, contacts. |
| `TestFreeTextSupplierMigration`, `TestNewCardAdoptsFreeTextRows` | The backfill, and later adoption of free-text rows. |
| `TestBranchScopeOfPayables` | Branch scope. |
| `TestCrossOrganization` | Cross-organization isolation. |

RT-A01 covers every new route.

## Open items

- **TCKN in plain text.** A sole trader's TCKN is personal data but is stored in plain text in `suppliers.tax_number`, so it stays searchable and matchable on import. Encrypting it (with a blind index for lookups) is a follow-up.
- **Old manual supplier payments.** These movements have no supplier name and stay unlinked. They can be reassigned by voiding them and recording a supplier payment.
- **Partner accounts.** Channel partners (`collecting_party <> 'us'`) are not suppliers. Their accounts are still open in `ledger.md`.
- **TEC-722.** Supplier invoices are not matched to e-invoices (GİB) yet, and VAT is not reported.
- **Statement label.** On a supplier statement the invoice accrual shows the generic purpose label "Ek ücret" (`charge`). A supplier-specific purpose label would read better.
