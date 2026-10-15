"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  CHARGE_STATUS_LABEL_KEYS,
  CHARGE_STATUS_TONES,
  RENTAL_STATUS_LABEL_KEYS,
  RENTAL_STATUS_TONES,
  TASK_STATUS_LABEL_KEYS,
  TASK_STATUS_TONES,
} from "@/features/rentals/constants";
import type {
  ChargeStatus,
  OperationTaskStatus,
  RentalStatus,
} from "@/features/rentals/types";
import { useLocale } from "@/providers/locale-provider";

export function RentalStatusChip({ status }: { status: RentalStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(RENTAL_STATUS_LABEL_KEYS[status])}
      tone={RENTAL_STATUS_TONES[status]}
    />
  );
}

export function ChargeStatusChip({ status }: { status: ChargeStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(CHARGE_STATUS_LABEL_KEYS[status])}
      tone={CHARGE_STATUS_TONES[status]}
    />
  );
}

export function OperationTaskStatusChip({
  status,
}: {
  status: OperationTaskStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(TASK_STATUS_LABEL_KEYS[status])}
      tone={TASK_STATUS_TONES[status]}
    />
  );
}

type PlaceLike = {
  branch: { code?: string; name: string } | null;
  point: { name: string } | null;
} | null;

/** "CODE · Branch (point)" of a rental place, or a dash. */
export function placeText(place: PlaceLike): string {
  if (!place?.branch) return "—";
  const branch = place.branch.code
    ? `${place.branch.code} · ${place.branch.name}`
    : place.branch.name;
  return place.point ? `${branch} (${place.point.name})` : branch;
}
