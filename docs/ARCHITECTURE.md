# Architecture

Three apps (Go API, Next.js panel, Next.js branded site) plus Compose files. Dependencies point inward: HTTP handlers and Next.js pages call use cases / services; they do not own SQL, SMTP or provider calls.

The Go module path stays `github.com/furkanmeclis/nextjs-go-boilerplate/backend` on purpose: renaming it touches every import for no gain, so do not rename it.

## Layout

```
.
├── backend/                         Go API + worker
│   ├── cmd/                         server, worker, create-super-admin, search-reindex,
│   │                                catalog-import (TSB list), enqueue-ping,
│   │                                iyzico-fake, netgsm-fake (local provider fakes)
│   ├── internal/modules/            one package per HTTP area (grouped below)
│   ├── internal/platform/           engines, provider layers, shared helpers (table below)
│   ├── internal/{httpserver,queue,middleware,realtime,observability,config,database}
│   ├── migrations/                  sequential golang-migrate SQL
│   └── docs/openapi.yaml            OpenAPI 3.1 (Scalar at /docs/)
├── frontend/                        Next.js BFF
│   ├── src/app/(landing)            / product landing page
│   ├── src/app/(platform)           /platform admin
│   ├── src/app/(tenant)             /t/{slug} company panel
│   ├── src/app/(account)            /home (router), /profile, /organizations, /no-access
│   ├── src/app/(guest)              login, reset, verify
│   ├── src/app/(public)             /r/{orgSlug} customer portal, /pay/{token}, /d/{token},
│   │                                /invite/{token}
│   ├── src/app/share                public storage share links
│   ├── src/app/api/v1               BFF proxy → Go
│   └── src/features/                one folder per feature (grouped below)
├── site/                            branded site (Next.js, own image, one image for every company)
├── deploy/                          env schema (prod-env.sh), Centrifugo config
├── compose.local.yml                infra only
└── compose.prod.yml                 full stack
```

### Backend modules (`internal/modules/*`)

| Group | Modules |
|-------|---------|
| Auth and account | `auth`, `qrlogin`, `authsettings`, `access` (step-up policy), `notifications` |
| Platform | `catalog` (TSB vehicle catalog), `platformhealth`, `settings` (letterhead), `activity`, `logs`, `storage`, `exports`, `imports`, `bulk`, `search`, `integrations` (`accounts`, `jobs`, `github`, `oauthprovider`) |
| Organization core | `organizations` (platform organization CRUD, tenant members, invitations, roles), `tenant` (`/v1/tenant/context`, live channel rules), `orgsettings`, `branches`, `overview` |
| Fleet | `fleet` (segments, rental groups, products), `vehicles`, `vehiclerecords`, `fleetops` (blocks, transfers, one-way rules), `availability` |
| Sales | `pricing`, `extras`, `campaigns`, `reservations`, `customers`, `agreements` |
| Operations | `rentals`, `fieldtasks`, `inspections`, `contracts`, `kabis`, `tolls` |
| Finance | `finance` (ledger, cash book, collections, deposits, expenses), `payments`, `invoices` (e-invoice), `suppliers`, `profitability`, `reports` |
| Messaging and channels | `notifyengine`, `sms`, `whatsapp`, `channels` (Yolcu360, Obilet) |
| Customer-facing | `publicapi` (`/v1/public/*`), `customerportal` (`/v1/customer/{orgSlug}/*`), `site`, `siteleads` |

Each domain module's package comment names its design doc under `docs/design/`.

### Frontend features (`src/features/*`)

