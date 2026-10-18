import "server-only";

import { cache } from "react";

import { ApiError, apiFetch } from "./api";
import { isArticleSlug } from "./articles";
import { isLocale } from "./i18n";
import { getClientIp, getSite } from "./server";
import type { ArticlePage, Locale, PublicArticle } from "./types";

/**
 * Public guide reads (TEC-785) for the request's site: the API checks the
 * host's published, enabled and accessible site and serves published
 * articles only. These are site routes (`?host=`), not booking calls, so
 * the guide works while online booking is off.
 */

export const GUIDE_PAGE_SIZE = 12;

/** One page of the visitor locale's published cards (empty on errors). */
export async function loadArticlePage(
  locale: Locale,
  page: number,
): Promise<ArticlePage | null> {
  const site = await getSite();
  const offset = (Math.max(1, page) - 1) * GUIDE_PAGE_SIZE;
  try {
    return await apiFetch<ArticlePage>(
      `/v1/public/site/articles?host=${encodeURIComponent(site.host)}&locale=${locale}&limit=${GUIDE_PAGE_SIZE}&offset=${offset}`,
      { clientIp: await getClientIp() },
    );
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null;
    throw err;
  }
}

/** A published article by locale and slug, or null (→ 404). */
export const loadArticle = cache(
  async (locale: string, slug: string): Promise<PublicArticle | null> => {
    if (!isLocale(locale) || !isArticleSlug(slug)) return null;
    const site = await getSite();
    try {
      const article = await apiFetch<PublicArticle>(
        `/v1/public/site/articles/by-slug/${locale}/${encodeURIComponent(slug)}?host=${encodeURIComponent(site.host)}`,
        { clientIp: await getClientIp() },
      );
      return article && article.slug === slug ? article : null;
    } catch (err) {
      if (err instanceof ApiError && (err.status === 404 || err.status === 400))
        return null;
      throw err;
    }
  },
);
