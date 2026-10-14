"use client";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { STATUS_LABEL_KEYS, STATUS_TONES } from "@/features/vehicles/constants";
import type { Vehicle, VehicleStatus } from "@/features/vehicles/types";
import { currency } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Operational status chip. */
export function VehicleStatusChip({ status }: { status: VehicleStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONES[status]}
    />
  );
}

/** Plate in the Turkish plate style (monospace, bordered). */
export function PlateBadge({ plate }: { plate: string }) {
  return (
    <span className="border-foreground/30 bg-background inline-flex rounded border px-1.5 py-0.5 font-mono text-xs font-semibold tracking-wide">
      {plate}
    </span>
  );
}

/** Kasko value (TRY) with its TSB period, or a dash. */
export function KaskoValue({
  value,
  period,
  showPeriod = false,
}: {
  value: string | null;
  period: string | null;
  showPeriod?: boolean;
}) {
  const { t, locale } = useLocale();
  if (!value) return <span className="text-muted-foreground">—</span>;
  const amount = currency(Number(value), "TRY", locale);
  return (
    <span
      className="tabular-nums"
      title={period ? t("vehicles.kasko.period", { period }) : undefined}
    >
      {amount}
      {showPeriod && period ? (
        <span className="text-muted-foreground ms-1 text-xs">({period})</span>
      ) : null}
    </span>
  );
}

/** Rental group chips of a vehicle. */
export function GroupChips({ vehicle }: { vehicle: Vehicle }) {
  if (!vehicle.rental_groups.length) {
    return <span className="text-muted-foreground">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {vehicle.rental_groups.map((group) => (
        <Badge key={group.uuid} variant="outline" title={group.name}>
          {group.code}
        </Badge>
      ))}
    </div>
  );
}

/** Kilometres with the locale's grouping. */
export function Km({ value }: { value: number }) {
  const { locale } = useLocale();
  return (
    <span className="tabular-nums">
      {new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US").format(value)}{" "}
      km
    </span>
  );
}
