/**
 * WCAG contrast helpers for template colors derived from the organization's
 * tokens (pure: node --test imports it). Only #rrggbb input is read;
 * anything else is treated as black, and callers pass validated tokens.
 */

const HEX = /^#[0-9a-fA-F]{6}$/;

/** Relative luminance of a #rrggbb color (0 = black, 1 = white). */
export function luminance(hex: string): number {
  if (!HEX.test(hex)) return 0;
  const [r, g, b] = [1, 3, 5]
    .map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return r * 0.2126 + g * 0.7152 + b * 0.0722;
}

/** The WCAG contrast ratio of two colors (1–21). */
export function contrastRatio(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/**
 * Text on a filled color: near-black on light fills, white on dark ones
 * (classic's accent buttons; the studio's `accentForeground`).
 */
export function textOn(fill: string): "#111827" | "#ffffff" {
  return luminance(fill) > 0.179 ? "#111827" : "#ffffff";
}

/**
 * The accent as small text on the background when it is readable (4.5:1),
 * otherwise the foreground (Aurum's kickers; the studio's `readableAccent`).
 */
export function readableAccent(
  accent: string,
  background: string,
  foreground: string,
): string {
  return HEX.test(accent) && contrastRatio(accent, background) >= 4.5
    ? accent
    : foreground;
}
