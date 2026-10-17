import { Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/form";
import { embedPath } from "@/lib/embed";
import { TIME_SLOTS, istanbulDate } from "@/lib/format";
import type { Ctx } from "@/lib/server";
import type { Location } from "@/lib/types";

export type SearchValues = {
  pickup?: string;
  return?: string;
  pickup_date?: string;
  pickup_time?: string;
  return_date?: string;
  return_time?: string;
};

/** "branch" or "branch:point" option values of the locations. */
function LocationOptions({ locations }: { locations: Location[] }) {
  return (
    <>
      {locations.map((l) => (
        <optgroup
          key={l.uuid}
          label={l.city ? `${l.name} · ${l.city}` : l.name}
        >
          <option value={l.uuid}>{l.name}</option>
          {l.points
            .filter((p) => !p.delivery)
            .map((p) => (
              <option key={p.uuid} value={`${l.uuid}:${p.uuid}`}>
                {l.name} — {p.name}
              </option>
            ))}
        </optgroup>
      ))}
    </>
  );
}

/**
 * The availability search. A GET form to /search: it works without
 * JavaScript and the result URL can be shared.
 */
export function SearchForm({
  ctx,
  locations,
  values = {},
  compact = false,
}: {
  ctx: Ctx;
  locations: Location[];
  values?: SearchValues;
  compact?: boolean;
}) {
  const { t } = ctx;
  const today = istanbulDate(0);
  return (
    <form
      action={embedPath("/search", ctx.embed).split("?")[0]}
      method="get"
      aria-label={t("search.title")}
      className={
        compact
          ? "grid gap-3 sm:grid-cols-2 lg:grid-cols-7 lg:items-end"
          : "grid gap-4 sm:grid-cols-2 lg:grid-cols-6 lg:items-end"
      }
    >
      {ctx.embed
        ? Object.entries(ctx.embed.fields).map(([name, value]) => (
            <input key={name} type="hidden" name={name} value={value} />
          ))
        : null}
      <Field
        id="pickup"
        label={t("search.pickup")}
        className="sm:col-span-2 lg:col-span-2"
      >
        <Select name="pickup" required defaultValue={values.pickup ?? ""}>
          <option value="" disabled>
            {t("search.choose")}
          </option>
          <LocationOptions locations={locations} />
        </Select>
      </Field>
      <Field
        id="return"
        label={t("search.return")}
        className="sm:col-span-2 lg:col-span-2"
      >
        <Select name="return" defaultValue={values.return ?? ""}>
          <option value="">{t("search.same_return")}</option>
          <LocationOptions locations={locations} />
        </Select>
      </Field>
      <Field id="pickup_date" label={t("search.pickup_date")}>
        <Input
          type="date"
          name="pickup_date"
          required
          min={today}
          defaultValue={values.pickup_date ?? istanbulDate(1)}
        />
      </Field>
      <Field id="pickup_time" label={t("search.pickup_time")}>
        <Select name="pickup_time" defaultValue={values.pickup_time ?? "10:00"}>
          {TIME_SLOTS.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </Select>
      </Field>
      <Field id="return_date" label={t("search.return_date")}>
        <Input
          type="date"
          name="return_date"
          required
          min={today}
          defaultValue={values.return_date ?? istanbulDate(4)}
        />
      </Field>
      <Field id="return_time" label={t("search.return_time")}>
        <Select name="return_time" defaultValue={values.return_time ?? "10:00"}>
          {TIME_SLOTS.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </Select>
      </Field>
      <Button
        type="submit"
        size="lg"
        className={
          compact
            ? "w-full lg:col-span-1"
            : "w-full sm:col-span-2 lg:col-span-2"
        }
      >
        <Search aria-hidden />
        {t("search.submit")}
      </Button>
    </form>
  );
}
