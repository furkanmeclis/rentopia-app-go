# Fleet availability inputs — blocks, transfers, one-way rules

Status: **implemented** (TEC-694, TEC-695). Consumer: the M2 availability / capacity engine (TEC-697). This page is the contract between the two: what the engine reads, where it reads it and which rules are already enforced by the time it runs.

## 1. One place for "when is vehicle V busy"

```sql
-- migration 000053
CREATE VIEW vehicle_busy_intervals (
  organization_id BIGINT,
  vehicle_id      BIGINT,
  kind            TEXT,       -- block | transfer   (M2 adds: reservation)
  ref_uuid        UUID,       -- vehicle_blocks.uuid / vehicle_transfers.uuid
  during          TSTZRANGE,  -- half-open [start, end); upper NULL = until further notice
  branch_after_id BIGINT      -- where the vehicle is when the interval ends (NULL = unchanged)
);
```

Query it with `organization_id` first (it is in `queries/tenant_tables.txt`, so the isolation lint applies) and the overlap operator:

```sql
SELECT kind, ref_uuid, lower(during), upper(during), branch_after_id
FROM vehicle_busy_intervals
WHERE organization_id = $1 AND vehicle_id = $2
  AND NOT isempty(during)
  AND during && tstzrange($from, $to, '[)');
```

`ListVehicleBusyIntervals` (`queries/vehicle_blocks.sql`) is that query; transfers already use it to refuse a window that overlaps a block.

| kind | rows | `during` | `branch_after_id` |
|------|------|----------|-------------------|
| `block` | `vehicle_blocks` where `deleted_at IS NULL` | the block range; open-ended blocks have no upper bound | NULL |
| `transfer` (planned) | `vehicle_transfers.status = 'planned'` | `planned` | `to_branch_id` |
| `transfer` (in transit) | `status = 'in_transit'` | `[actual_departure, max(planned arrival, now))` — a late vehicle stays busy until someone completes the transfer | `to_branch_id` |
| `transfer` (completed) | `status = 'completed'` | `[actual_departure, actual_arrival)` | `to_branch_id` |

Cancelled transfers and deleted blocks are not busy. Released blocks keep their (now closed) range: history stays visible to the engine and the UI.

**M2: adding reservations.** Append a third `UNION ALL` branch to the view (`'reservation'`, the reservation uuid, its pick-up → return range plus preparation buffer, `branch_after_id` = return branch) in the M2 migration (`CREATE OR REPLACE VIEW`, same column list and order). Nothing else in this page changes.

## 2. Blocks (TEC-694)

`vehicle_blocks(organization_id, vehicle_id, kind, during, reason, created_by, released_at, released_by, deleted_at)` — kinds `maintenance`, `damage`, `cleaning`, `out_of_service`, `other`.

- `during` is `[start, end)`; start is finite, end may be unbounded ("Bakıma al" = from now, open-ended).
- **No overlap per vehicle**: `EXCLUDE USING gist (organization_id WITH =, vehicle_id WITH =, during WITH &&) WHERE (deleted_at IS NULL)` (needs `btree_gist`, created in 000052). Why not allow overlap: one reason at a time keeps the engine trivial (no merging / ranking) and the UI unambiguous; extending or re-classifying a block is an update of the same row. The use case pre-checks and answers `409 CONFLICT` (`block_overlap`) with the colliding blocks; the constraint covers races.
- "Engeli kapat" sets the upper bound to now (`released_at/by`). A block that has not started is deleted instead.
- Every mutation returns `affected[]` = open transfers the block now overlaps (`kind: "transfer"`). M2 adds `kind: "reservation"` there; the frontend already renders the list generically.
- Vehicle status sync is explicit (`sync_status`): on create, when the block covers now, the vehicle status becomes maintenance / damaged / cleaning / out_of_service (vehicle event through the vehicles use case, same transaction); on release it goes back to `available` if it is still the block's status. Blocks never change the status by themselves — the engine must read blocks, not the status.

## 3. Transfers (TEC-695)

`vehicle_transfers(vehicle_id, from_branch_id, to_branch_id, responsible_user_id, planned tstzrange, actual_departure, actual_arrival, cost NUMERIC(14,2), status, note)`.

