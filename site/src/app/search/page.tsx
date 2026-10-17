import type { Metadata } from "next";

import { selectVehicle } from "@/app/actions";
import { ArrowRight } from "lucide-react";

import { PageHeading, PageSection } from "@/components/site/page-heading";
import { SearchForm, type SearchValues } from "@/components/site/search-form";
import { VehicleCard } from "@/components/site/vehicle-card";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/card";
import { ApiError } from "@/lib/api";
import { loadLocations, splitPlace } from "@/lib/catalog";
import { dateTime, money, toRFC3339 } from "@/lib/format";
import { bookingApi, getCtx, type Ctx } from "@/lib/server";
import type { Availability, AvailabilityItem, Subgroup } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("results.title"), robots: { index: false } };
}

type Params = Record<string, string | string[] | undefined>;
const one = (v: string | string[] | undefined) =>
  (Array.isArray(v) ? v[0] : v) ?? "";

/** An availability result: the group card with its total and the select action. */
function ResultCard({
  ctx,
  item,
  subgroup,
  hidden,
}: {
  ctx: Ctx;
  item: AvailabilityItem;
  subgroup?: Subgroup;
  hidden: Record<string, string>;
}) {
  const { t, locale } = ctx;
  const model = subgroup?.representative_model || item.subgroup.title;
  const price = item.price;
  return (
    <VehicleCard
      ctx={ctx}
      subgroup={subgroup}
      fallback={item.subgroup}
      heading="h2"
      muted={!item.available}
    >
      {item.available && price ? (
        <div className="rt-price">
          <p className="rt-price-total">
            {money(price.total, price.currency, locale)}
          </p>
          <p>
            {t("results.days", { days: price.billing_days })} ·{" "}
            {t("results.per_day", {
              amount: money(price.daily_average, price.currency, locale),
            })}
          </p>
          {Number(price.prepayment) > 0 ? (
            <p>
              {t("results.prepayment", {
                amount: money(price.prepayment, price.currency, locale),
              })}
            </p>
          ) : null}
        </div>
      ) : (
        <p className="rt-price rt-price--none">{t("results.unavailable")}</p>
      )}
      {item.available && price ? (
        <form action={selectVehicle} className="rt-select-form">
          {Object.entries(hidden).map(([k, v]) => (
            <input key={k} type="hidden" name={k} value={v} />
          ))}
          <input
            type="hidden"
            name="subgroup_uuid"
            value={item.subgroup.uuid}
          />
          {item.units_left > 0 && item.units_left <= 3 ? (
            <p className="rt-units-left">
              {t("results.units_left", { count: item.units_left })}
            </p>
          ) : null}
          <Button type="submit" aria-label={`${t("results.select")}: ${model}`}>
            {t("results.select")}
            <ArrowRight aria-hidden size={16} />
          </Button>
        </form>
      ) : null}
    </VehicleCard>
  );
}

