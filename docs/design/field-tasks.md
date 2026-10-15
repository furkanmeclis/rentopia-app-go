# Field tasks: delivery, collection, transfer and cleaning

Status: **implemented** (M4: TEC-724; acceptance test RT-A29). It builds on rentals and operation tasks ([rentals.md](./rentals.md)), reservations ([reservations.md](./reservations.md)), inspections ([inspections.md](./inspections.md)) and fleet transfers.

- Backend code: `backend/internal/modules/fieldtasks`
  - `model`: statuses, the transition table, task view, filters, delivery details
  - `placement`: where a task happens (branch desk, service point, address)
  - `delivery`: pure eligibility of a delivery point (area, hours, notice, channel)
  - `usecase`: board and own lists, actions, reservation sync, delivery quotes and fees
  - `handler`, `adapters` (bulk assign, rentals finisher), `routes.go`
- Panel: `frontend/src/features/field-tasks` (board, staff day timeline, "Görevlerim", task detail, delivery block of the reservation wizard).

## 1. Decision: extend `operation_tasks`

Field tasks are **not** a new table. A delivery is the `handover` task of a reservation whose pick-up is a delivery point or an address, and a collection is its `return` task. Rentals already open, reschedule and close these tasks, the handover / return forms already finish them, and the rentals pages already list them. A separate `field_tasks` table would have duplicated the same work item and needed two-way syncing.

So migration `000095_field_tasks` widens `operation_tasks`:

| Area | Columns / objects |
|------|-------------------|
| Links | `reservation_id` (tasks exist before the rental), `origin` (`reservation`, `rental`, `manual`), `transfer_id` (fleet transfer of a transfer task), `to_branch_id` |
| Place | `location_kind` (`branch`, `point`, `address`), `point_id`, `address`, `district`, `city`, `latitude`, `longitude` |
| Meeting | `contact_name`, `contact_phone`, `flight_number`, `meeting_note` |
| Plan / actual | `due_at` + `planned_minutes`; `assigned_at`, `started_at`, `completed_at`; `issue_note`, `issue_reported_at`; `updated_by` |
| Types | adds `transfer` and `other` |
| Statuses | `planned`, `assigned`, `en_route`, `issue_reported`, `completed`, `cancelled` (the old `open` / `in_progress` / `done` rows are migrated) |
| Timeline | `operation_task_events` + trigger `operation_task_timeline()` |
| Indexes | `uq_operation_tasks_live` (one live task per rental, type, vehicle), `uq_operation_tasks_reservation_live` (one live task per reservation and type before the rental) |

When the rental is created, its handover / return insert **adopts** the live reservation task (same row, `rental_id` set) instead of opening a second one.

Service points gain delivery settings: `delivery_enabled`, `delivery_extra_id` (the fee is a published `delivery` extra), `service_districts`, `latitude` / `longitude` + `service_radius_km`, `min_notice_minutes`, `delivery_conditions`. Quotes and reservations gain `pickup_delivery` / `return_delivery` (JSON details) and quotes `delivery_override`.

The down migration drops the events table, the new columns and permissions and maps the statuses back. It loses timelines, delivery details and transfer / other tasks; the file says so.

## 2. State machine

One table in `model.transitions`; every move is validated there and the trigger writes the timeline, whatever code path changes the row.

| From \ action | assign | unassign | start | report_issue | complete | cancel |
|---|---|---|---|---|---|---|
| planned | assigned | — | en_route | — | completed | cancelled |
| assigned | assigned | planned | en_route | issue_reported | completed | cancelled |
| en_route | assigned | planned | — | issue_reported | completed | cancelled |
| issue_reported | assigned | planned | en_route | — | completed | cancelled |

- `completed` and `cancelled` are final (`409 task_closed`); any other move is `409 invalid_transition`.
- Timeline actions: `created`, `status_changed`, `assignee_changed`, `rescheduled`, `relocated` (actor = `updated_by`).
- Completing a handover / return task by hand is refused (`record_handover` / `record_return`): the executor opens the handover / return form, which closes the task.
- Completing a `transfer` task completes its fleet transfer (the vehicle's branch moves to the target); fleet errors map to `vehicle_not_at_origin`, `vehicle_busy`, `transfer_overlap`, `block_overlap`.
- Completing cleaning / maintenance / preparation goes through the rentals finisher, which makes the vehicle available as before.

## 3. Delivery in quotes and the wizard

`POST /v1/tenant/quotes/search` and `POST /v1/tenant/quotes` accept `pickup_point_uuid` / `return_point_uuid` with `pickup_delivery` / `return_delivery` (address, district, city, coordinates, contact, flight number, meeting note). The reservations module calls the `DeliveryQuoter` port (implemented by field tasks) which:

1. checks eligibility with `delivery.Check`: the point is active and delivery-enabled, online channels only where allowed, minimum notice (`too_soon`), the point's hours (else the branch's hours and holidays: `outside_hours` / `closed`), an address for address zones (`address_required`), district list or radius (`outside_area`);
2. prices each side: the published price of the point's delivery extra (`per_day` × days, capped), else the point's flat `extra_fee`.

Fees become named `delivery` lines of the price breakdown (code `delivery` / `collection`) through `pricing.Input.Deliveries`, so they flow into totals, VAT, snapshots and changes like any other line. A manual `delivery_fee` replaces the computed fees and needs `tenant.price.override`. Search drops a product's price when delivery is not eligible and answers the problem codes per field (`pickup_point_uuid` …).

