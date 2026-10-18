import {
  BadgeCheck,
  Gauge,
  Users,
  Wallet,
  type LucideIcon,
} from "lucide-react";

import { money } from "@/lib/format";
import type { Ctx } from "@/lib/server";
import type { Subgroup } from "@/lib/types";

export type VehicleFact = { icon: LucideIcon; label: string; value: string };

/**
 * A vehicle group's published rental conditions as four short facts
 * (minimum age, licence years, included km, deposit). The classic and
 * Aurum cards show these where the studio showed demo fuel / gearbox /
 * seat / luggage values, which the public catalog does not publish.
 */
export function vehicleFacts(ctx: Ctx, s: Subgroup): VehicleFact[] {
  const { t, locale } = ctx;
  const c = s.conditions;
  if (!c) return [];
  return [
    {
      icon: Users,
      label: t("vehicle.age"),
      value: c.min_driver_age ? `${c.min_driver_age}+` : "—",
    },
    {
      icon: BadgeCheck,
      label: t("vehicle.licence"),
      value: c.min_license_years
        ? t("vehicle.years", { years: c.min_license_years })
        : "—",
    },
    {
      icon: Gauge,
      label: t("vehicle.km"),
      value: c.included_km_per_day
        ? t("vehicle.km_day", { km: c.included_km_per_day })
        : t("vehicle.km_unlimited"),
    },
    {
      icon: Wallet,
      label: t("vehicle.deposit"),
      value:
        Number(c.deposit_amount) > 0
          ? money(c.deposit_amount, "TRY", locale)
          : "—",
    },
  ];
}

/** Distinct, non-empty segments of the groups, in catalog order. */
export function segmentsOf(subgroups: Subgroup[]): string[] {
  return [...new Set(subgroups.map((s) => s.segment).filter(Boolean))];
}
