---
name: integration-adapter
description: >-
  Add or change an external provider (payment, SMS, e-invoice integrator,
  KABİS, HGS / toll, channel manager, WhatsApp): provider interface and
  adapter in internal/platform, integration_jobs kinds on the runner
  (Submit / CheckStatus), per-organization encrypted credentials, retry and
  idempotency rules, platform health, OpenAPI, the Ayarlar → Entegrasyonlar
  provider entry, i18n and tests. Worked examples: Netgsm SMS, KABİS.
---

# Add an integration adapter

Every external provider plugs into one skeleton. Each organization connects its **own** account; credentials are sealed per organization and never leave the backend. Outbound work runs on the generic job runner, which never sends the same work twice unless the provider confirmed it was not done.

Read first: `docs/design/integration-jobs.md` (runner rule, states, retry), `docs/design/payments.md` §2 (accounts, vault). Area designs: `docs/design/{sms,kabis,einvoice,tolls,channel-manager,whatsapp}.md`.

| Layer | Where | Example |
|-------|-------|---------|
| Skeleton | `backend/internal/platform/integrations` | `Adapter`, `Registry`, `Vault`, `Operation`, `Result`, outcomes |
| Provider layer (per kind) | `backend/internal/platform/<area>` | `platform/sms` (`Provider`), `platform/kabis` (`Provider`, `Official`, errors) |
| Adapter | `backend/internal/platform/<area>/<provider>` | `sms/netgsm`, `payments/iyzico`, `channels/yolcu360` |
| Fake | `backend/internal/platform/<area>/<provider>fake` or `<area>fake` | `sms/netgsm/netgsmfake`, `kabis/kabisfake`, `tolls/tollfake` |
| Fake CLI (optional) | `backend/cmd/<provider>-fake` | `cmd/netgsm-fake`, `cmd/iyzico-fake` |
| Operations + module | `backend/internal/modules/<area>` | `modules/sms/usecase/service.go` (`sendOperation`), `modules/kabis/usecase/operation.go` |
| Accounts API | `backend/internal/modules/integrations/accounts` | `/v1/tenant/integrations*` (no change needed for a new provider) |
| Runner + jobs API | `backend/internal/modules/integrations/jobs`, `backend/internal/queue/integrations.go` | `/v1/tenant/integration-jobs/*` |
| Wiring | `internal/httpserver/server.go` **and** `cmd/worker/main.go` | `integrationAdapters`, `integrationOps`, `integrationRunner` |
| Panel | `frontend/src/features/payments` (`integrations-page.tsx`, `integration-dialogs.tsx`, `integration-jobs.tsx`, `constants.ts`) | `/t/{slug}/settings/integrations` |

## 1. Provider interface and adapter

1. Kind: reuse an existing kind (`payment`, `sms`, `toll`, `kabis`, `einvoice`, `channel`, `whatsapp`) or add one (`const Kind = "<kind>"` in `platform/<area>`).
2. In `platform/<area>`, define `Provider` as `integrations.Adapter` plus the domain calls. Each call must make its result classifiable:
   - a definitive "not done" error (`*sms.Rejected`, `kabis.ErrRejected`),
   - "nothing left the process" (`kabis.ErrUnavailable`: safe to retry),
   - `integrations.ErrUnsupported` for a call the provider cannot do (never a silent success),
   - any other error = outcome unknown.
   - A lookup by our own reference or the provider reference (`Status`, `Lookup`) is what makes retries safe; without it the job stops at `needs_verification` for a person.
3. The adapter implements `Kind()`, `Provider()` (provider code, `[a-z0-9_]`, ≤ 24 chars), `Fields()` (`Key`, `LabelKey` = `payments.integrations.fields.<key>`, `Secret`, `Required`) and `Check(ctx, account)`: one cheap authenticated call, no money or message moves. It is what "Bağlantıyı test et" runs.
4. Base URLs: sandbox / live defaults in the adapter, overridable by `internal/config` (`<PROVIDER>_SANDBOX_BASE_URL` / `_LIVE_BASE_URL`, like iyzico and Netgsm) or by a non-secret `base_url` credential (Yolcu360, Obilet). `account.Mode` (`sandbox` | `live`) picks one. Credentials never come from env.
5. Unverified official method (KABİS, e-invoice integrator): keep an `Official` slot that stores credentials but answers `ErrDependencyPending` from `Check` / `Ready()` and `ErrUnsupported` from the calls; the module holds its jobs in `waiting_dependency` and never sends them.
6. Fake: same `Provider`, in-memory, provider code `<provider>_fake`, a non-secret `scenario` field (`ok`, `outage`, `timeout`, `reject`) plus `FailNext*` hooks for tests. Optional HTTP fake + `cmd/<provider>-fake` when the real adapter speaks HTTP (point the sandbox base URL at it).
7. Module helpers in `modules/<area>`: `Providers(cfg, appEnv, log)` returns the real adapters plus the fake **only outside production**; `Adapters(providers)` converts them to `[]integrations.Adapter`.

