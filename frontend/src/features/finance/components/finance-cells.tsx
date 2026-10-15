"use client";

import { Paperclip } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  BALANCE_STATUS_LABEL_KEYS,
  BALANCE_STATUS_TONES,
  ENTRY_KIND_LABEL_KEYS,
  ENTRY_KIND_TONES,
} from "@/features/finance/constants";
import { amountText, balanceTone } from "@/features/finance/lib/format";
import type { BalanceStatus, EntryKind } from "@/features/finance/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export function BalanceStatusChip({ status }: { status: BalanceStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(BALANCE_STATUS_LABEL_KEYS[status])}
      tone={BALANCE_STATUS_TONES[status]}
    />
  );
}

export function EntryKindChip({ kind }: { kind: EntryKind }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(ENTRY_KIND_LABEL_KEYS[kind])}
      tone={ENTRY_KIND_TONES[kind]}
    />
  );
}

/** A tabular amount; `signed` colours receivable / payable balances. */
export function Amount({
  value,
  currency = "TRY",
  signed = false,
  muted = false,
  className,
}: {
  value: string | null | undefined;
  currency?: string;
  signed?: boolean;
  muted?: boolean;
  className?: string;
}) {
  const { locale } = useLocale();
  return (
    <span
      className={cn(
        "whitespace-nowrap tabular-nums",
        signed && value ? balanceTone(value) : undefined,
        muted ? "text-muted-foreground" : undefined,
        className,
      )}
    >
      {amountText(value, locale, currency)}
    </span>
  );
}

export function AttachmentBadge({ name }: { name: string }) {
  return (
    <Badge variant="secondary" className="max-w-[12rem] gap-1">
      <Paperclip className="size-3 shrink-0" />
      <span className="truncate">{name}</span>
    </Badge>
  );
}
