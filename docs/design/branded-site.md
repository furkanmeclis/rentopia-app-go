# Branded site

Status: **implemented** (M5: TEC-729 site, TEC-730 panel editor with draft / publish, TEC-731 custom-domain backend). Requirements §14.1. One template, one Docker image: every organization gets its own look from data. Ten organizations means one image with ten looks. The site books only through the public booking API ([public-api.md](./public-api.md)). Later work builds on it:
- **TEC-731**: verified custom domains (backend implemented; panel managed separately);
- **TEC-732**: embeddable widget.

| Area | Path |
|------|------|
| App (Next.js App Router, own image) | `site/` (`site/Dockerfile`, service `site` in `compose.prod.yml`, CI job `site`) |
| Host → site, booking session | `site/src/proxy.ts`, `site/src/lib/{resolve,session,server}.ts` |
| Settings model, publish, preview, images, catalog state, host resolution | `backend/internal/modules/site/{model,usecase,handler}`, `routes.go` |
| Panel editor (Ayarlar → Markalı site) | `frontend/src/features/site`, route `/t/{slug}/settings/site` |
| Preview in the site | `site/src/lib/preview.ts`, `site/src/proxy.ts`, `site/src/app/preview/exit` |
| Migrations | `000122_branded_site`, `000123_seed_branded_site_rbac`, `000124_site_publish`, `000125_catalog_show_online`, `000127_site_config_v2`, `000128_site_media` |
| Tests | `backend/internal/modules/site/{site,site_publish}_test.go`, `site/src/lib/{theme,preview}.test.ts` |

## 1. Request flow

```
browser ── acme.rentopia.app ──▶ site (Next.js, one image)
                                   │ proxy.ts: Host → GET /v1/public/site/resolve?host=  (cached 30 s per host;
                                   │           with a preview token: &preview=…, never cached, §8)
                                   │   unknown host / disabled site / suspended org → neutral 404, no branding
                                   │   booking on → POST /v1/public/sessions {publishable_key}
                                   │                with X-Forwarded-Origin = resolved origin → httpOnly cookie rtp_bs
                                   ▼
                       pages / server actions ── Bearer rtp_ps_… + X-Forwarded-Origin ──▶ /v1/public/* (TEC-728)
```

- The site talks to the API **server side only** (`API_URL`, the internal address, the same as the panel BFF). The browser only reaches the site's own origin. Images go through `/media/...`, which passes through only this host's site images and this organization's product images.
- No staff credential and no secret key exists in the site. The site uses the publishable key that the resolve answer returns, and mints one booking session (30 min) per visitor for the **origin the API resolved**. The session cookie records that origin; a cookie from another origin is never used, and the API also refuses a session from another origin.
- `X-Forwarded-For` carries the visitor IP, so the public API's per-IP and per-session limits apply per visitor, not per site container.
- Every page renders dynamically (`force-dynamic`). The only cache is the per-process resolve cache, keyed by host (30 s; unknown hosts 10 s). Nothing about an organization exists at build time.

## 2. Hosts

- `SITE_BASE_URL` (backend; prod default = `APP_URL`; dev `http://localhost:3100`) sets the base. A site answers on `{subdomain}.{host}[:port]` with the same scheme. This subdomain address is also the **preview** of every site.
- `NormalizeHost` lower-cases the host, strips the port and trailing dot, and allows only DNS characters. Only one label directly under the base host resolves (`x.acme.rentopia.app` does not). The bare base host never resolves.
- Reserved subdomains (`www`, `app`, `api`, `realtime`, `portal`, `widget`, …: `model.ReservedSubdomains`) can never be chosen. Subdomains are globally unique (`uq_site_settings_subdomain`).
- **Custom domains (TEC-731)**: `usecase.Domains` implements `CustomDomains`, wired with `Service.SetCustomDomains` in the API and worker composition roots. `Lookup` returns only verified domains; `Hosts` supplies their HTTPS origins. Every domain belongs to exactly one organization, and an unverified, failed or disabled domain returns `404 SITE_NOT_FOUND` when resolved. The site's enabled, published and organization-access checks still apply.
- Migration **000126_site_domains** stores UUID, organization, globally unique normalized host, random 256-bit TXT token, verification/TLS state, external provider ID, creator, timestamps, next check, attempts, origin-sync flag and worker lease. A parent-row lock and trigger enforce **three domains per organization**, including disabled records awaiting cleanup, across concurrent requests. The same migration seeds `platform.site_domains.read` and `.manage` for `super_admin`; existing `tenant.site.read` and `.manage` cover tenant actions.
- Registration refuses IP literals, localhost/local/reserved names, malformed DNS labels, and the base host or any host below it. Fully qualified external names such as `www.customer.com` are allowed. Hosts are lowercase with the trailing dot removed; registration refuses ports, URLs and wildcards. Unicode names must be supplied in ASCII DNS form.
- **Ownership and routing**: publish TXT `_rentopia-verify.{host}` equal to the returned token, and CNAME `{host}` to `SITE_DOMAIN_TARGET_HOST` (defaults to the host of `SITE_BASE_URL`). Alternatively publish A/AAAA records using `SITE_DOMAIN_TARGET_IPS` or the target host's addresses. Verification requires the exact TXT value and a resolving host, then either a matching canonical CNAME or every returned address belonging to the platform. Keep both records after verification. DNS is resolved by the worker, with an injectable resolver and a 15-second per-domain timeout.
- The Asynq task `app:site:domains` runs every minute on the maintenance queue. A tenant verify or platform recheck also enqueues it immediately; durable due times preserve pending work across queue outages. Batches claim up to 50 records with `FOR UPDATE SKIP LOCKED` and a ten-minute lease. Pending checks back off from one minute to six hours and fail after **72 hours**, even if DNS first appears at the deadline; register again after expiry. Verified domains are checked daily. Missing TXT, broken DNS or a wrong target changes a verified domain to failed, so host resolution stops immediately. A platform/tenant explicit recheck can recover a previously verified domain when DNS is repaired.
- Verification, failure and removal synchronize the site's publishable key origins without changing the draft or its version. Rotation predecessors and successors are synchronized too, revoking sessions that still use keys in their grace period. Unrelated origins configured on those keys are retained. A persisted dirty flag and one-minute reconciliation retries recover failed origin/provisioning updates. Removal first disables the domain and revokes its origin, then the worker removes routing and deletes the disabled row after successful cleanup. DNS verification state and TLS provisioning state are independent.
- `custom_domains` is registered as a platform health source in both processes: failed verification/TLS is critical and verified domains awaiting manual TLS are warnings. The existing health alert sweep notifies platform administrators of new critical issues through the notification engine.