## 2. Job kinds and operations

1. Job kind `<area>.<verb>` (`sms.send`, `kabis.rental_start`, `einvoice.send`, `einvoice.cancel`, `toll.fetch`) as a constant in the module's `model`.
2. Implement `integrations.Operation` in the module (`Kind`, `Submit`, `CheckStatus`), one per provider and kind, registered in the service constructor: `ops.Register(p.Provider(), &sendOperation{svc: s, provider: p})`. Mapping:

| Provider answer | `Submit` returns | `CheckStatus` returns |
|---|---|---|
| done | `OutcomeSucceeded` (+ `ProviderRef`) | `OutcomeSucceeded` |
| queued, final state later | `OutcomeAccepted` (+ `ProviderRef`) | `OutcomePending` |
| definitive refusal | `OutcomeFailed`, `Retryable` only for temporary provider errors | `OutcomeFailed` |
| provider has no record | — | `OutcomeNotFound` |
| nothing left (unavailable) | `OutcomeFailed`, `Retryable: true` | — |
| timeout / 5xx after send / unreadable 2xx | `integrations.Uncertain(err, req)` | `OutcomeUncertain` |
| cannot do it | `OutcomeUnsupported` + `ErrUnsupported` | same (job stays `needs_verification`) |
| our stored data unusable (provider not called) | `OutcomeFailed` with the reason | — |

3. `Result.Request` / `Response` are short summaries for `integration_attempts`: references, codes, counts, masked values (`pii.MaskPhone`). Never credentials, message bodies, OTP codes, identity numbers or card data.
4. Job payloads hold references only (`{"sms_message_uuid": …}`); the operation loads the sensitive data from the module's own table, sealed with its own purpose (`fc.Sealer("<area>.payload")`, AAD bound to organization + row uuid).
5. Create jobs with `runner.Enqueue(ctx, jobsusecase.EnqueueInput{OrganizationID, AccountID, Provider, Kind, IdempotencyKey: "org:<id>:<area>:<natural key>", Payload, RefType, RefUUID, Critical, MaxAttempts})`. A repeated key returns the existing job and sends nothing. When the job row must be created inside your own transaction (KABİS records it in the handover / return transaction), insert it `pending` with `next_attempt_at = now()` and call `runner.Kick(ctx, org, job)` after commit; the minute sweep catches it if the process dies.
6. Follow job state in the module with `runner.AddListener` (`IntegrationJobChanged`), filtering by kind prefix (`sms.`, `kabis.`). Changes made outside a provider call: `runner.Changed(ctx, org, job)`.
7. `Critical: true` only for OTP / transactional messages (queue `integrations_critical`).

## 3. Credentials and encryption

- Credentials are validated with `integrations.ValidateCredentials`, sealed by `integrations.Vault` (field cipher purpose `integration.credentials`, AAD `integration_accounts.credentials|org:<id>|account:<uuid>`), and shown only as `integrations.Hint` masks. All keys derive from `APP_ENCRYPTION_KEY` (`platform/crypto`); never change it on a deployed install.
- The accounts API (`/v1/tenant/integrations`: list, connect, rotate, test, disable / enable, jobs) is generic: it lists every registered adapter from the `Registry`, behind `tenant.integrations.manage`. A new provider needs no new account route.
- Migration (`.agents/skills/db-migration`): rewrite `integration_accounts_provider_check` (and `integration_accounts_kind_check` for a new kind) with the **full** list, naming the previous migration in a comment (see `000121_obilet_channel.up.sql`). The down migration deletes the provider's jobs and accounts, then restores the old list. Check the one-active-account index for the kind (`uq_integration_accounts_organization_active_kind`; channels use their own per-channel index, `000118`).
- Module tables go in `queries/tenant_tables.txt`; `integration_jobs` / `integration_attempts` are mixed scope (`mixed_scope_tables.txt`).

## 4. Retry and idempotency (do not bypass)

- The runner owns `integration_jobs.status`; modules move only the held states (`blocked`, `waiting_dependency`) and staff resolutions (`reported_manually`, `reconciled`).
- Backoff 15 s doubling to a 30 min cap (`usecase.Backoff`); `max_attempts` bounds automatic calls (`EnqueueInput.MaxAttempts`, else `DefaultMaxAttempts`); at the limit a person decides. Asynq retries only cover infrastructure errors.
- Staff retry `POST /v1/tenant/integration-jobs/{uuid}/retry` runs `CheckStatus` first; a module-specific retry (KABİS re-validation: `/v1/tenant/kabis-notifications/{uuid}/retry`) must keep that order.
- Inbound webhooks: public route under `/v1/public/...`, verify the signature with the account's secret, upsert an `inbound` `integration_jobs` row by the provider's event id so a redelivery only counts `deliveries` (payments: `modules/payments`, `/v1/public/payments/iyzico/webhook`).

