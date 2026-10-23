# Rentopya

Rentopya is a multi-company, multi-branch car rental SaaS. One installation serves many rental companies (organizations); each company works in its own panel with its own branches, roles, fleet, prices, customers and provider accounts, and gets its own branded booking site.

Monorepo: Go HTTP API + Asynq worker, Next.js BFF panel, Next.js branded site, Docker infra. It grew out of a Next.js + Go boilerplate; the platform layer (auth, RBAC, engines) is still shared with it.

| Path | Role |
|------|------|
| [`backend/`](./backend) | Go HTTP API + Asynq worker (`:8080`, `/v1/...`) |
| [`frontend/`](./frontend) | Next.js BFF: landing (`/`), platform panel (`/platform`), company panel (`/t/{slug}`), customer portal (`/r/{orgSlug}`) |
| [`site/`](./site) | Branded site: one image serves every company's site by host (`{subdomain}.<base host>` or a verified custom domain) |
| [`compose.local.yml`](./compose.local.yml) | Local infra only |
| [`compose.prod.yml`](./compose.prod.yml) | Full stack + Dokploy network |
| [`deploy/`](./deploy) | Production env schema, Centrifugo config |
| [`docs/`](./docs) | Architecture, local/prod setup, how to extend, [brand kit](./docs/brand.md), [design docs](./docs/design) per domain area |
| [`AGENTS.md`](./AGENTS.md) | Agent skill index |

## What you get

**Company panel** (`/t/{slug}`, `/v1/tenant/*`; organization picked per request by the `X-Organization` header)

- **Organization core**: members and invitations, several roles per member, company-specific roles on top of template roles, branch-scoped access, versioned company settings ([tenant-core](./docs/design/tenant-core.md))
- **Branches and fleet**: branches + service points, segments, rental groups, versioned products, vehicles, documents / maintenance / expiry alerts, usage blocks, transfers, one-way rules, fleet calendar
- **Pricing**: rate plans with duration bands and branch / channel overrides, date rules, extras and equipment stock, campaigns / coupons, price preview on the pure pricing engine
- **Reservations**: quotes, the reservation state machine, availability and capacity engine (holds, channel capacity rules), plate plan, resolution tasks, approvals
- **Rentals**: handover / return, extensions, vehicle swap, charges, operation and field tasks (delivery, collection, transfers), mobile inspections with damage diagram and photo evidence
- **Contracts**: versioned templates, rendered and hashed documents, PDFs (headless Chromium), remote approval with an SMS code
- **Customers**: individual / corporate customers, drivers with rental condition checks, encrypted and masked personal data, corporate agreements
- **Finance**: customer / supplier ledger, collections, deposits (pre-authorization), cash / bank book, expenses, suppliers and their invoices / payments, e-invoice / e-archive, vehicle profitability
- **Reports**: occupancy, income / expense, channel performance, receivable / payable aging, branch comparison; operations overview on the home page
- **Notifications**: notification engine (rules, templates, delivery log) over SMS, e-mail, web push and WhatsApp
- **Site settings**: branded site editor (draft / publish, themes, SEO, articles), site leads, public API keys and webhooks

**Customer-facing**

- **Branded site** (`site/`): one Docker image, every company's look comes from data; books only through the public API ([branded-site](./docs/design/branded-site.md))
- **Public booking API** (`/v1/public/*`): for the site, the embeddable widget and a company's own frontends or partners ([public-api](./docs/design/public-api.md))
- **Customer portal** (`/r/{orgSlug}`, `/v1/customer/{orgSlug}/*`): sign-in with reservation reference + one-time code, reservation view, payment, change / cancel requests ([customer-portal](./docs/design/customer-portal.md))
- **Pay links** (`/pay/{token}`) and **remote document approval** (`/d/{token}`)

