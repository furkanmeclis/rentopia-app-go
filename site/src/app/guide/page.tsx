import type { Metadata } from "next";

import { loadArticlePage } from "@/lib/articles-server";
import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

function pageOf(raw: string | string[] | undefined): number {
  const v = Array.isArray(raw) ? raw[0] : raw;
  const n = v && /^\d{1,4}$/.test(v) ? Number(v) : 1;
  return n >= 1 ? n : 1;
}

export async function generateMetadata({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}): Promise<Metadata> {
  const { t } = await getCtx();
  const page = pageOf((await searchParams).page);
  return {
    title: t("page.guide"),
    description: t("tw.guide.lead"),
    alternates: { canonical: page > 1 ? `/guide?page=${page}` : "/guide" },
  };
}

/** Guide list (TEC-785): published articles of the visitor's language. */
export default async function GuidePage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const ctx = await getCtx();
  const page = pageOf((await searchParams).page);
  const data = await loadArticlePage(ctx.locale, page);
  const theme = themeFor(ctx.site);
  return <theme.Guide ctx={ctx} page={page} data={data} />;
}
