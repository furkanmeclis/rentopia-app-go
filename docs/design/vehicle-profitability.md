# Vehicle profitability report (TEC-727)

Status: **implemented** (M4, §11.5). Per vehicle and period: what the vehicle earned, what it cost, its share of the shared overhead and the operating contribution, on an accrual or a cash basis. It reads the finance foundation ([ledger.md](./ledger.md), [collections.md](./collections.md), [deposits.md](./deposits.md), [cash-accounts.md](./cash-accounts.md), [suppliers.md](./suppliers.md)), rentals and their legs ([rentals.md](./rentals.md)), tolls ([tolls.md](./tolls.md)) and the fleet tables. It owns no table and writes nothing.

| Area | Where |
|------|-------|
| Queries | `backend/internal/database/queries/profitability.sql` |
| Engine (attribution, pure) | `backend/internal/modules/profitability/usecase/engine.go`, `math.go` |
| Service (facts, filters, scope) | `backend/internal/modules/profitability/usecase/service.go`, `filter.go` |
| HTTP, routes | `backend/internal/modules/profitability/handler`, `routes.go` |
| Export (io engine) | `backend/internal/modules/profitability/adapters/io.go` |
| Panel | `frontend/src/features/profitability` (fleet page, vehicle tab) |

## 1. Choice: query time, no cache table

The report is computed on each request: a handful of period-bounded queries (every one starts with `organization_id`) load the facts, and Go attributes them. Nothing is materialised, so there is nothing to invalidate and nothing to keep tenant-safe besides the queries themselves. Migration 001100 adds the four indexes the period scans needed: `ledger_entries (organization_id, entry_date)`, `rental_vehicles (organization_id, out_at)`, maintenance done with a cost by date, completed transfers with a cost by arrival. The down migration drops them and loses no data.

The engine works on all vehicles of the organization for the whole period, then selects the rows. A vehicle's figures therefore never depend on the row filters (branch, rental group, search): the overhead pools and the revenue / rental-day weights are always the full fleet.

## 2. Period, buckets, money

- `from` / `to` are Europe/Istanbul calendar days (default: the first of this month to today; capped at 366 days).
- The period is cut into calendar months (the first and last may be partial). Every rule below runs per month; the period figures are the sum of the months, and the monthly trend is those months.
- Money is TRY and **VAT inclusive**, as the ledger and the expense totals record it (the ledger has no VAT split of the rent; see open items). Splits use integer kuruş with the largest-remainder method, so parts always add up to the whole.

## 3. Income

Only rental-linked ledger entries count. The rental of an entry is the entry's own, the reversed entry's, its collection document's, or the live rental of its reservation (an online prepayment posted before the rental existed).

**Accrual basis** — accruals (debit) dated in the period minus reversals of accruals dated in the period:

| Accrual | Class |
|---------|-------|
| purpose `rent` | split into **rent** and **extras**: extras = the rent accrual × (extra, one-way and delivery lines of the quote snapshot + the extras of every extension) ÷ the reservation total |
| charge kind `toll` | **pass-through** (kept apart) |
| charge kind `toll_fee` | **toll fees** (income) |
| any other charge (fuel, km, late, damage, equipment, early return, other) | **charges** |

**Cash basis** — per rental, collections dated in the period (purpose ≠ deposit) minus their voids minus refunds dated in the period. That net cash is classified like the rental's billing: it is split across rent / extras / charges / toll fees / pass-through in proportion to the rental's live (unreversed) accruals of all dates. A rental that billed nothing books its cash as rent. Deposit deductions are captured as `collection` entries (purpose charge), so they count as cash income when captured.

**Gross income** = rent + extras + charges + toll fees. Pass-through tolls are never income.

### Vehicle swap

A rental's income (and every other rental-level amount: commissions, refunds, deposits, pass-through) is split across its vehicles by **time on each leg** (`rental_vehicles`): a returned leg counts to its return, an open leg to the later of now and the planned return. A rental not handed over yet belongs to its current vehicle. The split is the whole rental's, not the period's: a rental that swapped after the period still splits its March accrual by its final legs.