| Group | Features |
|-------|----------|
| Shell and engines | `auth`, `account`, `tenant`, `nav-engine`, `search-engine`, `step-up-engine`, `bulk-engine`, `io`, `storage`, `notifications`, `landing` |
| Platform | `platform-overview`, `organizations`, `users`, `roles`, `catalog`, `platform-health`, `auth-settings`, `access`, `integrations` (GitHub + OAuth providers), `logs`, `settings` |
| Organization core | `tenant-settings`, `tenant-members`, `tenant-roles`, `invitations`, `branches`, `overview` |
| Fleet | `fleet`, `vehicles`, `vehicle-records`, `fleet-ops`, `fleet-calendar`, `availability` |
| Sales | `pricing`, `extras`, `campaigns`, `reservations`, `customers`, `agreements` |
| Operations | `rentals`, `field-tasks`, `inspections`, `contracts`, `kabis`, `tolls` |
| Finance | `finance`, `payments` (also the organization's integration accounts and jobs), `deposits`, `invoices`, `suppliers`, `profitability`, `reports` |
| Messaging and channels | `notification-engine`, `whatsapp`, `channels` |
| Customer-facing settings | `site`, `site-articles`, `site-leads`, `public-api`, `customer-portal` |

## Shells

| Surface | Path | Who |
|---------|------|-----|
| Landing | `/` | everyone |
| Platform | `/platform` | `super_admin` or any `platform.*` permission |
| Organization (company panel) | `/t/{slug}` | members of that organization (`tenant.*` permissions from their roles in it) |
| Account | `/home`, `/profile`, `/organizations`, `/no-access` | any signed-in user |
| Guest | `/platform/login` (aliases at `/login`, …) | signed-out users |
| Customer portal | `/r/{orgSlug}` | a company's customers (reservation reference + one-time code; own session, never the staff JWT) |
| Branded site | `site/`, `{subdomain}.<SITE_BASE_URL host>` or a verified custom domain | public; books through `/v1/public/*` |

After sign-in `next` wins; otherwise platform users land on `/platform`, users with one organization on `/t/{slug}`, users with several on `/organizations`, and everyone else on `/no-access` (`defaultHomeForUser`). There is no workspace or `/business` shell. JWT claims are `sub`, `roles`, `is_super_admin`, `exp`, optional `imp` (impersonator) and `sid`; the organization is never in the token.

## Request flow

```
Browser
  → same-origin /api/v1/*          Next.js BFF (HttpOnly session)
  → $API_URL/...                   Go API (Bearer JWT; X-Organization on /v1/tenant/*)
  → use case → repository (sqlc)   PostgreSQL

Branded site (site/)
  → $API_URL/public/...            Go API (publishable key → organization; booking session)
```

- Access and refresh tokens never enter JavaScript. The BFF stores them in HttpOnly cookies and strips them from JSON responses.
- NextAuth is the frontend session layer; the Go adapter talks to `/v1/internal/auth/*`.
- Tenant pages call `tenantRequest(slug, …)`; the BFF forwards `X-Organization` on `/api/v1/tenant/*` only. Two tabs on two organizations stay independent.
- The customer portal uses its own session cookie (`frontend/src/lib/server/customer-session.ts`) against `/v1/customer/{orgSlug}/*`.
- OpenAPI types are generated into `frontend/src/generated/api.d.ts` via `pnpm api:generate`.

## Backend layers

```
handler → usecase → repository interface → sqlc/pgx → PostgreSQL
```

Cross-module signals go through `internal/platform/events` (in-process, fail-soft) or narrow ports wired in `internal/httpserver/server.go` (for example reservations → payments, rentals → KABİS). Handlers never call SMTP, SMS or a provider; they go through the notification service, the notification engine or the integration job runner.

Notifications of an organization (tenant export / import / bulk jobs) carry `EnqueueInput.OrganizationID` (`notifications.organization_id`). The inbox is scoped by shell: `/v1/tenant/notifications*` (with `X-Organization`) shows that organization's rows plus un-stamped ones, `/v1/notifications*` only un-stamped ones; live cards are filtered the same way by `organization_uuid`. A finished tenant export links to `/t/{slug}/exports/{uuid}/download`, a tenant page that downloads through `/v1/tenant/exports/{uuid}/download` (a link cannot carry the organization header).

Audit: `activity.Recorder.Record` (fail-soft; stamps `organization_id` from the context). Application logs: `internal/modules/logs`.

Composition roots: `internal/httpserver/server.go` (API) and `cmd/worker/main.go` (Asynq worker, sweeps). Adapters of every engine and provider register in **both**.

## Platform engines and provider layers (`internal/platform/*`)

| Package | Job |
|---------|-----|
| `resourcemeta` | `GET /v1/.../meta`: columns, filters, capabilities |
| `ioengine` | Export / import adapters; jobs on Asynq `exports` / `imports` |
| `bulkengine` | Batch mutations with rollback; jobs on Asynq `bulk` |
| `searchengine` | Meilisearch indexing + `GET /v1/search`, `/v1/tenant/search` for Cmd+K |
| `resourceevents` | "List resource changed" events to Centrifugo for live DataTables |
| `storage` (+ `modules/storage`) | Object bytes in S3; metadata in Postgres; organization keys under `organizations/{org_uuid}/` |
| `activity` | Audit events for mutations and I/O |
| `stepup` | Re-auth grant (password / passkey) before sensitive routes |
| `orgctx` (+ `orgcache`) | Organization scope of a request: membership, permissions, branch scope, engine scope, Redis-cached resolver |
| `rbac` | Permission and role slugs (platform, tenant, tenant template roles) |
| `crypto` | `SecretBox` (GitHub / OAuth secrets) and `FieldCipher` (column encryption, blind indexes, purpose-separated `Sealer`s), all derived from `APP_ENCRYPTION_KEY` |
| `pii` | Turkish identity checksums, normalization for blind indexes, masking rules |
| `outbox`, `dbtx`, `events` | Transactional outbox, ambient transaction, in-process events |
| `pricing` | Pure rental pricing engine: resolved input → breakdown, no DB or clock |
| `integrations` | Provider skeleton: `Adapter` + `Registry`, credential `Vault`, `Operation` contract (`Submit` / `CheckStatus`) of the `integration_jobs` runner |
| `healthcheck` | Pluggable health sources behind the platform connection health dashboard |
| `payments` | Payment `Provider` interface; `iyzico` adapter (+ `iyzicofake`) |
| `sms` | SMS `Provider`; `netgsm` (+ `netgsmfake`), `logsms` (development) |
| `whatsapp` | One WhatsApp Web session per organization: `wuzapi`, `whatsappfake` |
| `einvoice` | E-Fatura / e-Arşiv integrator layer (official slot waits on dependency; `einvoicefake`); documents are built with go-ubltr in `modules/invoices` |
| `kabis` | KABİS rental notifications (official slot waits on dependency; `kabisfake`) |
| `tolls` | HGS / OGS toll pass providers (provider not chosen; `tollfake`) |
| `channels` | Channel manager providers: `yolcu360`, `obilet`, `channelfake` |
| `pdfrender` | HTML → PDF with headless Chromium (go-ubltr render/chromium) |
| `tsbcatalog` | Reads the monthly TSB kasko value list for the platform catalog |
| `mail`, `jwt`, `password`, `totp`, `oidc`, `appleauth`, `ratelimit`, `i18n`, `slug`, `httpkit`, `authctx` | Auth and HTTP helpers, backend i18n catalog |

The notification engine (`modules/notifyengine`) sits on top: rules and templates per organization, channels SMS, e-mail, push and WhatsApp, sends from the worker sweep.

## Data stores

| Store | Role |
|-------|------|
| PostgreSQL 18 | Source of truth |
| Redis | Cache (organization resolver), Asynq, rate limits, QR sign-in state |
| SeaweedFS (S3 gateway) | Object bytes; metadata stays in Postgres. Any S3 works (`STORAGE_DRIVER=minio` means "self-hosted S3-compatible") |
| Meilisearch | Cmd+K record search; master key stays on the API |
| Centrifugo | Websocket fan-out; HMAC stays on the API |
| wuzapi (prod) | WhatsApp gateway on the private app network |

Media: the browser must only use API-returned `public_url` / stream paths. See `frontend/src/lib/media/urls.ts`.

## RBAC

Users are global. Two permission scopes:

- **Platform** (`platform.*`, plus `auth.session`, `notifications.*`): union of the user's global roles (`user_roles`). `super_admin` (system role, created by `make create-super-admin`) bypasses `HasPermission`. `cms_user` is the legacy seed role with `auth.session` + `notifications.read`. Routes: `RequirePermission`.
- **Organization** (`tenant.*`): resolved per request from the membership in the **current** organization only (`RequireOrganization` → `RequireTenantPermission`). A member holds one or more roles (`organization_member_roles`), permissions are their union. Roles are the read-only template roles (`owner`, `admin`, `member`, `merkez_yoneticisi`, `sube_yoneticisi`, `rezervasyon_satis`, `muhasebe`, `teslim_iade`) plus company-specific roles each organization creates (`/v1/tenant/roles`, `/t/{slug}/settings/roles`). A member's `branch_scope` is `all` or `selected` branches; branch-bound modules filter with `orgctx.BranchScope` / `CheckBranch` / `RequireAllBranches`. `super_admin` has no bypass here.

**Plan features (TEC-674).** On top of permissions, an organization's plan (Yönetim / Komple) and the platform's per-feature overrides decide which external features it may use (site, widget, public API, channel managers, KABİS, HGS, e-invoice sending, WhatsApp, SMS). Server side: `RequireFeature` on the routes that create work or send something, `features.Require` in use cases and background jobs; reads, exports and switching things off stay open, so a downgrade never hides history and never deletes data. The panel mirrors it: `FeatureProvider` / `useFeature`, nav items with `feature` (hidden, or kept with a "not in plan" label when `featureOff: "readonly"`), `FeatureGate` route layouts that keep the page readable under a banner (or replace a pure settings form), and the `FEATURE_NOT_ENABLED` toast.

New permission slugs are added only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`. Details: [backend/docs/auth.md](../backend/docs/auth.md). Organization design: [design/tenant-core.md](./design/tenant-core.md).

## Organization isolation

Every tenant table carries `organization_id`; every query on it filters by it (`internal/database/queries_test.go` over `queries/tenant_tables.txt`), and `internal/httpserver/isolation_test.go` (RT-A01) walks every `/v1/tenant` route with two organizations. Engine jobs are stamped with the organization and re-scoped in the worker. Checklist: `.agents/skills/add-tenant-resource`.

`TENANCY_ENABLED=false` (config default) mounts no `/v1/tenant` routes; this product runs with it on (`.env.example`, `deploy/env/schema`).
