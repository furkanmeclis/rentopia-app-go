# Extras and equipment stock (TEC-702, RT-A13)

Status: **implemented** (M2). Code: `backend/internal/modules/extras` (`usecase` = catalog, prices, stock, allocations, resolver; `handler` + `routes.go` = HTTP). Migrations 000068 (catalog + prices), 000069 (stock + allocations + trigger), 000071 (permissions).

## 1. Catalog

`extras` (one row per extra, soft delete, code unique per organization, case-insensitive):

| Field | Meaning |
|-------|---------|
| `extra_type` | `additional_driver`, `child_seat`, `km_package`, `delivery`, `other`. An additional driver is only the sellable line; the driver's identity binds to the contract later (drivers come from TEC-705). |
| `charge_type` | `per_day` = unit × quantity × billing days; `per_unit` = unit × quantity (once); `per_rental` = unit once (quantity is always 1, CHECK) |
| `min_quantity` / `max_quantity` | bounds of a selection (1..100); default quantity = min |
| `max_total` | cap of the line total (NULL = none), e.g. "150 TL/gün, en fazla 1 000 TL" |
| `channels` | sales channel codes (orgsettings); NULL = every channel |
| `status` | `draft` / `published` / `archived`; only published extras are sold (resolver) |
| `is_physical` | physical equipment with branch stock (§3) |

## 2. Prices (effective-dated, snapshot-able)

`extra_prices`: one row per (extra, rental product) and the **default row** (`rental_product_id NULL`) used when the product has no row. Rows are `[valid_from, valid_to)`; rows of one extra and product never overlap (`EXCLUDE … tstzrange`).

Rows are **immutable** (trigger): a price change from `effective_from` (default now, never in the past) closes the row effective then (`valid_to = effective_from`), drops rows that would only start later (they were never quoted), and inserts the new row. A quote records `pricing.Line.PriceID` (the row uuid) and `ExtraID`, so a reservation can always show the exact price it was sold with. This is the effective-dated variant of the versioned-record pattern of `docs/design/pricing.md`: prices of an extra are many small independent rows (one per product), so a head/draft/activate cycle per extra would add a step without protecting anything a closed row does not already protect.

API: `GET /v1/tenant/extras/{uuid}/prices` (default, every product with `source` = `product` / `default` / `none`, scheduled rows), `PUT …/prices` (only the listed rows), `GET …/price-history`.

## 3. Equipment stock and allocations

```text
extra_branch_stock      (organization_id, extra_id, branch_id) UNIQUE, quantity
equipment_allocations   extra_id, branch_id, quantity, kind hold|reservation|rental,
                        status active|released|expired, period tstzrange [starts_at, ends_at),
                        expires_at (holds), vehicle_allocation_id, ref_type/ref_uuid
```

**Invariant:** for every (extra, branch) and every instant, the quantities of the active, unexpired allocations covering it sum to at most the branch stock.

**Concurrency (counter + row lock + sweep line).** `ReserveEquipment` runs in one transaction:

1. `SELECT quantity FROM extra_branch_stock … FOR UPDATE` — the serialisation point of every writer of that extra at that branch (no row = stock 0 = refused);
2. read the active allocations overlapping `[starts_at, ends_at)` (lapsed holds excluded) and compute the **peak** concurrent quantity with a sweep line over the clipped starts / ends (ends sort before starts at the same instant: half-open ranges);
3. `peak + quantity > stock` → `*StockError` (`409 EQUIPMENT_UNAVAILABLE`, `data` = stock / available / requested), nothing written; otherwise insert.

A concurrent second writer blocks on step 1 until the first commits; under READ COMMITTED its step-2 statement takes a new snapshot and sees the first writer's row, so the last unit can never be sold twice. Releases, expiries and confirmations only lower or keep consumption and need no lock. A stock change takes the same row lock (upsert). `TestRT_A13_LastChildSeatConcurrency` (2 and 8 goroutines, the winner pauses between check and insert) gets exactly one success; `TestRT_A13_WithoutLockOversells` is the control experiment: with the lock disabled every worker passes the check and the seat is oversold. Both run with `-race -count=20`.

Compared with vehicles (availability engine) this is deliberately simpler: units of one extra at one branch are interchangeable, so there is no plan to find and no reassignment — a counter checked under a lock is complete and exact.

**Stock below commitments** (a seat broke): recorded, not refused (an operational fact must always be recordable, as with vehicle blocks), and reported as a `below_committed` warning; new sales are refused until stock is back.