## 4. Commissions

- **Payment commission:** the provider commissions of the rental's live collections (`collections.commission_amount`, online payments). The matching `payment_commission` expense (written by the settlement) is excluded from the overhead, so it is counted once.
- **Channel commission:** `reservations.commission_amount` when we collect (`collecting_party = us`). Channel-collected reservations post no accrual (ledger open item), so they show no income and no commission.

Both follow the rental's income: commission in a month = commission × the rental's gross income in that month (basis) ÷ the rental's billed gross income (all dates). A reversal month gives a negative commission.

## 5. Costs

Direct costs of the vehicle:

| Source | Accrual basis | Cash basis |
|--------|---------------|------------|
| Expenses naming the vehicle (recorded, not void) | expense date | expense date |
| Supplier invoices naming the vehicle (their expense row) | invoice date | the posted supplier payments allocated to the invoice, by payment date |
| Maintenance records done with a cost (`vehicle_maintenance`) | done date | done date |
| Completed transfers with a cost (fleet ops) | arrival day | arrival day |

Expenses of category `vehicle_procurement` are an investment: shown apart as **capex**, never an operating cost. The direct expenses are also broken down by category on the vehicle tab.

### Shared overhead

Expenses that name no vehicle (and are not procurement or provider commission) are shared overhead, allocated **per month**:

- an expense of a branch goes to the vehicles whose **home branch** is that branch;
- a head-office expense (no branch) goes to the whole fleet;
- only vehicles in service in that month are in a pool.

Methods (`allocation`):

| Method | Weight of a vehicle in the month |
|--------|----------------------------------|
| `rental_days` (default) | rented time |
| `vehicle_count` | 1 |
| `revenue` | gross income (negative counts as 0) |

When a pool has no weight under the chosen method (nobody rented, no income), that pool is shared by vehicle count and the report says so (`method.allocation_fallback`). A pool without any vehicle in service stays unallocated (`method.unallocated_overhead`, not part of any row).

**Operating contribution** = gross income − commissions − direct costs − allocated overhead. Margin = contribution ÷ gross income.

## 6. Kept apart

Shown in their own columns, never income or cost:

- refunded collections (in the cash basis the net cash is already net of them),
- deposits held and returned (`deposit_hold`, `deposit_return`, older deposit-purpose collections / refunds),
- pass-through tolls (re-billed to the customer; the processing fee is income),
- vehicle procurement (capex).

Fines have no charge kind or table yet; when they arrive they belong with the pass-through class.

## 7. Occupancy and days

Per vehicle and month, clipped at now:

- **service time** = the month ∩ the service life (`in_service_date`, else the creation day, to `out_of_service_date` / deletion);
- **available** = service time − usage blocks (`vehicle_blocks`: maintenance, damage, cleaning, out of service, other; an open block runs to the end);
- **rented** = the union of the vehicle's legs inside the service time, minus blocked time;
- occupancy = rented ÷ available; rental days = rented ÷ 24 h. Maintenance records carry only a date, so they do not reduce availability (blocks do).

Totals: Σ rented ÷ Σ available.

## 8. Scope and permissions

