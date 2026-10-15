# Availability and capacity engine

Status: **implemented** (TEC-697, M2; RT-A03 – RT-A06). One engine answers "can we sell this?" for the panel, the website, the widget, the public API and the sales channels. Inputs: [fleet-availability-inputs.md](./fleet-availability-inputs.md) (blocks, transfers, `LocationAt`, one-way rules). Code: `backend/internal/modules/availability` (`engine` = pure planner, `usecase` = service, `handler` + `routes.go` = HTTP).

## 1. Model

**A commitment always carries a provisional plate plan.** Every sold unit is a row in `vehicle_allocations` that names a concrete vehicle, even when the customer was promised only a group ("Kompakt SUV veya benzeri"). The set of active commitments is therefore satisfiable *by construction*: the plan is the witness. Selling means finding a plan for the new unit (possibly moving flexible commitments to other vehicles) and writing it atomically.

```text
vehicle_allocations (migration 000063)
  organization_id, uuid, vehicle_id                 -- the plan (composite FKs)
  rental_product_id, product_version                -- what was sold; the guarantee of that immutable version
  kind      hold | reservation | rental             -- rental = M3 (handed over)
  status    active | released | expired             -- only active consumes capacity
  starts_at, ends_at, prep_minutes
  period    tstzrange [starts_at, ends_at + prep)   -- prep = return branch prep_minutes (default 60)
  pickup_branch_id, return_branch_id, pickup_point_id, return_point_id
  fleet_pool                                        -- pool of the vehicle at sale time (pinned)
  channel                                           -- sales channel (orgsettings)
  plate_fixed                                       -- promised plate: never reassigned
  expires_at                                        -- holds only
  ref_type, ref_uuid                                -- related record (TEC-706 reservation, M3 rental)
  EXCLUDE USING gist (organization_id WITH =, vehicle_id WITH =, period WITH &&)
          WHERE (status = 'active') DEFERRABLE INITIALLY IMMEDIATE
```

| State | Consumes capacity | Notes |
|-------|-------------------|-------|
| quote (search) | no | `POST /v1/tenant/availability/search` writes nothing |
| hold ("opsiyon") | yes, until `expires_at` | default: org `reservation.payment_hold_minutes`, else 30 min (max 1440) |
| confirmed reservation without plate | yes | `plate_fixed = false`: the engine may move it |
| plate assigned | yes, once | `AssignPlate` re-plans the *same row* onto the vehicle and fixes it |
| released / expired | no | leaves `active` exactly once (`UPDATE … WHERE status = 'active'`) |

Preparation time: `branches.prep_minutes` of the **return** branch (already present, default 60). It extends `period`, so the next sale on that vehicle can start only after `ends_at + prep`.

## 2. Why double selling is impossible, and why it is concurrency-safe

Three layers, each sufficient for its own invariant:

1. **EXCLUDE (database).** Two active allocations of a vehicle can never overlap, whatever the application does. A concurrent writer that loses gets `23P01` (immediately, or at commit when the constraint is deferred) and the service retries with fresh data.
2. **Guard trigger (database).** `vehicle_allocations_guard` refuses an allocation inserted / moved onto a deleted or inactive vehicle, or overlapping a live block or an open transfer of that vehicle (same `23P01`, so it is a retry, not a 500). Status changes do not fire it: releasing an allocation that a later block overlaps always works.
3. **Component lock (application).** Writers of one eligibility component (§4) serialise on `pg_advisory_xact_lock`. Inside the lock the service re-reads locations, blocks, transfers and allocations, expires due holds, plans and writes. Reassignment updates are optimistic (`UPDATE … WHERE id = $1 AND vehicle_id = $from AND status = 'active'`): a row that moved since it was read makes the plan stale (retry).

The lock is an optimisation for throughput and plan quality, not the correctness argument: `TestRT_A04_LastUnitConcurrency/exclude-only` runs the last-unit race with the lock disabled and still gets exactly one winner (EXCLUDE + retry). Satisfiability never depends on the lock either, because every commitment always holds a valid vehicle (the witness).

**Blocks and transfers are not mirrored into allocations.** They stay in `vehicle_blocks` / `vehicle_transfers` (own EXCLUDEs, TEC-694/695) and are *hard constraints* for the planner. They are deliberately not refused by sales: an operational fact (the car broke) must always be recordable. A block created over an allocation reports it in `affected[]` (kind `hold` / `reservation`) and the allocation stays until someone (TEC-706) re-plans it. This is serialisable: a sale checks blocks (trigger + planner) but a block never checks sales, so a concurrent sale and block are equivalent to "sale, then block" — a state the system allows anyway. Mirroring blocks into the allocation EXCLUDE would have made "Bakıma al" fail whenever a hold existed; making transfers refuse allocations would create write skew (each side checking the other without a shared lock).

