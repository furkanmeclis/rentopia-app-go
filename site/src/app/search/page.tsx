import type { Metadata } from "next";

import { selectVehicle } from "@/app/actions";
import { SearchForm, type SearchValues } from "@/components/site/search-form";
import { Container, PageTitle } from "@/components/site/text";
import { Button } from "@/components/ui/button";
import { Alert, Badge, Card } from "@/components/ui/card";
import { ApiError } from "@/lib/api";
import { loadLocations, splitPlace } from "@/lib/catalog";
import { cn } from "@/lib/cn";
import { dateTime, money, toRFC3339 } from "@/lib/format";
import { bookingApi, getCtx, mediaUrl, type Ctx } from "@/lib/server";
import type { Availability, AvailabilityItem, Subgroup } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("results.title"), robots: { index: false } };
}

type Params = Record<string, string | string[] | undefined>;
const one = (v: string | string[] | undefined) =>
  (Array.isArray(v) ? v[0] : v) ?? "";

function VehicleCard({
  ctx,
  item,
  subgroup,
  hidden,
  variant,
}: {
  ctx: Ctx;
  item: AvailabilityItem;
  subgroup?: Subgroup;
  hidden: Record<string, string>;
  variant: string;
}) {
  const { t, locale } = ctx;
  const image = mediaUrl(subgroup?.images?.[0]);
  const model = subgroup?.representative_model || item.subgroup.title;
  const price = item.price;
  const cond = subgroup?.conditions;
  const list = variant === "list";
  return (
    <Card
      className={cn(
        "w-full overflow-hidden p-0",
        list && "sm:flex-row",
        !item.available && "opacity-60",
      )}
    >
      <div
        className={cn(
          "bg-muted relative aspect-[16/10]",
          list && "sm:aspect-auto sm:w-72 sm:shrink-0",
        )}
      >
        {image ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={image}
            alt={model}
            loading="lazy"
            className="absolute inset-0 size-full object-cover"
          />
        ) : null}
      </div>
      <div
        className={cn(
          "flex flex-1 flex-col gap-3 p-5",
          variant === "compact" && "gap-2 p-4",
        )}
      >
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">
            {t("results.or_similar", { model })}
          </h2>
          <p className="text-muted-foreground text-sm">
            {[
              item.subgroup.title !== model ? item.subgroup.title : "",
              subgroup?.segment,
            ]
              .filter(Boolean)
              .join(" · ")}
          </p>
        </div>
        {cond && variant !== "compact" ? (
          <ul className="text-muted-foreground flex flex-wrap gap-2 text-xs">
            {cond.min_driver_age ? (
              <li>
                <Badge>
                  {t("results.min_age", { age: cond.min_driver_age })}
                </Badge>
              </li>
            ) : null}
            {cond.min_license_years ? (
              <li>
                <Badge>
                  {t("results.licence_years", {
                    years: cond.min_license_years,
                  })}
                </Badge>
              </li>
            ) : null}
            <li>
              <Badge>
                {cond.included_km_per_day
                  ? t("results.km", { km: cond.included_km_per_day })
                  : t("results.km_unlimited")}
              </Badge>
            </li>
            {Number(cond.deposit_amount) > 0 ? (
              <li>
                <Badge>
                  {t("results.deposit", {
                    amount: money(cond.deposit_amount, "TRY", locale),
                  })}
                </Badge>
              </li>
            ) : null}
          </ul>
        ) : null}
        <div className="mt-auto flex flex-wrap items-end justify-between gap-3">
          {item.available && price ? (
            <div>
              <p className="text-2xl font-bold">
                {money(price.total, price.currency, locale)}
              </p>
              <p className="text-muted-foreground text-sm">
                {t("results.days", { days: price.billing_days })} ·{" "}
                {t("results.per_day", {
                  amount: money(price.daily_average, price.currency, locale),
                })}
              </p>
              {Number(price.prepayment) > 0 ? (
                <p className="text-muted-foreground text-xs">
                  {t("results.prepayment", {
                    amount: money(price.prepayment, price.currency, locale),
                  })}
                </p>
              ) : null}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("results.unavailable")}
            </p>
          )}
          {item.available && price ? (
            <form action={selectVehicle}>
              {Object.entries(hidden).map(([k, v]) => (
                <input key={k} type="hidden" name={k} value={v} />
              ))}
              <input
                type="hidden"
                name="subgroup_uuid"
                value={item.subgroup.uuid}
              />
              {item.units_left > 0 && item.units_left <= 3 ? (
                <p className="mb-1 text-right text-xs font-medium text-red-700">
                  {t("results.units_left", { count: item.units_left })}
                </p>
              ) : null}
              <Button
                type="submit"
                aria-label={`${t("results.select")}: ${model}`}
              >
                {t("results.select")}
              </Button>
            </form>
          ) : null}
        </div>
      </div>
    </Card>
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
      <Container className="py-10">
        <Alert>{t("search.booking_off")}</Alert>
      </Container>
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
  const items = [...(availability?.items ?? [])].sort((a, b) => {
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
  const variant = site.config.layout.cards;
  const error = one(sp.error);

  return (
    <Container className="space-y-8 py-8">
      <details
        className="border-border rounded-[var(--radius)] border p-4"
        open={!valid}
      >
        <summary className="cursor-pointer font-medium">
          {t("search.edit")}
        </summary>
        <div className="pt-4">
          <SearchForm ctx={ctx} locations={locations} values={values} compact />
        </div>
      </details>

      {!valid ? <Alert tone="error">{t("search.invalid")}</Alert> : null}
      {failed ? <Alert tone="error">{t("error.body")}</Alert> : null}
      {error ? (
        <Alert tone="error">
          {error === "CAPACITY_UNAVAILABLE"
            ? t("book.capacity")
            : t("book.failed")}
        </Alert>
      ) : null}

      {availability ? (
        <section aria-labelledby="results-title" className="space-y-4">
          <div className="space-y-1">
            <PageTitle>
              <span id="results-title">{t("results.title")}</span>
            </PageTitle>
            <p className="text-muted-foreground">
              {t("results.summary", {
                pickup: availability.request.pickup_branch.name,
                return: availability.request.return_branch.name,
                start: dateTime(availability.request.starts_at, locale),
                end: dateTime(availability.request.ends_at, locale),
              })}
            </p>
          </div>
          {items.length === 0 || !items.some((i) => i.available) ? (
            <Alert>{t("results.empty")}</Alert>
          ) : null}
          <ul
            className={cn(
              "grid gap-4",
              variant === "list" && "grid-cols-1",
              variant === "compact" && "sm:grid-cols-2 lg:grid-cols-4",
              variant !== "list" &&
                variant !== "compact" &&
                "sm:grid-cols-2 lg:grid-cols-3",
            )}
          >
            {items.map((item) => (
              <li key={item.subgroup.uuid} className="flex">
                <VehicleCard
                  ctx={ctx}
                  item={item}
                  subgroup={byUuid.get(item.subgroup.uuid)}
                  hidden={hidden}
                  variant={variant}
                />
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </Container>
  );
}
