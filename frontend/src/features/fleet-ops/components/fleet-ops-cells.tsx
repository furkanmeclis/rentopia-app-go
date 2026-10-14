"use client";

import { ArrowRight } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  BLOCK_KIND_LABEL_KEYS,
  BLOCK_STATE_LABEL_KEYS,
  BLOCK_STATE_TONES,
  TRANSFER_STATUS_LABEL_KEYS,
  TRANSFER_STATUS_TONES,
} from "@/features/fleet-ops/constants";
import type {
  BlockKind,
  BlockState,
  TransferStatus,
} from "@/features/fleet-ops/types";
import { currency, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function BlockKindBadge({ kind }: { kind: BlockKind }) {
  const { t } = useLocale();
  return <Badge variant="outline">{t(BLOCK_KIND_LABEL_KEYS[kind])}</Badge>;
}

export function BlockStateChip({ state }: { state: BlockState }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(BLOCK_STATE_LABEL_KEYS[state])}
      tone={BLOCK_STATE_TONES[state]}
    />
  );
}

export function TransferStatusChip({ status }: { status: TransferStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(TRANSFER_STATUS_LABEL_KEYS[status])}
      tone={TRANSFER_STATUS_TONES[status]}
    />
  );
}

/** "MRK → IST" with the branch names as a tooltip-free subtitle. */
export function RouteCell({
  from,
  to,
}: {
  from: { code: string; name: string };
  to: { code: string; name: string };
}) {
  return (
    <div className="min-w-0">
      <p className="flex items-center gap-1 font-medium">
        <span>{from.code}</span>
        <ArrowRight className="text-muted-foreground size-3.5" />
        <span>{to.code}</span>
      </p>
      <p className="text-muted-foreground truncate text-xs">
        {from.name} → {to.name}
      </p>
    </div>
  );
}

/** TRY amount (decimal string) or a dash. */
export function MoneyCell({ value }: { value: string | null | undefined }) {
  const { locale } = useLocale();
  if (value == null || value === "") return <span>—</span>;
  return (
    <span className="tabular-nums">
      {currency(Number(value), "TRY", locale)}
    </span>
  );
}

/** "14.10.2026 09:00 – 15.10.2026 18:00"; open end shows "süresiz". */
export function RangeCell({
  start,
  end,
}: {
  start: string;
  end: string | null | undefined;
}) {
  const { t, locale } = useLocale();
  return (
    <span className="text-sm whitespace-nowrap tabular-nums">
      {datetime(start, undefined, locale)} –{" "}
      {end ? datetime(end, undefined, locale) : t("vehicle_blocks.open_ended")}
    </span>
  );
}

export function DateTimeCell({ value }: { value: string | null | undefined }) {
  const { locale } = useLocale();
  if (!value) return <span>—</span>;
  return (
    <span className="whitespace-nowrap tabular-nums">
      {datetime(value, undefined, locale)}
    </span>
  );
}