- Lifecycle: `planned → in_transit → completed`, `planned | in_transit → cancelled`. Start sets the vehicle status `in_transfer`; complete moves `vehicles.current_branch_id` to the destination and sets the status to `available` (default) or `preparing` — both through `vehicleusecase.ApplyOperationalChange`, which writes `vehicle_events`, in the transfer's transaction. Cancelling an in-transit transfer leaves the vehicle at the origin, `available`.
- One open (planned / in transit) transfer per vehicle and time: `EXCLUDE … planned WITH && WHERE status IN ('planned','in_transit')`, pre-checked as `409 transfer_overlap`.
- A transfer may not overlap a block: `409 block_overlap` (read from `vehicle_busy_intervals`). The reverse is allowed: a new block reports the transfers it overlaps in `affected`.

## 4. "A vehicle is not available at another branch by itself" — LocationAt

`vehicleusecase.ResolveLocation(current, now, movements, t)` / `Service.LocationAt(ctx, org, vehicle, t)`:

- `t ≥ now`: start at `current_branch_id`, apply planned / in-transit movements in departure order (arrived at or before `t` → destination; departed but not arrived → in transit; later ones ignored).
- `t < now`: undo completed movements that arrived after `t`, newest first (before departure → origin, between → in transit). Planned movements never happened.
- Movements are half-open like the intervals: at the arrival instant the vehicle is at the destination.

The only thing that changes where a vehicle is, is a movement. Creating a transfer defaults `from_branch` to `LocationAt(planned_departure)` and refuses another origin (`400 not_at_branch`). The engine should call `LocationAt` (or replay `branch_after_id` of the busy intervals) instead of trusting `current_branch_id` for future times.

**M3 note.** One-way rentals are movements too (`picked at A, returned at B` → at B after the return; RT-A05 is a unit test of `ResolveLocation`). Whether the engine uses the planned or the actual return branch of a rental — and what happens when a customer returns elsewhere — is decided in M3; `MovementsFromTransfers` is the only adapter today.

## 5. One-way rules (TEC-695)

`one_way_rules(from_branch_id, to_branch_id, rental_product_id NULL, allowed, fee NUMERIC(14,2) TRY, note, is_active)`, one live rule per route and product (`COALESCE(rental_product_id, 0)` in the unique index). Routes are directional.

`ResolveOneWay(from, to, product?)` / `GET /v1/tenant/one-way-rules/resolve`: the active rule of the product ("alt grup" = rentable product) overrides the generic rule of the route; no active rule = not allowed; the same branch is allowed for free. Pricing (M2/M3) calls it.

## 6. Branch scope and permissions

| Resource | Visible to a `selected` member when | Create needs |
|----------|-------------------------------------|--------------|
| blocks | the vehicle is (home OR current branch) | the vehicle visible |
| transfers | from OR to branch in scope | from or to in scope |
| one-way rules | from OR to branch in scope | from in scope |

Slugs (migration 000055): `tenant.blocks.read|write`, `tenant.transfers.read|write`, `tenant.one_way.read|write`. Separate from `tenant.vehicles.*` so delivery staff (`teslim_iade`) can block and move vehicles without editing them and accounting (`muhasebe`) can read costs and fees without block controls. Grants: owner / admin / merkez_yoneticisi / sube_yoneticisi all; teslim_iade blocks + transfers read/write; rezervasyon_satis and member read all; muhasebe transfers + one-way read. Exports use the read slug, bulk actions the write slug ("Bakıma al" on the vehicles list = `tenant.blocks.write`).

## 7. Fleet calendar (TEC-696)

`GET /v1/tenant/fleet/calendar?from=&to=` (`tenant.vehicles.read`) is the second reader of the view: one page of vehicles (plate order, default 50) in the caller's branch scope and, for those vehicles only, the busy intervals overlapping `[from, to)` plus maintenance days. Three queries, `organization_id` first (`queries/fleet_calendar.sql`): the vehicle page (window `COUNT(*) OVER ()` for the total), `vehicle_busy_intervals` with `vehicle_id = ANY($ids) AND during && tstzrange($from, $to, '[)')` joined to the block / transfer rows for their details, and `vehicle_maintenance` by planned / done day. The window is at most 93 days; bars carry the real bounds and the bounds clipped to the window. Block and transfer layers additionally need `tenant.blocks.read` / `tenant.transfers.read` (`layers` in the answer).

**M2 / M3.** A reservation row added to the view (§1) appears on the calendar once `kind = 'reservation'` is added to the layers in `usecase.CalendarLayers` and the frontend legend (`features/fleet-calendar/constants.ts`, today a disabled "M2" entry; rentals and preparation are "M3"). The timeline already exposes a drag extension point (`FleetTimeline` `onBarDrop`, disabled): moving a reservation bar onto another vehicle row is the "plaka değişimi" of the issue and must call the reservation API, which does not exist yet. Blocks and transfers are not draggable.
