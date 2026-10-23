# Backend docs

Entry points for the Rentopya API (Go module `github.com/furkanmeclis/nextjs-go-boilerplate/backend`; the path is kept on purpose). Interactive contract: `/docs/` (Scalar) when the server is running.

## Route families

| Prefix | Who | Auth |
|--------|-----|------|
| `/v1/auth/*`, `/v1/notifications*`, `/v1/realtime/*`, `/v1/search*` | signed-in users | Bearer JWT |
| `/v1/platform/*` | platform admins | Bearer JWT + `platform.*` |
| `/v1/tenant/*` | organization members (company panel) | Bearer JWT + `X-Organization` header + `tenant.*` from the member's roles in that organization |
| `/v1/customer/{orgSlug}/*` | a company's customers (portal) | customer session token, never the staff JWT |
| `/v1/public/*` | branded site, widget, partners, provider webhooks | publishable / secret API key, booking session or signature |

Details: [`../API_CONVENTIONS.md`](../API_CONVENTIONS.md). Health probes `/healthz` and `/readyz` sit outside the envelope.

## Auth & HTTP

| Doc | Description |
|-----|-------------|
| [`auth.md`](auth.md) | JWT, sign-in methods, platform roles / permissions, tenancy (organization resolution, several roles per member, branch scope) |
| [`api/errors.md`](api/errors.md) | Error code catalog |
| [`../API_CONVENTIONS.md`](../API_CONVENTIONS.md) | Route families, envelope, lists, meta |
| [`openapi.yaml`](openapi.yaml) | OpenAPI 3.1 contract (every `/v1` route, same change) |

## Data & toolchain

| Doc | Description |
|-----|-------------|
| [`../DATABASE_RULES.md`](../DATABASE_RULES.md) | Database / migration / sqlc |
| [`organizations.md`](organizations.md) | Company delete: soft delete via the API, hard delete cascade guarantee |
| [`../TOOLCHAIN.md`](../TOOLCHAIN.md) | Local toolchain |
| [`../README.md`](../README.md) | Commands, CLIs and curl examples |

## Elsewhere

- Module and engine map: [`../../docs/ARCHITECTURE.md`](../../docs/ARCHITECTURE.md)
- Domain design per area (reservations, pricing, payments, integration jobs, KABİS, …): [`../../docs/design/`](../../docs/design)
- Checklists: `.agents/skills/add-tenant-resource`, `.agents/skills/integration-adapter`, `.agents/skills/db-migration`

Live routes are registered in `internal/modules/*/routes.go` (domain packages name their design doc in the package comment) and wired in `internal/httpserver/server.go`. The spec must match them: RT-A01 (`internal/httpserver/isolation_test.go`) cross-checks every `/v1/tenant` route against `openapi.yaml`.
