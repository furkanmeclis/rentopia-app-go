import { SearchForm, type SearchValues } from "@/components/site/search-form";
import { PageHeading, PageSection } from "@/components/site/page-heading";
import { Alert } from "@/components/ui/card";
import { loadLocations } from "@/lib/catalog";
import { istanbulDate } from "@/lib/format";
import { getCtx } from "@/lib/server";

export default async function EmbedPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const ctx = await getCtx();
  const sp = await searchParams;
  const value = (key: string) =>
    typeof sp[key] === "string" ? (sp[key] as string) : undefined;
  const initial = ctx.site.config.widget?.initial_filters;
  const locations = await loadLocations();
  const values: SearchValues = {
    pickup:
      value("pickup") || value("branch") || initial?.pickup_branch_uuid || "",
    return: value("return"),
    pickup_date: value("pickup_date") || istanbulDate(1),
    pickup_time: value("pickup_time") || "10:00",
    return_date:
      value("return_date") || istanbulDate(1 + (initial?.rental_days || 3)),
    return_time: value("return_time") || "10:00",
  };
  return (
    <>
      <PageHeading ctx={ctx} title={ctx.t("search.title")} />
      <PageSection>
        {locations?.length ? (
          <SearchForm ctx={ctx} locations={locations} values={values} compact />
        ) : (
          <Alert>{ctx.t("search.no_locations")}</Alert>
        )}
      </PageSection>
    </>
  );
}
