# Development

## Prerequisites

- Go (version in `backend/go.mod`, currently 1.26)
- Node (version in `frontend/.nvmrc` / `site/.nvmrc`, currently 22) and pnpm
- Docker + Compose
- golang-migrate, sqlc, air, golangci-lint, gofumpt — see [backend/TOOLCHAIN.md](../backend/TOOLCHAIN.md)

## First clone

```sh
cp .env.example .env
```

Set these **before** sharing a Docker host with another stack:

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `*_VOLUME_NAME` | Named volumes so SeaweedFS/Postgres do not collide |

In `compose.prod.yml` every internal host is qualified as `<service>.${APP_NETWORK_NAME}` (`postgres.myapp-app`, `backend.myapp-app`, …). Never use a bare service name: on the shared `dokploy-network` it can resolve to another stack's container.

Root `.env` is canonical. `backend/.env` is a fallback only. Do not keep a long-lived `frontend/.env.local` for API URLs; `API_URL` comes from the root env.

`API_URL` **must** include `/v1` (Go routes are `/v1/...`). Default: `http://127.0.0.1:8080/v1`.

## Local

```sh
make local-dev
make create-super-admin
```

Sign in at http://localhost:3000/platform/login, create an organization under Platform → Organizations and open its panel at `/t/{slug}`. Organization routes need `TENANCY_ENABLED=true` (the `.env.example` value).

Helpers:

| Target | What it does |
|--------|----------------|
| `make infra` / `make infra-down` | Compose infra only (`make infra-ps`, `make infra-logs`) |
| `make backend-dev` | air on `:8080` |
| `make frontend-dev` | `pnpm dev` on `:3000` |
| `make site-dev` | Branded site (`site/`) on `:3100`: open `http://{subdomain}.localhost:3100` ([branded-site.md](./design/branded-site.md)) |
| `make -C backend worker` | Asynq worker as its own process (set `QUEUE_WORKER_INPROCESS=false`; by default the API runs it in-process) |
| `make migrate-up` | Apply SQL migrations |
| `make create-super-admin` | Upsert platform admin + `super_admin` role |
| `make search-reindex` | Rebuild Meilisearch indexes (users, roles, …) |
| `make check-i18n` | Find missing / unwired translation keys (en+tr, activity, RBAC, exports) |
| `make free-dev-ports` | Free :8080 / :3000 from stale air / Next.js processes |
| `make tools` | Print Docker / Compose / Go toolchain versions |

### Provider fakes

