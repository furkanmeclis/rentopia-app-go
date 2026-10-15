"use client";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import type { AppLocale } from "@/config/i18n";
import {
  REVIEW_REASON_LABEL_KEYS,
  TOLL_STATUS_LABEL_KEYS,
  TOLL_STATUS_TONES,
} from "@/features/tolls/constants";
import type {
  TollPass,
  TollReviewReason,
  TollStatus,
} from "@/features/tolls/types";
import { currency } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function TollStatusChip({ status }: { status: TollStatus }) {
  const { t } = useLocale();
  return (
    <span className="whitespace-nowrap">
      <StatusChip
        label={t(TOLL_STATUS_LABEL_KEYS[status])}
        tone={TOLL_STATUS_TONES[status]}
      />
    </span>
  );
}

export function reviewReasonText(
  t: (key: string) => string,
  reason: TollReviewReason,
): string {
  if (!reason) return "";
  return t(REVIEW_REASON_LABEL_KEYS[reason]);
}

/** The reason a pass waits for a person (never shown as "no debt"). */
export function ReviewReasonBadge({ reason }: { reason: TollReviewReason }) {
  const { t } = useLocale();
  if (!reason) return null;
  return (
    <Badge variant="outline" className="text-xs font-normal">
      {reviewReasonText(t, reason)}
    </Badge>
  );
}

export function tollMoney(amount: string, code: string, locale: AppLocale) {
  const n = Number(amount);
  return Number.isFinite(n) ? currency(n, code || "TRY", locale) : amount;
}

/** Station, or "entry → exit" for a motorway route. */
export function stationText(pass: Pick<TollPass, "station" | "exit_station">) {
  return pass.exit_station
    ? `${pass.station} → ${pass.exit_station}`
    : pass.station;
}