### 2.1 Domain API

| Route | Permission / authentication | Response |
|-------|-----------------------------|----------|
| `GET /v1/tenant/site/domains` | `tenant.site.read` | All of the organization's domains, DNS records and TLS instructions |
| `POST /v1/tenant/site/domains` | `tenant.site.manage` | `{host}` → 201 pending domain; 409 `DOMAIN_LIMIT` / `DOMAIN_TAKEN` |
| `GET /v1/tenant/site/domains/{uuid}` | `tenant.site.read` | Domain; another organization's UUID is 404 |
| `DELETE /v1/tenant/site/domains/{uuid}` | `tenant.site.manage` | 200 `{removed: true}` after disabling; routing cleanup is asynchronous |
| `POST /v1/tenant/site/domains/{uuid}/verify` | `tenant.site.manage` | 202 domain; immediate DNS check queued |
| `GET /v1/platform/site-domains?limit=50&offset=0` | `platform.site_domains.read` | All organizations, newest first; limit 1–200 |
| `POST /v1/platform/site-domains/{uuid}/recheck` | `platform.site_domains.manage` | 202 domain; immediate check queued |
| `POST /v1/platform/site-domains/{uuid}/mark-tls-active` | `platform.site_domains.manage` | Manual verified domain → TLS active; otherwise 409 `DOMAIN_TLS_MODE` |
| `GET /v1/internal/site/domain-allowed?host=` | `X-Auth-Adapter-Key: AUTH_ADAPTER_SECRET` | 200 `{allowed: true}` only for verified hosts; 404 otherwise; 401 for invalid/missing internal credentials |

Tenant routes derive organization scope from authentication and `X-Organization`; bodies never accept an organization ID. Every domain response contains UUID, organization ID, host, status, verification token, last check, first verification time, last error, TLS status, provisioner, timestamps and `dns_records[{type,name,value}]`. Internal domain authorization follows the existing internal auth adapter credential convention, fails closed for empty configuration, and sets `Cache-Control: no-store`. Do not expose the adapter secret to browsers.

### 2.2 Provisioners and deployment

| Key | Default / purpose |
|-----|-------------------|
| `SITE_DOMAIN_TARGET_HOST` | Empty → host of `SITE_BASE_URL`; public routing target |
| `SITE_DOMAIN_TARGET_IPS` | Empty; comma-separated IPv4/IPv6 targets accepted in addition to target-host addresses |
| `SITE_DOMAIN_PROVISIONER` | `manual`; also `dokploy`, `traefik_file` |
| `SITE_DOMAIN_TRAEFIK_DIR` | Production `/app/site-domains`; local configuration empty until file mode is selected |
| `DOKPLOY_URL` | Empty; Dokploy origin |
| `DOKPLOY_API_KEY` | Empty secret; only required for Dokploy mode |
| `DOKPLOY_COMPOSE_ID` | Empty; compose application ID |
| `DOKPLOY_SITE_SERVICE` | `site` |
| `DOKPLOY_SITE_PORT` | `3000` |
| `APP_NETWORK_NAME` | Production `{NAME_PREFIX}-app`; Docker DNS suffix used by file mode |

All new keys are in production schema version **6**, the example configuration and the backend/worker shared compose environment. No credential is generated for Dokploy. A domain remembers its provisioner; keep an older provider configured until its domains have been removed before switching modes.

**Manual (default).** After a domain verifies, `tls_status=manual`. Platform administrators add its host to the compose **site** service, port **3000**, HTTPS with **Let's Encrypt** in Dokploy, deploy, confirm the certificate works, then call `mark-tls-active`. The platform list includes these instructions. Remove the Dokploy route manually when a domain is removed or fails verification; verification and origin revocation already prevent it serving tenant content. The backend does not issue certificates in manual mode.