**Holds** expire lazily (they stop counting at `expires_at`) and the worker's minute sweep marks them `expired` (`app:availability:expire-holds` now also runs `extras.Service.ExpireDue`). Default hold 30 minutes, max 1 440.

### Service API (TEC-706 builds on it)

| Method | Effect |
|--------|--------|
| `Reserve(org, ReserveInput{ExtraUUID, BranchUUID, Quantity, StartsAt, EndsAt, Kind hold\|reservation, HoldMinutes, VehicleAllocationUUID?, RefType/RefUUID}, actor)` | ReserveEquipment (above) |
| `Confirm(org, uuid, {RefType, RefUUID})` | ConfirmEquipment: hold → reservation on the same row (`ErrHoldExpired` / `ErrNotHold` / `ErrNotActive`) |
| `Release(org, uuid, reason, actor)` | ReleaseEquipment: frees exactly once (`ErrNotActive` afterwards) |
| `Availability(org, {BranchUUID, StartsAt, EndsAt, ExtraUUIDs})` | stock, reserved / held peak, available per physical extra (read-only) |
| `ExpireDue(ctx)` | sweep of lapsed standalone holds |

HTTP: `POST /v1/tenant/extras/availability`, `GET|POST /v1/tenant/equipment-allocations`, `GET|DELETE …/{uuid}`, `POST …/{uuid}/confirm`. Creating, confirming and releasing allocations needs `tenant.availability.hold` (the same sales action as a vehicle hold) plus `tenant.extras.read`.

### Tied to an availability hold

With `vehicle_allocation_uuid` (an active availability hold or reservation, inside the caller's branch scope) the equipment takes the vehicle allocation's kind and expiry, defaults to its pick-up branch and range, and **follows it**. The trigger `vehicle_allocations_cascade_equipment` (migration 000069) does this in the same transaction as the vehicle change, whichever code path makes it (Release, Confirm, ExtendHold, the expiry sweep, a future M3 path):

- vehicle allocation released / expired → its active equipment rows get the same status (`release_reason` `vehicle_allocation_released` / `…_expired`);
- hold → reservation → equipment holds become reservations (`expires_at` cleared, `ref` copied);
- hold expiry moved → equipment holds get the same `expires_at`.

Reserve locks the vehicle allocation row `FOR SHARE` while linking, so a concurrent release either waits and then releases the new equipment too, or is seen by the reserve (refused with `not_active`). Lock order is stock row → vehicle allocation row on one side and vehicle allocation row → equipment rows (never the stock row) on the other, so there is no cycle. `TestEquipmentFollowsVehicleAllocation` drives it through the real availability service.

## 4. Pricing integration

`usecase.Service.Resolve(org, ResolveInput{ProductID, Channel, Items, At})` turns selections into `pricing.Input.Extras`: the product's price row effective at the quote instant, else the default row; `PerDay` for `per_day`; quantity 1 for `per_rental`; `MaxTotal` = `max_total`. Problems are field details `extras.N.extra_uuid` / `extras.N.quantity`: `not_found`, `not_available` (not published), `channel_not_allowed`, `out_of_range`, `duplicate`, `no_price`.

The engine (`internal/platform/pricing`) caps an extra line at `MaxTotal` (`Line.Capped`) and reports `UnitPrice`, `ExtraID`, `PriceID`; VAT is split per line as before. `POST /v1/tenant/pricing/preview` accepts `extras: [{extra_uuid, quantity}]` (pricing adapter `NewExtras`, wired with `SetExtrasResolver`).

## 5. Permissions (migration 000071)

| Slug | owner / admin / merkez_yoneticisi | sube_yoneticisi | rezervasyon_satis, muhasebe, member | teslim_iade |
|------|---|---|---|---|
| `tenant.extras.read` | ✓ | ✓ | ✓ | ✓ |
| `tenant.extras.write` (catalog, prices, stock, bulk) | ✓ | ✓ (stock) | – | – |

Stock rows and allocations follow the member's branch scope (an out-of-scope branch is `not_found`, an out-of-scope allocation `404`).

## 6. Open items

- TEC-706 calls `Reserve` / `Confirm` / `Release` from the reservation flow and stores the breakdown (with `price_id`).
- Additional drivers bind to the contract with TEC-705 drivers.
- Import of an extras price list (product code, extra code, price) is not built; export of the catalog is.
- Allocations do not move between branches (equipment transfers) yet.
