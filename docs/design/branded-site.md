# Branded site

Status: **implemented** (M5: TEC-729 site, TEC-730 panel editor with draft / publish). Requirements §14.1. One template, one Docker image: every organization gets its own look from data. Ten organizations means one image with ten looks. The site books only through the public booking API ([public-api.md](./public-api.md)). Later work builds on it:
- **TEC-731**: verified custom domains;
- **TEC-732**: embeddable widget.

| Area | Path |
|------|------|
| App (Next.js App Router, own image) | `site/` (`site/Dockerfile`, service `site` in `compose.prod.yml`, CI job `site`) |
| Host → site, booking session | `site/src/proxy.ts`, `site/src/lib/{resolve,session,server}.ts` |
| Settings model, publish, preview, images, catalog state, host resolution | `backend/internal/modules/site/{model,usecase,handler}`, `routes.go` |
| Panel editor (Ayarlar → Markalı site) | `frontend/src/features/site`, route `/t/{slug}/settings/site` |
| Preview in the site | `site/src/lib/preview.ts`, `site/src/proxy.ts`, `site/src/app/preview/exit` |
| Migrations | `000122_branded_site`, `000123_seed_branded_site_rbac`, `000124_site_publish`, `000125_catalog_show_online` |
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
- **Custom domains (TEC-731 hook)**: `usecase.CustomDomains` is the hook, set with `Service.SetCustomDomains`:
  - `Lookup(host) → organization id` (or `ErrNotFound`). It is called for hosts that are not under the base, and the site is loaded with `ResolveSiteByOrganization`. The resolved origin is `https://{host}`.
  - `Hosts(org) → []Host`. These are the verified domains: they appear in the settings view and are added to the site key's allowed origins on save.

  TEC-731 adds a `site_domains` table (verification state, TLS) and the implementation, and calls the site save (or an equivalent origin sync) when a domain is verified or removed. Nothing in the site app changes.
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

`config`:
- `theme`:
  - `colors` / `dark_colors` (`primary`, `primary_foreground`, `accent`, `background`, `foreground`, `muted`, `muted_foreground`, `border`; `#rrggbb`, empty = default);
  - `font` (`inter|roboto|open_sans|montserrat|poppins|lato|nunito|system`; self-hosted with `next/font`, so no visitor request goes to Google);
  - `radius` (`none|sm|md|lg|full`);
  - `mode` (`light|dark|system`).
- `layout`: `hero` (`centered|split|image|minimal`), `cards` (`grid|list|compact`), `footer` (`simple|columns|minimal`).
- `content.{tr,en}`: `hero_title`, `hero_subtitle`, `about`, `faq[{question, answer}]` (≤ 40), `terms`, `privacy` (KVKK). Plain text only: blank lines separate paragraphs, and no HTML is ever rendered.
- `seo.{tr,en}`: `title` (≤ 70), `description` (≤ 300).
- `contact`: `phone`, `email`, `address`, `whatsapp`.
- `analytics`: `ga4_measurement_id` (`G-XXXX`). The site loads gtag only for a valid ID, with `anonymize_ip` (never in a preview). A cookie-consent banner is still open (§7).
- `widget` (stored for TEC-732, not enforced yet): `allowed_domains` (≤ 20 origins, `https://host[:port]`, `http` only for localhost; normalized and de-duplicated) and `initial_filters {pickup_branch_uuid, rental_days (0-90)}`.

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

### 3.3 Catalog publish state (000125)

Which branches, vehicle subgroups and extras the online channels (public API: site, widget, API keys) offer. These switches are **live** (they change what is sold, not how the site looks):

| Kind | Flag | Effect |
|------|------|--------|
| Branch | `branches.show_online` (default on) | left out of `/v1/public/locations` (and its service points); availability / quotes naming it, directly or through a point, answer `400 … not_found` |
| Subgroup | `rental_products.show_online` (default on) | left out of `/v1/public/vehicle-subgroups` and availability items; quotes answer `400 subgroup_uuid not_found` |
| Extra | the existing `extras.channels` (no new column) | shown = `channels` is NULL or contains `web`. Hiding an "every channel" extra stores every other sales channel; showing adds `web` |

Service points keep their own `online_selectable`. API: `GET /v1/tenant/site/catalog`, `PATCH /v1/tenant/site/catalog/{branches|products|extras}/{uuid} {show_online}` (record looked up first: another organization's record is `404`).

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
- **TEC-732**: a widget entry point that reuses the site's booking pages and `lib/`; enforce `config.widget.allowed_domains` (e.g. as the widget key's allowed origins) and apply `initial_filters`.
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
- **Widget**: allowed domains and initial filters (stored, "yakında" notice); keys stay on Ayarlar → Public API.

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
| `TestCatalogOnlineState` | listing and switches; foreign record → not found; extras channel rules; locations / extras listings, availability and quotes respect the flags |
| `TestRenewKey` | rotation with a 1 h grace and the same origins; the site books with the new key at once; a rotation in Public API is followed on save; a revoked key is replaced |
| `site/src/lib/preview.test.ts` | token shape filter, cookie lifetime and attributes, frame headers (deny by default, panel origin only for previews, unsafe origins refused) |

RT-A01 covers the new routes (allowlist entries for the scope-only writes; `{revisionUuid}` and the catalog `{uuid}` routes get the cross-organization 404 cases).
