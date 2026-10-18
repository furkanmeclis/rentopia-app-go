import { campaignCards } from "@/lib/campaigns";
import { loadLocations, loadSubgroups } from "@/lib/catalog";
import { homeBlocks } from "@/lib/sections";
import type { Ctx } from "@/lib/server";

import { THEME_SECTIONS } from "../registry";
import { Guide } from "../technowide/sections/guide";
import {
  About,
  Campaigns,
  Faq,
  Fleet,
  Hero,
  SearchSection,
  pad,
} from "./sections";

/**
 * The Horizon home page: the template's enabled sections in the
 * organization's order (lib/sections.ts drops campaigns without a live
 * campaign; the guide section is the shared Technowide one, restyled). Sections are numbered
 * in reading order (the hero is 01), like the studio's section index.
 */
export async function Home({ ctx }: { ctx: Ctx }) {
  const blocks = homeBlocks(
    ctx.site.sections,
    THEME_SECTIONS["horizon-design"],
    {
      campaigns: campaignCards(ctx.site, ctx.locale).length > 0,
    },
  );
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
    <>
      {blocks.map((b, i) => {
        const index = pad(i + 1);
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
              <SearchSection
                key="search"
                ctx={ctx}
                locations={locations}
                index={index}
              />
            );
          case "campaigns":
            return <Campaigns key="campaigns" ctx={ctx} index={index} />;
          case "fleet":
            return (
              <Fleet
                key="fleet"
                ctx={ctx}
                subgroups={subgroups}
                index={index}
              />
            );
          case "guide":
            return <Guide key="guide" ctx={ctx} />;
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
          default:
            return null;
        }
      })}
    </>
  );
}
