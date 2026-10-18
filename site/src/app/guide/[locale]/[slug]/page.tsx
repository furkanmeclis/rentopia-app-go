import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { articleMediaUrl, articlePath } from "@/lib/articles";
import { loadArticle } from "@/lib/articles-server";
import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

type Params = Promise<{ locale: string; slug: string }>;

export async function generateMetadata({
  params,
}: {
  params: Params;
}): Promise<Metadata> {
  const { locale, slug } = await params;
  const { site } = await getCtx();
  const article = await loadArticle(locale, slug);
  if (!article) return {};
  const path = articlePath(article.locale, article.slug);
  const title = article.seo_title || article.title;
  const description =
    (article.seo_description || article.summary).slice(0, 300) || undefined;
  const cover = articleMediaUrl(site, article.cover_url);
  return {
    title,
    description,
    // The public article has no link to its other-language version yet:
    // only this locale is announced.
    alternates: { canonical: path, languages: { [article.locale]: path } },
    openGraph: {
      type: "article",
      title,
      description,
      url: path,
      locale: article.locale === "tr" ? "tr_TR" : "en_GB",
      publishedTime: article.published_at || undefined,
      modifiedTime: article.updated_at || undefined,
      images: cover ? [{ url: cover }] : undefined,
    },
  };
}

/** One published guide article (TEC-785); 404 when absent or a draft. */
export default async function ArticlePage({ params }: { params: Params }) {
  const { locale, slug } = await params;
  const ctx = await getCtx();
  const article = await loadArticle(locale, slug);
  if (!article) notFound();
  const theme = themeFor(ctx.site);
  return <theme.Article ctx={ctx} article={article} />;
}