**Integrations** (each company connects its own accounts under Ayarlar → Entegrasyonlar; credentials are encrypted and the API returns masked hints only. SMS, e-invoice, KABİS and HGS work runs on the integration job runner, which never resends work whose outcome is unknown: [integration-jobs](./docs/design/integration-jobs.md))

| Area | Provider | State |
|------|----------|-------|
| Payments | iyzico (checkout form, refunds, deposit pre-auth) | live adapter (pre-auth pending sandbox verification); `cmd/iyzico-fake` for local runs |
| SMS / OTP | Netgsm | live adapter; `cmd/netgsm-fake` for local runs |
| WhatsApp | wuzapi (self-hosted gateway, one session per company) | live adapter; `whatsapp_fake` outside production |
| E-invoice / e-archive | UBL-TR documents built and validated with go-ubltr; GİB private integrator | integrator not chosen: official slot waits on dependency; `einvoice_fake` outside production |
| KABİS (police rental notifications) | official method not verified | official slot waits on dependency; `kabis_fake` outside production |
| HGS / OGS toll passes | provider not chosen | file import + `hgs_fake` outside production |
| Channel manager | Yolcu360, Obilet | adapters behind assumptions listed in [channel-manager](./docs/design/channel-manager.md); fakes outside production |

**Platform panel** (`/platform`): organizations (access windows, suspension), platform users and roles, usage and connection health, custom site domains, the TSB vehicle catalog, notifications, letterhead / branding, auth policy and OAuth providers, step-up policy, storage explorer, imports / exports, activity and application logs.

**Platform layer** (shared with the boilerplate)

- **Auth**: password, email code, passkeys, TOTP, OAuth (GitHub, Google, Facebook, Apple; native mobile id_tokens), QR sign-in, sessions, impersonation
- **Engines**: resource meta, I/O (CSV/XLSX/PDF/JSON export and import with rollback), bulk actions with undo, Cmd+K search (Meilisearch; the browser never talks to it), live lists over Centrifugo, activity log, step-up re-auth
- **Storage**: SeaweedFS (any S3); the API owns every public URL
- **Queue**: Asynq (in-process by default, or `make -C backend worker`)

## Quick start

```sh
cp .env.example .env
# set NAME_PREFIX if you share a Docker host with other stacks
make local-dev
```

In another terminal (after Postgres is up):

```sh
make create-super-admin
# or: make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1'
```

Then open http://localhost:3000/platform/login and sign in as `super_admin`. Create an organization under Platform → Organizations; its panel is at `/t/{slug}`.

`make local-dev` starts Postgres, Redis, SeaweedFS, MailHog, Adminer, Centrifugo and Meilisearch in Docker, migrates the database, then runs **air** (API) and **pnpm dev** (frontend) on the host. It does **not** build app images. The branded site runs separately: `make site-dev` (http://{subdomain}.localhost:3100).

| Who | Lands on |
|-----|----------|
| `super_admin` or any `platform.*` permission | `/platform` |
| Member of one organization | `/t/{slug}` |
| Member of several organizations | `/organizations` (picker) |
| Anyone else | `/no-access` (waiting for an invitation) |

## Docs

| Doc | When to read |
|-----|----------------|
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Repo map, shells, RBAC, engines, request flow |
| [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md) | Env, local commands, production / Dokploy |
| [docs/EXTENDING.md](./docs/EXTENDING.md) | Adding a module, API + UI conventions |
| [docs/design/](./docs/design) | One design doc per domain area (reservations, pricing, payments, KABİS, …) |
| [CONTRIBUTING.md](./CONTRIBUTING.md) | How to work in this repo |
| [backend/README.md](./backend/README.md) | API commands and curl examples |
| [backend/docs/auth.md](./backend/docs/auth.md) | JWT, platform and tenant RBAC |
| [frontend/README.md](./frontend/README.md) | BFF, panels, scripts |
| [AGENTS.md](./AGENTS.md) | Which agent skill to load |

Agent playbooks live under [`.agents/skills/`](./.agents/skills).
