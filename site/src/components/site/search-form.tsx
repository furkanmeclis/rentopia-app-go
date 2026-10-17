import {
  ArrowRight,
  CalendarDays,
  ChevronDown,
  Clock3,
  MapPin,
} from "lucide-react";

import { cn } from "@/lib/cn";
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

function TimeSelect({
  id,
  name,
  label,
  value,
}: {
  id: string;
  name: string;
  label: string;
  value: string;
}) {
  return (
    <div className="rt-search-control rt-search-time">
      <label htmlFor={id} className="rt-sr-only">
        {label}
      </label>
      <Clock3 aria-hidden size={16} />
      <select id={id} name={name} defaultValue={value}>
        {TIME_SLOTS.map((s) => (
          <option key={s} value={s}>
            {s}
          </option>
        ))}
      </select>
    </div>
  );
}

/**
 * The availability search. A GET form to /search with native controls: it
 * works without JavaScript, phones show their own pickers and the result
 * URL can be shared. `title` / `tagline` draw it as the hero's booking card.
 */
export function SearchForm({
  ctx,
  locations,
  values = {},
  compact = false,
  title,
  tagline,
  id,
}: {
  ctx: Ctx;
  locations: Location[];
  values?: SearchValues;
  compact?: boolean;
  title?: string;
  tagline?: string;
  /** Anchor of the form (e.g. "search" on the home page). */
  id?: string;
}) {
  const { t } = ctx;
  const today = istanbulDate(0);
  const labelId = id ? `${id}-title` : undefined;
  return (
    <form
      id={id}
      action={embedPath("/search", ctx.embed).split("?")[0]}
      method="get"
      aria-label={title ? undefined : t("search.title")}
      aria-labelledby={title ? labelId : undefined}
      className={cn("rt-search", compact && "rt-search--compact")}
    >
      {ctx.embed
        ? Object.entries(ctx.embed.fields).map(([name, value]) => (
            <input key={name} type="hidden" name={name} value={value} />
          ))
        : null}
      {title ? (
        <div className="rt-search-head">
          <h2 id={labelId}>{title}</h2>
          {tagline ? <p>{tagline}</p> : null}
        </div>
      ) : null}
      <div className="rt-search-places">
        <div className="rt-search-field">
          <label htmlFor="pickup" className="rt-search-label">
            {t("search.pickup")}
          </label>
          <div className="rt-search-control">
            <MapPin aria-hidden size={18} />
            <select
              id="pickup"
              name="pickup"
              required
              defaultValue={values.pickup ?? ""}
            >
              <option value="" disabled>
                {t("search.choose")}
              </option>
              <LocationOptions locations={locations} />
            </select>
            <ChevronDown aria-hidden size={15} />
          </div>
        </div>
        <div className="rt-search-field">
          <label htmlFor="return" className="rt-search-label">
            {t("search.return")}
          </label>
          <div className="rt-search-control">
            <MapPin aria-hidden size={18} />
            <select
              id="return"
              name="return"
              defaultValue={values.return ?? ""}
            >
              <option value="">{t("search.same_return")}</option>
              <LocationOptions locations={locations} />
            </select>
            <ChevronDown aria-hidden size={15} />
          </div>
        </div>
      </div>
      <div className="rt-search-dates">
        <div className="rt-search-field">
          <label htmlFor="pickup_date" className="rt-search-label">
            {t("search.pickup_date")}
          </label>
          <div className="rt-search-datetime">
            <div className="rt-search-control rt-search-date">
              <CalendarDays aria-hidden size={16} />
              <input
                id="pickup_date"
                type="date"
                name="pickup_date"
                required
                min={today}
                defaultValue={values.pickup_date || istanbulDate(1)}
              />
            </div>
            <TimeSelect
              id="pickup_time"
              name="pickup_time"
              label={t("search.pickup_time")}
              value={values.pickup_time || "10:00"}
            />
          </div>
        </div>
        <ArrowRight aria-hidden className="rt-search-arrow" size={16} />
        <div className="rt-search-field">
          <label htmlFor="return_date" className="rt-search-label">
            {t("search.return_date")}
          </label>
          <div className="rt-search-datetime">
            <div className="rt-search-control rt-search-date">
              <CalendarDays aria-hidden size={16} />
              <input
                id="return_date"
                type="date"
                name="return_date"
                required
                min={today}
                defaultValue={values.return_date || istanbulDate(4)}
              />
            </div>
            <TimeSelect
              id="return_time"
              name="return_time"
              label={t("search.return_time")}
              value={values.return_time || "10:00"}
            />
          </div>
        </div>
      </div>
      <button
        type="submit"
        className="rt-button rt-button--primary rt-search-submit"
      >
        {t("search.submit")}
        <ArrowRight aria-hidden size={18} />
      </button>
    </form>
  );
}
