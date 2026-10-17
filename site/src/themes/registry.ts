/**
 * Template ids the site can render (pure: node --test imports it). The API
 * registry (backend/internal/modules/site/themes) defines which templates an
 * organization may select; the site renders the ones listed here and falls
 * back to Technowide for anything else, so a newer or unknown id never
 * breaks a published site.
 */
export const DEFAULT_THEME = "technowide-design";

/** Renderers shipped in this image (src/themes/index.ts). */
export const THEME_IDS = [DEFAULT_THEME] as const;

export type ThemeId = (typeof THEME_IDS)[number];

export function resolveThemeId(template: string | null | undefined): ThemeId {
  return (THEME_IDS as readonly string[]).includes(template ?? "")
    ? (template as ThemeId)
    : DEFAULT_THEME;
}

/**
 * Home sections each renderer supports, in its default order (the API's
 * theme registry: Technowide = hero, search, campaigns, fleet, advantages,
 * newsletter, guide, about, faq, app_promo). `branches` is rendered too
 * when a template's answer contains it.
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
};
