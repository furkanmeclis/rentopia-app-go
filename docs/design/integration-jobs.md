# Integration job runner

Status: **implemented** (M3: TEC-751). Builds on the integration skeleton and payments ([payments.md](./payments.md) §2: `integration_accounts`, `integration_jobs`, the credential vault). The first user is SMS ([sms.md](./sms.md)); KABİS, HGS, e-invoice and channel managers plug in the same way.

Code:

| Area | Package |
|------|---------|
| Operation contract (`Submit` / `CheckStatus`, outcomes, `ErrUnsupported`) | `backend/internal/platform/integrations/operation.go` |
| Runner (state machine, attempts, sweep, staff retry) | `backend/internal/modules/integrations/jobs/usecase` |
| API (`/v1/tenant/integration-jobs`) | `backend/internal/modules/integrations/jobs` |
| Queues, dispatcher, sweep schedule | `backend/internal/queue/integrations.go` |
| Panel (job list per account, job drawer, retry) | `frontend/src/features/payments/components/integration-jobs.tsx` |

## 1. The rule (requirements §8.4)

An outbound call whose result is not known — a timeout, a dropped connection, an HTTP 5xx after the request left, an unreadable 2xx — may have been carried out by the provider. Such a job goes to `needs_verification` and **only `CheckStatus` may move it on**. The same work is submitted again only when the provider confirmed that it was not done (`not_found`, or `failed` that is retryable). An operation an adapter cannot perform returns `integrations.ErrUnsupported`: a submit then fails explicitly, a status check leaves the job in `needs_verification` for a person. Work that was not done never looks successful; an error never counts as success (the runner reclassifies "error + success outcome" and "no outcome" as `uncertain`).

## 2. Contract

```go
type Operation interface {
    Kind() string                                    // "sms.send"
    Submit(ctx, account, job) (Result, error)        // succeeded | accepted | failed (+Retryable) | uncertain
    CheckStatus(ctx, account, job) (Result, error)   // succeeded | pending | failed | not_found | uncertain
}
```

Operations are registered per provider in an `integrations.OperationRegistry`; the runner looks them up by `(job.provider, job.kind)`. `Result` carries the provider reference (`ProviderRef`, e.g. the Netgsm jobid), short request / response summaries for the attempt row and a message. `integrations.Uncertain(err, request)` builds the unknown-outcome result.

## 3. Tables (migration 000086)

`integration_jobs` keeps its status column; the runner's vocabulary maps onto it and the CHECK is extended (the column widens to `VARCHAR(24)`):

| Runner state | `status` |
|--------------|----------|
| pending_submit | `pending` |
| (claimed by payments' inline flow) | `processing` (unchanged) |
| submitted (accepted, final state from CheckStatus) | `submitted` (new) |
| succeeded / failed | `succeeded` / `failed` (`failed` always means "the provider confirmed it was not done") |
| needs_verification | `needs_verification` (new) |
| notification needing no action | `ignored` (unchanged) |

New columns: `max_attempts` (default 5; SMS 8), `next_attempt_at`, `provider_ref`, `submitted_at`, `locked_until` (a crash-safe claim: one provider call at a time, an expired claim is free) and `priority` (`critical` for OTP / transactional SMS, `normal` otherwise). `idx_integration_jobs_due` serves the sweep.

`integration_attempts` (mixed scope like the jobs; in `mixed_scope_tables.txt`): one row per provider call — `attempt_no`, `operation` (`submit | check_status`), `trigger` (`worker | sweep | retry | inline`), `outcome`, `request_summary` / `response_summary` (JSONB, references and non-secret facts only: never credentials, SMS bodies, OTP codes, full phone numbers or card data), `error`, `started_at` / `finished_at` / `duration_ms`.

The down migration drops the attempts, the new columns and every SMS account / SMS job, and rewrites remaining `submitted` / `needs_verification` jobs to `failed` (stated in the file).

## 4. Runner

- `Enqueue` creates the job once per `(provider, idempotency_key)` (`org:<id>:…`); a repeated key returns the existing job and sends nothing. With a queue the first step goes to the worker; without one (tests, a server without Redis) the first step runs inline and later steps wait for the sweep.
- `Process` runs one step: `pending` → `Submit`; `submitted` / `needs_verification` → `CheckStatus`. Every call is an attempt row. Transitions:
  - Submit: `succeeded` → succeeded; `accepted` → submitted (poll the status after 30 s); `failed` → pending with exponential backoff when retryable and attempts remain, else failed; `unsupported` → failed ("unsupported: …"); `uncertain` → needs_verification (check after the backoff).
  - CheckStatus: `succeeded` → succeeded; `pending` → submitted (poll again with backoff); `not_found` / retryable `failed` → pending (submit again, now) while attempts remain; non-retryable `failed` → failed; `uncertain` → needs_verification (backoff); `unsupported` → needs_verification with no automatic next step.
- Backoff: 15 s, 30 s, 1 min, 2 min … capped at 30 min (`usecase.Backoff`). `max_attempts` bounds automatic calls; at the limit the job keeps its state with no next step (a person decides).
- **Asynq**: queues `integrations` (weight 2) and `integrations_critical` (weight 6, OTP / transactional), task `app:integrations:run` (`{organization_id, job_id}`). Later steps are enqueued with `ProcessIn(backoff)`. Asynq's own retries (max 5, exponential 10 s … 10 min via the server's `RetryDelayFunc`) only cover infrastructure errors of a step. `app:integrations:sweep` runs every minute on the maintenance queue: due jobs (lost tasks, delivery report polling).
- Listeners (`Runner.AddListener`) hear every state change; the SMS log follows its send jobs this way. Each change publishes the live list resource `tenant.integration_jobs` (read permission `tenant.integrations.manage`).

