import type { Metadata } from "next";

import { campaignCard } from "@/lib/campaigns";
import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ uuid: string }>;
}): Promise<Metadata> {
  const { uuid } = await params;
  const { t, site, locale } = await getCtx();
  const card = campaignCard(site, uuid, locale);
  if (!card) return { title: t("page.campaigns") };
  return {
    title: card.title || t("tw.campaigns.default_title"),
    description: card.summary.slice(0, 300) || undefined,
  };
}

/** One live campaign; 404 when it is unknown or no longer live. */
export default async function CampaignPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  const ctx = await getCtx();
  const theme = themeFor(ctx.site);
  return <theme.Campaign ctx={ctx} uuid={uuid} />;
}
