# Contributing

Rentopya is a multi-company car rental SaaS built on a Next.js + Go boilerplate. Keep the conventions strict: most work is organization-scoped (`/t/{slug}`, `/v1/tenant/*`), and the platform layer (auth, RBAC, engines) stays shared.

## Working rules

- Prefer the nearest app boundary (`backend/`, `frontend/` or `site/`) before widening scope.
- Canonical env is the repo-root `.env`.
- Do not treat `.next/` or generated output as source. Commit `backend/internal/database/db` after `make -C backend sqlc`.
- New or changed `/v1` routes update `backend/docs/openapi.yaml` in the same change.
- Media URLs are API-owned. Never assemble S3 keys or bucket URLs in the browser.
- Organization-owned data: `organization_id` in every table and query, organization from `orgctx`, another organization's record → 404 (`.agents/skills/add-tenant-resource`).
- Provider credentials are per organization and encrypted; never in env, logs or responses (`.agents/skills/integration-adapter`).
- If you change a convention (envelope, RBAC, shells, env), update the matching file under `docs/` or `backend/docs/` in the same change.
- Agent checklists live under `.agents/skills/`. Index: [AGENTS.md](./AGENTS.md).

## Backend

Start from [backend/API_CONVENTIONS.md](./backend/API_CONVENTIONS.md) and [backend/DATABASE_RULES.md](./backend/DATABASE_RULES.md).

```sh
make -C backend test
make -C backend lint
make -C backend openapi-lint
```

Database tests and the RT-A01 isolation test run only with `TEST_DATABASE_URL` (CI does not set it); run them locally when you touch tenant tables, queries or `/v1/tenant` routes.

## Frontend

```sh
cd frontend
pnpm typecheck
pnpm lint
```

From repo root: `make check-i18n` (missing translation keys). Date fields use the shared pickers (`.agents/skills/admin-date-picker`). Do not add native `<input type="date" />`.

## Site

```sh
cd site
pnpm typecheck
pnpm lint
pnpm test
```

## Docs

Domain designs live in `docs/design/` (one file per area, linked from the module's package comment). Update the matching design doc when a change alters its behaviour. The full pre-PR checklist is `.agents/skills/verify-changes`.
