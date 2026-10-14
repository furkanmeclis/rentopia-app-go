"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  DOCUMENT_STATUS_LABEL_KEYS,
  DOCUMENT_STATUS_TONES,
  MAINTENANCE_STATUS_LABEL_KEYS,
  MAINTENANCE_STATUS_TONES,
  VEHICLE_MAINTENANCE_LABEL_KEYS,
} from "@/features/vehicle-records/constants";
import type {
  MaintenanceStatus,
  VehicleDocumentStatus,
} from "@/features/vehicle-records/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Document validity chip: valid / expiring / expired / no expiry. */
export function DocumentStatusChip({
  status,
}: {
  status: VehicleDocumentStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(DOCUMENT_STATUS_LABEL_KEYS[status])}
      tone={DOCUMENT_STATUS_TONES[status]}
    />
  );
}

/** Maintenance item chip: done / scheduled / due / overdue. */
export function MaintenanceStatusChip({
  status,
}: {
  status: MaintenanceStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(MAINTENANCE_STATUS_LABEL_KEYS[status])}
      tone={MAINTENANCE_STATUS_TONES[status]}
    />
  );
}

/** Vehicle-level maintenance state (list column). */
export function VehicleMaintenanceChip({
  status,
}: {
  status: "none" | "scheduled" | "due" | "overdue";
}) {
  const { t } = useLocale();
  if (status === "none") {
    return <span className="text-muted-foreground">—</span>;
  }
  return (
    <StatusChip
      label={t(VEHICLE_MAINTENANCE_LABEL_KEYS[status])}
      tone={MAINTENANCE_STATUS_TONES[status]}
    />
  );
}

/** Days left (or overdue) until a date, as a short muted hint. */
export function DaysLeft({ days }: { days: number | null | undefined }) {
  const { t } = useLocale();
  if (days == null) return null;
  const text =
    days < 0
      ? t("vehicle_records.days_overdue", { count: -days })
      : days === 0
        ? t("vehicle_records.days_today")
        : t("vehicle_records.days_left", { count: days });
  return (
    <span
      className={
        days < 0 ? "text-destructive text-xs" : "text-muted-foreground text-xs"
      }
    >
      {text}
    </span>
  );
}

/**
 * Expiry date with a warning tone: red when passed, amber inside the
 * reminder horizon (`horizonDays`), plain otherwise; a dash without a date.
 */
export function ExpiryDate({
  value,
  horizonDays = 30,
}: {
  value: string | null | undefined;
  horizonDays?: number;
}) {
  const { locale } = useLocale();
  if (!value) return <span className="text-muted-foreground">—</span>;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const day = new Date(`${value}T00:00:00`);
  const days = Math.round((day.getTime() - today.getTime()) / 86_400_000);
  const tone =
    days < 0
      ? "text-destructive font-medium"
      : days <= horizonDays
        ? "text-amber-600 dark:text-amber-400 font-medium"
        : "";
  return (
    <span className={`tabular-nums ${tone}`}>
      {date(value, undefined, locale)}
    </span>
  );
}

/** Kilometres with the locale's grouping, or a dash. */
export function KmValue({ value }: { value: number | null | undefined }) {
  const { locale } = useLocale();
  if (value == null) return <span className="text-muted-foreground">—</span>;
  return (
    <span className="tabular-nums">
      {new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US").format(value)}{" "}
      km
    </span>
  );
}
