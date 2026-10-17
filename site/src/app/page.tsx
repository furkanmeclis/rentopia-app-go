import { getCtx } from "@/lib/server";
import { themeFor } from "@/themes";

/** The home page of the active template (sections from the resolved config). */
export default async function HomePage() {
  const ctx = await getCtx();
  const theme = themeFor(ctx.site);
  return <theme.Home ctx={ctx} />;
}
