"use client";

import { CarFront } from "lucide-react";

import { useLocale } from "@/providers/locale-provider";

/**
 * Eligible vehicles placeholder: vehicle records arrive with the vehicles
 * module; the count stays 0 until then (capacity is computed, never stored).
 */
export function VehiclesPlaceholder() {
  const { t } = useLocale();
  return (
    <div className="border-border flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center">
      <CarFront className="text-muted-foreground size-8" aria-hidden />
      <p className="font-display text-3xl font-semibold tabular-nums">0</p>
      <h3 className="text-sm font-medium">
        {t("fleet.vehicles_placeholder.title")}
      </h3>
      <p className="text-muted-foreground max-w-sm text-sm">
        {t("fleet.vehicles_placeholder.description")}
      </p>
    </div>
  );
}
