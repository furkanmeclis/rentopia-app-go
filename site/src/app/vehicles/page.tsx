import type { Metadata } from "next";

import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("fleet.title") };
}

/** The fleet: live vehicle subgroups of the storefront. */
export default async function VehiclesPage({
  searchParams,
}: {
  searchParams: Promise<{ segment?: string | string[] }>;
}) {
  const ctx = await getCtx();
  const { segment } = await searchParams;
  const theme = themeFor(ctx.site);
  return (
    <theme.Vehicles
      ctx={ctx}
      segment={typeof segment === "string" ? segment.slice(0, 120) : undefined}
    />
  );
}
