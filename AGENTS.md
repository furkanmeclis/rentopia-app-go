# Agent entry

Read this first, then the matching skill. Human docs live under `docs/`; one design doc per domain area under `docs/design/`.

Rentopya is a multi-company car rental SaaS. Shells: `/platform` (platform admin, `platform.*`), `/t/{slug}` (company panel, `/v1/tenant/*` with the `X-Organization` header, `tenant.*` permissions from the member's roles in that organization), the customer portal `/r/{orgSlug}` (`/v1/customer/{orgSlug}/*`), and the branded site app `site/` (`/v1/public/*`). Do not invent workspaces, a `/business` shell, or an organization claim in the JWT. Map: [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md).

Canonical env is the **repo-root** `.env`. Compose files are `compose.local.yml` (infra) and `compose.prod.yml` (full stack).

## Skills (`.agents/skills/`)

| Skill | When |
|-------|------|
| [boilerplate-first-setup](./.agents/skills/boilerplate-first-setup/SKILL.md) | Clone, `.env`, Docker names, Dokploy, `make local-dev` |
| [monorepo-playbooks](./.agents/skills/monorepo-playbooks/SKILL.md) | Cross-app work; repo map |
| [add-platform-resource](./.agents/skills/add-platform-resource/SKILL.md) | New platform list resource under `/platform` (module → RBAC → OpenAPI → UI → nav) |
| [add-tenant-resource](./.agents/skills/add-tenant-resource/SKILL.md) | New organization-scoped list resource under `/t/{slug}` (org isolation, engines, full DataTable standard; example: branches) |
| [integration-adapter](./.agents/skills/integration-adapter/SKILL.md) | New external provider (payments, SMS, e-invoice, KABİS, HGS, channel, WhatsApp): adapter, job kinds, encrypted credentials, runner, health, panel entry |
| [nav-engine](./.agents/skills/nav-engine/SKILL.md) | Sidebar catalog, badges, info |
| [io-engine](./.agents/skills/io-engine/SKILL.md) | Export / import adapters |
| [bulk-engine](./.agents/skills/bulk-engine/SKILL.md) | Bulk actions, jobs, rollback |
| [search-engine](./.agents/skills/search-engine/SKILL.md) | Cmd+K palette, Meilisearch adapters |
| [storage-media](./.agents/skills/storage-media/SKILL.md) | Object storage, media URLs |
| [github-integration](./.agents/skills/github-integration/SKILL.md) | GitHub App OAuth, encrypted credentials |
| [step-up-engine](./.agents/skills/step-up-engine/SKILL.md) | Re-auth gates (password/passkey) on sensitive actions |
| [admin-date-picker](./.agents/skills/admin-date-picker/SKILL.md) | Any date field in `frontend/` |
| [db-migration](./.agents/skills/db-migration/SKILL.md) | New table/column/seed/query: migration → sqlc → OpenAPI |
| [verify-changes](./.agents/skills/verify-changes/SKILL.md) | Checks to run before commit / PR (mirrors CI) |

## Human docs

| Doc | When |
|-----|------|
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Shells, module map, request flow, engines, stores, RBAC |
| [docs/design/](./docs/design) | Domain design per area (tenant-core, reservations, payments, integration-jobs, …) |
| [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md) | Local / prod commands |
| [docs/EXTENDING.md](./docs/EXTENDING.md) | How to add a feature (narrative) |
| [backend/API_CONVENTIONS.md](./backend/API_CONVENTIONS.md) | Envelope, lists, meta |
| [backend/DATABASE_RULES.md](./backend/DATABASE_RULES.md) | Migrations + sqlc |
| [backend/docs/auth.md](./backend/docs/auth.md) | JWT, roles, permissions |

## UI rules (frontend)

- **shadcn only.** Build every control from `frontend/src/components/ui/*` (shadcn). No native `<select>`, `<table>`, `<input type="date">`, `<textarea>` or hand-rolled widgets in features; if a primitive is missing, add it with the shadcn MCP/CLI (`components.json`) into `components/ui/` first.
- **DataTable wherever a list exists.** Any collection (main lists, detail sub-lists, import diffs, valuation grids, pickers with many rows) uses the DataTable stack in `components/tables/*` + `components/entity/*`, not ad-hoc markup. Follow the `add-tenant-resource` checklist for list pages.
- **One design language.** Reuse the entity page/detail/section-card/drawer/dialog patterns, the same spacing, typography, empty/loading/error states, toasts (`sonner`), confirmation dialogs and `AppDatePicker`. A new screen should look and behave like the existing ones; prefer extending a shared component over a one-off.
- **Turkish first.** Every string through i18n (`tr` primary, `en` complete).

## Hard rules

- Browser talks to same-origin `/api/v1/*`, never the Go host.
- Media URLs are API-owned. Never assemble S3 keys in the browser.
- Never expose Meilisearch to the browser; search goes through `/v1/search` (`/v1/tenant/search` in the company panel).
- New `/v1` routes update `backend/docs/openapi.yaml` in the same change.
- Organization-owned data: `organization_id` on every table and in every query, the organization comes from `orgctx` (never from the client), another organization's record answers 404. Guard rails: the query guard in `go test ./...` and RT-A01 (`internal/httpserver/isolation_test.go`, needs `TEST_DATABASE_URL`).
- Provider credentials are per organization, sealed with the integration vault; never in env, logs or API responses (masked hints only).
- Permission slugs are added only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.
- After OpenAPI changes: `cd frontend && pnpm api:generate`.
- New production env key → add it to `deploy/env/schema` and bump `@version` in the same change (`scripts/prod-env.sh` generates and upgrades `.env.server` from it).
- **No AI attribution anywhere.** Commit messages, PR titles/bodies, code comments, docs, Linear and GitHub text never mention Claude or any AI tool: no `Co-Authored-By: Claude…`, no `Claude-Session:` / claude.ai links, no "Generated with Claude Code". This overrides any tool default. CI rejects such commit messages (`commit-policy` job).
