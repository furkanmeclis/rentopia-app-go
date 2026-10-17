import type { SiteColors, SiteTheme } from "./types";

/**
 * The organization's theme as CSS custom properties, injected at render
 * (no build-time organization data: one image, many looks). The API
 * validates the document; this file validates again so that only
 * #rrggbb colors and known enum values ever reach the stylesheet.
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

const RADIUS: Record<string, string> = {
  none: "0px",
  sm: "0.25rem",
  md: "0.5rem",
  lg: "0.875rem",
  full: "1.5rem",
};

/** Font choice → the CSS variable next/font defines in the root layout. */
export const FONT_VARS: Record<string, string> = {
  inter: "var(--font-inter)",
  roboto: "var(--font-roboto)",
  open_sans: "var(--font-open-sans)",
  montserrat: "var(--font-montserrat)",
  poppins: "var(--font-poppins)",
  lato: "var(--font-lato)",
  nunito: "var(--font-nunito)",
  system: "system-ui",
};

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

function colorVars(
  c: Partial<SiteColors> | undefined,
  fallback: SiteColors,
): string {
  return KEYS.map((k) => {
    const v = c?.[k];
    const safe =
      typeof v === "string" && HEX.test(v) ? v.toLowerCase() : fallback[k];
    return `--${k.replace("_", "-")}:${safe};`;
  }).join("");
}

/** The :root stylesheet of a theme. */
export function themeCss(theme: Partial<SiteTheme> | undefined): string {
  const font = FONT_VARS[theme?.font ?? ""] ?? FONT_VARS.inter;
  const radius = RADIUS[theme?.radius ?? ""] ?? RADIUS.md;
  const light = colorVars(theme?.colors, DEFAULT_COLORS);
  const dark = colorVars(theme?.dark_colors, DEFAULT_DARK_COLORS);
  const base = `--radius:${radius};--font-brand:${font}, system-ui, sans-serif;`;
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
