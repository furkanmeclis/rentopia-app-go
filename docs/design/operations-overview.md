# Operations overview

Status: **implemented** (M3: TEC-716, requirement §15.1 "Operasyon özeti"). The tenant panel home page `/t/{slug}`. It reads data owned by reservations ([reservations.md](./reservations.md)), rentals and operation tasks ([rentals.md](./rentals.md)), payments ([payments.md](./payments.md)), integration jobs ([integration-jobs.md](./integration-jobs.md)) and contracts ([contracts.md](./contracts.md)). It owns no table.

- Backend code: `backend/internal/modules/overview`
  - `usecase`: the service, its types and the DB tests
  - `handler` and `routes.go`: HTTP
  - Queries: `backend/internal/database/queries/overview.sql`
- Panel: `frontend/src/features/overview` (page, sections, nav badges); live helper `features/tenant/hooks/use-tenant-live-refetch.ts`; list deep links `components/entity/use-url-list-defaults.ts`.

## 1. API: one aggregated endpoint

```text
GET /v1/tenant/overview[?branch=<uuid>]   KPIs + sections
GET /v1/tenant/overview/badges            sidebar counts
```

**Choice.** One aggregated endpoint instead of one per widget. The page needs every part at once and refreshes as a whole on live events, so one request (one membership resolution, one round trip) is cheaper than ten. The service runs one small query per part, in sequence. Each query is bounded (window counts plus `LIMIT 5`, or `LIMIT 100` for today's table), uses an index that starts with `organization_id`, and joins its rows in SQL, so there is no N+1. The badges get their own tiny endpoint because the sidebar is mounted on every page and needs only two counts.

Both routes need only a membership (`RequireOrganization`). There is **no new permission slug**: each part is computed only when the caller's tenant role grants the read permission of the module it comes from. Otherwise the part is `null` and the panel hides it.

| Part | Permission | Contents |
|------|------------|----------|
| `kpis.occupancy` | `rentals.read` + `vehicles.read` | rentable vehicles today, how many are out on an active rental, rate in % (one decimal) |
| `kpis.active_rentals` | `rentals.read` | active rentals, how many are late |
| `kpis.collections_today` | `payments.read` | TRY payments confirmed today (count, amount), refunds that succeeded today, net (`pkg/money` strings) |
| `today_operations` | `rentals.read` | handover and return tasks due today (not cancelled): time, plate, customer, rental, branch, status; counts done / open per type |
| `overdue_returns` | `rentals.read` | active rentals whose planned return passed, or that the overdue sweep marked; late minutes |
| `approvals` | `reservations.read` + `reservations.approve` | approval queue (`request`, `awaiting_review`) by review deadline; overdue = SLA passed |
| `awaiting_payment` | `reservations.read` | `awaiting_payment` by payment deadline; amount due = prepayment − paid |
| `availability_problems` | `resolution_tasks.read` | open resolution tasks (late_return, late_payment, block / release / extension conflicts, replan_failed), counts per type |
| `integration_jobs` | `integrations.manage` | jobs in `failed` or `needs_verification`, any provider or kind |
| `documents` | `contracts.read` | confirmed reservations picked up between today 00:00 and now + 48 h, not handed over yet, whose rental contract is missing or not approved |

"Today" is the Europe/Istanbul calendar day (`rentaltime`), returned as `today.start` / `today.end`.

### Branch scope and the branch filter

Every query applies the caller's `orgctx.BranchScope` with the visibility rule of the module that owns the data:

| Data | Visible when |
|------|--------------|
| operation tasks | the task's branch is in scope |
| rentals | planned pick-up, planned return or actual return branch |
| reservations, resolution tasks, payments, documents | the reservation's pick-up or return branch |
| vehicles (occupancy) | home or current branch |

`branches` lists the live branches in the scope. The panel shows the branch filter when there are two or more. `?branch=` narrows every query to that branch. A branch of another organization, or one outside the scope, answers `400` with a `not_found` detail on `branch` (same answer for both, so out-of-scope branches do not leak).

Integration jobs belong to no branch. They are organization-wide, like `/v1/tenant/integration-jobs`, and the card says the branch filter does not apply.

### Definitions

- **Occupancy.** Rentable means a live, active vehicle (`deleted_at IS NULL`, `is_active`) that is in service today (service dates) and not `out_of_service`, `maintenance` or `damaged`. Rented means a rentable vehicle with an `active` rental. Taking the vehicle count from active rentals, not from `vehicles.status`, keeps the figure right when a status was edited by hand. The two-branch vehicle rule means branch figures do not add up to the organization total when vehicles are away from home.
- **Overdue.** Active, and `overdue_at` is set or the planned return passed. The panel shows rentals that are late but still inside the grace period too. The full list link opens the rentals list on `status=active` sorted by planned return.
- **"Süresi yaklaşan belgeler".** Contracts have no expiry. A handover with an active rental contract template needs an approved contract (rentals `ContractState`), so the deadline that matters is the handover. The card lists handovers in the next 48 hours whose contract is not approved, with a countdown to the pick-up. Two states are shown: "Onay bekliyor" (a draft document exists) and "Hazırlanmadı" (no document yet). Without an active template nothing is required: `documents.required = false` and the card says so. Remote approval links that expire (TEC-711) are not listed. Their expiry is short and they are visible in the document dialog.

### Indexes (migration 000090)

Two partial indexes back the queries that no module index served:
- `idx_payments_organization_verified_collected (organization_id, verified_at)` where the status is succeeded / partially_refunded / refunded
- `idx_integration_jobs_organization_attention (organization_id, updated_at DESC)` where the status is failed / needs_verification

The other queries reuse the module indexes: operation tasks by due time, the approval queue index, the waiting reservations index, active rentals, open resolution tasks, and reservations by start. The down migration drops the two indexes and loses no data.

## 2. Panel

- **Header.** Organization name, the branch filter (URL `?branch=`, so it survives reloads and can be shared), refresh, and the last update time.
- **KPI cards** (each only with its permission).
- **"Bugünkü teslim ve iadeler".** A DataTable: client-side search, type and status facets, pagination and mobile cards. A row opens the rental.
- **Compact cards.** At most 5 rows each, a count badge and "Tümünü gör". Countdown badges turn amber under one hour and red when the deadline has passed. A row opens the reservation or rental.
- Empty, loading (skeleton) and error states follow the shared components. All strings are in `locales/{tr,en}/overview.json`.

"Tümünü gör" opens the full list with matching filters. List pages read them once with `useUrlListDefaults` (column filters and sort from the URL):

| Card | Target |
|------|--------|
| today | `/operation-tasks?task_type=handover,return&due_from&due_to&branch` (the due window shows as a removable badge) |
| overdue | `/rentals?status=active&sort=planned_return_at&branch` |
| approvals | `/reservations/approvals` |
| awaiting payment | `/reservations?status=awaiting_payment&branch` |
| availability | `/reservations/tasks?status=open` |
| documents | `/reservations?status=confirmed&sort=starts_at&branch` |
| jobs | `/settings/integrations` (job lists per account) |

**Live.** The page subscribes to the `org:{uuid}:resource:{name}` channels the panel already publishes, but only those the member may read: reservations, rentals, operation tasks, vehicles, payments, resolution tasks, integration jobs and documents. Any event triggers one debounced refetch (600 ms). A 5-minute poll is the safety net.

**Nav badges** (nav-engine `Adornment`):
- "Onay kuyruğu" shows the queue count. It is red when one passed its SLA, and the info popover splits waiting and overdue.
- "Çözüm görevleri" shows the open tasks.

Both read `/overview/badges` (permission- and branch-scoped on the server) and refresh live from reservations and resolution tasks.

## 3. Tests

- `overview/usecase/service_test.go` (real Postgres, `TEST_DATABASE_URL`) seeds two organizations and checks:
  - every KPI and count for a member of all branches
  - a member limited to one branch: counts, rows and branch list; the other branch as a filter is refused
  - the all-branches member narrowed to one branch
  - permission hiding of each part and of the badges
  - no cross-organization leakage: B's branch as A's filter is refused, and B sees only its own records
- RT-A01 (`internal/httpserver/isolation_test.go`): both routes are leak-scanned; `?branch=` with B's branches is a probe.

## 4. Open items

- KABİS and e-document jobs will show in the jobs card without changes (generic over `provider` / `kind`). Their kind labels go into `INTEGRATION_JOB_KIND_LABEL_KEYS`.
- A per-job link: there is no job detail page yet, so rows open the integrations settings.
- The today table is capped at 100 rows. Larger days continue in the operation tasks list.
