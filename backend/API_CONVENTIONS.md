# API conventions

HTTP contract of the Rentopya API (Go module `github.com/furkanmeclis/nextjs-go-boilerplate/backend`; the path is kept on purpose). Paths use `/v1/...` (no `/api` prefix).

| Prefix | Who | Auth |
|--------|-----|------|
| `/v1/auth/*`, `/v1/notifications*`, `/v1/realtime/*`, `/v1/search*` | signed-in users | Bearer JWT |
| `/v1/platform/*` | platform admins | Bearer JWT + `platform.*` |
| `/v1/tenant/*` | organization members | Bearer JWT + `X-Organization` header + `tenant.*` |
| `/v1/customer/{orgSlug}/*` | a company's customers (portal) | customer session token, never the staff JWT |
| `/v1/public/*` | branded site, widget, partners, provider webhooks | publishable / secret API key, booking session or signature |

## Response envelope

All `/v1/*` handlers return:

```json
{
  "success": true,
  "data": {},
  "meta": { "request_id": "uuid" }
}
```

Errors:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "...",
    "details": [{ "field": "email", "message": "...", "code": "invalid" }]
  },
  "meta": { "request_id": "uuid" }
}
```

- Success: `response.JSON(w, r, status, data)`
- Errors: `Unauthorized` / `Forbidden` / `NotFound` / `Conflict` / `ValidationError` / `Internal` / `FeatureNotEnabled` (`403 FEATURE_NOT_ENABLED`: a change in a plan-gated area outside the organization's plan; `details[0]` is `{field: "feature", code: <feature key>}`; module error mappers call `features.WriteError` first)
- Branch on `error.code`, not message text
- Correlation: middleware sets `X-Request-ID` (generate if missing) and mirrors into `meta.request_id`
- **Outside envelope:** `GET /healthz`, `GET /readyz`

## Lists

```json
{
  "success": true,
  "data": { "items": [], "total": 0, "limit": 20, "offset": 0 },
  "meta": { "request_id": "..." }
}
```

Parse with `pkg/apiquery`:

| Param | Notes |
|-------|--------|
| `limit` | default 20, max 100 |
| `offset` | default 0 |
| `q` | search text |
| `sort` | e.g. `-created_at,name` |
| `include` / `fields` | sparse / expand (when supported) |

**Forbidden query names:** `page`, `page_size`, `per_page`, `search` (use `q` only). Unknown `sort` → `400 VALIDATION_ERROR` with details.

## Resource meta

Primary list resources expose `GET /v1/{resource}/meta` via `internal/platform/resourcemeta`:

- `GET /v1/platform/users/meta`
- `GET /v1/platform/roles/meta`
- `GET /v1/notifications/meta`
- `GET /v1/platform/notifications/meta`
- `GET /v1/platform/activity/meta`
- `GET /v1/platform/storage/meta`
- `GET /v1/platform/logs/meta`
- `GET /v1/platform/log-rules/meta`
- `GET /v1/platform/catalog/{brands,models,types,imports}/meta`
- every organization list: `GET /v1/tenant/<plural>/meta` (`/v1/tenant/branches/meta`, `/v1/tenant/vehicles/meta`, …)

Register `/meta` **before** `/{uuid}` on `ServeMux`. Capabilities must match live routes (do not advertise unfinished bulk/export).

## Auth

- Bearer JWT on protected routes
- Global users; no organization or workspace claim in the token
- Tenant routes: `X-Organization: <slug|uuid>` on every request → `400 ORGANIZATION_REQUIRED` when missing, `403 NO_TENANT_MEMBERSHIP` for an unknown organization or a non-member (same answer), `403 FORBIDDEN` without the tenant permission, `403 FEATURE_NOT_ENABLED` for a change in a plan-gated area the organization's plan does not include (`details[0].code` = feature key; reads and exports stay open, TEC-674). Another organization's record answers `404` ([docs/auth.md](docs/auth.md#tenancy-organizations))
- User fields: `name` / `surname`
- Platform routes require the matching `platform.*` permission (`super_admin` bypasses `HasPermission`); tenant permissions have no super-admin bypass

## Notifications

Domain / auth use cases call `notifications.Service.Enqueue(...)` for user notifications. Customer messages of an organization (SMS, e-mail, push, WhatsApp) go through the notification engine (`modules/notifyengine`), provider work through the integration job runner. HTTP handlers never call SMTP, SMS or a provider inline.

- Channels: `inapp` | `email` | `realtime` | `sms` | `push` (sms/push = noop unless VAPID is configured)
- Delivery: Asynq task `app:notification:deliver` on queue `notifications`
- **Security emails** (`auth.password_reset`, `auth.email_verification`) ignore `email_enabled` preference
- Realtime channel hint: `user:{user_uuid}`

Cross-module signals use the platform event bus (`internal/platform/events`). Use cases `Publish`; bus handlers may enqueue notifications — never SMTP.

## OpenAPI

Contract: `docs/openapi.yaml` (OpenAPI 3.1) + Scalar at `/docs/`. Run `make openapi-lint` after path/schema changes.

Live routes are registered in `internal/modules/**/routes.go`; every `/v1` route must be in the spec in the same change (RT-A01 cross-checks `/v1/tenant` routes against it). Tenant routes use tag `Tenant` and the `XOrganization` header parameter.
