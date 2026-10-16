# Corporate agreements and long-term rentals (TEC-723, RT-A24)

Status: **implemented** (M4). Module: `backend/internal/modules/agreements` (CRUD, contacts, permitted drivers, the views). The pricing engine (`internal/platform/pricing`), pricing, reservations, rentals and finance (`usecase/billing.go`) carry the sale and billing rules. Panel: `frontend/src/features/agreements`, Müşteriler → Kurumsal anlaşmalar, and the reservation wizard.

## Data model (migration 000108)

| Table | Purpose |
|-------|---------|
| `corporate_agreements` | Code `KA-000001` (per organization, advisory-lock serialised), name, the corporate customer (`customer_type = corporate`, checked by the API), managing branch (NULL = head office, every branch), status (`draft` → `active` ↔ `suspended`, any → `terminated` with a reason), validity `valid_from` / `valid_to` (Istanbul days, both inclusive, `valid_to` NULL = open ended), invoice period (`monthly` / `per_rental`), payment term (days), open account + credit limit (a CHECK requires the limit), coupon eligibility, long-term terms (monthly km allowance, maintenance interval in days / km), currency, notes, soft delete. |
| `agreement_contacts` | Authorised contacts: a `customer_contacts` row of the agreement's customer with a role (`signatory`, `billing`, `operations`, `other`). Composite FK (migration adds `UNIQUE (organization_id, id)` on `customer_contacts`). |
| `agreement_drivers` | Permitted drivers: exactly one of a `drivers` row or an individual `customers` row (every driver record of that person). No row = anyone of the customer may drive. |
| `rate_plans.agreement_id` | Scope `agreement` (CHECK rewritten with the full list `common, branch, channel, branch_channel, agreement`; `(scope = 'agreement') = (agreement_id IS NOT NULL)`). |
| `quotes.agreement_id`, `reservations.agreement_id`, `reservations.open_account` | The sale's agreement; `open_account` = confirmed without prepayment on the agreement's open account. |
| `rentals.rent_billing` | `handover` (default) or `periodic`; fixed when the rental is created (`InsertRental`: periodic when the reservation's agreement bills monthly). |
| `rental_billing_periods` | The billing plan of a periodic rental (below). |
| `ledger_entries.billing_period_id` | The period a periodic rent accrual (and its reversal) belongs to. |

Every table carries `organization_id` first in its indexes and composite FKs, and is in `tenant_tables.txt`. Branch scope: an agreement with a branch is visible inside that branch's scope; a head office agreement to every member. Creating a head office agreement needs every branch.

The payment term is written onto the customer's ledger account (`finance.ApplyCustomerTerms`, opening the account on first use; customer accounts only, `party_kind = 'customer'`: supplier accounts keep their own terms) whenever an agreement is created or its term changes, so the account shows the agreement's terms. A customer may hold several agreements and the account keeps the last term written, so accruals of a rental sold under an agreement (whole rent, periods, charges) take their due date from that agreement's own term (`SyncRental`, `agreement_term_days`).

## Agreement tariffs (pricing scope `agreement`)

- An agreement tariff is a normal versioned rate plan (`rate_plans` + `rate_plan_versions` + `rate_bands`) with scope `agreement` and the agreement. It is created from the agreement detail (Tarifeler → Tarife ekle) or `POST /v1/tenant/rate-plans` with `scope: agreement, agreement_uuid`, and edited / activated on the rate plan page. Versioning, immutability and activation are unchanged; online coverage does not apply (staff sell agreements), the equal-priority check compares plans of the same agreement.
- Engine (`pricing.Calculate` stays pure): `Input.AgreementID`; an agreement plan matches only a sale under its agreement and outranks every public scope (`ScopeRank`: agreement 5 > branch+channel 4 > branch 3 > channel 2 > common 1). An agreement without its own plan for the product falls back to the public plans. `PlanRef.AgreementID` names it in the breakdown.
- `PricingPreviewSide.agreement` / `QuoteBreakdown.agreement` = `{uuid, code, name, tariff_applied, coupon_eligible, invoice_period}`: quotes show which agreement priced the sale and whether its tariff applied. Overriding a price in a quote stays `tenant.price.override`.
- Coupon eligibility: an agreement without it refuses every code (`coupon_corporate`) and no automatic campaign applies; with it, campaigns are evaluated with `Corporate: true`, so only `corporate_eligible` campaigns apply (`campaigns.Evaluate` / `Redeem`).

## Selling under an agreement

`POST /quotes` and `/quotes/search` take `agreement_uuid` (active, valid on the pick-up day, in the caller's branch scope). Changes and extensions of a reservation are priced under its agreement. `POST /reservations` re-checks the quote's agreement and:

- the agreement's customer must be the customer or the invoice addressee; an employee as the customer gets the agreement's customer as invoice addressee automatically (the rent lands on the corporate account);
- every driver must be permitted (`drivers.N.driver_uuid` `not_permitted`) when the agreement lists drivers;
- **open account** (`use_open_account`, default true): the reservation is confirmed without prepayment (`prepayment_amount = 0`, `open_account = true`) when `balance + pending + total ≤ credit_limit`, where `balance` is the customer's ledger balance (deposits aside) and `pending` the rent of the agreement's live reservations not accrued yet. Above the limit: `422 CREDIT_LIMIT_EXCEEDED` with the figures; `use_open_account: false` follows the normal prepayment flow. The check runs again inside the create transaction under the agreement row lock, so two sales cannot both take the last of the limit.

## Long-term rentals

- Planned end = the reservation's `ends_at`; extensions use the existing rental / reservation extension rule (price of the added days at the band of the new length).
- Periodic km allowance: each billing period carries `km_allowance = round(monthly × period days / days of that month)`. Maintenance schedule: a period is marked `maintenance_due` when a multiple of the maintenance interval (days from the rental start) falls in it.
- Vehicle swap: rentals already support it (`POST /rentals/{uuid}/swap`, one `rental_vehicles` leg per vehicle: the old vehicle's return and the new vehicle's handover are separate records). The billing plan belongs to the rental, so the agreement, the reservation and the periods continue unchanged.

## Periodic accrual (RT-A24)

**Periods.** Billing day *i* starts at `start + i·24h` (rentaltime); consecutive billing days whose Istanbul date falls in the same calendar month form one period, numbered from 1. The first and last periods are partial when the rental does not start on the 1st or end at a month end.

**Formula (pro rata by days).** With `R` the amount to spread, `D` the billing days it covers and `C_i` the cumulative days up to and including period *i*:

```
amount_i = round(R · C_i / D) − round(R · C_(i−1) / D)      (half away from zero, to the kuruş)
```

Every period gets its day share, rounding never drifts and the amounts add up to `R` exactly. Initially `R` = the reservation total and `D` its billing days. Example: 85 days from 15 January at 8 500 → 1 700 (17 days), 2 800, 3 100, 900 (9 days).

**Posting.** `rental_billing_periods` holds the plan; `SyncRental` (handover, extension, return, charge decisions, and the job) reconciles it in the caller's transaction after locking the rental row:

1. posted periods are fixed; the rest is re-planned from the end of the last posted period to the planned end with `R = total − posted`;
2. an early return at billing day `k` cuts the plan: periods starting at or after `k` are reversed, the period containing `k` is superseded by a new revision with `round(amount · kept days / days)`;
3. planned rows are rewritten only when the plan changed;
4. every posted period plus every planned period that started by today (rental under way) is a desired accrual, key `agreement:<agreement id>:r<rental id>:p<n>`, entry date = period start, due = start + the agreement's payment term. The existing reconciliation reverses and re-accrues an amount change (a superseded period) and reverses what is no longer due (a reversed period).

Posted rows are history: a trigger refuses changing their figures or deleting them (only `posted → superseded / reversed`); planned rows never touched the ledger and are replaced freely.

**Never twice.** The single-accrual rule of the ledger (RT-A23: insert trigger + `uq_ledger_entries_accrual`) refuses a second live accrual of a period key; the rental row lock serialises concurrent runs; posted periods are skipped. Periods sum to the contract total, so periods and the total can never both be billed:

- the handover accrual (`rental:<id>:rent`) is only desired for `rent_billing = handover`;
- the database refuses it anyway: `ledger_entries_rent_billing` rejects a rent accrual without `billing_period_id` on a periodic rental, one with a period on a handover rental, and one whose period belongs to another rental or key;
- `rentals_rent_billing_fixed` refuses changing `rent_billing` while the rental has a live rent accrual.

**Job.** `app:agreements:accrue-periods` (worker, hourly at :17, `finance.AccruePeriods`): every periodic rental under way or returned whose plan is missing or has a due period is reconciled, one transaction each; failures are logged and retried next hour.

**Vehicle profitability (TEC-727).** The report reads the period accruals like any rent accrual: on the accrual basis each posted period counts in its own month (a period reversal counts on the reversed accrual's date), on the cash basis the collections count; a periodic rental has no handover accrual, so nothing is counted twice ([vehicle-profitability.md](./vehicle-profitability.md)).

**Invoicing (TEC-722).** Not built here: `GET /agreements/{uuid}/billing` exposes each posted period's accrual entry (`entry_uuid`, entry / due dates) for invoices to link through `ledger_entry_links`.

## API (`/v1/tenant/agreements`)

`meta`, `facets`, list (q, status, customer, branch, invoice_period, validity, valid_from / valid_to window; sort code / name / customer / status / valid_from / valid_to / created_at), create, `export` (io engine), `{uuid}` get / patch / delete (draft without sales), `{uuid}/status`, `{uuid}/contacts` (+ `DELETE …/{agreementContactUuid}`), `{uuid}/drivers` (+ `DELETE …/{agreementDriverUuid}`), `{uuid}/tariffs`, `{uuid}/rentals`, `{uuid}/billing?rental=`, `{uuid}/statement`, `{uuid}/activity`. Rate plans gain `agreement_uuid` / the `agreement` filter; previews, quotes and searches `agreement_uuid`; reservations `use_open_account`, `agreement`, `open_account`.

Permissions (migration 000109, `rbac.go`, `permissions.ts`): `tenant.agreements.read` (owner, admin, merkez_yoneticisi, sube_yoneticisi, muhasebe, rezervasyon_satis, teslim_iade, member), `.write` (owner, admin, merkez_yoneticisi), `.export` (+ sube_yoneticisi, muhasebe). Tariffs need `tenant.pricing.write` / `activate`.

Activity `agreements.created / updated / deleted / status_changed / contact_added / contact_removed / driver_added / driver_removed`; live list `tenant.agreements`.

## UI

- **Kurumsal anlaşmalar**: DataTable with status, customer, validity, invoice period and branch facets, a validity window (`DatePicker`s), export, live updates, create / edit drawer.
- **Detail** tabs: Genel bakış (terms, payment conditions, open-account exposure, activity), Tarifeler, Sürücüler ve yetkililer, Kiralamalar, Dönemler / faturalama planı, Ekstre (running balance, link to the ledger account).
- **Reservation wizard**: a corporate customer's active, valid agreements are offered (one is preselected); the quote summary shows the agreement, whether its tariff applied and the period plan; the open account switch; a credit-limit refusal offers "Ön ödemeli devam et". The reservation detail links the agreement.

## Tests

- `internal/platform/pricing/agreement_test.go`: agreement scope precedence, the period plan (partial first / last), no rounding drift, truncation, km proration.
- `internal/modules/agreements/integration_test.go` (real Postgres): CRUD and account terms; the agreement tariff beats the public tariff (and the quote's plan matches the engine); coupon eligibility; the open-account credit limit; agreement customer / drivers / suspended agreement; **RT-A24** (no whole-rent accrual at the handover, the job twice, eight concurrent runs, an extension re-planning only the future, every period key once, totals); early termination (reversed and superseded periods, posted rows immutable); vehicle swap keeps the agreement and billing running; handover vs periodic exclusivity in the database; branch scope and cross-organization isolation.
- `internal/httpserver/isolation_test.go` (RT-A01): every new route and every body naming an agreement.

## Open items

- Invoices per period (TEC-722) link to the exposed entries; nothing is invoiced here.
- An extension of an open-account reservation does not re-check the credit limit.
- Km overage at period end and maintenance tasks are not generated automatically; periods only carry the allowance and the maintenance marker.
- Agreement search documents (Cmd+K) and bulk actions are not provided.
