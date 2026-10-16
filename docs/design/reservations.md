# Quotes, reservations and the provisional plate plan

Status: **implemented** (M2: TEC-704 quotes, TEC-706 reservations, TEC-698 plate plan and resolution tasks; RT-A31). Builds on the availability engine ([availability-engine.md](./availability-engine.md)), pricing ([pricing.md](./pricing.md)), extras ([extras.md](./extras.md)), campaigns ([campaigns.md](./campaigns.md)) and customers ([customers.md](./customers.md)). Code: `backend/internal/modules/reservations` (`model` = state machine, ports, types; `usecase` = service; `adapters` = io / bulk / extras / campaigns; `handler` + `routes.go` = HTTP). Panel: `frontend/src/features/reservations`.

## 1. Tables (migrations 000072 – 000075)

| Migration | Tables | Notes |
|-----------|--------|-------|
| 000072_quotes | `quotes`, `quote_lines` | A quote is an immutable price snapshot (`quote_lines` rows cannot be updated: trigger). `status` open → used / superseded; `valid_until` from `reservation.quote_validity_minutes` (default 30). |
| 000073_reservations | `reservation_sequences`, `reservations`, `reservation_drivers`, `reservation_events`, `reservation_vehicle_assignments` (+ FK `quotes.reservation_id`) | Reference `PREFIX-YYYY-NNNNNN` per organization and year (`reservation.reference_prefix`, default from the slug). `equipment_allocations UUID[]` holds the extras holds. |
| 000074_resolution_tasks | `resolution_tasks` | Partial unique index: one open task per reservation / type / source. |
| 000075_seed_reservations_rbac | permissions + template grants | See §8. |

Every table carries `organization_id` first in its indexes, composite `(organization_id, id)` FKs and is listed in `backend/internal/database/tenant_tables.txt`. Down migrations drop the tables (data loss: every quote, reservation, event and task; the down notes say so).

## 2. Quote → reservation

```text
POST /v1/tenant/quotes/search   products + availability + a price per product (nothing stored)
POST /v1/tenant/quotes          one product priced and stored (extras, coupon, manual discount)
POST /v1/tenant/reservations    { quote_uuid, customer_uuid, drivers[], request_only?, confirm_now? }
```

The quote is **re-validated server-side** when the reservation is created:

1. open and not expired → else `409 QUOTE_EXPIRED` / `QUOTE_USED`;
2. re-priced now (same product version, same total) → else the quote is superseded by a fresh one and the answer is `409 QUOTE_PRICE_CHANGED` with `data` = the new quote (the panel offers "Yeni fiyatla devam et");
3. drivers checked against the **sold version's** conditions on the pick-up day (age, licence years, licence expiry) → `422 DRIVER_NOT_ELIGIBLE` with `data` = drivers + reasons; `eligibility_override { reason }` needs `tenant.reservations.override_eligibility` and is recorded;
4. coupon redeemed (atomic campaign counter) before capacity, so a lost race on the last coupon use never holds a car;
5. capacity reserved through the engine with a **transaction hook**: the allocation and the reservation row (status, snapshot, drivers, events, first assignment) commit in the same transaction (`availability.ReserveInput.Hook`). `409 CAPACITY_UNAVAILABLE` carries the engine's alternatives;
6. physical extras held, tied to the vehicle allocation (`vehicle_allocation_uuid`); a shortage undoes the whole create (allocation released in one engine transaction, row removed, quote reopened, coupon use returned) → `409 EQUIPMENT_UNAVAILABLE`.

The quote breakdown (lines, pricing, extras, coupon, manual discount) and the **policy part of the settings version** are copied onto the reservation as an immutable snapshot (`price.snapshot`, `policy`, `settings_version`). Later rate-plan, settings or product edits never change a sold reservation.

### Delivery and collection (M4)

