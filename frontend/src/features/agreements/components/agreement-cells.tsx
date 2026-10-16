"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  AGREEMENT_STATUS_LABEL_KEYS,
  AGREEMENT_STATUS_TONES,
  PERIOD_STATUS_LABEL_KEYS,
  PERIOD_STATUS_TONES,
  VALIDITY_LABEL_KEYS,
  VALIDITY_TONES,
} from "@/features/agreements/constants";
import type {
  AgreementStatus,
  AgreementValidity,
  PeriodStatus,
} from "@/features/agreements/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function AgreementStatusChip({ status }: { status: AgreementStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(AGREEMENT_STATUS_LABEL_KEYS[status])}
      tone={AGREEMENT_STATUS_TONES[status]}
    />
  );
}

export function ValidityChip({ validity }: { validity: AgreementValidity }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(VALIDITY_LABEL_KEYS[validity])}
      tone={VALIDITY_TONES[validity]}
    />
  );
}

export function PeriodStatusChip({ status }: { status: PeriodStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(PERIOD_STATUS_LABEL_KEYS[status])}
      tone={PERIOD_STATUS_TONES[status]}
    />
  );
}

/** "01.02.2026 – 31.01.2027" or "01.02.2026 – süresiz" (both inclusive). */
export function ValidityRange({
  from,
  to,
}: {
  from: string;
  to: string | null | undefined;
}) {
  const { t, locale } = useLocale();
  return (
    <span className="tabular-nums">
      {date(from, undefined, locale)} –{" "}
      {to ? date(to, undefined, locale) : t("agreements.open_ended")}
    </span>
  );
}
