import type { Metadata } from "next";

import { getCtx } from "@/lib/server";
import { themeFor, type Query } from "@/themes";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("page.franchise") };
}

/** The franchise application form (404 while it is switched off). */
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<Query>;
}) {
  const ctx = await getCtx();
  const theme = themeFor(ctx.site);
  return (
    <theme.Inquiry ctx={ctx} kind="franchise" query={await searchParams} />
  );
}