A quote side can be a delivery point (airport, hotel, address zone) with `pickup_delivery` / `return_delivery` details (address, district, city, coordinates, contact, flight number, meeting note). The `DeliveryQuoter` port (field tasks) checks eligibility (active delivery point, channel, minimum notice, point or branch hours and holidays, address required, district / radius) and prices each side from the point's delivery extra or flat fee. Fees are `delivery` lines (code `delivery` / `collection`) of the breakdown; a manual `delivery_fee` needs `tenant.price.override`. Problems answer per field (`pickup_point_uuid` …); search drops the price of an ineligible product. The details are stored on the quote and copied to the reservation, and changes and extensions re-check them. See [field-tasks.md](./field-tasks.md).

The lifecycle listener is a fan-out (`Listeners{rentals, fieldTasks}`); `confirmed` is also sent when a reservation is created confirmed, when a request is accepted and on reinstate, so delivery and collection tasks open as soon as the booking is firm.

## 3. State machine

```text
            submit                 approve (prepayment due)        confirm
request ───────────▶ awaiting_review ───────────────▶ awaiting_payment ───────▶ confirmed
   │                   │   │ approve / confirm (nothing due)                    │   │
   │                   │   └────────────────────────────────────────────────────┘   │
   │ cancel            │ cancel / expire          cancel / expire                    │ cancel / no_show
   ▼                   ▼                          ▼                                  ▼
cancelled          cancelled / expired       cancelled / expired             cancelled / no_show
```

`model.CanTransition(from, action, to)` is the single table (unit-tested exhaustively). Where a new reservation starts:

| Settings | Prepayment | Status | Capacity |
|----------|------------|--------|----------|
| `approval_mode = staff` | any | `awaiting_review` until `staff_review_window_minutes` | option (hold) |
| `approval_mode = auto` | due | `awaiting_payment` until `payment_hold_minutes` | option (hold) |
| `approval_mode = auto` | none | `confirmed` | firm |
| `request_only` | – | `request` | none (reserved on submit) |
| `confirm_now` (`tenant.reservations.confirm`) | – | `confirmed` | firm |

Capacity effects: confirm turns the **same** allocation from hold into a firm reservation (never consumed twice); cancel / no-show / expiry release it exactly once (`UPDATE … WHERE status = 'active'`); dependents are re-planned by the engine. Expiry: the worker sweep (`app:availability:expire-holds`, every minute) runs reservations first (`ExpireDue`: waiting reservations whose window passed → `expired`, the option released in the same transaction as the status change; a hold that lapsed at the same moment is accepted as the release), then the engine's own hold sweep, then extras. `RecordPayment` (M3 payments) moves `payment_status` and confirms an `awaiting_payment` reservation once the prepayment is covered; a payment after expiry re-validates capacity (`expired --reinstate--> confirmed`) or opens a `late_payment` task. M3 also adds `request --approve-->`, `reject` and per-channel approval protocols: see [payments.md](./payments.md) §4–§5.

Every transition writes a `reservation_events` row (from, to, action, reason, actor) and publishes a live-list event.

## 4. Changes, upgrades and extensions

`POST /reservations/{uuid}/quotes` prices a change of dates / places / product / extras as a quote with `difference` to the current total; `POST /reservations/{uuid}/change { quote_uuid, reason?, keep_price? }` applies it. Purpose is derived:

- **extension**: only the return moves later. Past lines stay as sold. The whole new range is priced to choose the band of the **new total length**; only the added days (day index > current billing days) are charged, each with its own date rule; extras are charged for the added days only; discounts are not applied to added days. Example: 3 days in the 1-3 band at 1500 extended to 5 days → the 4-6 band (1300) prices days 4 and 5 = 2600.
- **upgrade**: another product. The answer carries `breakdown.previous_conditions` (guarantee, deposit, driver rules, km) for the condition-difference view. When the products share vehicles the engine moves the allocation (`ChangeProduct`); otherwise `reserveThenRelease` secures the new unit first and releases the old one after, so the customer never loses the car.
- **change**: anything else (`ChangeDates` re-plans atomically; the old plan stays when no new one exists).

All three are atomic: capacity, plan, price snapshot, events and equipment re-hold commit together or not at all (date-change atomicity is tested). `keep_price` needs `tenant.price.override`.

`POST /reservations/{uuid}/extend { ends_at }` prices and applies an extension in one call. When the planned vehicle is committed in the extended period and the engine cannot re-plan (**RT-A31**) the answer is `409 EXTENSION_CONFLICT` with `data`:

```json
{ "reason": "extension_conflict",
  "extension_quote": { ... },
  "vehicles": [ { "vehicle": { "plate": "34 ABC 102" }, "quote": { ... } } ],
  "products": [ { "product": { ... }, "sellable": 2, "quote": { "difference": "..." } } ] }
```

Each alternative carries a ready quote: applying it is a normal `/change`. The refused request is recorded as an `extension_conflict` resolution task; the reservation stays as it was.

## 5. Provisional plate plan (TEC-698)

The sold unit is the product (guarantee: exact vehicle / variant / group criteria); the vehicle is a **plan**:

- `plan.planned_vehicle` — where the engine currently keeps the unit; it may move to keep capacity (another sale, a block, a cancellation re-plan);
- `plan.assigned_vehicle` — set once the plate is **fixed** (`plate_fixed`), promised to the customer; the engine never moves it;
- `plan.history` — `reservation_vehicle_assignments` (vehicle, product, kind planned / fixed, source `engine | staff | drag | replan | upgrade | extension`, actor, note).

`POST /reservations/{uuid}/plate { vehicle_uuid, fix, source }` moves the plan (`availability.MovePlan`): the vehicle must keep the sold guarantee and later commitments are moved or the change is refused (`409 CAPACITY_UNAVAILABLE`). `GET /reservations/{uuid}/plate-candidates` lists the vehicles the planner accepts (read-only). The fleet calendar drags a reservation bar whose plate is not fixed onto another vehicle row (`source: drag`).

Engine moves of *other* reservations during a sale or change are reported by the hook (`HookResult.Moved`) and written to their assignment history (`plan_moved`).

## 6. Resolution tasks

Operational changes over sold capacity call `AllocationsAffected` (fleetops `AffectedHandler`: a vehicle block over allocations; M3: late returns). For each reservation allocation the engine re-plans automatically (another eligible vehicle, moving flexible commitments). Success is recorded (`replanned` event + history). Failure opens a **resolution task** — the reservation is never cancelled automatically:

| Type | Source |
|------|--------|
| `block_conflict` | a block the plan could not absorb |
| `release_conflict` | a release whose dependents could not be re-planned |
| `extension_conflict` | a refused extension (RT-A31) |
| `replan_failed` | reserved for other operational sources (vehicle changes); a failed "retry re-plan" keeps the original task open |
| `late_return` | M3 |

`/v1/tenant/resolution-tasks`: list (live), resolve with a note, assign a member, retry the re-plan (success resolves the task). One open task per reservation / type / source (partial unique index).

## 7. Ports to extras, coupons and payments

Reservations own neither the extras catalog nor campaigns; they use narrow ports (`model/ports.go`) wired in `httpserver` / `cmd/worker`:

| Port | Implementation | Contract |
|------|----------------|----------|
| ExtrasPricer | `pricing.Preview` extras resolver (TEC-702) | Extras are priced inside the quote: `extras: [{extra_uuid, quantity}]` → `extra` lines and `extras_total`; channel / product prices, per-day / per-unit / per-rental, caps. |
| `EquipmentReserver` | `adapters.Equipment` → `extras.Reserve / Release` | Holds physical equipment for the reservation tied to its `vehicle_allocation_uuid`: the extras trigger then follows the vehicle allocation (hold → reservation on confirm, expiry, release), so lifecycle calls need nothing else. Non-physical extras return `uuid.Nil`. Shortage → `EquipmentError` (409 `EQUIPMENT_UNAVAILABLE`). After a change the holds are re-synced for the new allocation (best effort, warning + timeline event). |
| `CouponRedeemer` | `adapters.Coupons` → `campaigns.Redeem / Release / Consume` | `Redeem` on create (one live redemption per reservation, atomic usage counter, reason codes as `CouponError`), `Release` on cancel / expiry / no-show / failed create (usage returned per `release_on_cancel`), `Consume` when the rental completes (M3). |
| `PaymentRecorder` | `usecase.Service.RecordPayment` | M3 payments entry point (payment status, confirm after prepayment, late payments); see [payments.md](./payments.md) §4. `PaymentLinker` (payments module) creates the link of a staff accept. |

