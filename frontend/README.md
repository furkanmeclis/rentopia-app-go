# Frontend

Next.js BFF of Rentopya: the landing page, the platform panel, the company panel and the customer portal. The browser talks to same-origin `/api/v1/*`; the BFF forwards to the Go API and keeps JWTs in **HttpOnly** cookies. The branded company sites are a separate app (`../site`).

## Stack

- Next.js 16 App Router + React 19 + TypeScript
- Tailwind CSS 4 + shadcn/ui (`src/components/ui`), DataTable stack (`src/components/tables`, `src/components/entity`)
- TanStack Query + TanStack Table
- openapi-fetch + generated types from `../backend/docs/openapi.yaml`
- NextAuth (Go adapter) + Centrifugo client
- pnpm; `pnpm lint` / `pnpm typecheck` / `pnpm format`

## Panels

| Who                                          | Path                                                                                                                                                                                                                 |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Everyone                                     | `/` — product landing page                                                                                                                                                                                           |
| `super_admin` or any `platform.*` permission | `/platform` — overview, organizations, users, roles, usage and connection health, site domains, vehicle catalog, notifications, settings (branding, auth, OAuth, step-up, storage, imports, exports, logs, activity) |
| Organization members                         | `/t/{slug}` — company panel (operations, reservations, rentals, finance, reports, customers, fleet, pricing, settings); one organization lands there, several → `/organizations` picker                              |
| A company's customers                        | `/r/{orgSlug}` — customer portal (reservation reference + one-time code; own session cookie)                                                                                                                         |
| Anyone else signed in                        | `/no-access` — waiting for an invitation; `/profile` works for every signed-in user                                                                                                                                  |

Guest login: `/platform/login` (aliases at `/login`, `/forgot-password`, …). Public token pages: `/pay/{token}` (pay link), `/d/{token}` (remote document approval), `/invite/{token}` (organization invitation), `/share/*` (storage shares).

Tenant pages read the slug with `useTenant()` and call the API through `tenantRequest(slug, …)` (`src/lib/api/tenant-request.ts`), which sends the `X-Organization` header; the BFF forwards it on `/api/v1/tenant/*` only. There is no workspace or `/business` shell.

## Setup

Prefer `make frontend-dev` or `make local-dev` from the **repo root** so `API_URL` comes from root `.env`.

```sh
cd frontend
pnpm install
pnpm api:generate   # refresh types from ../backend/docs/openapi.yaml
pnpm typecheck && pnpm lint
pnpm dev            # http://localhost:3000
```

### Env (root `.env`)

| Variable                       | Purpose                                                     |
| ------------------------------ | ----------------------------------------------------------- |
| `API_URL`                      | Server-only Go upstream, default `http://127.0.0.1:8080/v1` |
| `AUTH_SECRET` / `AUTH_URL`     | NextAuth                                                    |
| `NEXT_PUBLIC_BFF_BASE_URL`     | Browser BFF base (`/api`) when set                          |
| `NEXT_PUBLIC_REALTIME_ENABLED` | `false` skips Centrifugo connect                            |
| `NEXT_PUBLIC_CENTRIFUGO_URL`   | Fallback WS URL if the token response omits `ws_url`        |

Cookie names: `app_access_token` / `app_refresh_token` (HttpOnly).

Go API listens on `APP_HTTP_ADDR` (default `:8080`). Paths are `/v1/...`.
Centrifugo HMAC stays server-side — never expose it to the browser.

## BFF

```
Browser  →  /api/v1/...              →  Next Route Handler  →  $API_URL/...
Browser  →  /api/auth/[...nextauth]  (session; no tokens in JS)
```

Login / refresh responses strip `access_token` / `refresh_token` before returning to the client; cookies are set server-side.

SSE / long-lived streams are proxied without buffering when `Accept: text/event-stream`.

## Features

`src/features/<name>/` holds `components`, `hooks`, `services`, `schemas`, `types.ts`, `constants.ts` and an `index.ts`. Grouped map: [`../docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md#frontend-features-srcfeatures).

| Group                  | Features                                                                                                                                                                       |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Shell and engines      | `auth`, `account`, `tenant`, `nav-engine`, `search-engine`, `step-up-engine`, `bulk-engine`, `io`, `storage`, `notifications`, `landing`                                       |
| Platform               | `platform-overview`, `organizations`, `users`, `roles`, `catalog`, `platform-health`, `auth-settings`, `access`, `integrations` (GitHub + OAuth providers), `logs`, `settings` |
| Organization core      | `tenant-settings`, `tenant-members`, `tenant-roles`, `invitations`, `branches` (reference list page), `overview`                                                               |
| Fleet                  | `fleet`, `vehicles`, `vehicle-records`, `fleet-ops`, `fleet-calendar`, `availability`                                                                                          |
| Sales                  | `pricing`, `extras`, `campaigns`, `reservations`, `customers`, `agreements`                                                                                                    |
| Operations             | `rentals`, `field-tasks`, `inspections`, `contracts`, `kabis`, `tolls`                                                                                                         |
| Finance                | `finance`, `payments` (also Ayarlar → Entegrasyonlar: provider accounts and integration jobs), `deposits`, `invoices`, `suppliers`, `profitability`, `reports`                 |
| Messaging and channels | `notification-engine`, `whatsapp`, `channels`                                                                                                                                  |
| Customer-facing        | `site`, `site-articles`, `site-leads`, `public-api`, `customer-portal`                                                                                                         |

New organization list page: `.agents/skills/add-tenant-resource` (worked example `branches`). New provider entry: `.agents/skills/integration-adapter`.

Shared infra: `components/ui`, `tables`, `entity`, `forms`, `layout`, `dialogs`, `lib/query`, `lib/i18n`, `lib/realtime`. Translations: `src/locales/{tr,en}/*.json` (Turkish first), registered in `src/lib/i18n/messages.ts`; check with `node ../scripts/check-i18n.mjs`.

Brand: `src/config/brand.ts`, `src/styles/theme.css`, logos in `src/components/brand/` ([../docs/brand.md](../docs/brand.md)).

Media: never assemble S3 URLs in the browser. Use `assertServiceMediaURL` in `src/lib/media/urls.ts`.

## Scripts

| Script              | Purpose                         |
| ------------------- | ------------------------------- |
| `pnpm dev`          | Next.js dev server              |
| `pnpm build`        | Production build                |
| `pnpm typecheck`    | `tsc --noEmit`                  |
| `pnpm lint`         | ESLint                          |
| `pnpm format:check` | Prettier check                  |
| `pnpm api:generate` | Regenerate OpenAPI client types |

`src/generated/api.d.ts` is generated: never edit it, run `pnpm api:generate` after every OpenAPI change and commit the result.