## 3. Planning

`engine.State` holds, for one component: vehicles (pool, current branch, service window, blocks / open transfers as segments), `Eligible[key]` (vehicles that keep each product version's guarantee, `fleet/eligibility.SatisfiesGuarantee`) and the active commitments.

**Constraints a placement must satisfy** (`State.Verify` checks all of them):

- the vehicle keeps the commitment's guarantee (the version it was sold with), is in its pool, and is in service for the whole customer range (`in_service_date` 00:00 – `out_of_service_date` 24:00, Türkiye);
- no overlap with another commitment (`period`), a block or an open transfer — **continuous availability**: one vehicle covers the whole range, never "a free car each day";
- **location continuity**: walking the vehicle's timeline from `current_branch_id`, every future commitment starts at its pick-up branch and every planned transfer at its origin; each commitment moves the vehicle to its return branch, each transfer to its destination (RT-A05: picked at A, returned at B → after preparation sellable at B, not at A). A vehicle is never at another branch "by itself".

Commitments already broken by later operational facts are *affected*: a plan never creates a new violation and is never required to repair an old one (except the re-planned commitment itself, which must end up valid).

**Placement and the augmenting-path search** (`planner.place`):

1. Candidates = eligible vehicles that fit in time, ordered by (number of conflicting commitments, flexibility = how many on-sale products accept the vehicle, id). A broad product therefore prefers the Clio over the Egea, keeping the Egea for the Egea-guaranteed product.
2. A candidate without conflicts and without continuity breaks is taken.
3. Otherwise the conflicting commitments (and commitments *stranded* by the new return branch, or the latest earlier one-way commitment that moved the vehicle away) are lifted off the vehicle and placed recursively — an augmenting path whose nodes are commitments. Commitments with a promised plate, commitments already under way and commitments already in the current chain never move.
4. Limits: `MaxDepth = 4` commitments per chain, `MaxExpansions = 4000` candidate tries per call. Every intermediate step is checked, and the final plan is verified on every touched vehicle.

Complexity: one expansion costs O(E·k) (E eligible vehicles, k commitments per vehicle) plus a continuity walk O(k log k); a call is bounded by `MaxExpansions`. The general problem (interval scheduling with machine eligibility and location chains) is NP-complete, so the search is **sound but not complete**: it never accepts an unsatisfiable state, and it may refuse a feasible request in rare shapes. The engine property test (`engine/property_test.go`, 10 000 random operations per run, brute force over every assignment as the oracle) measures 0.4–0.7 % of refusals that brute force could have satisfied (threshold 1.5 %); the known gap is relocation through another commitment (moving a one-way sale onto a vehicle only to bring it to the requested branch).

**Capacity** (`State.Capacity`) places identical copies of the request one by one with the same planner Reserve uses, so search and reserve agree. `physical = free + reassignable`.

**Release** removes the commitment, then re-plans the commitments of that vehicle that relied on its return branch (moving them, possibly moving others); what cannot be repaired is returned as `affected` (cancel always succeeds).

## 4. Components and locking

Eligibility graph: bipartite (product version ↔ vehicle). Two vehicles are in the same component when some version accepts both (all versions of the organization's products, live or not, because active allocations keep the version they were sold with). Computed with union-find; component id = lowest vehicle id of the component. Every operation touches exactly one component (a product's eligible vehicles are connected through that product), so one advisory lock per transaction, no lock ordering, no deadlocks. Lock key = FNV-64 of `availability:{org}:{component}`; collisions only over-serialise.

Cache: the graph (vehicles' static facts, guarantees, eligible sets, components) is cached per organization in process and rebuilt when `GetAvailabilityFingerprint` changes (count + max `updated_at` / `created_at` of vehicles, products, versions, rental groups and group criteria) or after 2 minutes (platform catalog edits). A stale graph is safe (§2): locations, blocks, transfers and allocations are always re-read inside the lock, the trigger and EXCLUDE refuse bad writes; staleness can only cost a retry or a missed reassignment. Exclusion violations invalidate the organization's cache before retrying.

## 5. Service API (TEC-706 builds on it)

`usecase.Service` — every method takes the organization id; branch scope from `orgctx` (allocation visible when its pick-up **or** return branch is in the caller's scope).

| Method | Effect |
|--------|--------|
| `Search(org, SearchInput)` | capacity + breakdown per product on sale (read-only repeatable-read snapshot; due holds no longer count) |
| `Reserve(org, ReserveInput{Kind: hold\|reservation, Where, ProductUUID, VehicleUUID?, HoldMinutes?, RefType/RefUUID, ActorID})` | one transaction under the component lock: expire due holds → plan → channel rules → write moves + allocation. `CapacityError{Reason, Alternatives}` otherwise (nothing written). EXACT_VEHICLE products and `VehicleUUID` fix the plate |
| `Confirm(org, uuid, refType, refUUID)` | hold → reservation, same row (no second unit); `ErrHoldExpired` / `ErrNotHold` / `ErrNotActive` |
| `ChangeDates(org, uuid, ChangeInput)` | **not release-then-search**: plans the new range with the old allocation in place (it only ignores itself), may move others, swaps atomically; without a plan the old allocation stays (`CapacityError`) |
| `AssignPlate(org, uuid, vehicleUUID)` | re-plans the same allocation onto the vehicle (moving flexible commitments off it) and sets `plate_fixed`; never consumes twice |
| `Release(org, uuid, reason, actor)` | frees exactly once (`ErrNotActive` afterwards), re-plans dependents, returns `Affected` |
| `ExtendHold(org, uuid, minutes)` | `expires_at = now + minutes` for an active, unexpired hold |
| `ExpireDue(ctx)` | worker sweep (`app:availability:expire-holds`, every minute): expires due holds per component under the lock, re-plans dependents. Lapsed holds stop counting at `expires_at` even before the sweep, and every writer expires them first |

TEC-706: create the reservation with `Reserve(kind=reservation, ref=reservation uuid)` or convert a hold with `Confirm`; edit dates with `ChangeDates`; "plaka değişimi" (calendar drag, `FleetTimeline.onBarDrop`) is `AssignPlate`; cancellation is `Release`. M3 rentals will set `kind = rental` and move `current_branch_id` on check-in.

Added for reservations ([reservations.md](./reservations.md)): `TxHook` (`ReserveInput.Hook`, `ChangeInput.Hook`, `PlanOptions.Hook`) runs inside the engine transaction with the result (allocation, vehicle, product version, moved and affected allocations), so the reservation row commits with the capacity change; `ReserveInput.HoldUntil` sets an option's expiry to the review / payment window; `ChangeProduct` (upgrade inside a component), `MovePlan` (plan to a vehicle, fixed or provisional; replaces `AssignPlate` for callers that need a hook), `Replan` (re-plan as it is), `CandidateVehicles` (read-only plate candidates), `SetHoldExpiry`, `ConfirmWith` / `ReleaseWith` (hooked variants; a hook caller accepts a hold that lapsed in the same transaction as the release).

HTTP (tenant): `POST /v1/tenant/availability/search` (`tenant.availability.read`), `GET|POST /v1/tenant/availability/holds`, `GET|DELETE /v1/tenant/availability/holds/{uuid}`, `POST …/holds/{uuid}/extend` (`tenant.availability.hold` for writes), `/v1/tenant/capacity-rules` CRUD (`tenant.capacity_rules.read|write`). `409 CAPACITY_UNAVAILABLE` carries the reason as detail code (`no_vehicle`, `no_eligible_vehicle`, `one_way_not_allowed`, `channel_rules`, `product_not_sellable`) and `data` = alternatives (up to 5 other sellable products for the same request; nearby ranges ±2 h / ±1 day for the same product).

Grants (migration 000065): owner / admin / merkez_yoneticisi / sube_yoneticisi all four; rezervasyon_satis availability read + hold + rules read; muhasebe, teslim_iade, member: availability read + rules read.

## 6. Channels

Physical capacity comes from the engine; channel rules (`channel_capacity_rules`, migration 000064) only decide how much of it a channel may sell. Without rules every channel sells from the shared pool.

| Rule | Meaning | Reserves? |
|------|---------|-----------|
| `sales_limit(channel, N)` | at most N concurrent commitments of the product sold via the channel (peak inside the requested range) | no |
| `guaranteed_quota(channel, N)` | N units kept for the channel; while unused, other channels cannot take them | yes |
| `safety_margin(channel?, N)` | N units never sold (every channel when channel is NULL) | yes |

Scope: `rental_product_id` NULL = every product (each separately), `branch_id` NULL = every pick-up branch; the most specific matching rule wins (product > branch > channel). Evaluation for channel *c*:

```text
avail    = physical − Σ unused quotas of other channels
own      = min(unused quota of c, avail)
shared   = max(0, avail − unused quota of c − safety margin)
sellable = min(own + shared, sales limit remaining of c)
```

Unused quotas are scaled down proportionally when together they exceed the physical capacity, so a quota never promises a car that does not exist (and over-subscribed quotas cannot block each other). On write, the quotas of a product (its own + the "every product" ones) may not exceed its eligible vehicles. Reserve checks the rules inside the component lock (it needs `physical ≥ quotas of others + margin + 1`, computed with the planner and capped at that number). Date changes do not re-check channel rules (the unit is already sold).

Search breakdown (`AvailabilityBreakdown`): `eligible`, then each eligible vehicle in the first matching bucket — `out_of_service`, `blocked` (block / open transfer), `reserved` (confirmed load), `held` (options), `elsewhere` (free in time but not at the pick-up branch), `free` — plus `reassignable`, `physical`, `safety_margin`, `reserved_for_other_channels`, `own_quota_remaining`, `channel_limit_remaining`, `sellable`, and `reasons` (`one_way_not_allowed`, `no_eligible_vehicle`).

## 7. Location, pools, delivery points

- Location = `current_branch_id` (where the vehicle is now), moved by open transfers (`vehicle_busy_intervals.branch_after_id`) and by earlier commitments' return branches — the same rule as `vehicleusecase.ResolveLocation`, extended with allocations. Completed transfers are already in `current_branch_id`. Commitments that ended in the past do not move the vehicle (M3 check-in updates `current_branch_id`).
- One-way: a different return branch needs an allowed one-way rule (`ResolveOneWay`, product rule over route rule); otherwise the product is not sellable for that route (`one_way_not_allowed`).
- Delivery points (`service_points`) are fed by their branch's stock: a request may name a point; capacity is the branch's and is never counted twice.
- Pools: a commitment is pinned to the `fleet_pool` of the vehicle it was sold on; reassignment never crosses pools, so independent pools never share capacity. Requests may restrict to one pool (`fleet_pool`); without it any pool may serve a new sale.

## 8. Tests

| Test | What |
|------|------|
| `TestRT_A03_ThirtyVehiclesOneProduct` | 30 vehicles, one product → one product, capacity 30 (and 29 after a hold) |
| `TestRT_A04_LastUnitConcurrency` | 2 and 6 goroutines (own service, own connections) for the last unit → exactly one success; with and without the component lock; run with `-race -count=20` |
| `TestRT_A05_OneWayReturnMovesCapacity` | picked at A, returned at B → sellable at B after preparation, not at A, not during preparation; no rule = not sellable |
| `TestRT_A06_BlockInTheMiddle` | block inside the range → not sellable; direct SQL insert over a block refused by the trigger |
| `TestModelGuaranteeNotSoldOnOtherModel` | exact-model product not sold when only another model is free; alternatives offered |
| `TestSharedCarsReassignment` | group + exact model share the Egea: the new sale is accepted by moving the group sale (reassigned = 1) and refused when not satisfiable; promised plates never move |
| `TestChangeDates` | change into a full period keeps the old reservation; into its own range swaps |
| `TestHoldLifecycle` | quote consumes nothing; hold / confirm / plate consume once; release and expiry free exactly once (lazy and sweep) |
| `TestReleaseReplansDependents`, `TestPoolsNeverShare`, `TestServicePointsShareStock`, `TestChannelRules`, `TestCalendarAndBlocksSeeHolds` | §3, §6, §7 |
| `engine/property_test.go` | 10 000 random reserve / cancel / date change / plate operations per run over overlapping products, pools, blocks, transfers and one-way returns; every state verified, every answer compared with brute force |
| `TestRandomOperationsKeepInvariants` | 4 concurrent workers + a sequential phase against Postgres; database invariants (no double booking, eligibility, continuity) checked independently of the engine |
| RT-A01 | every new route in the isolation test (foreign org 403, foreign uuids 404, bodies naming foreign records rejected) |

## 9. Limits and open items

- The reassignment search is bounded (§3); refusals of feasible requests are rare but possible. Raising `MaxDepth` / `MaxExpansions` trades latency for completeness.
- A release can leave a later commitment `affected` (it relied on the released one-way return and nothing else can serve it); the caller reports it. Blocks over allocations are reported, not auto re-planned (TEC-706 decides how reservations react).
- Search and Reserve compute capacity per component in process; very large components (thousands of vehicles in one product family) would want a cached per-day occupancy index.
- Channel quotas / limits use the peak over the requested range per product; per-day quota calendars are not modelled.
- The panel UI for capacity rules is intentionally small (list + drawer); per-channel availability in sales channels arrives with their integrations.
