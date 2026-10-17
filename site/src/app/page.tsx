import { getCtx } from "@/lib/server";
import { themeFor, type Query } from "@/themes";

/** The home page of the active template (sections from the resolved config). */
export default async function HomePage({
  searchParams,
}: {
  searchParams: Promise<Query>;
}) {
  const ctx = await getCtx();
  const theme = themeFor(ctx.site);
  return <theme.Home ctx={ctx} query={await searchParams} />;
}
