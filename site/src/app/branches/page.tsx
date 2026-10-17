import type { Metadata } from "next";

import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("branches.title") };
}

/** Offices: live locations of the storefront (GET /v1/public/locations). */
export default async function BranchesPage({
  searchParams,
}: {
  searchParams: Promise<{ city?: string | string[] }>;
}) {
  const ctx = await getCtx();
  const { city } = await searchParams;
  const theme = themeFor(ctx.site);
  return (
    <theme.Branches
      ctx={ctx}
      city={typeof city === "string" ? city.slice(0, 120) : undefined}
    />
  );
}
