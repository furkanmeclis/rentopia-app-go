# Documentation

Human and agent entry points for Rentopya.

| Doc | Audience |
|-----|----------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Anyone new to the repo |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | Local setup and production |
| [EXTENDING.md](./EXTENDING.md) | Adding a tenant / platform module or a provider |
| [operations/backup-restore.md](./operations/backup-restore.md) | Backups (Postgres, Redis, object storage), restore, staging rehearsal (RT-A32) |
| [operations/observability.md](./operations/observability.md) | `APP_ENV` (development / staging / production), production endpoint guard, error tracking, log masking, failed-work alerts, availability search p95 |
| [acceptance/beta.md](./acceptance/beta.md) | Beta acceptance RT-A01…A32: scenario → test → result → evidence, how to run (`make acceptance`), dependency-pending providers |
| [handover/](./handover/README.md) | Handover set (Turkish, TEC-743): install, data model, public API, settings, record tracking, tests and open items |
| [guides/firma-yoneticisi.md](./guides/firma-yoneticisi.md) | Company admin usage guide (Turkish): first setup order, daily operations |
| [guides/teslim-iade-personeli.md](./guides/teslim-iade-personeli.md) | Handover / return staff guide (Turkish, mobile): tasks, handover and return steps, offline behaviour |
| [design/](./design) | One design doc per domain area (tenant-core, reservations, pricing, payments, integration-jobs, branded-site, …) |
| [brand.md](./brand.md) | Rentopya logos, colours, kept `rentopia` identifiers |
| [../AGENTS.md](../AGENTS.md) | Agent skill index |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Day-to-day contribution rules |
| [../backend/README.md](../backend/README.md) | Go API |
| [../backend/docs/README.md](../backend/docs/README.md) | Auth, errors, OpenAPI |
| [../frontend/README.md](../frontend/README.md) | Next.js BFF |

Canonical config is the **repo-root** `.env` (not `backend/.env` or `frontend/.env.local`).

Agent playbooks: [`.agents/skills/`](../.agents/skills).