- Rows: vehicles whose home **or** current branch is in the caller's branch scope (the vehicles rule), that were in service or moved money in the period.
- The `branch` filter selects by **home** branch (the branch that owns the vehicle's costs); an unknown branch or one outside the scope answers 400 `not_found`. `rental_group` filters by group membership.
- Overhead visibility: a member with all branches gets head office + every branch overhead (`method.overhead_scope = all`). A member limited to some branches gets only their branches' overhead and no head-office share (`branches`); the method notes say so.
- A vehicle of another organization or outside the scope answers 404 on the vehicle route.
- Permissions reuse the existing slugs: `tenant.finance.reports.read` (owner, admin, merkez_yoneticisi, muhasebe, sube_yoneticisi) reads; `tenant.report.export` (with read) exports. No new slug, so no migration, `rbac.go` or `permissions.ts` change.

## 9. API

```text
GET  /v1/tenant/reports/vehicle-profitability                 from, to, basis, allocation, branch, rental_group, q
GET  /v1/tenant/reports/vehicle-profitability/vehicles/{uuid} from, to, basis, allocation
POST /v1/tenant/reports/vehicle-profitability/export          io engine (pdf, xlsx, csv, json); query = the report filters
```

The report answers `method` (basis, allocation, timezone, currency, VAT inclusive, overhead scope, fallback, unallocated overhead), `rows` (vehicle + figures), `totals` (sum of the rows), `trend` (one point per month). The vehicle route adds the expense categories. The export has one row per vehicle plus a totals row; the period, basis and allocation are in every row (CSV) and in notes above the table (PDF landscape, XLSX, JSON). The io engine got two optional, additive dataset fields for that (`Notes`, `Landscape`).

## 10. Panel

- **Finans → Araç kârlılığı** (`/t/{slug}/finance/profitability`): period preset / custom days, basis toggle, allocation, branch and rental group filters; KPI tiles; "Aylık gelir ve gider" (income vs total cost per month) and "Araç bazında katkı" (best and worst five, diverging blue / red) charts; the per-vehicle DataTable with sortable KPI columns (secondary columns hidden by default), the totals row, export and the method notes. A plate opens the vehicle's Kârlılık tab.
- **Vehicle detail → Kârlılık** (replaces the "Gelir / gider" placeholder; `?tab=finance` still lands on it): the same filters (last 12 months by default), KPIs, trend, the income / cost / kept-apart breakdown table and the notes.

Chart colours come from a validated palette (light and dark steps, CVD-checked per chart).

## 11. Tests

`internal/modules/profitability/integration_test.go` (real Postgres) seeds a March 2026 fixture with known numbers: three vehicles in two branches, one swap (4 + 2 days), extras in the quote, fuel and a reversed late fee, a toll (pass-through) and its fee, a desk collection partly refunded, an online collection reached through the reservation with a provider commission (and its commission expense), a cash deposit held and returned, a direct expense, a supplier invoice on a vehicle paid in part, a maintenance cost, a transfer cost, procurement, a void and an out-of-period expense, branch and head-office overhead, and a maintenance block.

- `TestProfitabilityBasesAndAllocations`: every income, commission, cost, overhead, contribution and kept-apart figure of every vehicle to the kuruş, for both bases × three methods; totals = sum of rows; the overhead allocated in full.
- `TestProfitabilityOccupancyAndDetail`: rental / available days and occupancy with the block, totals, the vehicle tab (categories, trend), filters (group, branch, search), a two-month trend, invalid filters.
- `TestProfitabilityBranchScope`: a branch member sees its vehicles only, no head-office overhead, 404 / not_found outside the scope.
- `TestProfitabilityCrossOrganization`: another organization's vehicles, branches, groups and money never reach the report.
- `TestProfitabilityExport`: rows + totals row, method in every row and in the notes, CSV / XLSX / PDF / JSON render.
- Unit tests (`usecase/math_test.go`): largest-remainder split, rounding, intervals, month buckets, swap shares with an open leg, fallback and unallocated pools. `routes_test.go`: permission gates and validation.
- RT-A01 (`httpserver/isolation_test.go`): the vehicle route with B's vehicle (404), the report probed with B's branch / rental group (not_found) and leak-scanned, the export file scanned.

## 12. Open items

- **VAT:** income is VAT inclusive (the ledger has no VAT split of the rent); expenses use their totals for consistency. An ex-VAT view needs the e-invoice lines (TEC-722).
- **Channel-collected reservations** post no accrual (ledger open item, TEC-721 partner accounts), so their income and channel commission are missing from the report until partner accounts exist.
- **Maintenance records vs expenses:** maintenance costs and expenses are not linked yet; recording the same work in both counts it twice. Link `vehicle_maintenance.expense_id` when the maintenance flow writes expenses.
- **Fines:** no charge kind yet (pass-through class when added).
- Collections not linked to a rental or reservation (plain account credit) are not attributed to a vehicle.
- PDF headers are single-line and truncate long labels on the landscape page; the XLSX / CSV carry full labels.
