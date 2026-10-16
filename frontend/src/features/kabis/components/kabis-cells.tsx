"use client";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  KABIS_EVENT_LABEL_KEYS,
  KABIS_FIELD_LABEL_KEYS,
  KABIS_FIELD_SCOPE_KEYS,
  KABIS_HOLD_LABEL_KEYS,
  KABIS_OFFICIAL,
  KABIS_STATUS_LABEL_KEYS,
  KABIS_STATUS_TONES,
} from "@/features/kabis/constants";
import type { KabisEvent, KabisStatus } from "@/features/kabis/types";
import { useLocale } from "@/providers/locale-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

export function kabisStatusText(t: T, status: KabisStatus) {
  return t(KABIS_STATUS_LABEL_KEYS[status] ?? status);
}

/** The job state, worded so a local record never reads as reported. */
export function KabisStatusChip({ status }: { status: KabisStatus }) {
  const { t } = useLocale();
  return (
    <span className="whitespace-nowrap">
      <StatusChip
        label={kabisStatusText(t, status)}
        tone={KABIS_STATUS_TONES[status] ?? "default"}
      />
    </span>
  );
}

/** "Resmi bildirim" / "Resmi bildirim değil" next to the status. */
export function KabisOfficialBadge({ status }: { status: KabisStatus }) {
  const { t } = useLocale();
  const official = KABIS_OFFICIAL.includes(status);
  return (
    <Badge
      variant={official ? "success" : "outline"}
      className="text-xs font-normal whitespace-nowrap"
    >
      {official ? t("kabis.official") : t("kabis.not_official")}
    </Badge>
  );
}

export function kabisEventText(t: T, event: KabisEvent, sequence?: number) {
  const label = t(KABIS_EVENT_LABEL_KEYS[event] ?? event);
  return event === "correction" && sequence ? `${label} #${sequence}` : label;
}

export function holdText(t: T, reason: string) {
  return reason ? t(KABIS_HOLD_LABEL_KEYS[reason] ?? reason) : "";
}

/** "drivers.0.identity_number" → "1. sürücü: T.C. / pasaport no". */
export function missingFieldText(t: T, path: string) {
  const parts = path.split(".");
  const last = parts[parts.length - 1] ?? path;
  const field = t(KABIS_FIELD_LABEL_KEYS[last] ?? last);
  if (parts[0] === "drivers" && parts.length === 3) {
    return `${t(KABIS_FIELD_SCOPE_KEYS.drivers, { n: Number(parts[1]) + 1 })}: ${field}`;
  }
  if (parts.length === 2 && KABIS_FIELD_SCOPE_KEYS[parts[0]]) {
    return `${t(KABIS_FIELD_SCOPE_KEYS[parts[0]])}: ${field}`;
  }
  return field;
}
