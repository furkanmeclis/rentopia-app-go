# Rentopya brand

The product name is **Rentopya** in text. The wordmark is lowercase `rentopya`.
The technical name stays `rentopia` (see [Kept identifiers](#kept-identifiers)).

## Brand kit

Source SVGs live in [`frontend/src/assets/brand/`](../frontend/src/assets/brand). They are the official kit,
cleaned of editor metadata and pixel-identical to it, with two fixes: the
colourful icon's gradient is a plain `linearGradient` (the kit used an
`feImage` filter that some renderers drop), and the white icon's background
covers the full 850×850 canvas (the kit's stopped at 843).

| File | Art | Use on |
|------|-----|--------|
| `rentopya-logo-colorful.svg` | Horizontal: indigo mark + ink letters (1536×550) | Light backgrounds |
| `rentopya-logo-white.svg` | Horizontal, all white | Dark or indigo backgrounds, photos |
| `rentopya-logo-vertical-colorful.svg` | Stacked (825×605) | Light backgrounds, square spaces |
| `rentopya-logo-vertical-white.svg` | Stacked, all white | Dark backgrounds |
| `rentopya-icon-colorful.svg` | White "R" on an indigo → ink gradient square (850×850) | App icon, favicon, apple-touch icon, sidebar tile |
| `rentopya-icon-white.svg` | Indigo "R" on a white square | Icon on dark surfaces where a light tile is needed |

The kit's file names are the other way round from what you might expect: the
*colourful* icon has the white letter, the *white* icon has the white
background.

## Colours

| Token | Hex | Where |
|-------|-----|-------|
| Indigo (primary) | `#2E358C` | Mark, light-mode `--primary`, `--sidebar-primary`, `--chart-1`, active nav text, PDF letterhead default |
| Ink | `#141527` | Wordmark letters, light-mode `--foreground`, text on the dark-mode primary |
| Indigo light (dark-mode primary) | `#8E95EC` | Dark-mode `--primary`, `--sidebar-primary` |
| Ring | `#5A62C4` light / `#7C83E0` dark | `--ring`, `--sidebar-ring` (dark `--chart-1` too) |
| Active nav tint | `#ECEDF8` light / `#22264A` dark | `--sidebar-accent` (text `#2E358C` / `#C7CAF6`) |

Contrast (WCAG 2.x):

| Pair | Ratio |
|------|-------|
| White on `#2E358C` (button text, light) | 10.5:1 |
| `#2E358C` on white (links, light) | 10.5:1 |
| `#2E358C` on `#ECEDF8` (active nav, light) | 9.0:1 |
| `#141527` on `#8E95EC` (button text, dark) | 6.6:1 |
| `#8E95EC` on `#020617` (links, dark background) | 7.4:1 |
| `#8E95EC` on `#0F172A` (links, dark card) | 6.5:1 |
| `#C7CAF6` on `#22264A` (active nav, dark) | 9.2:1 |

CSS tokens are in [`frontend/src/styles/theme.css`](../frontend/src/styles/theme.css); JS hex aliases in
[`frontend/src/config/brand.ts`](../frontend/src/config/brand.ts). Organization sites (`site/`) and the tenant
`[data-brand]` override keep their own colours; only the platform credit text
there says Rentopya.

## Usage rules

- Light background → colourful logo. Dark, indigo or photo background → white logo.
  Never the colourful logo on dark: indigo on navy has too little contrast.
- The icon tile works on any background and is never recoloured.
- Don't stretch, outline, shadow or recolour the art, and don't retype the
  wordmark in a font. Write the name as "Rentopya" in running text.
- Keep clear space of at least the mark's stem width around the logo.

## In the panel

`@/components/brand`:

| Component | Renders |
|-----------|---------|
| `AppLogo variant="logo"` | Horizontal lockup (default) |
| `AppLogo variant="stacked"` | Vertical lockup |
| `AppLogo variant="wordmark"` | Letters only |
| `AppLogo variant="mark"` / `AppMark` | "R" glyph |
| `AppLogo variant="icon"` | App icon tile |
| `AppWordmark` | Horizontal lockup sized for the auth, payment and approval pages |
| `AppSidebarLogo` | Icon tile + name + tagline (sidebar and account header) |

`tone` names the background the logo sits on: `light` → colourful, `dark` →
white, `auto` (default) → colourful in light mode, white in dark mode. The
paths are inline (`rentopya-paths.ts`), so no request is made for the logo.

Favicon: `frontend/src/app/icon.svg`. Apple touch icon: `frontend/src/app/apple-icon.tsx`
(rendered with `ImageResponse`, no PNG committed). There is no web manifest.

## Kept identifiers

These still say `rentopia` on purpose: changing them would break
integrations, deployments or stored data.

- Webhook headers `Rentopia-Webhook-Id`, `-Event`, `-Timestamp`, `-Signature`
  and the `Rentopia-Webhooks/1` user agent (public API contract; also quoted in
  the UI and OpenAPI text that documents them).
- DNS verification record `_rentopia-verify`.
- Widget messages: `source: "rentopia-widget"` / `"rentopia-host"`, DOM events
  `rentopia:ready|completed|error`, the `#rentopia-booking` embed target in the
  snippet.
- HMAC domain strings (`rentopia/widget-payment/v1`, `rentopia/embed/v1`,
  `rentopia/site-preview/v1`, `rentopia/inspection-media/v1`); changing them
  invalidates issued tokens and links.
- Browser storage names (IndexedDB `rentopia-inspections`), cookie names.
- Wuzapi session names `rentopia-<uuid>`, obilet `rentopia-channel/1` version
  string, invoice XSL style name `rentopia-invoice`.
- The repo name and the Go module path.
- Migration files and code comments that mention Rentopia (history, not UI).
- Test fixtures (EXIF make `Rentopia`, letterhead `Rentopia`, host names).

Production infra moved to the new name with the `rentopya.app` install:
`NAME_PREFIX=rentopya` (images, containers, volumes, network `rentopya-app`),
DB user / name and S3 bucket `rentopya`, `/opt/rentopya` paths.