## 5. Staff retry ("Yeniden dene")

`POST /v1/tenant/integration-jobs/{uuid}/retry` (`tenant.integrations.manage`):

1. `pending`: nothing was sent yet (or the provider already confirmed "not done") → submit now.
2. `failed`, `needs_verification`, `submitted`: **CheckStatus first** (attempt row with trigger `retry`).
   - done → `succeeded`, answer `already_done`;
   - still pending → `submitted`, answer `in_progress`;
   - confirmed not done → submit again, answer `resubmitted`;
   - unknown / unsupported → nothing is sent, `409 INTEGRATION_JOB_UNVERIFIED` with the job in `data`.
3. A `failed` job without a provider reference was rejected by the provider's own definitive answer (a `failed` status is never "unknown"); that recorded answer is the status check (`response_summary.source = recorded_provider_answer`).
4. A staff retry may use three calls beyond `max_attempts`.

Not retryable (`409 INTEGRATION_JOB_NOT_RETRYABLE`): succeeded / ignored jobs, inbound notifications and `payment.refund` jobs (refunds run through `/v1/tenant/payments/{uuid}/refund`). A job held by a worker answers `409 INTEGRATION_JOB_BUSY`.

**Step-up**: the tenant shell has no step-up policy yet ([payments.md](./payments.md) §2.4). When one exists, `retry` joins `connect` / `PATCH` / `disable` behind `middleware.RequireStepUp` in `integrations/jobs/routes.go`.

## 6. Payments

The payments flow is unchanged (RT-A16 passes): the webhook keeps its upsert / claim / finish on `integration_jobs` (`pending → processing → succeeded | failed`), and refunds stay inline. The one adaptation is low-risk: every iyzico refund call now also writes an `integration_attempts` row (trigger `inline`), so payment jobs show their provider calls in the same drawer. Moving refunds onto `Submit` / `CheckStatus` needs an iyzico refund lookup (by `conversationId`) that is not verified against the sandbox yet; it is listed in §9.

## 7. API

```text
GET  /v1/tenant/integrations/{uuid}/jobs        page of an account's jobs (status, kind filters; sort created_at / updated_at)
GET  /v1/tenant/integration-jobs/{uuid}         job + attempt log
POST /v1/tenant/integration-jobs/{uuid}/retry   §5
```

All behind `RequireOrganization` + `tenant.integrations.manage`; RT-A01 covers them (`integration-jobs/uuid` → the seeded job of a test SMS).

## 8. Tests

`integrations/jobs/usecase/runner_test.go` (DB + a scripted fake operation): uncertain submit → needs_verification → CheckStatus → succeeded with exactly one submit (and a later retry refuses); not_found → resubmitted once; unsupported status → stays needs_verification, the retry answers `VerifyError` and sends nothing; definitive failure → retry checks the recorded answer and resubmits; unknown operation fails explicitly; retryable failures stop at `max_attempts`; idempotency key; another organization gets 404.

## 9. Open items

- iyzico refunds as runner jobs (needs a verified refund lookup for `CheckStatus`).
- A tenant step-up policy for retry (§5).
- A platform-level job list (jobs with `organization_id IS NULL`); none exist yet.
- Retention of `integration_attempts` (no purge rule yet; rows are small).
