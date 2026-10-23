# Extending Rentopya

Most product work is **organization-scoped**: data a rental company owns, shown in its panel at `/t/{slug}` and served under `/v1/tenant/*`. Platform work (global data under `/platform`) is the exception. Narrative is here; checklists are in skills:

| Adding | Skill | Worked example |
|--------|-------|----------------|
| Organization-scoped list resource | `.agents/skills/add-tenant-resource` | `branches` |
| Platform list resource | `.agents/skills/add-platform-resource` | `users`, `roles` |
| External provider / integration job kind | `.agents/skills/integration-adapter` | Netgsm SMS, KABİS |
| Table, column, seed, permission slug, query | `.agents/skills/db-migration` | |

Before a domain feature, read its design doc in `docs/design/` (one per area: reservations, pricing, rentals, payments, …) and the module's package comment.

## Backend module

1. Create `backend/internal/modules/<name>/` with `handler`, `usecase`, `repository`, `model`, and `routes.go` (plus `adapters` for engine adapters).
2. Register routes from `internal/httpserver/server.go`; anything the worker runs (engine adapters, sweeps, integration operations) is wired in `cmd/worker/main.go` too.
   - Tenant routes: `Authenticate` → `RequireOrganization(resolver)` → `RequireTenantPermission("tenant.<plural>.<action>")`; the handler reads `orgctx.Must(ctx).ID` and every query filters `organization_id` (and the branch scope for branch-bound data). A nil resolver (tenancy off) mounts nothing.
   - Platform routes: `Authenticate` → `RequirePermission("platform.<plural>.<action>")`.
