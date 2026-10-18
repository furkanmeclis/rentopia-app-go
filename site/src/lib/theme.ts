import { readableAccent, textOn } from "./contrast.ts";
import type { SiteColors, SiteTheme } from "./types";

/**
 * The organization's theme as CSS custom properties, injected at render
 * (no build-time organization data: one image, many looks). The API
 * validates the document; this file validates again so that only
 * #rrggbb colors and known enum values ever reach the stylesheet.
 *
 * Variables (consumed by the theme stylesheets under src/themes):
 * - colors: --primary, --primary-foreground, --accent, --background,
 *   --foreground, --muted, --muted-foreground, --border;
 * - fonts: --font-brand (body), --font-heading;
 * - corners: --radius-card, --radius-control, --radius-image (absent for
 *   `soft`: the theme's own radii apply) and the legacy --radius.
 */

const HEX = /^#[0-9a-fA-F]{6}$/;

export const DEFAULT_COLORS: SiteColors = {
  primary: "#1d4ed8",
  primary_foreground: "#ffffff",
  accent: "#f59e0b",
  background: "#ffffff",
  foreground: "#0f172a",
  muted: "#f1f5f9",
  muted_foreground: "#475569",
  border: "#e2e8f0",
};

export const DEFAULT_DARK_COLORS: SiteColors = {
  primary: "#60a5fa",
  primary_foreground: "#0b1220",
  accent: "#fbbf24",
  background: "#0b1220",
  foreground: "#e2e8f0",
  muted: "#1e293b",
  muted_foreground: "#94a3b8",
  border: "#334155",
};

/** Legacy single radius of the shared Tailwind classes. */
const RADIUS: Record<string, string> = {
  none: "0px",
  sharp: "0px",
  soft: "0.875rem",
  xs: "0.125rem",
  xl: "1rem",
  pill: "1.5rem",
  sm: "0.25rem",
  md: "0.5rem",
  lg: "0.875rem",
  full: "1.5rem",
};

/**
 * The eight corner styles of config v2 in pixels (cards and images).
 * `soft` is the template's own character: no variables are written and
 * every element keeps the radius its theme stylesheet gives it (the
 * `var(--radius-card, <native>)` fallbacks). Legacy `none` / `full` read
 * as `sharp` / `pill`. Controls of the `pill` style are fully rounded.
 */
export const CORNER_PX: Record<string, number> = {
  sharp: 0,
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  pill: 32,
};

export type Corners = { card: string; control: string; image: string };

/**
 * A template's own corners for `soft`, when its character differs from the
 * Technowide fallbacks its pages share: Horizon's editorial cards and
 * controls are square, photos barely rounded (the studio's look).
 */
export const TEMPLATE_CORNERS: Record<string, Corners> = {
  "horizon-design": { card: "0px", control: "0px", image: "2px" },
  // The studio's `soft` radius for classic (10 px) and Aurum (12 px).
  classic: { card: "10px", control: "10px", image: "10px" },
  aurum: { card: "12px", control: "12px", image: "12px" },
};

/** The uniform corners of a radius style, or null for the template's own (`soft`). */
export function corners(radius: string | undefined): Corners | null {
  const id = radius === "none" ? "sharp" : radius === "full" ? "pill" : radius;
  if (!id || !(id in CORNER_PX)) return null;
  const px = CORNER_PX[id];
  return {
    card: `${px}px`,
    image: `${px}px`,
    control: id === "pill" ? "9999px" : `${px}px`,
  };
}

/** Font choice → the CSS variable next/font defines (src/lib/fonts.ts). */
export const FONT_VARS: Record<string, string> = {
  inter: "var(--font-inter)",
  roboto: "var(--font-roboto)",
  open_sans: "var(--font-open-sans)",
  montserrat: "var(--font-montserrat)",
  poppins: "var(--font-poppins)",
  lato: "var(--font-lato)",
  nunito: "var(--font-nunito)",
  manrope: "var(--font-manrope)",
  dm_sans: "var(--font-dm-sans)",
  plus_jakarta_sans: "var(--font-plus-jakarta-sans)",
  sora: "var(--font-sora)",
  lora: "var(--font-lora)",
  wix_madefor: "var(--font-wix-madefor)",
  system: "system-ui",
};