export default async function SearchPage({
  searchParams,
}: {
  searchParams: Promise<Params>;
}) {
  const ctx = await getCtx();
  const { t, locale, site } = ctx;
  const sp = await searchParams;
  const values: SearchValues = {
    pickup: one(sp.pickup),
    return: one(sp.return),
    pickup_date: one(sp.pickup_date),
    pickup_time: one(sp.pickup_time),
    return_date: one(sp.return_date),
    return_time: one(sp.return_time),
  };
  const locations = site.booking_enabled ? await loadLocations() : null;
  if (!locations) {
    return (
      <>
        <PageHeading ctx={ctx} title={t("search.title")} />
        <PageSection>
          <Alert>{t("search.booking_off")}</Alert>
        </PageSection>
      </>
    );
  }

  const pickup = splitPlace(values.pickup);
  const ret = values.return ? splitPlace(values.return) : pickup;
  const startsAt = toRFC3339(
    values.pickup_date ?? "",
    values.pickup_time ?? "",
  );
  const endsAt = toRFC3339(values.return_date ?? "", values.return_time ?? "");
  const valid =
    pickup &&
    ret &&
    startsAt &&
    endsAt &&
    new Date(endsAt) > new Date(startsAt);

  let availability: Availability | null = null;
  let subgroups: Subgroup[] = [];
  let failed = false;
  if (valid) {
    const q = new URLSearchParams({
      pickup_branch_uuid: pickup.branch,
      return_branch_uuid: ret.branch,
      starts_at: startsAt,
      ends_at: endsAt,
    });
    if (pickup.point) q.set("pickup_point_uuid", pickup.point);
    if (ret.point) q.set("return_point_uuid", ret.point);
    try {
      [availability, subgroups] = await Promise.all([
        bookingApi<Availability>(`/v1/public/availability?${q}`),
        bookingApi<Subgroup[]>("/v1/public/vehicle-subgroups").catch(
          () => [] as Subgroup[],
        ),
      ]);
    } catch (err) {
      failed = !(err instanceof ApiError && err.status === 400);
    }
  }

  const byUuid = new Map(subgroups.map((s) => [s.uuid, s]));
  const items = [...(availability?.items ?? [])]
    .filter(
      (item) =>
        !ctx.embed?.fields.subgroup ||
        item.subgroup.uuid === ctx.embed.fields.subgroup,
    )
    .sort((a, b) => {
      if (a.available !== b.available) return a.available ? -1 : 1;
      return (
        Number(a.price?.total ?? Infinity) - Number(b.price?.total ?? Infinity)
      );
    });
  const back = `/search?${new URLSearchParams(Object.entries(values).filter(([, v]) => v) as [string, string][])}`;
  const hidden = {
    pickup: values.pickup ?? "",
    return: values.return ?? "",
    starts_at: startsAt ?? "",
    ends_at: endsAt ?? "",
    back,
  };
  const error = one(sp.error);
  // A bare /search (e.g. "Hemen kirala") opens the form without an error.
  const attempted = Boolean(values.pickup_date || values.return_date);

  return (
    <>
      <PageHeading
        ctx={ctx}
        title={availability ? t("results.title") : t("search.title")}
        lead={
          availability
            ? t("results.summary", {
                pickup: availability.request.pickup_branch.name,
                return: availability.request.return_branch.name,
                start: dateTime(availability.request.starts_at, locale),
                end: dateTime(availability.request.ends_at, locale),
              })
            : undefined
        }
      />
      <PageSection className="rt-search-page">
        <details className="rt-disclosure" open={!valid}>
          <summary>{t("search.edit")}</summary>
          <div className="rt-disclosure-body">
            <SearchForm
              ctx={ctx}
              locations={locations}
              values={values}
              compact
            />
          </div>
        </details>

        {!valid && attempted ? (
          <Alert tone="error">{t("search.invalid")}</Alert>
        ) : null}
        {failed ? <Alert tone="error">{t("error.body")}</Alert> : null}
        {error ? (
          <Alert tone="error">
            {error === "CAPACITY_UNAVAILABLE"
              ? t("book.capacity")
              : t("book.failed")}
          </Alert>
        ) : null}

        {availability ? (
          <section aria-labelledby="results-title" className="rt-results">
            <h2 id="results-title" className="rt-sr-only">
              {t("results.title")}
            </h2>
            {items.length === 0 || !items.some((i) => i.available) ? (
              <Alert>{t("results.empty")}</Alert>
            ) : null}
            <ul className="rt-vehicle-grid">
              {items.map((item) => (
                <li key={item.subgroup.uuid}>
                  <ResultCard
                    ctx={ctx}
                    item={item}
                    subgroup={byUuid.get(item.subgroup.uuid)}
                    hidden={hidden}
                  />
                </li>
              ))}
            </ul>
          </section>
        ) : null}
      </PageSection>
    </>
  );
}
