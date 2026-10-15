# Rentals and operation tasks

Status: **implemented** (M3: TEC-709). This builds on reservations ([reservations.md](./reservations.md)) and the availability engine ([availability-engine.md](./availability-engine.md)). Documents are covered in [contracts.md](./contracts.md).

- Backend code: `backend/internal/modules/rentals`
  - `model`: types, statuses, ports, policy snapshot
  - `usecase`: create, lifecycle, overdue, charges, tasks, views
  - `adapters`: reservations port, io export
  - `handler` and `routes.go`: HTTP
- Engine side: `availability/usecase/rentals.go`.
- Panel: `frontend/src/features/rentals`.

## 1. Tables (migrations 000081 – 000082, seed 000084)

| Migration | Tables | Notes |
|-----------|--------|-------|
| 000081_rentals | `rentals`, `rental_vehicles`, `rental_drivers`, `rental_events`, `rental_charges` | `uq_rentals_organization_reservation`: one live rental per reservation (`status <> 'cancelled'`). `rental_vehicles` holds one leg per vehicle. `uq_rental_charges_policy_late`: one policy late fee per rental. |
| 000082_operation_tasks | `operation_tasks` | Has a partial unique index allowing one live task per rental, type and vehicle. |
| 000084_seed_rentals_contracts_rbac | permissions and template grants | See §7. |

These tables follow the same tenant rules as every other tenant table:
- `organization_id` comes first in every index.
- FKs are composite `(organization_id, id)`.
- The tables are listed in `tenant_tables.txt`.

Down migrations drop the tables. This loses every rental, leg, charge, event and task, and the down notes say so.

**Planned and actual values are kept apart.** The reservation stays the single source of the plan: planned times and places, product, price and policy snapshot. A rental reads those through a join. It stores only what physically happened:
- vehicle legs, with km and fuel out and in
- actual times and branches
- late minutes
- charges

## 2. Lifecycle

```text
               handover                return                 complete-return
preparation ───────────▶ active ───────────▶ return_check ───────────────▶ returned
     │ cancel (close, before the handover)
     ▼
 cancelled
```

**Creation.** `POST /v1/tenant/rentals { reservation_uuid, vehicle_uuid? }` needs a confirmed reservation. It fixes the plate first when needed (`FixPlate` through the reservations port). The reservation lifecycle listener also creates the rental by itself when a confirmed reservation gets a fixed plate, or when a reservation with a fixed plate is confirmed.

Creation does the following:
- Copies the drivers.
- Opens the preparation and handover tasks.
- Prepares the rental contract when an active template exists.

**Handover.** `POST …/handover { at?, branch_uuid?, point_uuid?, km, fuel, waiver_reason? }` records the delivery:
- `km` may not be lower than the vehicle odometer.
- `fuel` is 0–100.
- The engine turns the **same** allocation into kind `rental` with `StartRental`.
- An early handover moves the allocation start when the plan allows it.
- Inside the engine transaction, the vehicle becomes `rented` and its odometer and fuel are updated (`vehicles.ApplyOperationalChange`).
- The handover task completes and a return task opens.

When the organization has an active `rental_contract` template, the handover also needs that contract approved. Without approval it is refused with `contract_not_approved`, unless the request carries `waiver_reason` and the member has `tenant.signature.waive`. The waiver is stored on the rental (`contract_waiver`) and in the timeline.

**Return.** `POST …/return { at?, branch_uuid?, km, fuel, note?, damage_note? }` closes the current leg and moves the allocation end to the actual time with `SetRentalEnd`:
- The vehicle becomes `returned`.
- The return task completes and the return form is prepared.
- Late minutes are counted from the planned return.
- An **early return** frees the rest of the plan. It records the minutes and the policy text, and creates no automatic refund or fee.

**Return check.** `POST …/complete-return { vehicle_status: cleaning | available }` needs `tenant.rentals.close`:
- `cleaning` opens a cleaning task. Finishing that task makes the vehicle available.
- `available` frees the vehicle straight away.