const SERIF = new Set(["lora"]);
const GEORGIA = 'Georgia, "Times New Roman", serif';

/** The font stack of a body font id (unknown → Inter). */
export function bodyFont(id: string | undefined): string {
  const v = id && FONT_VARS[id] ? id : "inter";
  return `${FONT_VARS[v]}, ${SERIF.has(v) ? "Georgia, serif" : "system-ui, sans-serif"}`;
}

/**
 * The heading font stack: `theme` (or empty / unknown) follows the body
 * font, except Horizon, whose character is a serif; `georgia` is the
 * device's Georgia; any body font id is that font.
 */
export function headingFont(
  heading: string | undefined,
  body: string | undefined,
  template = "",
): string {
  if (heading === "georgia") return GEORGIA;
  if (heading && heading !== "theme" && FONT_VARS[heading])
    return bodyFont(heading);
  return template === "horizon-design" ? GEORGIA : bodyFont(body);
}

const KEYS: (keyof SiteColors)[] = [
  "primary",
  "primary_foreground",
  "accent",
  "background",
  "foreground",
  "muted",
  "muted_foreground",
  "border",
];

function safeColors(
  c: Partial<SiteColors> | undefined,
  fallback: SiteColors,
): SiteColors {
  const out = { ...fallback };
  for (const k of KEYS) {
    const v = c?.[k];
    if (typeof v === "string" && HEX.test(v)) out[k] = v.toLowerCase();
  }
  return out;
}

/**
 * Colors a template derives from the validated tokens: classic's text on
 * its accent buttons, Aurum's accent as readable small text (4.5:1 on the
 * background, else the foreground).
 */
export function templateColorVars(template: string, c: SiteColors): string {
  switch (template) {
    case "classic":
      return `--cl-accent-fg:${textOn(c.accent)};`;
    case "aurum":
      return `--au-text-accent:${readableAccent(c.accent, c.background, c.foreground)};`;
    default:
      return "";
  }
}

function colorVars(
  c: Partial<SiteColors> | undefined,
  fallback: SiteColors,
  template: string,
): string {
  const safe = safeColors(c, fallback);
  return (
    KEYS.map((k) => `--${k.replace("_", "-")}:${safe[k]};`).join("") +
    templateColorVars(template, safe)
  );
}

/** The :root stylesheet of a theme for a template (validated values only). */
export function themeCss(
  theme: Partial<SiteTheme> | undefined,
  template = "",
): string {
  const radius = RADIUS[theme?.radius ?? ""] ?? RADIUS.md;
  const c = corners(theme?.radius) ?? TEMPLATE_CORNERS[template] ?? null;
  const light = colorVars(theme?.colors, DEFAULT_COLORS, template);
  const dark = colorVars(theme?.dark_colors, DEFAULT_DARK_COLORS, template);
  const cornerVars = c
    ? `--radius-card:${c.card};--radius-control:${c.control};--radius-image:${c.image};`
    : "";
  const base =
    `--radius:${radius};${cornerVars}` +
    `--font-brand:${bodyFont(theme?.font)};--font-heading:${headingFont(theme?.heading_font, theme?.font, template)};`;
  switch (theme?.mode) {
    case "dark":
      return `:root{${base}${dark}color-scheme:dark;}`;
    case "system":
      return `:root{${base}${light}color-scheme:light dark;}@media (prefers-color-scheme: dark){:root{${dark}}}`;
    default:
      return `:root{${base}${light}color-scheme:light;}`;
  }
}

/** GA4 measurement IDs only (anything else renders no script). */
export function safeGa4Id(id: string | undefined): string | null {
  return id && /^G-[A-Z0-9]{4,16}$/.test(id) ? id : null;
}
