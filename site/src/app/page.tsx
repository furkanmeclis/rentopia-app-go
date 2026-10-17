import { Hero } from "@/components/site/hero";
import { SearchForm } from "@/components/site/search-form";
import { Container, Paragraphs } from "@/components/site/text";
import { Alert } from "@/components/ui/card";
import { loadLocations } from "@/lib/catalog";
import { getCtx } from "@/lib/server";

export default async function HomePage() {
  const ctx = await getCtx();
  const locations = ctx.site.booking_enabled ? await loadLocations() : null;
  return (
    <>
      <Hero ctx={ctx}>
        {!ctx.site.booking_enabled || locations === null ? (
          <Alert>{ctx.t("search.booking_off")}</Alert>
        ) : locations.length === 0 ? (
          <Alert>{ctx.t("search.no_locations")}</Alert>
        ) : (
          <SearchForm ctx={ctx} locations={locations} />
        )}
      </Hero>
      {ctx.content.about ? (
        <Container className="py-12">
          <section
            aria-labelledby="about-title"
            className="mx-auto max-w-3xl space-y-4"
          >
            <h2 id="about-title" className="text-2xl font-semibold">
              {ctx.t("nav.about")}
            </h2>
            <Paragraphs
              text={ctx.content.about
                .split(/\n\s*\n/)
                .slice(0, 2)
                .join("\n\n")}
              className="text-muted-foreground"
            />
          </section>
        </Container>
      ) : null}
    </>
  );
}