**Cancel.** Cancelling needs `close` and is only possible before the handover. The allocation stays with the reservation, and the documents of the rental become void. A reservation that is cancelled or marked no-show cancels a rental still in preparation through the listener.

**Extension.** `POST …/extend { ends_at, reason? }` runs the reservation's own extension rule: price difference, same vehicle, `EXTENSION_CONFLICT` with alternatives. The engine keeps the started rental's start (`changedWhere` keeps `StartsAt`). Pricing a range that starts in the past is allowed only for an extension (`SearchInput.KeepStart`).

**Vehicle swap.** `POST …/swap { vehicle_uuid, at?, branch_uuid?, in_km, in_fuel, out_km, out_fuel, reason }` makes these changes:
- The current leg returns, and the old vehicle goes to cleaning with a cleaning task.
- A new leg goes out.
- `SwapRentalVehicle` shortens the old allocation and inserts a new rental allocation for the new vehicle, from the swap time to the planned end.

The rental and reservation point to the new vehicle, and live documents are refreshed (a vehicle change requires a new contract approval; see RT-A18 in contracts.md).

Every action writes a `rental_events` row and an activity entry (`rentals.*`), and publishes `tenant.rentals` (and `tenant.operation_tasks`) live events.

## 3. Engine integration

The availability engine is the only writer of `vehicle_allocations`. Rentals add three engine calls. Each runs under the component advisory lock and calls the rental `TxHook` inside the engine transaction.

| Call | Effect |
|------|--------|
| `StartRental(org, alloc, at, hook)` | Turns the hold or reservation allocation into kind `rental`. The plate is fixed, and an early start moves the range when a plan exists. |
| `SetRentalEnd(org, alloc, RentalEnd{EndsAt, ReturnBranch, Force, Hook})` | Moves the end. It is used for return, extension and the overdue push. With `Force`, if no plan can absorb the new end, it clips before the next commitment, block or transfer and returns `Affected` / `Clipped`. |
| `SwapRentalVehicle(org, alloc, RentalSwap{…})` | Shortens the old allocation and inserts the new vehicle's rental allocation. |

Guards keep a started rental fixed: release, product change, a re-plan to another vehicle and a start or pick-up change of an allocation of kind `rental` all answer `ErrRentalUnderway`, which reservations map to `rental_underway`. The planner never moves a started commitment.

## 4. Late return

- **Overdue sweep.** The worker runs it every minute, inside the `expire-holds` chain, after reservations and holds. It finds active rentals whose planned return plus the policy's grace has passed and whose allocation has ended. For each one:
  - It calls `SetRentalEnd(now + 2h, Force)`, so the vehicle stays busy (`OverdueHorizon`).
  - Later flexible commitments move to other vehicles first.
  - For any reservation that still has to be clipped, it opens `late_return` resolution tasks through `LateReturnTasks`, which are the reservations' resolution tasks.
  - The rental records `overdue_at`.
- **Fee.** The fee is calculated only from the **policy snapshot** of the reservation (`settings_version`, `late_return_grace_minutes`, `extra_hour_fee`). When late minutes exceed the grace and `extra_hour_fee` is set, the fee is `ceil(late / 60) × extra_hour_fee`. It is stored as a `proposed` charge with `source = policy` and a `calculation` JSON. Nothing is invoiced in M3.
- **Charges.**
  - Manual charges (damage, fuel, mileage, early_return, other) need a reason.
  - Approving a charge needs `tenant.rentals.close`; damage can also be approved with `tenant.damage.approve`.
  - Waiving a charge needs `close` and a note.

## 5. Operation tasks

`operation_tasks` are typed work items of a branch, also used later by the mobile handover and return flows (TEC-712 / 713).

Since M4 these rows are also the field tasks ([field-tasks.md](./field-tasks.md), migration 000300):

