import { campaignMediaUrl } from "../lib/campaigns.ts";
import type { Locale, Site } from "../lib/types";

/**
 * Guide cards of the classic and Aurum home sections (pure: node --test
 * imports it). Resolve carries `articles: {tr: ArticleCard[], en: …}`, up to
 * three published cards per locale (TEC-785). Only well-formed cards are
 * kept: a title, a slug of the article grammar, a parseable publication
 * time; the cover only when it is this site's library stream path (served
 * through /media), else the card shows a token gradient. Links follow the
 * article route contract `/guide/{locale}/{slug}`.
 *
 * The section itself renders only while `lib/sections.ts` lets `guide`
 * through (the guide pages ship separately).
 */

export type GuideCard = {
  title: string;
  summary: string;
  href: string;
  image: string | null;
  /** ISO date (YYYY-MM-DD) of the publication. */
  date: string | null;
};

const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

type RawCard = {
  title?: unknown;
  summary?: unknown;
  slug?: unknown;
  cover_url?: unknown;
  published_at?: unknown;
};

export function guideCards(
  site: Pick<Site, "site_uuid"> & { articles?: unknown },
  locale: Locale,
  limit = 3,
): GuideCard[] {
  const all = site.articles;
  const list =
    all && typeof all === "object"
      ? (all as Record<string, unknown>)[locale]
      : null;
  if (!Array.isArray(list)) return [];
  const out: GuideCard[] = [];
  const seen = new Set<string>();
  for (const raw of list as RawCard[]) {
    if (!raw || typeof raw !== "object") continue;
    const { title, summary, slug, cover_url, published_at } = raw;
    if (typeof title !== "string" || !title.trim()) continue;
    if (typeof slug !== "string" || slug.length > 160 || !SLUG.test(slug))
      continue;
    if (seen.has(slug)) continue;
    seen.add(slug);
    const time =
      typeof published_at === "string" ? Date.parse(published_at) : NaN;
    out.push({
      title: title.trim(),
      summary: typeof summary === "string" ? summary.trim() : "",
      href: `/guide/${locale}/${slug}`,
      image: campaignMediaUrl(
        site,
        typeof cover_url === "string" ? cover_url : null,
      ),
      date: Number.isNaN(time)
        ? null
        : new Date(time).toISOString().slice(0, 10),
    });
    if (out.length >= limit) break;
  }
  return out;
}
