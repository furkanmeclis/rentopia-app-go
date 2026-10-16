"use client";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  DEPOSIT_METHOD_LABEL_KEYS,
  DEPOSIT_STATUS_LABEL_KEYS,
  DEPOSIT_STATUS_TONES,
} from "@/features/deposits/constants";
import type { DepositMethod, DepositStatus } from "@/features/deposits/types";
import { useLocale } from "@/providers/locale-provider";

export function DepositStatusChip({ status }: { status: DepositStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(DEPOSIT_STATUS_LABEL_KEYS[status])}
      tone={DEPOSIT_STATUS_TONES[status]}
    />
  );
}

export function DepositMethodBadge({ method }: { method: DepositMethod }) {
  const { t } = useLocale();
  return (
    <Badge variant="outline" className="whitespace-nowrap">
      {t(DEPOSIT_METHOD_LABEL_KEYS[method])}
    </Badge>
  );
}