- Statuses are `planned`, `assigned`, `en_route`, `issue_reported`, `completed` and `cancelled` (formerly open / in_progress / done); assigning moves planned → assigned, unassigning back. Every transition is validated by one table and written to `operation_task_events` by a trigger.
- Types add `transfer` (completion moves the vehicle's branch through the fleet transfer) and `other`.
- A task has a place: the branch desk, a service point, or an address (with optional coordinates), plus contact, flight number, meeting note and a planned duration. Handover / return tasks of the rental are placed at the reservation's pick-up / return point (`placement.Locate`).
- `origin` tells who opened it (`reservation`, `rental`, `manual`). A delivery reservation opens its handover / return tasks before the rental exists; the rental's insert adopts that row (same task, assignee kept) instead of opening a second one.

| Type | Opened when | Closed by |
|------|-------------|-----------|
| preparation | rental created | staff (complete) |
| handover | rental created (due = planned pick-up) | the handover itself |
| return | handover (due = planned return) | the return itself |
| cleaning | return check with `cleaning`, vehicle swap | staff; completing makes the vehicle available |
| maintenance | return check with `maintenance` | staff; completing makes the vehicle available |
| preparation (after a return) | return check with `preparing` | staff; completing makes the vehicle available |
| deposit | mobile return form ([inspections.md](./inspections.md)), or the return check with `deposit_task` | staff |

- Handover and return tasks cannot be completed by hand (`record_handover` / `record_return`).
- Reservation changes reschedule the due times of live tasks.
- Tasks can be assigned, started, completed with a note, or cancelled with a reason.
- List: `GET /v1/tenant/operation-tasks`, with facets status, type and branch, branch scope, and live updates.

## 6. API and panel

The `/v1/tenant/rentals` routes are:
- `meta`, `facets` and the list (q, status, branch, product, vehicle, customer, reservation, overdue)
- `export` through ioengine
- `{uuid}` and `events`
- the actions above
- `charges` and `charges/{chargeUuid}/approve|waive`

Every route is in `backend/docs/openapi.yaml` and RT-A01 (`isolation_test.go`).

Panel:
- **"Kiralamalar" nav group:**
  - **Kiralamalar list:** DataTable, facets, live updates, export.
  - **Kiralama detail:** planned vs actual values, legs, drivers, charges with approve and waive, documents, tasks and timeline. Action dialogs use `AppDatePicker withTime`.
  - **Operasyon görevleri.**
- **Reservation detail:** shows the rental card (link, or "Kiralama başlat") and the documents section.

## 7. Permissions

| Slug | Grants |
|------|--------|
| `tenant.rentals.read` | list, detail, tasks |
| `tenant.rentals.write` | create, handover, return, extend, swap, propose charges, work tasks, record document approvals |
| `tenant.rentals.close` | return check, cancel, approve / waive charges |

Reused slugs: `tenant.signature.waive` (handover without an approved contract), `tenant.damage.approve` (damage charges), `tenant.document.download` (PDF).

Template grants:
- owner, admin, merkez_yoneticisi and sube_yoneticisi: everything
- teslim_iade: rentals read and write, contracts read
- rezervasyon_satis and muhasebe: rentals read, contracts read
- member: rentals read

## 8. Tests

`rentals/integration_test.go` (real Postgres) covers:
- `TestRentalLifecycle`: the rental is created when the plate is fixed; an early handover is checked against the odometer; the reservation can no longer be cancelled once the vehicle is out; an early return at another branch, the return check and the cleaning task.
- `TestLateReturn`: the next customer has a fixed plate, so the overdue push clips and opens a late_return task. The policy fee is 3 h × fee after a 30 min grace, and approving it needs `close`.
- `TestSwapAndExtend`: a mid-rental swap (legs, cleaning task, new allocation) and the extension of a started rental.
- `TestRT_A18_ContractReapproval`, described in contracts.md.

`routes_test.go` covers permission gates and live resources. RT-A01 covers every new route.
