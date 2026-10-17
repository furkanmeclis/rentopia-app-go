# Branded site

Status: **implemented** (M5: TEC-729). Requirements §14.1. One template, one Docker image: every organization gets its own look from data. Ten organizations means one image with ten looks. The site books only through the public booking API ([public-api.md](./public-api.md)). Later work builds on it:
- **TEC-730**: theme management in the panel;
- **TEC-731**: verified custom domains;
- **TEC-732**: embeddable widget.

| Area | Path |
|------|------|
| App (Next.js App Router, own image) | `site/` (`site/Dockerfile`, service `site` in `compose.prod.yml`, CI job `site`) |
| Host → site, booking session | `site/src/proxy.ts`, `site/src/lib/{resolve,session,server}.ts` |
| Settings model, host resolution | `backend/internal/modules/site/{model,usecase,handler}`, `routes.go` |
| Migrations | `000620_branded_site`, `000621_seed_branded_site_rbac` (temporary numbers, renumbered at integration) |
| Tests | `backend/internal/modules/site/site_test.go`, `site/src/lib/theme.test.ts` |

## 1. Request flow

```
browser ── acme.rentopia.app ──▶ site (Next.js, one image)
                                   │ proxy.ts: Host → GET /v1/public/site/resolve?host=  (cached 30 s per host)
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

| Column | Notes |
|--------|-------|
| `enabled` | The host resolves only while the site is on. |
| `subdomain` | Defaults to the organization slug (`SubdomainFromSlug`). |
| `default_locale` | `tr` \| `en`. Visitors switch with `/lang?l=…` (cookie `site_locale`). |
| `config` JSONB | The typed document `model.Config` (below). Validated on save (`NormalizeConfig`); a stored document that no longer validates falls back to the defaults instead of breaking the site. |
| `api_key_id` | The publishable `web` key the site books with (composite FK to `public_api_keys`). |
| `logo/favicon/cover_object_key` | Images; TEC-730 adds the uploads. The logo falls back to the organization logo. Served by `GET /v1/public/site/assets/{site_uuid}/{kind}?v={version}`. |
| `version` | +1 on every save: optimistic concurrency (`409 VERSION_CONFLICT`) and the cache-busting value of asset URLs. |

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
- `analytics`: `ga4_measurement_id` (`G-XXXX`). The site loads gtag only for a valid ID, with `anonymize_ip`. A cookie-consent banner is still open (§7).

Theme tokens become CSS variables in a `<style>` in the SSR head (`site/src/lib/theme.ts`). The site validates them again, so only hex colors and known enum values reach CSS. The Tailwind theme maps these variables, and the shadcn-style components in `site/src/components/ui` read them.

### API

| Route | Who | Notes |
|-------|-----|-------|
| `GET /v1/tenant/site` | `tenant.site.read` | The settings, or the defaults with `exists: false, version: 0`. Also returns `url`, `hosts`, `api_key {uuid, status}`, `assets` (which images exist) and `booking_enabled`. |
| `PUT /v1/tenant/site` | `tenant.site.manage` | `{version, enabled?, subdomain?, default_locale?, config?}`: omitted fields are kept, and `config` replaces the whole document. Errors: `400` field details, `409 SUBDOMAIN_TAKEN`, `409 VERSION_CONFLICT`. Activity: `site.updated`. |
| `GET /v1/public/site/resolve?host=` | public, 600 / min / IP | The resolved site (`model.Resolved`): `site_uuid`, `version`, `origin`, `host_kind`, `organization {uuid, slug, name}`, `publishable_key`, `booking_enabled`, `config`, `assets` (API paths), `portal_url`. Errors: `404 SITE_NOT_FOUND`. |
| `GET /v1/public/site/assets/{uuid}/{kind}` | public | `logo` \| `favicon` \| `cover` of an enabled site. |

Permissions (000621): `tenant.site.read` and `tenant.site.manage`, granted to owner, admin and merkez_yoneticisi (like the public API keys).

### The site key

- **Switching the site on** links a usable publishable `web` key. It creates one named "Markalı site", with all scopes, when the linked key is missing, revoked or expired.
- **Every save** keeps the key's `allowed_origins` equal to the hand-added origins plus the site's current origins. A subdomain change replaces the old origin; custom domains come in through `CustomDomains.Hosts`.
- **Disabling** the site keeps the key: the host stops resolving, so the site stops.
- `booking_enabled` is true when all of these hold:
  - the site is on;
  - the key is active (not revoked or expired);
  - the organization's **web** sales channel is on (the public API's `checkChannel` gate; packages arrive with TEC-674).

  When it is false, the site still renders its content pages and shows "online booking is closed".
- **Rotating the key** in Ayarlar → Public API leaves the site on the old key until its grace period ends. Saving the site after the old key expires links a new one. TEC-730 should offer "renew key" or follow `rotated_from_id`.

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
- **Headers**: frames denied, and `form-action` limited to self and https (so the pay redirect works).

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

- **TEC-730**: the panel editor on `GET/PUT /v1/tenant/site`, image uploads (`*_object_key`), draft / publish (today a save is live; `version` is ready for a published / draft split), and "renew site key".
- **TEC-731**: the `CustomDomains` implementation (`site_domains`, DNS verification, TLS on the site service).
- **TEC-732**: a widget entry point that reuses the site's booking pages and `lib/`.
- A cookie-consent banner before analytics; a sitemap; branch detail pages.
- **Time zone**: organizations sell in Turkey, so the site uses Europe/Istanbul (+03:00) for search times.