**Dokploy.** Set the URL, API key and compose ID, then choose `dokploy`. The worker uses [Dokploy's domain API](https://docs.dokploy.com/docs/api/domain): `domain.byComposeId` discovers a previously staged route on retries; POST `domain.create` sends the host, compose ID, service, port, HTTPS and `certificateType: letsencrypt`; POST `domain.delete` removes it. Requests carry `x-api-key`, have bounded responses/timeouts and do not follow redirects. POST `compose.deploy` applies staged changes **once per batch**; no-op checks do not redeploy. External IDs and provisioning state survive deployment failures. Active means Dokploy accepted the routing/deployment configuration; certificate issuance is performed by the proxy, so this is not an independent TLS handshake probe.

**Traefik file.** Choose `traefik_file` and set the directory and network name. Production mounts the named volume `{NAME_PREFIX}-prod-site-domains` at `/app/site-domains` in the worker; the image creates that directory for its unprivileged user. Mount the **same volume read-only** in the edge Traefik container, enable its file provider with directory watching, configure entrypoints `web`/`websecure` and the `letsencrypt` certificate resolver, and connect it to the application's network so `site.{APP_NETWORK_NAME}` resolves. For an external directory instead, replace the worker volume mount with a bind mount, grant UID 10001 write permission and point `SITE_DOMAIN_TRAEFIK_DIR` there. Each verified host gets one atomically replaced `site-domain-{host}.yml` with a host rule, HTTPS router, HTTP→HTTPS redirect and backend `http://site.{APP_NETWORK_NAME}:3000`; removal or failed ownership removes the file. Router/service identifiers derive from a host hash to avoid collisions between DNS label boundaries. Active means configuration was written; Traefik handles certificate issuance.

- **Deployment**: in Dokploy, route the wildcard domain `*.{APP_HOST}` (wildcard certificate, DNS challenge) to the `site` service on port 3000. Custom domains are added to the same service later.

## 3. Settings model (`site_settings`, one row per organization)

Two kinds of fields (TEC-730):
- **live**: `enabled`, `subdomain`, `default_locale` and the linked key take effect on save. Switching a site off or moving its address must not wait for a publish, and the key is a credential, not content.
- **draft**: `config` and the three images. The panel edits and previews them; visitors only get the **published revision** (§3.1).

| Column | Notes |
|-------|-------|
| `enabled` | Live. The host resolves only while the site is on **and** a revision is published. |
| `subdomain` | Live. Defaults to the organization slug (`SubdomainFromSlug`). |
| `default_locale` | Live. `tr` \| `en`. Visitors switch with `/lang?l=…` (cookie `site_locale`). |
| `config` JSONB | Draft. The typed document `model.Config` (below). Validated on save (`NormalizeConfig`); a stored document that no longer validates falls back to the defaults instead of breaking the site. |
| `api_key_id` | Live. The publishable `web` key the site books with (composite FK to `public_api_keys`). |
| `logo/favicon/cover_object_key` | Draft images (§3.2). The logo falls back to the organization logo. |
| `published_revision` | The live snapshot in `site_setting_versions` (NULL = never published: the host does not resolve). |
| `version` | +1 on every write (save, image, restore, renew key): optimistic concurrency (`409 VERSION_CONFLICT`). Publish does not change it. |

`config` (v2 registry, content and media additions are detailed in §10):
- `theme`:
  - `colors` / `dark_colors` (`primary`, `primary_foreground`, `accent`, `background`, `foreground`, `muted`, `muted_foreground`, `border`; `#rrggbb`, empty = default);
  - `font` (legacy fonts plus the v2 union in §10); the current site self-hosts its original font set with `next/font`. Additional theme rendering is owned by the template integration;
  - `heading_font` (body font IDs plus `theme|georgia`);
  - `radius` (`soft|sharp|xs|sm|md|lg|xl|pill`; legacy `none|full` normalize to `sharp|pill`);
  - `mode` (`light|dark|system`).
- `layout`: `hero` (`centered|split|image|minimal`), `cards` (`grid|list|compact`), `footer` (`simple|columns|minimal`).
- `content.{tr,en}`: `hero_title`, `hero_subtitle`, `about`, `faq[{question, answer}]` (≤ 40), `terms`, `privacy` (KVKK). Plain text only: blank lines separate paragraphs, and no HTML is ever rendered.
- `seo.{tr,en}`: `title` (≤ 70), `description` (≤ 300).
- `contact`: `phone`, `email`, `address`, `whatsapp`.
- `analytics`: `ga4_measurement_id` (`G-XXXX`). The site loads gtag only for a valid ID, with `anonymize_ip` (never in a preview). A cookie-consent banner is still open (§7).
- `widget` (TEC-732, enforced after publish): `allowed_domains` (≤ 20 origins, `https://host[:port]`, `http` only for localhost; normalized and de-duplicated) and `initial_filters {pickup_branch_uuid, rental_days (0-90)}`.

Theme tokens become CSS variables in a `<style>` in the SSR head (`site/src/lib/theme.ts`). The site validates them again, so only hex colors and known enum values reach CSS. The Tailwind theme maps these variables, and the shadcn-style components in `site/src/components/ui` read them.

### 3.1 Draft, publish and history (`site_setting_versions`, 000124)

- **Publish** (`POST /v1/tenant/site/publish {version}`) snapshots the draft (`config` + image keys) as the next per-organization `revision` (1, 2, 3 …) and points `published_revision` at it, in one SQL statement (`PublishSiteDraft`: `FOR UPDATE` on the draft row at the expected version, insert, update). A stale version is `409 VERSION_CONFLICT`; an unchanged draft `409 NOTHING_TO_PUBLISH`. A never-saved site publishes its defaults (the row is stored first, switched off).
- **Resolve** serves the published snapshot only: `config`, image paths and `version` (= the revision, the `?v=` of image URLs) come from `site_setting_versions`. A site that is on but never published answers `404`.
- **History** (`GET /v1/tenant/site/versions`): revisions newest first with publisher and images; `current` marks the live one.
- **Restore / roll back** (`POST /v1/tenant/site/versions/{revisionUuid}/restore {version}`): copies a revision back into the draft; publishing it makes a new revision with that content (history is append-only). Another organization's revision is `404`.
- `has_unpublished_changes` compares the normalized draft with the published document and image keys.
- **Migration of live rows**: sites saved before TEC-730 were live on save. 000124 backfills each existing row as revision 1 with its current document and images and sets `published_revision = 1`, so e.g. a production site at version 1 keeps serving exactly what it served (the backfill is idempotent and covered by `TestLegacyRowStaysPublished`).

### 3.2 Images

- `PUT /v1/tenant/site/assets/{logo|favicon|cover}` (multipart `file`, `version`) and `DELETE …?version=`: literal routes per kind (the isolation test enumerates route literals).
- The type is sniffed from the bytes (`storage.DetectLogoMIME`): PNG, JPEG or WebP. **SVG is refused** (script carrier; no sanitizer exists), as is HTML declared as an image. Limits: logo 2 MB, favicon 1 MB, cover 5 MB (the body is read with the limit + 1).
- Keys are API-owned: `organizations/{org_uuid}/site/{kind}-{file_uuid}.{ext}` (`storage.SiteImageObjectKey`), a new object per upload. Snapshots keep pointing at the object they were published with; an object is deleted only when neither the draft nor any revision names it (`SiteObjectKeyInUse`).
- An upload is a draft change (visible in the preview, live after publish). `GET /v1/public/site/assets/{site_uuid}/{kind}?v=` streams the published image of an enabled, published site; with `&preview=<token>` the draft image (`Cache-Control: private, no-store`). Responses carry `nosniff` and `Content-Security-Policy: default-src 'none'; sandbox`.

### 3.3 Storefront publish state of the catalog (000125)

Which branches, vehicle subgroups and extras the **storefront** offers: the branded site and the embeddable widget (TEC-732). The discriminator is the credential: a public API caller with a **publishable key** (always through a browser booking session; `Credential.Storefront()` = `Kind == publishable`) is a storefront. Integrations calling with a **secret server key** (agencies, third-party software) ignore the flags and keep seeing and quoting everything their channel sells. These switches are **live** (no publish):

| Kind | Flag | Effect |
|------|------|--------|
| Branch | `branches.show_online` (default on) | left out of `/v1/public/locations` (and its service points); availability / quotes naming it, directly or through a point, answer `400 … not_found` |
| Subgroup | `rental_products.show_online` (default on) | left out of `/v1/public/vehicle-subgroups` and availability items; quotes answer `400 subgroup_uuid not_found` |
| Extra | `extras.show_online` (default on) | left out of `/v1/public/extras`; quotes naming it answer `400 extras.N.extra_uuid not_found`. The sales-channel rule (`extras.channels`) still applies to every caller and is not touched by the switch |

All three rules apply to storefront credentials only (listings via a `storefront` query argument, availability / quote refusals via `offlineCatalog`, which is empty for secret keys). Service points keep their own `online_selectable`. API: `GET /v1/tenant/site/catalog`, `PATCH /v1/tenant/site/catalog/{branches|products|extras}/{uuid} {show_online}` (record looked up first: another organization's record is `404`).

### API

| Route | Who | Notes |
|-------|-----|-------|
| `GET /v1/tenant/site` | `tenant.site.read` | The settings (draft document), or the defaults with `exists: false, version: 0`. Also returns `url`, `hosts`, `api_key {uuid, status}`, `assets` (which draft images exist), `booking_enabled` + `booking_blockers` (`site_disabled`, `key_missing`, `key_inactive`, `channel_off`), `published_revision`, `published_at`, `has_unpublished_changes`, `live`. |
| `PUT /v1/tenant/site` | `tenant.site.manage` | `{version, enabled?, subdomain?, default_locale?, config?}`: omitted fields are kept, and `config` replaces the whole draft document. Errors: `400` field details, `409 SUBDOMAIN_TAKEN`, `409 VERSION_CONFLICT`. Activity: `site.updated`. |
| `POST /v1/tenant/site/publish` | `tenant.site.manage` | §3.1. Activity: `site.published`. |
| `GET /v1/tenant/site/versions` | `tenant.site.read` | §3.1. |
| `POST /v1/tenant/site/versions/{revisionUuid}/restore` | `tenant.site.manage` | §3.1. Activity: `site.restored`. |
| `POST /v1/tenant/site/preview-token` | `tenant.site.read` | §8: `{token, url, expires_at, assets}`. |
| `POST /v1/tenant/site/renew-key` | `tenant.site.manage` | Below. Activity: `site.key_renewed`. |
| `PUT` / `DELETE /v1/tenant/site/assets/{logo,favicon,cover}` | `tenant.site.manage` | §3.2. Activity: `site.image_updated` / `site.image_removed`. |
| `GET /v1/tenant/site/catalog`, `PATCH …/catalog/{branches,products,extras}/{uuid}` | read / manage | §3.3. Activity: `site.catalog_updated`. |
| `GET /v1/public/site/resolve?host=[&preview=]` | public, 600 / min / IP | The resolved site (`model.Resolved`): `site_uuid`, `version`, `origin`, `host_kind`, `organization {uuid, slug, name}`, `publishable_key`, `booking_enabled`, `config`, `assets` (API paths), `portal_url`, `preview`, `panel_origin`. Errors: `404 SITE_NOT_FOUND`, `401 PREVIEW_INVALID`. |
| `GET /v1/public/site/assets/{uuid}/{kind}` | public | `logo` \| `favicon` \| `cover` of an enabled, published site (or the draft with `preview`). |

Permissions (000123): `tenant.site.read` and `tenant.site.manage`, granted to owner, admin and merkez_yoneticisi (like the public API keys). TEC-730 adds no slug: reading (including preview links) is `read`, every write is `manage`.

### The site key

- **Switching the site on** links a usable publishable `web` key. It creates one named "Markalı site", with all scopes, when the linked key is missing, revoked or expired.
- **Every save** keeps the key's `allowed_origins` equal to the hand-added origins plus the site's current origins. A subdomain change replaces the old origin; custom domains come in through `CustomDomains.Hosts`.
- **Disabling** the site keeps the key: the host stops resolving, so the site stops.
- `booking_enabled` is true when all of these hold:
  - the site is on;
  - the key is active (not revoked or expired);
  - the organization's **web** sales channel is on (the public API's `checkChannel` gate; packages arrive with TEC-674).

  When it is false, the site still renders its content pages and shows "online booking is closed".
- **Rotating the key** in Ayarlar → Public API: the next save follows `rotated_from_id` to the newest active successor (`GetSiteKeySuccessor`) and links it.
- **Renew key** (`POST /v1/tenant/site/renew-key {version}`): a usable key is rotated through the public API module (`RotateKey`, new key with `rotated_from_id`, same scopes and origins; the old key keeps working for **1 hour** so open booking sessions finish); a missing, revoked or expired key is replaced by a new "Markalı site" key. Live at once.

## 4. Pages (`site/src/app`)

| Path | What |
|------|------|
| `/` | Hero variant + search form. A GET form with native date and time controls (works without JavaScript, platform pickers on phones). Pick-up / return location: branch or online-selectable non-delivery point. |
| `/search` | `GET /v1/public/availability` + `vehicle-subgroups`. Cards follow the `cards` variant: "X veya benzeri", conditions, total, daily average, prepayment, "son N araç". Selecting a car runs `POST /v1/public/quotes` and goes to the extras step. |
| `/book/{quote}` | Extras (`GET /v1/public/extras`). Prices always come from a new quote that includes the chosen extras. |
| `/book/{quote}/details` | Quote summary; renter (individual / corporate), driver (licence data), note, terms and KVKK consent. Then `POST /v1/public/reservations` with an `Idempotency-Key` per form render. `QUOTE_PRICE_CHANGED` → the replacement quote with a notice; `DRIVER_NOT_ELIGIBLE`, `CAPACITY_UNAVAILABLE` and field errors are shown inline. |
| `/reservation/{reference}` | Status, price summary and cancellation terms. The access token from booking is kept in an httpOnly cookie (`rtp_rt`, per origin, 5 references, 90 days). **Pay**: `POST …/payment-links` → redirect to the hosted pay page (iyzico 3DS), then the request / confirmation status. |
| `/lookup` | The customer portal (`portal_url` = `{PUBLIC_FRONTEND_URL}/r/{slug}`, M3) and this device's bookings. |
| `/branches`, `/about`, `/faq`, `/contact`, `/terms`, `/privacy` | From `GET /v1/public/locations` and the settings document. |

Common to all pages:
- **Locales**: tr and en. `site/src/messages/en.ts` is typed `Record<MessageKey, string>`, so a missing English key fails typecheck.
- **Accessibility**: labelled fields with `aria-invalid` / `aria-describedby`, a skip link, a no-JS mobile menu (`<details>`), and clear empty and error states.
- **Errors**: an unreachable API shows a neutral 503 page.
- **Headers**: frames denied (`frame-ancestors 'none'` + `X-Frame-Options: DENY`; set by the proxy for pages and by `next.config.ts` for `_next/`, `media/`, `health`), except a preview page (§8), and `form-action` limited to self and https (so the pay redirect works).

**UI note**: `frontend/`'s shadcn-only rule is about the panel. The site is a separate public app. It uses shadcn-styled primitives (`components/ui`) but **native** `<select>` and date inputs, so that every form works without JavaScript and phones show their own pickers. The embeddable widget (TEC-732) can reuse `lib/`, `components/ui` and the booking pages.

## 5. Configuration and running

| Key | Where | Notes |
|-----|-------|-------|
| `SITE_BASE_URL` | backend (`compose.prod.yml` backend env, `deploy/env/schema`) | Default `{APP_URL}`. |
| `SITE_IMAGE` | compose | `{NAME_PREFIX}-site:latest` (`deploy/env/schema`). |
| `API_URL` | site container | `http://backend.<app network>:8080/v1`. |

Local: `make site-dev` serves the site on `http://{subdomain}.localhost:3100`; browsers resolve `*.localhost` to loopback. The public API accepts `http://*.localhost` origins for this.

## 6. Tests

| Test | What it checks |
|------|----------------|
| `TestSettingsAndKey` | defaults; `version` required and stale saves refused; validation of every free-form field (CSS injection in colors, enums, locales, GA ID, e-mail, FAQ, reserved / invalid subdomain); first save creates the publishable `web` key for the subdomain origin; subdomain taken; subdomain change replaces the old origin and keeps hand-added ones; a revoked key is replaced on the next save; a disabled site creates no key |
| `TestResolveIsolation` | two organizations on one base domain get their own theme, content, key and origin, and neither answer names the other; unknown / nested / bare / suffix / malformed hosts → 404 `SITE_NOT_FOUND`; the resolved key mints a session only for its origin; that session works for its own host's data and not from the other host; web channel off → booking off; disabled site and suspended organization → 404 |
| `TestCustomDomainHookAndAssets` | a custom domain resolves through the hook and is added to the key's origins; an unverified domain → 404; logo falls back to the organization logo and streams; unknown kind / site → 404 |
| `TestHostHelpers` | dev origins (`http://acme.localhost:3100`) are valid public API origins; host normalization; slug → subdomain |
| `site/src/lib/theme.test.ts` | the theme CSS drops anything that is not a hex color or a known value; GA ID; same-site redirects only; session cookies bound to their origin |

RT-A01 covers `GET/PUT /v1/tenant/site` (the PUT is on the allowlist: it acts on the caller's own document).

## 7. Open items

- **TEC-731**: the `CustomDomains` implementation (`site_domains`, DNS verification, TLS on the site service).
- Widget loader and panel embed-code generation are delivered separately; the implemented server contract is in §9.
- A cookie-consent banner before analytics; a sitemap; branch detail pages.
- **Time zone**: organizations sell in Turkey, so the site uses Europe/Istanbul (+03:00) for search times.

## 8. Panel editor and preview (TEC-730)

**Panel** (`/t/{slug}/settings/site`, nav: Ayarlar → Markalı site, `tenant.site.read`; writes `tenant.site.manage`). One form over the draft with tabs, a sticky save bar, field errors from `400` details inline (tab dots), and a reload prompt on `409 VERSION_CONFLICT`:
- **Genel**: on / off, subdomain with the site URL (copy / open), default language (live); publish and booking state with `booking_blockers`; the site key status, a link to Ayarlar → Public API and "Anahtarı yenile".
- **Tema**: light / dark color tokens (swatch popover + hex), font, radius, mode; hero / cards / footer variants as visual choice cards.
- **İçerik**: tr / en tabs: hero title / subtitle, about, terms, privacy (shadcn Textarea, plain text), FAQ add / remove / reorder (≤ 40).
- **SEO ve iletişim**: SEO title / description per language with length counters, contact, GA4.
- **Görseller**: logo, favicon, cover upload / replace / remove with previews (draft paths from the preview token).
- **Yayın**: publish state and the revision history (DataTable) with "Taslağa geri yükle".
- **Katalog**: branches, subgroups and extras (DataTables) with a "Çevrimiçi" switch column (§3.3).
- **Widget**: allowed domains and initial filters; embed-code generation follows §9. Keys stay on Ayarlar → Public API.

Header actions: status chips, **Önizle** (sheet with the site in an iframe, desktop / mobile width, new tab) and **Yayınla** (confirmation; disabled while the form has unsaved edits or nothing changed). Uploads and key renewal keep unsaved form edits (only the version moves); save and restore reload the form.

**Preview token** (`POST /v1/tenant/site/preview-token`): `sp1.{site_uuid}.{expiry}.{HMAC-SHA256}` with a key derived from `APP_ENCRYPTION_KEY` (`HMAC(key, "rentopia/site-preview/v1")`; no new configuration). Valid 30 minutes, bound to one site (so one organization): a token for another site, a tampered or expired one, or one signed by another deployment is `401 PREVIEW_INVALID` and never falls back to another site's draft. `url` = the site's own address with `?preview=<token>`.

**Site side**: the proxy accepts the token from `?preview=` (then stored in the `rtp_preview` cookie: httpOnly, Secure, `SameSite=None`, `Partitioned` so it works inside the panel's frame, max 30 min) or from that cookie, checks only its shape, and resolves `…/resolve?host=&preview=` uncached. A preview page:
- renders the draft with a banner ("Önizleme …", "Önizlemeden çık" → `/preview/exit` drops the cookie);
- may be framed only by the panel: `frame-ancestors {panel_origin}` (the API's `PUBLIC_FRONTEND_URL` origin, returned as `panel_origin`), no `X-Frame-Options`; every other response keeps `frame-ancestors 'none'`;
- is `noindex`, `Cache-Control: private, no-store`, loads no analytics;
- browses catalog and prices but never books or pays: `bookingApi` refuses `POST /v1/public/reservations*` (`PREVIEW_READ_ONLY`) and the booking form is disabled with a notice;
- streams draft images through `/media/…?draft=1`, which forwards the visitor's token.

### Tests (TEC-730)

| Test | What it checks |
|------|----------------|
| `TestDraftPublishAndHistory` | first publish of a never-saved site; nothing-to-publish; live fields apply on save while the document stays a draft; resolve never shows the draft; stale publish refused; history order and `current`; restore is draft-only until published (roll back); another organization's revision → not found; switching off is live |
| `TestPreviewToken` | the draft for the site's own token (also for an off, unpublished site) with `panel_origin`; without a token the published revision; another organization's token (both ways), a forged site id, tampered and garbage tokens, an expired token and another secret → `401 PREVIEW_INVALID` |
| `TestImages` | SVG, HTML declared as PNG, empty and oversized files refused; stale version; uploads are drafts; keys under the organization prefix; published objects kept on replace / remove, unreferenced drafts deleted; draft image only with the token |
| `TestLegacyRowStaysPublished` | the 000124 backfill keeps an old live row published unchanged; idempotent |
| `TestCatalogOnlineState` | listing and switches; foreign record → not found; the extra switch leaves sales channels alone; storefront listings, availability and quotes (hidden branch, subgroup, extra) respect the flags; a secret key still lists them |
| `publicapi TestStorefrontPublishState` | with hidden branch, subgroup and extra: a booking session gets none of them and its availability / quotes are refused; a secret key lists them, gets availability and creates a quote (201) |
| `TestRenewKey` | rotation with a 1 h grace and the same origins; the site books with the new key at once; a rotation in Public API is followed on save; a revoked key is replaced |
| `site/src/lib/preview.test.ts` | token shape filter, cookie lifetime and attributes, frame headers (deny by default, panel origin only for previews, unsafe origins refused) |

RT-A01 covers the new routes (allowlist entries for the scope-only writes; `{revisionUuid}` and the catalog `{uuid}` routes get the cross-organization 404 cases).

TEC-731 tests in `backend/internal/modules/site/domains_test.go` cover fake DNS success/failure, token mismatch, wrong/mixed targets, dangling CNAME, IPv6, pending backoff and 72-hour expiry, verified DNS loss and origin removal, host validation, internal authorization, manual state preservation, Dokploy payloads/idempotent retries/batched deployment/failures, and atomic Traefik configuration/removal. Database integration additionally covers the concrete resolver hook, cross-organization reads/deletes/rechecks, global host uniqueness, the database limit, and publishable key origins; it skips without `TEST_DATABASE_URL`. RT-A01 seeds custom domains and covers all five tenant routes.


## 9. Embeddable booking widget (TEC-732)

The same site image serves `/embed`, `/embed/search`, `/embed/book/{quote}`, `/embed/book/{quote}/details`, `/embed/reservation/{reference}` and `/embed/pay`. Search, results, extras, customer details and confirmation reuse the normal site's components, pages, actions and public API client. The compact layout omits the site header/footer and analytics and includes a small Rentopia credit. Turkish and English remain available. The iframe **must use the organization's site origin**, either its subdomain or a verified custom domain; there is no central widget host and no key-to-host discovery endpoint.

### 9.1 Exact loader / panel contract

The coordinator owns `site/public/widget.js` and the panel Widget tab. The server contract they use is:

```html
<script src="https://acme.rentopia.app/widget.js"
        data-key="rtp_pk_..."
        data-branch="PICKUP_BRANCH_UUID"
        data-subgroup="SUBGROUP_UUID"></script>
```

The loader derives `SITE_ORIGIN` from its own script URL, generates a unique `widgetId` per instance and builds this iframe URL with `URL` / `URLSearchParams` (omit unset optional parameters):

```text
{SITE_ORIGIN}/embed?key={rtp_pk_...}&branch={uuid}&subgroup={uuid}&pickup={branch[:point]}&return={branch[:point]}&pickup_date={YYYY-MM-DD}&pickup_time={HH:mm}&return_date={YYYY-MM-DD}&return_time={HH:mm}&locale={tr|en}&parent={HOST_PAGE_ORIGIN}&widgetId={INSTANCE_ID}
```

| Parameter | Meaning / default |
|---|---|
| `key` | Optional. `data-key` maps here. Must equal this resolved site's current `publishable_key`, otherwise 403. Actual keys start with `rtp_pk_`, not `pk_`. The request host always selects the organization; a key never selects another site. Omitting it relies on the resolved host. Removed on the first session redirect. |
| `branch` | `data-branch`; initial pickup branch UUID, overridden by `pickup`. Otherwise published `widget.initial_filters.pickup_branch_uuid`, then no selection. |
| `subgroup` | `data-subgroup`; optional results filter, retained across searches. Only this subgroup's availability cards are shown. |
| `pickup`, `return` | Optional location values (`branch_uuid` or `branch_uuid:point_uuid`). Empty return means same as pickup. |
| `pickup_date`, `pickup_time` | Optional initial local Turkey date/time; default tomorrow at 10:00. |
| `return_date`, `return_time` | Optional initial date/time; default tomorrow + published `initial_filters.rental_days` (0 = 3 days), at 10:00. |
| `locale` | Optional `tr` / `en`; otherwise the site's default locale in embed mode (no locale cookie dependency). |
| `parent` | `window.location.origin` of the page containing the iframe. Must be an exact published allowed origin for messaging to work. Never derive it from a referrer or accept it from a received message. |
| `widgetId` | Unique instance ID, at most 100 characters; echoed in every message. |
| `s` | Server-owned opaque encrypted session reference, added by the site's first redirect. Loader/panel must never generate, inspect, replace or copy it across instances. All subsequent embed links, GET forms and action redirects preserve it. |

Only `data-key`, `data-branch` and `data-subgroup` are required loader attribute mappings; additional attributes may map one-to-one to the optional query parameters above. Setting pickup/return defaults does not automatically submit search.

Use an iframe with an accessible title, `width:100%`, `display:block`, `border:0`, an initial height of 640px and `referrerpolicy="no-referrer"`. Do not impose a fixed minimum height or set `scrolling="no"`; keep scroll fallback until the first resize. The widget observes its content wrapper (not viewport height), so it can both grow and shrink on mobile and desktop. If using a sandbox, include `allow-scripts allow-same-origin allow-forms allow-popups allow-top-navigation-by-user-activation`; otherwise top-window payment and its fallback links cannot work. No sandbox is required by the server.

Site → parent messages:

```typescript
{
  source: "rentopia-widget",
  v: 1,
  type: "ready" | "resize" | "navigate" | "completed" | "error",
  widgetId: string,
  payload: object
}
```

| Type | Payload / loader behavior |
|---|---|
| `ready` | `{}`; emitted after the bridge mounts on each step. |
| `resize` | `{height: number}`; content height in CSS pixels, rounded up. Loader accepts finite positive values only, clamps to 100–100000px, and sets iframe height. |
| `navigate` | `{url: string, target: "_top", reason: "payment"}`; absolute hosted payment handoff URL. Loader navigates its top window to this URL. If it cannot navigate, the iframe already renders a `_top` link and a new-tab fallback. |
| `completed` | `{reference: string, status: string}`; emitted when a reservation confirmation renders, including request bookings. This reports the reservation status, not proof of payment. Loader may dispatch `CustomEvent("rentopia:completed", {detail: {widgetId, reference, status}})` on its script element. |
| `error` | `{code: "RENDER_FAILED"}` for a booking-page render error. HTTP 403/410/503 before bridge mount show a bilingual error page; the loader should retain a normal iframe/load-timeout fallback. |

The loader **must** check `event.origin === SITE_ORIGIN`, `event.source === iframe.contentWindow`, `source`, `v` and `widgetId` before accepting a message. The site sends only with an explicit `targetOrigin` equal to `parent`, after checking it against the published allowlist; it never posts to `*`.

Optional parent → site messages use `{source:"rentopia-host", v:1, type:"init"|"filters", widgetId, payload}`. The site checks the exact allowed `event.origin`, `event.source === window.parent`, protocol version and instance ID. On `/embed` only, string payload keys `pickup`, `return`, `pickup_date`, `pickup_time`, `return_date`, `return_time`, `subgroup` update the initial search form. They do not auto-submit, change the organization, inject navigation URLs, or replace session references. All other messages are ignored.

**Panel data:** existing `GET /v1/tenant/site` returns `url` (canonical subdomain base), `hosts[{origin,kind,...}]` (including verified custom origins), `api_key.publishable_key`, `config.widget`, `live` and `booking_enabled`. Use `url` or a verified `hosts[].origin` as both the script origin and iframe base. Save through existing `PUT /v1/tenant/site`, then publish using the existing publish action: widget configuration is draft content until published. No new public or tenant endpoint, permission or migration is needed. `GET /v1/public/site/resolve?host=...` already includes the **published** `config.widget` and the key. The loader needs no API call.

### 9.2 Framing and URL sessions

Saving widget domains accepts at most 20 exact hostnames or HTTPS origins (ports 1–65535), normalizes hostnames to HTTPS and deduplicates them. `http` is allowed only for localhost development names. Credentials, paths other than `/`, query strings, fragments, malformed DNS labels and **all wildcards** (including `*.`) are refused. Subdomains must be listed individually. Empty means deny all framing.

Every `/embed/*` response gets `Content-Security-Policy: frame-ancestors <published origins>; base-uri 'self'; form-action 'self' https:` (empty list → `frame-ancestors 'none'`) with no `X-Frame-Options`. Embed pages cannot enter draft-preview mode. Normal site pages retain `frame-ancestors 'none'` and `X-Frame-Options: DENY`, with the existing panel-preview exception. Resolve caching means publication may take up to 30 seconds to change the policy.

The proxy mints the ordinary origin-bound public booking session server-side and redirects the initial GET to add `s=e1.<opaque authenticated ciphertext>`. AES-256-GCM encrypts and authenticates the booking secret, origin, expiry, and later the reservation reference/access token and payment URL. No raw API bearer token reaches HTML or a URL. The reference expires in at most 30 minutes, no later than the original booking session. It is checked against the resolved origin on every embed request, so moving from subdomain to custom origin starts a new instance. Invalid or expired references and actions receive 410 rather than silently changing booking sessions. Embed mode neither reads nor writes `rtp_bs` / `rtp_rt`; GET forms, links and server-action requests keep the URL reference. Confirmation displays the reservation reference and emits `completed`. URLs remain short-lived bearer capabilities: do not log or share them. Embed and return pages use `no-store` and `no-referrer`.

`SITE_EMBED_SECRET` is a shared random secret (at least 32 characters) for **site and frontend** servers and all their replicas. Production schema **7** generates 32 random bytes as hex, and compose passes it to both apps. Configure it locally without exposing it to client variables. Missing configuration fails closed for embed sessions and payment handoffs; normal site bookings continue working. Rotating it invalidates outstanding embed references and payment returns. No new backend credential or third-party cookie is needed.

### 9.3 Payment outside the frame

Submitting pay obtains the existing hosted payment link through the public reservation API and moves to `/embed/pay`. That page emits `navigate` and renders both a `_top` link and an `noopener` new-tab link. The destination is the payment app's `/pay/{token}/widget?handoff=h1...` route, not an embedded iyzico checkout. Thus the ordinary hosted page and 3DS run in the top window (or a user-opened tab).

The site signs the payment token, organization UUID, full-site `/reservation/{reference}?s=...` URL and expiry with an HMAC-SHA256 purpose specific to payment handoff. The server-only frontend handoff route verifies the signature, token and expiry, then calls the backend host resolver and requires **both exact resolved origin and organization UUID** before storing the signed handoff in an HttpOnly, SameSite=Lax, 30-minute cookie scoped to this payment token. No caller-supplied unverified redirect is used.

The existing provider callback returns to `/pay/{token}?returned=1`. This top-level GET can read the first-party handoff cookie even after a provider POST; its server page revalidates the signature and live site resolution, asks the public payment endpoint to verify the provider result, and redirects to the organization's full-site reservation page. The encrypted `s` reference grants reservation access there without depending on iframe cookies. Returned browser data is never proof of payment; the reservation API remains authoritative. An invalid/expired handoff or removed/unverified domain leaves the existing payment page in place. Ordinary payment links without a handoff are unchanged.

### 9.4 Verification

Backend tests cover normalization, deduplication, the 20-entry limit, empty lists, wildcard/path/credential/query/fragment/port rejection and published resolve output. Site node tests cover CSP, parent-origin matching, encrypted references (tamper, expiry, secret and origin binding), navigation propagation, and payment handoff signature/expiry/token/live-origin/organization checks. `site/` has no Playwright dependency or configuration, so TEC-732 uses the existing node test runner; mobile browser width/height and real provider 3DS were **not** exercised. The loader coordinator should test 320/375/768px viewports and resizing through all steps once the loader is available.

## 10. Theme foundation and library (TEC-780 / TEC-781)

Config v2 is additive: all earlier theme, layout, content, SEO, contact,
analytics, widget and image fields remain valid. `config_version` normalizes
to 2. Migration **000127** upgrades both drafts and every historical snapshot
idempotently. `none` becomes `sharp`, `full` becomes `pill`; the eight current
corner styles are `soft|sharp|xs|sm|md|lg|xl|pill`. Body fonts add `manrope`,
`dm_sans`, `plus_jakarta_sans`, `sora`, `lora`, `wix_madefor`; `heading_font`
accepts all body fonts plus `theme` (follow body) and `georgia`. Existing dark
colors and light/dark/system mode are retained. The JSON body limit remains
**1 MiB**: the combined document remains bounded independently of each
field's character limit, and uploads use their own multipart limit.

`config.template` defaults to `technowide-design`. `horizon-design`, `classic`
and `aurum` are registered but unavailable for selection. The Go registry in
`backend/internal/modules/site/themes` defines each theme's supported section
order, required first section and typed enum settings. The persisted
`templates.{id} = {sections:[{id,enabled}], settings:{...}}` keeps each theme's
configuration independently. Saving one theme retains omitted other themes.
Only registered IDs and settings are accepted, duplicates are rejected, and
hero must be first and enabled. Omitted supported sections are appended disabled
in default order; a new template receives its defaults. Settings are
`heroLayout=image|split` (Technowide), `heroAlign=left|center` (Horizon), and
`fleetColumns=2|3` (Classic/Aurum; values are strings).

The API uses `guide` for the studio's `journal`, `app_promo` for `app`, and
`aurum` for `aurum-design`. `search` is an explicit supported section in every
theme. Classic additionally supports branches. Technowide's supported order is
hero, search, campaigns, fleet, advantages, newsletter, guide, about, faq,
app_promo. The panel answer includes `theme_registry` and `section_statuses`
for every union section: `available`, `needs_service`, or
`unsupported_by_theme`, plus `can_enable`. Campaigns (TEC-784), guide (TEC-785)
and newsletter (TEC-786) require their future backend services and cannot yet
be enabled. App promotion is available in Technowide's schema but
`can_enable=false` until store links exist. Enabling any of these returns a
validation detail with code **SECTION_UNAVAILABLE**. Unsupported theme sections
are independently rejected. These checks apply server-side regardless of UI.

New editorial content:

- `content.{tr,en}.hero_slides`: 1–8 slides with title (200), subtitle (1000),
  cta_label (120), cta_href (2048), media UUID or empty, enabled. Legacy hero
  text seeds a single enabled slide when slides are omitted.
- `content.{tr,en}.advantages`: up to 12 items with allowlisted Lucide kebab-case
  icon name, title (120), text (1000). The exact allowlist is in OpenAPI.
- `about.image`: media UUID or empty; `about.highlights.{tr,en}`: up to six
  strings of 300 characters. This sits at config root because legacy
  `content.{locale}.about` remains the plain-text story.
- `brand.tagline.{tr,en}` (300) and `brand.socials` keyed by instagram, facebook,
  x, youtube, linkedin, tiktok, whatsapp. URLs must use HTTPS on that network's
  host or a subdomain, without credentials or a nonstandard port. Host suffix
  lookalikes are rejected.
- `menus.header/footer`: up to 12 `{label:{tr,en},href,kind:link|page}` items;
  labels max 120. Links and slide CTAs accept relative `/` paths or HTTPS only;
  script schemes, protocol-relative paths, backslashes and control characters
  are rejected. All editorial strings remain escaped plain text.

Resolve returns the selected `template`, its ordered enabled `sections`, typed
`settings`, the full config, and a `media` map of UUID to API-owned stream URL.
A valid preview token selects draft content and is included in its media URLs.
The existing panel preserves new config fields when saving its legacy controls.

### Library API

Migration **000128** adds organization-owned `site_media`: immutable object
UUID/key, kind=image, byte-sniffed MIME, byte size, width/height, localized alt,
creator and timestamps. PNG/JPEG/WebP only; SVG/HTML are refused. Maximum file
size is **10 MiB**, dimensions **100 megapixels / 32768 pixels per side**.
The upload reads bounded bytes and parses image dimensions without allocating
pixel buffers. WebP dimensions are read from RIFF VP8/VP8L/VP8X headers.

| Route | Permission / behavior |
|-------|-----------------------|
| `GET /v1/tenant/site/media?limit=50&offset=0` | `tenant.site.read`; `{items,total,limit,offset}`, max limit 200 |
| `POST /v1/tenant/site/media` | `tenant.site.manage`; multipart file plus optional `alt` JSON string; 201 metadata |
| `PATCH /v1/tenant/site/media/{uuid}` | `tenant.site.manage`; `{alt:{tr,en}}` replaces alt, max 300 characters each |
| `DELETE /v1/tenant/site/media/{uuid}` | `tenant.site.manage`; 409 `MEDIA_IN_USE` for draft or **any historical snapshot** reference |
| `GET /v1/public/site/media/{siteUuid}/{mediaUuid}` | Only media referenced by the active publication of an enabled, accessible site, or draft with a valid `preview` token |

Keys come from `storage.SiteMediaObjectKey`, under the organization's site/media
prefix; API responses never expose storage keys. Public media carries
`public,max-age=31536000,immutable`, nosniff and a sandbox CSP; previews use
`private,no-store`. UUID bytes are immutable, so alt changes do not change URLs.
Previously cached public bytes can remain cached after unpublishing; new
requests must pass current publication and organization-access checks.

Database triggers serialize media inserts/deletes and config changes on the
organization row. They enforce **200 files / 500 MiB** transactionally
(`409 MEDIA_QUOTA_EXCEEDED`), reject unknown/deleted/cross-organization UUIDs
on draft and snapshot writes, and protect referenced objects from deletion.
Only `about.image` and `content.*.hero_slides[*].media` count as references;
UUID-like plain text is not a reference. Storage upload failures create no row;
failed quota/database inserts remove the staged object. A failed object deletion
after metadata removal is logged (an unreachable orphan), without reviving the
record. Migration down removes metadata and keeps stored bytes; its data loss
is documented in the migration.