Outside production the API registers fake providers (`kabis_fake`, `einvoice_fake`, `hgs_fake`, `yolcu360_fake`, `obilet_fake`, `whatsapp_fake`); connect them under `/t/{slug}/settings/integrations` like real accounts (WhatsApp under `/t/{slug}/settings/whatsapp`). The KABİS, e-invoice and channel fakes take a non-secret `scenario` field (for example `ok`, `outage`, `timeout`, `reject`; each fake's package comment lists its values). Two providers have HTTP fakes for end-to-end runs (from `backend/`):

| Fake | Command | Env |
|------|---------|-----|
| iyzico | `go run ./cmd/iyzico-fake -addr 127.0.0.1:18090 -webhook http://127.0.0.1:8080/v1/public/payments/iyzico/webhook` | `IYZICO_SANDBOX_BASE_URL=http://127.0.0.1:18090`, connect an iyzico account in sandbox mode with the fake's keys |
| Netgsm | `go run ./cmd/netgsm-fake -addr 127.0.0.1:18091` | `NETGSM_SANDBOX_BASE_URL=http://127.0.0.1:18091`, connect a Netgsm account in sandbox mode; received messages are listed on the fake's `/` page |

`SMS_LOG_PROVIDER=true` adds an SMS provider that only writes masked log lines.

### Local URLs

| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000 |
| API | http://127.0.0.1:8080 |
| Scalar docs | http://127.0.0.1:8080/docs/ |
| Health / ready | http://127.0.0.1:8080/healthz · `/readyz` |
| Adminer | http://127.0.0.1:8081 |
| SeaweedFS filer UI | http://127.0.0.1:8888 |
| MailHog | http://127.0.0.1:8025 |
| Meilisearch | http://127.0.0.1:7700 |
| Centrifugo WS | ws://127.0.0.1:8000/connection/websocket |

Login: http://localhost:3000/platform/login

## Branding

The product is **Rentopya**: name and colors in `frontend/src/config/brand.ts`, CSS tokens in `frontend/src/styles/theme.css`, logos in `frontend/src/components/brand/`. Brand kit, usage rules and the identifiers that intentionally stay `rentopia`: [brand.md](./brand.md).

## VAPID (optional Web Push)

```sh
npx web-push generate-vapid-keys
```

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` in `.env`. The frontend registers via `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Closed-browser push does not work until these three are set.

Optional frontend public vars (root `.env`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## Production / Dokploy

1. `make prod-env`: asks only for the public app URL (e.g. `https://panel.example.com`), derives every URL from it and generates all secrets into `.env.server` (mode 600). Centrifugo gets `realtime.<app host>`; edit `REALTIME_HOST` and the two Centrifugo URLs if you want another host. Company sites answer on `{subdomain}.<host of SITE_BASE_URL>` (default: the app host).
2. `make prod-config` then `make prod-up`. On Dokploy, paste `.env.server` into the compose app's environment and point the frontend domain at port 3000, the Centrifugo domain at port 8000 and the wildcard site domain (`*.<app host>`) at the `site` service, port 3000.
3. Plain Docker host only (Dokploy creates it for you): `docker network create dokploy-network` once

### Plain Docker host with automatic updates

`scripts/server-sync.sh` (or `make prod-sync`) follows `main` (only CI-green commits reach it): when `origin/main` moved it fast-forwards, upgrades `.env.server` if the env schema changed, runs `docker compose up -d --build`, waits for the backend to be healthy and prunes old images. One run at a time (flock). Cron example:

```
*/5 * * * * /opt/rentopia/scripts/server-sync.sh >> /var/log/rentopia-sync.log 2>&1
```

Rollback: `touch .sync-paused`, `git checkout <previous sha>`, `scripts/server-sync.sh --force` (rebuilds the checked-out commit without pulling; migrations are not reverted). Remove `.sync-paused` to resume. First admin: `make prod-super-admin SA_EMAIL=… SA_PASSWORD=…` (runs inside the backend container). TSB list import from the host: `docker compose --env-file .env.server -f compose.prod.yml exec backend /app/catalog-import -file /path/in/container.xlsx -apply` (or use Platform → Katalog → İçe aktarma).

### Upgrading the env for a new release

`make prod-env-upgrade` (or `scripts/prod-env.sh upgrade <your env file>`) brings an env written by an older release up to the current schema (`deploy/env/schema`, versioned by `ENV_SCHEMA_VERSION`):

- every existing value is kept, so secrets such as `DB_PASSWORD` and `APP_ENCRYPTION_KEY` never change;
- keys renamed in the schema move with their value, new keys are generated or given their default, retired keys are commented out;
- keys the schema does not know (your own additions) are kept under "Unmanaged";
- the file is rewritten in place and the previous one is kept as `<file>.bak.<timestamp>`.

`make prod-env-check` exits non-zero when the file is behind the schema. When a release needs a new env key, add it to `deploy/env/schema` and bump `@version` in the same change.

Networks: Compose creates a private bridge network per app (`${APP_NETWORK_NAME:-${NAME_PREFIX}-app}`) and every service joins it. Only **frontend** (`:3000`), **site** (`:3000`) and **centrifugo** (`:8000`) also join the shared `dokploy-network`, so the proxy can route their domains. The backend is reached through the frontend's `/api` proxy (the site calls it on the private network), and SeaweedFS, Postgres, Redis, Meilisearch, wuzapi and the worker stay private (files reach browsers through API endpoints, never a bucket URL). No service publishes host ports.

`APP_ENCRYPTION_KEY` (32-byte, raw or base64) derives every at-rest key: OAuth / GitHub secrets, organization integration credentials, encrypted personal data and blind indexes. Generate once (`openssl rand -base64 32`, `make prod-env` does it) and never change it on an install with data.

## Error tracking (optional, Sentry protocol)

The tracker is a self-hosted, Sentry-protocol endpoint (events only; it has no `sentry-cli` API, so nothing is uploaded at build time: no source maps, no releases, no auth token). Every variable is optional; an empty DSN turns reporting off.

| Env (Dokploy / `.env.server`) | Passed as | Read by |
|---|---|---|
| `BACKEND_SENTRY_DSN` | `SENTRY_DSN` | backend API + worker |
| `FRONTEND_SENTRY_DSN` | `SENTRY_DSN` | Next.js server, edge, browser |
| `SITE_SENTRY_DSN` | `SENTRY_DSN` | branded site server + edge |
| `SENTRY_ENVIRONMENT` (default: `APP_ENV`) | same | all |
| `SENTRY_RELEASE` (optional) | same | all |
| `SENTRY_TRACES_SAMPLE_RATE` (default `0`) | same | backend only |

Local dev reads `SENTRY_DSN` from the root `.env` directly. `APP_ENV` (development / staging / production), the production endpoint guard, log masking and failed-work alerts (`ALERT_EMAIL`): [operations/observability.md](./operations/observability.md).

- **Backend**: 5xx responses and panics (route pattern, status, `request_id` tag), auth misconfiguration warnings (adapter secret mismatch, provider disabled, id_token audience/issuer), failed queue tasks (`task_type`, `task_id` and the payload's ids), integration jobs that fail or need verification (`job_kind`, `job_uuid`), and `ERROR` log lines (one event per message per minute). Requests the client cancelled are logged as `499` at INFO and never reported. One `http_request` INFO line is written per 4xx and per `/v1/auth/*` / `/v1/internal/auth/*` request (method, route, status, duration, request_id, error_code, scrubbed reason).
- **Frontend**: `instrumentation.ts` (server/edge, `onRequestError`), `instrumentation-client.ts` (browser), `global-error.tsx`, route error boundaries, the `/api/v1` BFF proxy, and the NextAuth `logger.error` hook (user errors like `CredentialsSignin` are logged, not reported). The browser gets its DSN at runtime from `/api/client-env` (loaded `beforeInteractive` in the root layout), so the image needs no build arg.
- **Never sent**: request bodies, cookies, query strings, `Authorization` / adapter headers, tokens, secrets, emails, phone numbers. The user is the internal user uuid only. Code: `backend/internal/observability`, `frontend/src/lib/observability`.

## Super admin

From the repo root (loads `.env`):

```sh
make create-super-admin
make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1' SA_NAME=Ada SA_SURNAME=Admin
```

The same target exists under `backend/` (`make -C backend create-super-admin …`).

## After a backend contract change

```sh
make -C backend sqlc
make -C backend openapi-lint
cd frontend && pnpm api:generate && pnpm typecheck
```
