"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  PAYMENT_LABEL_KEYS,
  PAYMENT_TONES,
  QUOTE_STATUS_LABEL_KEYS,
  QUOTE_STATUS_TONES,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
  TASK_STATUS_LABEL_KEYS,
  TASK_STATUS_TONES,
} from "@/features/reservations/constants";
import type {
  PaymentStatus,
  QuoteStatus,
  ReservationStatus,
  TaskStatus,
} from "@/features/reservations/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function ReservationStatusChip({
  status,
}: {
  status: ReservationStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONES[status]}
    />
  );
}

export function PaymentStatusChip({ status }: { status: PaymentStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(PAYMENT_LABEL_KEYS[status])}
      tone={PAYMENT_TONES[status]}
    />
  );
}

export function QuoteStatusChip({
  status,
  expired,
}: {
  status: QuoteStatus;
  expired: boolean;
}) {
  const { t } = useLocale();
  if (status === "open" && expired) {
    return <StatusChip label={t("reservations.quotes.expired")} />;
  }
  return (
    <StatusChip
      label={t(QUOTE_STATUS_LABEL_KEYS[status])}
      tone={QUOTE_STATUS_TONES[status]}
    />
  );
}

export function TaskStatusChip({ status }: { status: TaskStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(TASK_STATUS_LABEL_KEYS[status])}
      tone={TASK_STATUS_TONES[status]}
    />
  );
}

/** Reference in a monospace badge ("IDS-2026-000123"). */
export function ReferenceText({ reference }: { reference: string }) {
  return (
    <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold whitespace-nowrap">
      {reference}
    </code>
  );
}

/** "12 Eki 2026 10:00 → 15 Eki 2026 10:00". */
export function RangeText({
  startsAt,
  endsAt,
}: {
  startsAt: string;
  endsAt: string;
}) {
  const { locale } = useLocale();
  return (
    <span className="text-sm whitespace-nowrap tabular-nums">
      {datetime(startsAt, "d MMM yyyy HH:mm", locale)}
      <span className="text-muted-foreground"> → </span>
      {datetime(endsAt, "d MMM yyyy HH:mm", locale)}
    </span>
  );
}

/** Person of the API (`name surname`, falling back to the e-mail). */
export function actorName(
  actor: { name: string; surname: string; email: string } | null | undefined,
): string {
  if (!actor) return "—";
  return `${actor.name} ${actor.surname}`.trim() || actor.email;
}