3. SQL: `make -C backend migrate-create NAME=…` → write up + down → `make -C backend migrate-up` → `make -C backend sqlc`.
4. Update `backend/docs/openapi.yaml` in the **same** change (RT-A01 cross-checks every `/v1/tenant` route against it).
5. Lists use `pkg/apiquery` (`limit`, `offset`, `q`, `sort`). Envelope via `pkg/response`. See [backend/API_CONVENTIONS.md](../backend/API_CONVENTIONS.md).
6. If the resource is listed, add `GET /v1/{platform|tenant}/<plural>/meta` via `internal/platform/resourcemeta` and register `/meta` **before** `/{uuid}`.
7. Side effects: publish on `internal/platform/events`, `notifications.Service.Enqueue`, the notification engine (customer messages) or the integration job runner (provider work). Never SMTP, SMS or a provider call from a handler.
8. Mutations that matter: `activity.Recorder.Record` (fail-soft).
9. New permission slugs: migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts` + `permissions.labels.*` in `en`/`tr`. Tenant slugs are also granted to the template roles in the same migration (see `add-tenant-resource`).

## Frontend feature

1. Add `frontend/src/features/<name>/` (`components`, `hooks`, `services`, `schemas`).
2. Add App Router pages under `(tenant)/t/[slug]` (services call `tenantRequest(slug, …)`, query keys include the slug) or `(platform)/platform`.
3. Register routes in `frontend/src/config/routes.ts`, sidebar items in `frontend/src/config/nav.ts`, and gate with permissions from `frontend/src/config/permissions.ts`.
4. Call the BFF (`/api/v1/...`), not the Go host, from the browser.
5. Dates: shared `DatePicker` / `AppDatePicker` — see `.agents/skills/admin-date-picker`.
6. After OpenAPI changes: `cd frontend && pnpm api:generate`.

## Sidebar (nav-engine)

Declarative catalog: `frontend/src/config/nav.ts`. Renderer: `frontend/src/features/nav-engine`. Skill: `.agents/skills/nav-engine`.

Canonical live-badge example: `frontend/src/features/users/nav/`.

## Import / export (ioengine)

Package: `internal/platform/ioengine`. HTTP: `internal/modules/{exports,imports}`. Skill: `.agents/skills/io-engine`.

1. Implement `ioengine.ResourceAdapter` in `internal/platform/ioengine/adapters/`.
2. Register in `ioengine.NewRegistry(...)` from `internal/httpserver/server.go` **and** `cmd/worker/main.go`.
3. Set `resourcemeta` `capabilities.export` / `import`. Expose `GET /v1/.../meta`.
4. RBAC: `{platform|tenant}.<resource>.export` / `.import` (migration + rbac + frontend permissions).
5. Mount `POST /v1/{platform|tenant}/<resource>/export` (and import + sample if create-capable). Tenant engine resources are named `tenant.<plural>`; jobs are stamped with the organization (`orgctx.EngineScope`).
6. Frontend: `ResourceIOToolbar` from `@/features/io` on the list page.

Formats: export PDF / XLSX / CSV / JSON. Import JSON / XLSX / CSV / TSV. Letterhead comes from `app_settings`. Source files land in SeaweedFS under `imports/{job_uuid}/`.

## Bulk actions (bulkengine)

Package: `internal/platform/bulkengine`. Skill: `.agents/skills/bulk-engine`.

1. Implement `bulkengine.BulkAdapter` in `internal/platform/bulkengine/adapters/`.
2. Register in `bulkengine.NewRegistry(...)` from `httpserver/server.go` and `cmd/worker/main.go`.
3. RBAC: `{platform|tenant}.<resource>.bulk.<action>`.
4. `resourcemeta` `capabilities.bulk = true` and `bulk_actions` from the adapter.
5. Mount `POST /v1/{platform|tenant}/<resource>/bulk`.
6. Frontend: `rowSelection`, `useBulkSelection`, `SelectionBanner`, `BulkActionMenu`.

Jobs above `BULK_SYNC_MAX` (default 50) return **202** and run on Asynq queue `bulk`. Rollback window: `BULK_ROLLBACK_HOURS` (default 24). Undo: `POST /v1/platform/bulk/{uuid}/rollback` (`/v1/tenant/bulk/{uuid}/rollback` in an organization).

## Command palette / search (search-engine)

Package: `internal/platform/searchengine`. HTTP: `internal/modules/search`. Skill: `.agents/skills/search-engine`.

1. Implement `searchengine.Adapter` in `internal/platform/searchengine/adapters/`.
2. Register in `searchengine.NewRegistry(...)` from `httpserver/server.go` and `cmd/worker/main.go`.
3. Enqueue `Indexer.EnqueueUpsert` / `EnqueueDelete` after mutations (fail-soft).
4. Frontend: optional `defineSearchSpec` in `frontend/src/features/<name>/search/`; pages come from `nav-engine` automatically.
5. `GET /v1/search?q=&spec=` and `GET /v1/search/specs` (organization: `/v1/tenant/search*`, specs with `Scope: searchengine.ScopeTenant`) — browser uses BFF only; Meilisearch stays on the private network.
6. Reindex: `make search-reindex`.

## Organization resources

Checklist: `.agents/skills/add-tenant-resource`; design: [design/tenant-core.md](./design/tenant-core.md).

- Tables carry `organization_id` (indexes start with it, composite FKs for children); queries filter it, including lookups by uuid. The table goes into `backend/internal/database/queries/tenant_tables.txt` so the query guard covers it.
- The organization comes from `orgctx`, never from the body or query. Another organization's record answers **404**.
- Branch-bound data applies the member's branch scope (`orgctx.BranchScope`, `CheckBranch`, `RequireAllBranches`).
- New organizations get seed rows through `OrganizationHooks.OnCreated`.
- RT-A01 (`internal/httpserver/isolation_test.go`, `TEST_DATABASE_URL`) must cover every new `/v1/tenant` route.

## Provider integrations

Checklist: `.agents/skills/integration-adapter`; design: [design/integration-jobs.md](./design/integration-jobs.md).

- A provider is an `integrations.Adapter` (kind, provider code, credential fields, `Check`) plus the area's provider interface in `internal/platform/<area>`.
- Each organization connects its own account (`/v1/tenant/integrations`, Ayarlar → Entegrasyonlar); credentials are sealed with the integration vault and shown as masked hints only.
- Outbound work is an `integration_jobs` row run by the runner through an `integrations.Operation` (`Submit` / `CheckStatus`). An unknown outcome is checked before anything is sent again.
- Fakes (`*_fake`) are registered outside production only.

## GitHub OAuth (integrations)

Skill: `.agents/skills/github-integration`. Super-admin settings at `/platform/settings/integrations?tab=github` (Google, Facebook, Apple on the same page).

- Credentials (`client_secret`, private key) encrypted with `APP_ENCRYPTION_KEY` via `internal/platform/crypto`. API never returns plaintext secrets.
- Users **link** GitHub from profile (`GET/DELETE /v1/auth/identities`). Unlinked GitHub accounts cannot self-register.
- NextAuth talks to internal adapter routes (`/v1/internal/auth/oauth/github`) — not in OpenAPI.

## Impersonation

`POST /v1/platform/users/{uuid}/impersonate` (`platform.users.impersonate`, step-up). Stop: `POST /v1/auth/impersonation/stop`. JWT optional claim `imp`. Cannot impersonate yourself or a super admin; stop the current session before starting another.

## Storage / media

Skill: `.agents/skills/storage-media`.

- Object bytes in S3; rows in Postgres. Browser never builds object URLs from keys.
- Use only API-returned `public_url` / `logo_url` / stream paths. Helper: `frontend/src/lib/media/urls.ts`.
- Key helpers: `internal/platform/storage/keys.go` (`app/branding/`, `exports/{job}/`, `imports/{job}/`, organization objects under `organizations/{org_uuid}/`; check `storage.OrganizationOwnsKey` before serving a key read from a row).
- Explorer API lives under `/v1/platform/storage/*` (`platform.storage.read` / `.write`).

## Activity and logs

- **Activity** (who did what): `activity.Recorder.Record(ctx, actorID, action, resource, resourceUUID, payload, req)`. List: `GET /v1/platform/activity`. Permission: `platform.activity.read`.
- New `action` / `resource` strings need `activity.actions.*` and `activity.resources.*` in `frontend/src/locales/{en,tr}/activity.json`, plus an entry in `RESOURCE_LABEL_KEYS`. `make check-i18n` flags gaps.
- **Logs** (application log viewer + purge rules): `/v1/platform/logs`, `/v1/platform/log-rules`. Permissions: `platform.logs.read` / `.write`.
- Do not log secrets (tokens, passwords, VAPID private key, SMTP password).

## Step-up (re-auth)

Sensitive mutations can require a fresh password or passkey confirmation even when the session is valid. Skill: `.agents/skills/step-up-engine`.

- Backend: `middleware.RequireStepUp` after `Authenticate`. `403` + `STEP_UP_REQUIRED` until grant TTL.
- Frontend: `StepUpGate` / `useStepUp()` from `@/features/step-up-engine`.
- Admin TTL/methods: `/platform/settings/access` (`platform.access.read` / `.write`).

## Settings (letterhead)

`/v1/platform/settings` backs PDF/XLSX letterhead (`platform.settings.read` / `.write`). Logo upload goes through storage; the API returns `logo_url`.

## Queue and realtime

- Task names use the `app:` prefix (`app:notification:deliver`, `app:bulk:process`).
- Redis keys should use `app:<env>:…` if you add caching.
- Realtime channels: prefer `user:{user_uuid}`. Issue tokens only through `/v1/realtime/*`.

## Live lists (resource events)

Package: `internal/platform/resourceevents`. A DataTable can follow changes made by other users without polling.

1. Backend: after a successful mutation call `Publisher.Publish(ctx, resource, action, uuids...)` (`created` / `updated` / `deleted`). Use the engine slug (`platform.users`). It is async and fail-soft, and a no-op when Centrifugo is off. Bulk and import jobs already publish one `bulk` event with the affected uuids (capped at 200, `truncated` beyond that).
2. Register the resource and its read permission: `liveResources.Register("platform.<name>", rbac.Perm…Read)` (see `authmodule.RegisterLiveResources`). Clients may subscribe to `resource:{name}` only with that permission; unregistered resources are refused.
3. Frontend: `useServerListState({ realtime: { resource: "platform.<name>" } })` and pass `realtime={listState.realtimeFor(listQuery.refetch)}` to `EntityTable` / `DataTable`. Rows must use the uuid as `getRowId`.

On an event (debounced 400 ms) the table refetches silently when a visible row changed, fades deleted rows out first, and shows an "N new records – Refresh" banner instead of moving a user who is off page 1 or scrolled down. After a dropped connection it refetches once. With `NEXT_PUBLIC_REALTIME_ENABLED=false` nothing subscribes.

Tenant resources (`tenant.<plural>`) are published on `org:{org_uuid}:resource:tenant.<plural>` by `tenantmodule.ChannelResolver`; the page uses `tenantLiveList(organization, "tenant.<plural>")` (`add-tenant-resource` §4a).

Other scoped channels: `Broadcaster.AddResolver` returns another channel for chosen resources, `AuthUseCase.AddChannelRules` authorizes it, and the page passes `realtime: { resource, channel }`.

## Do not

- Put the organization in the JWT, add a workspace / dealer level, or a `/business` shell: organizations are resolved per request (`X-Organization`, `orgctx`)
- Copy patterns from other products' repos or prompts instead of the worked examples above
- Rename the Go module path (`github.com/furkanmeclis/nextjs-go-boilerplate/backend`) or the `rentopia` technical identifiers listed in [brand.md](./brand.md)

Brand via `frontend/src/config/brand.ts`, `frontend/src/styles/theme.css` and [brand.md](./brand.md); the product name in user-facing copy is **Rentopya**.
