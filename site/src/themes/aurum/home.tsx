import { campaignCards } from "@/lib/campaigns";
import { loadLocations, loadSubgroups } from "@/lib/catalog";
import { homeBlocks } from "@/lib/sections";
import type { Ctx } from "@/lib/server";

import { guideCards } from "../guide";
import { THEME_SECTIONS } from "../registry";
import {
  About,
  Advantages,
  Campaigns,
  Faq,
  Fleet,
  Guide,
  Hero,
  SearchSection,
} from "./sections";

/**
 * The Aurum home page: the template's enabled sections in the
 * organization's order (lib/sections.ts drops sections without their data
 * service, campaigns without a live campaign and guides without articles).
 */
export async function Home({ ctx }: { ctx: Ctx }) {
  const blocks = homeBlocks(ctx.site.sections, THEME_SECTIONS.aurum, {
    campaigns: campaignCards(ctx.site, ctx.locale).length > 0,
    guide: guideCards(ctx.site, ctx.locale).length > 0,
  });
  const ids = new Set(blocks.map((b) => b.id));
  const needsLocations =
    ctx.site.booking_enabled &&
    (blocks.some((b) => b.id === "hero" && b.withSearch) || ids.has("search"));
  const [locations, subgroups] = await Promise.all([
    needsLocations ? loadLocations() : Promise.resolve(null),
    ctx.site.booking_enabled && ids.has("fleet")
      ? loadSubgroups()
      : Promise.resolve(null),
  ]);
  return (
    <div className="au-home">
      {blocks.map((b) => {
        switch (b.id) {
          case "hero":
            return (
              <Hero
                key="hero"
                ctx={ctx}
                locations={locations}
                withSearch={Boolean(b.withSearch)}
              />
            );
          case "search":
            return (
              <SearchSection key="search" ctx={ctx} locations={locations} />
            );
          case "advantages":
            return <Advantages key="advantages" ctx={ctx} />;
          case "fleet":
            return <Fleet key="fleet" ctx={ctx} subgroups={subgroups} />;
          case "campaigns":
            return <Campaigns key="campaigns" ctx={ctx} />;
          case "about":
            return <About key="about" ctx={ctx} />;
          case "faq":
            return <Faq key="faq" ctx={ctx} />;
          case "guide":
            return <Guide key="guide" ctx={ctx} />;
          default:
            return null;
        }
      })}
    </div>
  );
}
