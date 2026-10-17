import { BadgeCheck, Car, Gauge, Users, Wallet } from "lucide-react";

import { cn } from "@/lib/cn";
import { money } from "@/lib/format";
import { mediaUrl, type Ctx } from "@/lib/server";
import type { Subgroup, SubgroupRef } from "@/lib/types";

/**
 * A vehicle group card: segment, "model or similar", the group's first
 * product image (API media) or a neutral placeholder, and the rental
 * conditions the public API publishes. Prices and actions go in `children`.
 */
export function VehicleCard({
  ctx,
  subgroup,
  fallback,
  heading = "h3",
  muted = false,
  children,
}: {
  ctx: Ctx;
  subgroup?: Subgroup;
  /** The availability item's subgroup when the catalog entry is missing. */
  fallback?: SubgroupRef;
  heading?: "h2" | "h3";
  muted?: boolean;
  children?: React.ReactNode;
}) {
  const { t, locale } = ctx;
  const ref = subgroup ?? fallback;
  const model = subgroup?.representative_model || ref?.title || "";
  const image = mediaUrl(subgroup?.images?.[0]);
  const cond = subgroup?.conditions;
  const H = heading;
  const specs = cond
    ? [
        {
          icon: Users,
          label: t("vehicle.age"),
          value: cond.min_driver_age ? `${cond.min_driver_age}+` : "—",
        },
        {
          icon: BadgeCheck,
          label: t("vehicle.licence"),
          value: cond.min_license_years
            ? t("vehicle.years", { years: cond.min_license_years })
            : "—",
        },
        {
          icon: Gauge,
          label: t("vehicle.km"),
          value: cond.included_km_per_day
            ? t("vehicle.km_day", { km: cond.included_km_per_day })
            : t("vehicle.km_unlimited"),
        },
        {
          icon: Wallet,
          label: t("vehicle.deposit"),
          value:
            Number(cond.deposit_amount) > 0
              ? money(cond.deposit_amount, "TRY", locale)
              : "—",
        },
      ]
    : [];
  const sub = [
    ref?.title && ref.title !== model ? ref.title : "",
    subgroup?.segment ?? "",
  ].filter(Boolean);
  return (
    <article className={cn("rt-vehicle", muted && "rt-vehicle--muted")}>
      <div className="rt-vehicle-top">
        <span className="rt-vehicle-category">{sub.join(" · ")}</span>
      </div>
      <H className="rt-vehicle-name">{model}</H>
      <p className="rt-vehicle-similar">{t("vehicle.similar")}</p>
      <div className="rt-vehicle-image">
        {image ? (
          // Product images come from the API through /media (same origin).
          // eslint-disable-next-line @next/next/no-img-element
          <img src={image} alt={model} loading="lazy" />
        ) : (
          <span className="rt-vehicle-placeholder" aria-hidden>
            <Car size={56} strokeWidth={1.2} />
          </span>
        )}
      </div>
      {specs.length ? (
        <dl className="rt-vehicle-specs">
          {specs.map(({ icon: Icon, label, value }) => (
            <div key={label}>
              <dt>
                <Icon aria-hidden size={16} />
                <span>{label}</span>
              </dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {children ? <div className="rt-vehicle-foot">{children}</div> : null}
    </article>
  );
}
