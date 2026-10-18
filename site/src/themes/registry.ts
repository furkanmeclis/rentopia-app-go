/**
 * Template ids the site can render (pure: node --test imports it). The API
 * registry (backend/internal/modules/site/themes) defines which templates an
 * organization may select; the site renders the ones listed here and falls
 * back to Technowide for anything else, so a newer or unknown id never
 * breaks a published site.
 */
export const DEFAULT_THEME = "technowide-design";

/** Renderers shipped in this image (src/themes/index.tsx). */
export const THEME_IDS = [
  DEFAULT_THEME,
  "horizon-design",
  "classic",
  "aurum",
] as const;

export type ThemeId = (typeof THEME_IDS)[number];

export function resolveThemeId(template: string | null | undefined): ThemeId {
  return typeof template === "string" &&
    (THEME_IDS as readonly string[]).includes(template)
    ? (template as ThemeId)
    : DEFAULT_THEME;
}

/**
 * Home sections each renderer supports, in its default order (the API's
 * theme registry: Technowide = hero, search, campaigns, fleet, advantages,
 * newsletter, guide, about, faq, app_promo; Horizon = hero, search,
 * campaigns, fleet, guide, about, faq, like the studio's Horizon, which
 * has no advantages, newsletter, app or branches section; classic = hero,
 * search, fleet, advantages, branches, about, faq, campaigns, guide;
 * Aurum = hero, search, advantages, fleet, campaigns, about, faq, guide).
 * Technowide also renders `branches` when a template's answer contains it.
 */
export const THEME_SECTIONS: Record<ThemeId, readonly string[]> = {
  "technowide-design": [
    "hero",
    "search",
    "campaigns",
    "fleet",
    "advantages",
    "newsletter",
    "guide",
    "about",
    "faq",
    "app_promo",
    "branches",
  ],
  "horizon-design": [
    "hero",
    "search",
    "campaigns",
    "fleet",
    "guide",
    "about",
    "faq",
  ],
  classic: [
    "hero",
    "search",
    "fleet",
    "advantages",
    "branches",
    "about",
    "faq",
    "campaigns",
    "guide",
  ],
  aurum: [
    "hero",
    "search",
    "advantages",
    "fleet",
    "campaigns",
    "about",
    "faq",
    "guide",
  ],
};

/**
 * Horizon's `heroAlign` setting (left | center); anything else is the
 * default, left. The API validates the value; the site checks it again
 * because it becomes a class name.
 */
export function heroAlign(
  settings: Record<string, string> | null | undefined,
): "left" | "center" {
  return settings?.heroAlign === "center" ? "center" : "left";
}

/**
 * classic's and Aurum's `fleetColumns` setting ("2" | "3"): the desktop
 * columns of the vehicle grid (phones always show one). Anything else is
 * the default, three. It becomes a class name, so only these two values
 * pass.
 */
export function fleetColumns(
  settings: Record<string, string> | null | undefined,
): 2 | 3 {
  return settings?.fleetColumns === "2" ? 2 : 3;
}