## 5. Wiring

In `internal/httpserver/server.go` **and** `cmd/worker/main.go`:

```go
xProviders := xmodule.Providers(cfg.App.Env, log)                    // or (cfg.X, cfg.App.Env, log)
integrationAdapters = append(integrationAdapters, xmodule.Adapters(xProviders)...)
// after integrationRunner / integrationOps exist:
xSvc, err := xusecase.New(deps.DB, deps.Queries, fieldCipher, integrationAccountsSvc, integrationRunner, integrationOps, xProviders...)
```

The worker runs the steps (`app:integrations:run`) and the sweep (`app:integrations:sweep`); both need the operation registered there too. New env keys (base URLs): `internal/config`, `.env.example`, `.env.server.example`, and `deploy/env/schema` with an `@version` bump.

## 6. Platform health

The built-in sources (`modules/platformhealth/sources`: integration accounts, integration jobs) already cover every provider's account checks and failed / `needs_verification` / `waiting_dependency` / overdue jobs. A provider with its own state (sync cursors, sessions, inbound queue) adds a `healthcheck.Source` (`Code()`, `Collect()`; issues carry organization, provider, subject, last error, reference uuid, never secrets or personal data) and registers it on `platformhealthmodule.NewRegistry(...)` in both composition roots (`channelsmodule.NewHealthSource` is the example). Add `platform_health.sources.<code>` / `_hint` and new metric / kind labels in `locales/{tr,en}/platform_health.json` (unknown codes fall back to a humanized label).

## 7. OpenAPI

`backend/docs/openapi.yaml`: add the provider code to every `provider` enum of the integration account schemas (search `obilet_fake`), its credential keys to the connect body description, new job kinds to the job schema description, and any new module route (tag `Tenant`, `XOrganization` header). Then `npx --yes @redocly/cli@1 lint backend/docs/openapi.yaml` and `cd frontend && pnpm api:generate`.

## 8. Frontend provider entry

`frontend/src/features/payments/constants.ts`:

- `INTEGRATION_PROVIDER_LABEL_KEYS[<provider>]` and `INTEGRATION_PROVIDER_HINT_KEYS[<provider>]` (connect dialog hint: where the organization finds its keys; for an official slot, say it waits on dependency),
- `INTEGRATION_FIELD_LABEL_KEYS[<field>]` for every new credential key,
- `INTEGRATION_JOB_KIND_LABEL_KEYS[<area>.<verb>]`.

The connect dialog builds its form from `GET /v1/tenant/integrations/meta` (providers and fields), so no form code changes for a plain provider. Provider-specific actions (test SMS, WhatsApp QR pairing, channel mappings) live in the area's feature and stay shadcn + DataTable (AGENTS.md UI rules).

## 9. i18n

`frontend/src/locales/{tr,en}/payments.json` (Turkish first, en complete): `integrations.providers.<provider>`, `integrations.provider_hints.<provider>`, `integrations.fields.<key>`, `integrations.jobs.kinds.<area>_<verb>`. Activity actions / resources of the module: `activity.json`. Backend labels (`LabelKey`s, ioengine) in `internal/platform/i18n/catalog.go`. Run `node scripts/check-i18n.mjs` (0 errors).

## 10. Tests

- Adapter against its HTTP fake (`netgsm/netgsm_test.go`, `iyzico/iyzico_test.go`): request signing, success, definitive refusal, transport error = unknown, lookup.
- Operation outcome mapping and the runner rule with a scripted fake (`modules/integrations/jobs/usecase/runner_test.go`, `modules/kabis/integration_test.go` `TestRT_A21_*`): uncertain submit → `needs_verification` → `CheckStatus` → succeeded with exactly one submit; `not_found` → resubmitted once; unsupported → nothing sent; idempotency key; another organization's job → 404.
- Seed a job / account for RT-A01 when new `/v1/tenant` routes take uuids (`internal/httpserver/isolation_test.go`, needs `TEST_DATABASE_URL`).
- Migration round trip (`migrate up` → `down 1` → `up`).
- `.agents/skills/verify-changes`.

## Do not

- Put provider credentials in env, logs, job payloads, attempt summaries or API responses
- Resend work whose outcome is unknown without a `CheckStatus` that confirmed "not done"
- Return success for an operation the adapter cannot perform
- Register a fake provider in production
- Call a provider from an HTTP handler (go through the module use case and the runner)
