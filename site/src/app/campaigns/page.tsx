import type { Metadata } from "next";

import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("page.campaigns") };
}

/** Live storefront campaigns (from the resolve answer, TEC-784). */
export default async function CampaignsPage() {
  const ctx = await getCtx();
  const theme = themeFor(ctx.site);
  return <theme.Campaigns ctx={ctx} />;
}
