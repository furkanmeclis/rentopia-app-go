import { campaignCards } from "@/lib/campaigns";
import { loadLocations, loadSubgroups } from "@/lib/catalog";
import { homeBlocks } from "@/lib/sections";
import type { Ctx } from "@/lib/server";

import { guideCards } from "../guide";
import { THEME_SECTIONS } from "../registry";
import {
  About,
  Advantages,
  Branches,
  Campaigns,
  Faq,
  Fleet,
  Guide,
  Hero,
  SearchSection,
} from "./sections";

/**
 * The classic home page: the template's enabled sections in the
 * organization's order (lib/sections.ts drops sections without their data
 * service, campaigns without a live campaign and guides without articles).
 * Fleet, search and branches use the live catalog.
 */
export async function Home({ ctx }: { ctx: Ctx }) {
  const blocks = homeBlocks(ctx.site.sections, THEME_SECTIONS.classic, {
    campaigns: campaignCards(ctx.site, ctx.locale).length > 0,
    guide: guideCards(ctx.site, ctx.locale).length > 0,
  });
  const ids = new Set(blocks.map((b) => b.id));
  const needsLocations =
    ctx.site.booking_enabled &&
    (blocks.some((b) => b.id === "hero" && b.withSearch) ||
      ids.has("search") ||
      ids.has("branches"));
  const [locations, subgroups] = await Promise.all([
    needsLocations ? loadLocations() : Promise.resolve(null),
    ctx.site.booking_enabled && ids.has("fleet")
      ? loadSubgroups()
      : Promise.resolve(null),
  ]);
  return (
    <div className="cl-home">
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
          case "fleet":
            return <Fleet key="fleet" ctx={ctx} subgroups={subgroups} />;
          case "advantages":
            return <Advantages key="advantages" ctx={ctx} />;
          case "branches":
            return <Branches key="branches" ctx={ctx} locations={locations} />;
          case "about":
            return <About key="about" ctx={ctx} />;
          case "faq":
            return (
              <Faq
                key="faq"
                ctx={ctx}
                items={ctx.content.faq.slice(0, 6)}
                more={ctx.content.faq.length > 6}
              />
            );
          case "campaigns":
            return <Campaigns key="campaigns" ctx={ctx} />;
          case "guide":
            return <Guide key="guide" ctx={ctx} />;
          default:
            return null;
        }
      })}
    </div>
  );
}
