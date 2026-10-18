import type {
  SiteArticle,
  SiteArticleStatus,
} from "@/features/site-articles/types";

export const ARTICLE_STATUSES: SiteArticleStatus[] = ["draft", "published"];

export const STATUS_LABEL_KEYS: Record<SiteArticleStatus, string> = {
  draft: "site.articles.statuses.draft",
  published: "site.articles.statuses.published",
};

export const STATUS_TONES: Record<
  SiteArticleStatus,
  "default" | "success" | "warning" | "danger"
> = {
  draft: "default",
  published: "success",
};

/** The title in the panel language, else the other one. */
export function articleTitle(
  article: Pick<SiteArticle, "title">,
  locale: string,
): string {
  const first = locale === "en" ? "en" : "tr";
  const second = first === "en" ? "tr" : "en";
  return (article.title?.[first] || article.title?.[second] || "").trim();
}
