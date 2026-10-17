import type { Metadata } from "next";

import { loadLocations } from "@/lib/catalog";
import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ uuid: string }>;
}): Promise<Metadata> {
  const { uuid } = await params;
  const { t, site } = await getCtx();
  const l = site.booking_enabled
    ? (await loadLocations())?.find((x) => x.uuid === uuid)
    : undefined;
  return { title: l?.name || t("branches.title") };
}

/** One office (live location); unknown or hidden offices are 404. */
export default async function BranchPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const ctx = await getCtx();
  const { uuid } = await params;
  const theme = themeFor(ctx.site);
  return <theme.Branch ctx={ctx} uuid={uuid} />;
}
