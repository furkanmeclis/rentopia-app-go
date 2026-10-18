import type { SiteSection } from "./types";

/**
 * Which home sections render, in which order (pure: node --test imports it).
 *
 * The resolve answer lists the active template's enabled sections in the
 * organization's order. The site applies the API's section status again,
 * so a section never shows without its data service:
 * - a section listed in NEEDS_SERVICE renders nothing until its backend
 *   exists (none today: `campaigns`, TEC-784, `newsletter`, TEC-786, and
 *   `guide`, TEC-785, have their services);
 * - sections whose data is missing for this site (`ready[id] === false`:
 *   no live campaign, the newsletter form switched off or no usable key,
 *   no published guide article in the visitor's language)
 *   are dropped before the layout is decided;
 * - `app_promo` is always hidden (no store links yet);
 * - unknown ids, ids the renderer does not support, duplicates and disabled
 *   entries are dropped;
 * - `hero` always comes first (the API requires it; legacy answers without
 *   sections get the renderer's defaults).
 *
 * `search` directly after the hero is drawn inside the hero (the
 * Technowide booking card); anywhere else it is its own section.
 */

export const NEEDS_SERVICE: readonly string[] = [];
export const ALWAYS_HIDDEN = ["app_promo"] as const;

export type HomeBlock =
  { id: "hero"; withSearch: boolean } | { id: string; withSearch?: undefined };

export function sectionRenders(id: string): boolean {
  return (
    !NEEDS_SERVICE.includes(id) &&
    !(ALWAYS_HIDDEN as readonly string[]).includes(id)
  );
}

/** Default sections when the answer has none (pre-v2 API). */
export const LEGACY_SECTIONS: SiteSection[] = [
  { id: "hero", enabled: true },
  { id: "search", enabled: true },
  { id: "fleet", enabled: true },
  { id: "advantages", enabled: true },
  { id: "about", enabled: true },
  { id: "faq", enabled: true },
];

export function homeBlocks(
  sections: SiteSection[] | null | undefined,
  supported: readonly string[],
  ready: Partial<Record<string, boolean>> = {},
): HomeBlock[] {
  const list = Array.isArray(sections) ? sections : LEGACY_SECTIONS;
  const seen = new Set<string>();
  const ids: string[] = [];
  for (const s of list) {
    if (!s || typeof s.id !== "string" || s.enabled !== true) continue;
    if (seen.has(s.id) || !supported.includes(s.id)) continue;
    seen.add(s.id);
    if (sectionRenders(s.id) && ready[s.id] !== false) ids.push(s.id);
  }
  const rest = ids.filter((id) => id !== "hero");
  const withSearch = rest[0] === "search";
  const blocks: HomeBlock[] = [{ id: "hero", withSearch }];
  for (const id of withSearch ? rest.slice(1) : rest) blocks.push({ id });
  return blocks;
}
