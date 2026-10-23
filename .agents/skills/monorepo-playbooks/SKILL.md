---
name: monorepo-playbooks
description: >-
  Repository playbook for Rentopya (Go API + Next.js panel + branded site
  monorepo). Use when working across frontend, backend, site, compose, or
  setup conventions. Maps shells, engines, skills, and hard rules.
---

**Persona:** Keep the repo mental model sharp and prefer the nearest app boundary.

# Monorepo map

| Path | Role |
|------|------|
| `frontend/` | Next.js BFF — `/platform`, company panel `/t/{slug}`, customer portal `/r/{orgSlug}`, landing `/` |
| `site/` | Branded site, one image for every company (host → organization), books through `/v1/public/*` |
| `backend/` | Go HTTP API + Asynq worker |
| `compose.local.yml` / `compose.prod.yml` | Docker |
| `docs/` | Architecture, development, extending; `docs/design/` one design doc per domain area |
| `AGENTS.md` | Skill index |

Canonical env: **repo-root** `.env`. First clone: `.agents/skills/boilerplate-first-setup`.

## Engines (when to load a skill)

| Job | Skill |
|-----|-------|
| New platform list resource (full slice) | `add-platform-resource` |
| New organization-scoped resource under `/t/{slug}` | `add-tenant-resource` |
| New external provider / integration job kind | `integration-adapter` |
| Schema, seed, permission slug, query | `db-migration` |
| Sidebar item / badge / group | `nav-engine` |
| Export / import | `io-engine` |
| Bulk actions | `bulk-engine` |
| Uploads / media URLs / explorer | `storage-media` |
| Re-auth on sensitive actions | `step-up-engine` |
| Cmd+K / Meilisearch | `search-engine` |
| GitHub OAuth / encrypted secrets | `github-integration` |
| Date field | `admin-date-picker` |

## Working rules

- Prefer the nearest app boundary before widening scope.
- Backend: start from `backend/API_CONVENTIONS.md` and `backend/README.md`.
- Do not treat `.next/` or generated output as source. Commit `backend/internal/database/db` after sqlc.
- New `/v1` routes update `backend/docs/openapi.yaml` in the same change; then `pnpm api:generate`.
- Media URLs: API-owned only; never assemble S3 keys in the browser.
- Never expose Meilisearch to the browser (`GET /v1/search` only).
- Organizations are resolved per request from `X-Organization` (`orgctx`), never from the JWT (claims: `sub`, `roles`, `is_super_admin`, optional `imp`, `sid`). No workspaces, no `/business` shell.
- Tenant data: `organization_id` on every table and query; another organization's record answers 404 (`add-tenant-resource`).
- Handlers never call SMTP, SMS or a provider; go through notifications, the notification engine or the integration job runner.
- Permission slugs only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.

## Surfaces (today)

- `/platform` (`platform.*`): organizations, users, roles, usage and connection health, site domains, TSB catalog, notifications, settings (branding, auth, OAuth, step-up, storage, imports, exports, logs, activity).
- `/t/{slug}` (`tenant.*` from the member's roles in that organization): branches, fleet, pricing, reservations, rentals, field tasks, inspections, KABİS, tolls, customers, agreements, finance, reports, settings (members, roles, integrations, public API, branded site, notifications, WhatsApp, templates).
- `/r/{orgSlug}` customer portal, `/pay/{token}`, `/d/{token}`; `site/` branded site.

Cmd+K search: `/v1/search` (platform) and `/v1/tenant/search` (organization), per-spec RBAC. Module map: `docs/ARCHITECTURE.md`.
