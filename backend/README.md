# Backend API

Go HTTP API and Asynq worker of Rentopya (`backend/` in the monorepo). Paths are `/v1/...` (no `/api` prefix): `/v1/platform/*` (platform admin), `/v1/tenant/*` (company panel, `X-Organization` header), `/v1/customer/{orgSlug}/*` (customer portal), `/v1/public/*` (branded site, widget, partners, webhooks).

The module path stays `github.com/furkanmeclis/nextjs-go-boilerplate/backend` on purpose (renaming it is churn with no benefit); do not rename it.

Layout: `cmd/` (server, worker, CLIs, provider fakes), `internal/modules/*` (one package per HTTP area; domain packages name their design doc in `../docs/design/` in the package comment), `internal/platform/*` (engines and provider layers), `migrations/`, `docs/openapi.yaml`. Map: [`../docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md).

## Quick start

From the **repo root** (canonical `.env`):

```bash
make local-dev
make create-super-admin
```

Or from this directory, after infra is up:

```bash
make migrate-up
make sqlc
make create-super-admin
make dev             # air on APP_HTTP_ADDR (default :8080)
make worker          # optional: worker as its own process (QUEUE_WORKER_INPROCESS=false)
make health          # GET /healthz and /readyz
```

Infra runs in Docker; the API runs on the host via `make dev` / `make run`. Other targets: `make test`, `make lint`, `make openapi-lint`, `make migrate-create NAME=…`, `make migrate-down`, `make search-reindex`.

| CLI (`cmd/`) | Purpose |
|--------------|---------|
| `server`, `worker` | API and Asynq worker |
| `create-super-admin` | Upsert the platform admin (`make create-super-admin`) |
| `search-reindex` | Rebuild Meilisearch indexes (`make search-reindex`) |
| `catalog-import` | Import the monthly TSB kasko list into the platform catalog (`-file … -apply`) |
| `enqueue-ping` | Smoke-test the queue (`make ping-job`) |
| `iyzico-fake`, `netgsm-fake` | Local HTTP fakes of iyzico and Netgsm ([../docs/DEVELOPMENT.md](../docs/DEVELOPMENT.md#provider-fakes)) |

Auth and RBAC: [`docs/auth.md`](docs/auth.md).  
HTTP envelope / list / meta: [`API_CONVENTIONS.md`](API_CONVENTIONS.md).  
Doc index: [`docs/README.md`](docs/README.md).

### Bootstrap platform admin

```bash
make create-super-admin SA_EMAIL=admin@localhost SA_PASSWORD='Password1' SA_NAME=Platform SA_SURNAME=Admin
```

Requires migrations (`000005_seed_rbac`) so the `super_admin` role exists. The CLI upserts the user and assigns that role via `user_roles`.

### Register a user (self-serve, no roles)

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "user@example.test",
    "password": "Password1",
    "name": "Ada",
    "surname": "Lovelace"
  }'
```

Creates a user only (`201 { user }`, no tokens). Roleless users can sign in; without an organization membership they land on the "no access yet" page. Assign roles from platform admin (`POST/PATCH /v1/platform/users` with `role_uuids[]`).

Also enqueues welcome + email verification via the notification center (MailHog locally).

### Login / me / profile

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@localhost","password":"Password1"}'

curl -sS http://127.0.0.1:8080/v1/auth/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"

curl -sS -X PATCH http://127.0.0.1:8080/v1/auth/profile \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ada","surname":"Lovelace"}'
```

### Password forgot → MailHog → reset

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/auth/password/forgot \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.test"}'
# Open http://127.0.0.1:8025 — copy the 6-digit code from the email

curl -sS -X POST http://127.0.0.1:8080/v1/auth/password/reset \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.test","code":"123456","password":"Password2"}'
```

### Notifications inbox (test)

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/notifications/test \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"email":false}'

curl -sS 'http://127.0.0.1:8080/v1/notifications?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

### Platform roles & users

```bash
curl -sS 'http://127.0.0.1:8080/v1/platform/roles?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN"

curl -sS 'http://127.0.0.1:8080/v1/platform/users?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

### Organization (tenant) requests

Organization routes need `TENANCY_ENABLED=true` and a membership. Create an organization as platform admin (Platform → Organizations, or `POST /v1/platform/organizations`), then pass its slug on every tenant request:

```bash
curl -sS http://127.0.0.1:8080/v1/tenant/context \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'X-Organization: acme'

curl -sS 'http://127.0.0.1:8080/v1/tenant/branches?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'X-Organization: acme'
```

`super_admin` has no bypass here: without a membership in `acme` the answer is `403 NO_TENANT_MEMBERSHIP`.

### Storage / activity / logs (smoke)

```bash
curl -sS 'http://127.0.0.1:8080/v1/platform/storage/objects?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN"

curl -sS 'http://127.0.0.1:8080/v1/platform/activity?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN"

curl -sS 'http://127.0.0.1:8080/v1/platform/logs?limit=20&offset=0' \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Interactive contract: http://127.0.0.1:8080/docs/

## Database & SQL

| Topic | Detail |
|-------|--------|
| Rules | [`DATABASE_RULES.md`](DATABASE_RULES.md) |
| Toolchain | [`TOOLCHAIN.md`](TOOLCHAIN.md) |