`NoEquipment` / `NoCoupons` are the defaults (tests, deployments without those modules): a coupon code is then refused.

## 8. Permissions, scope and isolation

| Slug | owner, admin, merkez_yoneticisi, sube_yoneticisi | rezervasyon_satis | teslim_iade | muhasebe | member |
|------|----|----|----|----|----|
| `tenant.reservations.read` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `tenant.reservations.write` | ✓ | ✓ | | | |
| `tenant.reservations.cancel` | ✓ | ✓ | | | |
| `tenant.reservations.confirm` | ✓ | ✓ | | | |
| `tenant.reservations.export` | ✓ | ✓ | | ✓ | |
| `tenant.reservations.override_eligibility` | ✓ | | | | |
| `tenant.quotes.read` / `.write` | ✓ / ✓ | ✓ / ✓ | | ✓ / – | |
| `tenant.resolution_tasks.read` / `.write` | ✓ / ✓ | ✓ / – | ✓ / ✓ | | |

Branch scope: a member with `branch_scope = selected` sees reservations, quotes and tasks whose **pick-up or return** branch is granted (lists, facets, detail, export, bulk "all matching"); everything else answers 404. Every `/v1/tenant` route is covered by RT-A01 (`internal/httpserver/isolation_test.go`): B's uuids on A's routes are 404, bodies naming B's quote / customer / driver / vehicle / product / branch / member are refused.

Live lists: `tenant.reservations`, `tenant.quotes`, `tenant.resolution_tasks` (resource events after every write; the fleet calendar also listens to `tenant.reservations`). Engines: export (`tenant.reservations`), bulk (`cancel` with `params.reason`, `assign_staff` with `params.user_uuid`).

## 9. Panel

Nav group **Rezervasyonlar**: Rezervasyonlar (list: facets, live, export, bulk cancel / assign), Yeni rezervasyon (wizard: search → product → customer & drivers (create customer in a drawer) → extras & coupon / manual discount → summary with the server quote → create; every 409 / 422 above is shown inline with the next step), Teklifler, Çözüm görevleri. Detail: status actions, customer (masked, reveal through the customers API) and drivers, price snapshot, vehicle plan and history, tasks, timeline; payments and documents are M3 placeholders. Fleet calendar: reservation bars link to the reservation and can be dragged to another vehicle row.

## 10. Tests

`usecase/*_test.go` (DB-backed, `TEST_DATABASE_URL`): state machine and capacity effects, expiry sweep, quote revalidation (expired / used / price changed), date-change atomicity, extension band rule, RT-A31 alternatives + task, resolution task from a block, driver eligibility + override, branch scope, live events, extras / coupon ports, and two clients racing for the last car (`-race -count=10`). `model/statemachine_test.go` covers every transition. RT-A01 covers the routes.

## 10a. Corporate agreements (TEC-723)

Quotes and reservations may be sold under a corporate agreement (`agreement_uuid`): its tariff and coupon eligibility price the sale, the reservation checks the agreement's customer, validity and permitted drivers, and an open account confirms without prepayment within the credit limit (`422 CREDIT_LIMIT_EXCEEDED`). See [corporate-agreements.md](./corporate-agreements.md).

## 11. Open items

- Payments are implemented ([payments.md](./payments.md)); documents, handover and returns are in rentals.
- Rentals (M3, [rentals.md](./rentals.md)): a `LifecycleListener` (`SetLifecycleListener`) is told about these events: `plate_fixed`, `confirmed`, `terminated`, `changed` and `drivers_changed`. The rental is created when a confirmed reservation has a fixed plate. Once the vehicle is handed over, the engine answers `rental_underway` to cancel, product change and start or pick-up changes. Documents are in [contracts.md](./contracts.md).
- Customer-facing channels (web / widget / API) reuse the same quote → reservation flow; their auth and rate limits are out of M2.
- Delivery to a point or an address is implemented (M4, [field-tasks.md](./field-tasks.md)); the wizard offers it.
