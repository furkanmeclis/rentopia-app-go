---
name: verify-changes
description: >-
  Run the same checks CI runs (backend vet/test/lint, OpenAPI lint, frontend
  typecheck/lint/i18n/build, site typecheck/lint/test/build, prod env schema)
  before committing or opening a PR. Use before saying a
  change is done.
---

# Verify changes

CI (`.github/workflows/ci.yml`) runs on every branch push. Run the matching subset locally first.

## Backend (any `backend/**` change)

```bash
cd backend
gofmt -l .            # must print nothing
go build ./...
go vet ./...
go test ./...
make openapi-lint     # when backend/docs/openapi.yaml changed
```

`make lint` runs golangci-lint (needs `make tools`).

## Frontend (any `frontend/**` change)

```bash
cd frontend
pnpm api:generate     # after OpenAPI changes; commit src/generated/api.d.ts
pnpm typecheck
pnpm lint
pnpm format:check
pnpm build            # slow; run before a PR
```

From the repo root: `node scripts/check-i18n.mjs` (CI runs it; 0 errors).

Stale `.next/types` after deleting a route: `rm -rf .next/types` and typecheck again.

## Site (any `site/**` change)

```bash
cd site
pnpm typecheck
pnpm lint
pnpm format:check
pnpm test
pnpm build
```

## Database

New migration → round-trip it (see the `db-migration` skill). Database tests (`TEST_DATABASE_URL`, including the RT-A01 isolation test in `internal/httpserver`) are skipped in CI; run them locally when you touch tenant tables, queries or `/v1/tenant` routes.

## Prod compose / env schema

```bash
scripts/prod-env.sh init -o /tmp/prod.env --app-url https://ci.example.com
docker compose --env-file /tmp/prod.env -f compose.prod.yml config -q
scripts/prod-env.sh check /tmp/prod.env
```

(the CI `prod-env` job; run it when `deploy/env/schema` or `compose.prod.yml` changed)

`NAME_PREFIX` must be set; internal hosts stay `<service>.<app network>`.

## Shipping

Push a feature branch: when backend + frontend pass, the `auto-pr` job opens a PR into `main` and merges the tested commit. Put `[no-automerge]` in the head commit message to keep the PR open for review.
