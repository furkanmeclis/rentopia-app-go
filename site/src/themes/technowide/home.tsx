import { SearchForm } from "@/components/site/search-form";
import { Alert } from "@/components/ui/card";
import { loadLocations, loadSubgroups } from "@/lib/catalog";
import { homeBlocks } from "@/lib/sections";
import type { Ctx } from "@/lib/server";
import type { Location } from "@/lib/types";

import { THEME_SECTIONS } from "../registry";
import { About } from "./sections/about";
import { Advantages } from "./sections/advantages";
import { Branches } from "./sections/branches";
import { Faq } from "./sections/faq";
import { Fleet } from "./sections/fleet";
import { Hero } from "./sections/hero";

/** `search` placed away from the hero: the booking card in its own band. */
function SearchSection({
  ctx,
  locations,
}: {
  ctx: Ctx;
  locations: Location[] | null;
}) {
  const { t, site } = ctx;
  return (
    <section
      className="tw-section tw-search-section"
      aria-label={t("search.title")}
    >
      <div className="rt-container">
        {!site.booking_enabled || locations === null ? (
          <Alert>{t("search.booking_off")}</Alert>
        ) : locations.length === 0 ? (
          <Alert>{t("search.no_locations")}</Alert>
        ) : (
          <SearchForm
            ctx={ctx}
            locations={locations}
            id="search"
            title={t("tw.search.title")}
            tagline={t("tw.search.tagline")}
          />
        )}
      </div>
    </section>
  );
}

/**
 * The home page: the active template's enabled sections in the
 * organization's order (lib/sections.ts drops sections whose data service
 * does not exist yet). Catalog data is live from the public API.
 */
export async function Home({ ctx }: { ctx: Ctx }) {
  const blocks = homeBlocks(
    ctx.site.sections,
    THEME_SECTIONS["technowide-design"],
  );
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
    <>
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
          case "about":
            return <About key="about" ctx={ctx} />;
          case "faq":
            return <Faq key="faq" ctx={ctx} />;
          case "branches":
            return <Branches key="branches" ctx={ctx} locations={locations} />;
          default:
            return null;
        }
      })}
    </>
  );
}
