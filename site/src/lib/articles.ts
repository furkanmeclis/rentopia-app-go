import type { Locale, Site, SiteArticleCard } from "./types";

/**
 * Guide articles (TEC-785; pure: node --test imports it). The resolve
 * answer carries up to three published cards per locale for the home
 * section; the guide list and article pages read the public article API.
 * Article URLs are `/guide/{locale}/{slug}` (the sitemap's paths).
 */

const ID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const MEDIA = new RegExp(`^/v1/public/site/media/(${ID})/(${ID})$`, "i");
export const ARTICLE_SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/;

export type ArticleCard = {
  title: string;
  summary: string;
  href: string;
  image: string | null;
  publishedAt: string | null;
};

export function isArticleSlug(slug: string | null | undefined): boolean {
  return (
    typeof slug === "string" && slug.length <= 160 && ARTICLE_SLUG.test(slug)
  );
}

export function articlePath(locale: Locale, slug: string): string {
  return `/guide/${locale}/${slug}`;
}

/**
 * Same-origin image of an article (cover or inline): the API-owned stream
 * path of this site's library through the /media route; anything else →
 * null (token gradient / no image).
 */
export function articleMediaUrl(
  site: Pick<Site, "site_uuid">,
  path: string | null | undefined,
): string | null {
  const m = typeof path === "string" ? MEDIA.exec(path) : null;
  if (!m || m[1].toLowerCase() !== site.site_uuid.toLowerCase()) return null;
  return `/media/site/media/${m[1].toLowerCase()}/${m[2].toLowerCase()}`;
}

/** Well-formed cards of one locale (title and a valid slug). */
export function toArticleCards(
  site: Pick<Site, "site_uuid">,
  cards: SiteArticleCard[] | null | undefined,
  locale: Locale,
): ArticleCard[] {
  const out: ArticleCard[] = [];
  const seen = new Set<string>();
  for (const c of Array.isArray(cards) ? cards : []) {
    if (!c || !isArticleSlug(c.slug) || seen.has(c.slug)) continue;
    const title = typeof c.title === "string" ? c.title.trim() : "";
    if (!title) continue;
    seen.add(c.slug);
    const date =
      typeof c.published_at === "string" &&
      !Number.isNaN(Date.parse(c.published_at))
        ? c.published_at
        : null;
    out.push({
      title,
      summary: typeof c.summary === "string" ? c.summary.trim() : "",
      href: articlePath(locale, c.slug),
      image: articleMediaUrl(site, c.cover_url),
      publishedAt: date,
    });
  }
  return out;
}

/** The home section's cards: the resolve answer's (max three). */
export function homeArticleCards(
  site: Pick<Site, "site_uuid" | "articles">,
  locale: Locale,
): ArticleCard[] {
  return toArticleCards(site, site.articles?.[locale], locale).slice(0, 3);
}

/** "18 Eki 2026" / "18 Oct 2026" in Istanbul time. */
export function articleDate(iso: string, locale: Locale): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "Europe/Istanbul",
  }).format(d);
}