The wizard's search step has the delivery block (point, same-as-pick-up return, address fields, flight number, contact, meeting note) with the point's fee, notice and conditions; summary lines read "Teslimat · SAW Havalimanı" / "Araç toplama · SAW Havalimanı".

## 4. Reservation automation

Field tasks are a reservation `LifecycleListener` (fan-out `resusecase.Listeners{rentals, fieldTasks}`):

- **confirmed** (create, request accepted, reinstate): a delivery side opens a `handover` (due = start) and / or `return` (due = end) task at the point or address, `origin = reservation`, contact copied from the details or the customer.
- **changed**: times reschedule live tasks; a new point / address relocates them; a side that is no longer a delivery cancels its task; a new delivery side opens one.
- **terminated** (cancel, no-show, expiry): live reservation tasks are cancelled.
- When the plate is fixed and the rental is created, the rental adopts the tasks (see §1).

## 5. API

```text
GET   /v1/tenant/field-tasks/meta                  read      bulk actions, enums
GET   /v1/tenant/field-tasks/facets                read
GET   /v1/tenant/field-tasks/assignees             read | assign | execute
GET   /v1/tenant/field-tasks/mine                  execute   own tasks (today / tomorrow windows by due_from / due_to)
GET   /v1/tenant/field-tasks                       read      board list: date, branch, status, assignee (none = unassigned), type, location kind
POST  /v1/tenant/field-tasks                       assign    manual task (transfer opens a fleet transfer)
POST  /v1/tenant/field-tasks/bulk                  read      bulk engine (assign needs assign)
GET   /v1/tenant/field-tasks/{uuid}                any       executors only their own tasks
GET   /v1/tenant/field-tasks/{uuid}/events         any
PATCH /v1/tenant/field-tasks/{uuid}                assign    plan, place, contact
POST  /v1/tenant/field-tasks/{uuid}/assign         assign    { user_uuid | null }
POST  /v1/tenant/field-tasks/{uuid}/start          any       executor (own) or assign
POST  /v1/tenant/field-tasks/{uuid}/report-issue   any       { note }
POST  /v1/tenant/field-tasks/{uuid}/complete       any       { note }
POST  /v1/tenant/field-tasks/{uuid}/cancel         assign    { reason }
GET   /v1/tenant/delivery-points                   quotes.read
```

`/v1/tenant/operation-tasks` stays for the rentals pages and uses the same statuses.

## 6. Permissions, scope, live events, notifications

| Slug | owner, admin, merkez_yoneticisi, sube_yoneticisi | teslim_iade | rezervasyon_satis |
|------|---|---|---|
| `tenant.field_tasks.read` | ✓ | | ✓ |
| `tenant.field_tasks.assign` | ✓ | | |
| `tenant.field_tasks.execute` | ✓ | ✓ | |

- Branch scope applies to every list and action (`branch_not_allowed`); an assignee must be an active member with `execute` and the task's branch in scope (`not_field_staff`).
- Executors without `read` see only their own tasks (others answer 404), RT-A01 isolation covers every route.
- Live resources: `tenant.field_tasks` (board, `read`) and `tenant.my_field_tasks` (own list, `execute`); rentals and inspections task events are mirrored onto them.
- Assignment enqueues the `field_tasks.assigned` notification: in-app (realtime user channel), web push and Expo through the existing notification service.

## 7. Panel

- **Görev panosu** (`/t/{slug}/field/board`, `read`): DataTable with faceted filters (type, status, location kind, branch, staff incl. "Atanmamış"), a day filter (`DatePicker`), row selection + bulk assign, row actions (assign, edit, cancel); an alert counts unassigned / issue tasks with a shortcut filter; the **Personel zaman çizelgesi** tab shows one lane per staff member for a day (drag a task onto a person to assign).
- **Görevlerim** (`/t/{slug}/field`, `execute`): today (late ones included) and tomorrow as touch cards for 375 px; nav badge with today's open count.
- **Task detail** (`/t/{slug}/field/my/{uuid}`): place, contact (tel link), flight, meeting note, directions, the next step (start / resume, open handover or return form, complete, report issue) with a sticky mobile action bar, and the timeline.

## 8. Tests

`fieldtasks/integration_test.go` (DB, `TEST_DATABASE_URL`):

- `TestRT_A29_AirportDelivery`: airport delivery quote (fee lines 2 × 750, flight number normalized), the confirmed reservation opens delivery + collection tasks, assignment notifies the executor (who sees only their task), the rental adopts both tasks, completing by hand is refused and the handover form completes the delivery; the timeline records each step.
- `TestDeliveryEligibility`: districts and radius of an address zone, its hours, a missing address, the airport's notice, an office that does not deliver; quote search shows the same refusal.
- `TestDeliveryTasksFollowTheReservation`: a time change moves both tasks (assignee kept), a pick-up back at the desk cancels the delivery, cancelling the reservation cancels the collection.
- `TestFieldTaskTransitions`: refused moves, an issue needs a note and the trip resumes, cancel needs a reason and the planner, a cleaning task completes by hand, a transfer moves the vehicle's branch through the fleet transfer, the pure transition table.
- `TestFieldTaskBranchScope`, `TestFieldTaskIsolation`.

`delivery/delivery_test.go` unit-tests eligibility; `httpserver/isolation_test.go` covers the routes (RT-A01).

## 9. Open items

- The reservation detail page has no field task card yet (tasks are on the board and in the rental's task list).
- An engine plan move does not re-sync tasks; the vehicle is read through the join, so the board still shows the right plate.
- After a manual edit of contact details, later reservation changes overwrite them.
- Deposit tasks keep `origin = rental`.
- After the inspection wizard finishes, it returns to the rental, not to the task page.
