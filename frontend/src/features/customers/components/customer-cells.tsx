"use client";

import { Building2, EyeOff, User } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  CONSENT_CHANNEL_LABEL_KEYS,
  CUSTOMER_TYPE_LABEL_KEYS,
} from "@/features/customers/constants";
import type {
  ConsentChannel,
  Customer,
  CustomerType,
} from "@/features/customers/types";
import { useLocale } from "@/providers/locale-provider";

/** Individual / corporate chip with its icon. */
export function CustomerTypeChip({ type }: { type: CustomerType }) {
  const { t } = useLocale();
  const Icon = type === "corporate" ? Building2 : User;
  return (
    <Badge variant="outline" className="gap-1">
      <Icon className="size-3" aria-hidden />
      {t(CUSTOMER_TYPE_LABEL_KEYS[type])}
    </Badge>
  );
}

/**
 * A masked value (identity, phone, e-mail) in a monospace chip; the API
 * never sends the plaintext to lists.
 */
export function MaskedValue({ value }: { value: string | null | undefined }) {
  const { t } = useLocale();
  if (!value) return <span className="text-muted-foreground">—</span>;
  return (
    <span
      className="inline-flex items-center gap-1.5 font-mono text-xs tabular-nums"
      title={t("customers.masked_hint")}
    >
      <EyeOff className="text-muted-foreground size-3" aria-hidden />
      {value}
    </span>
  );
}

export function ConsentChips({ channels }: { channels: ConsentChannel[] }) {
  const { t } = useLocale();
  if (!channels.length) {
    return (
      <span className="text-muted-foreground text-xs">
        {t("customers.consent_channels.none")}
      </span>
    );
  }
  return (
    <span className="flex flex-wrap gap-1">
      {channels.map((channel) => (
        <Badge key={channel} variant="success" className="text-[11px]">
          {t(CONSENT_CHANNEL_LABEL_KEYS[channel])}
        </Badge>
      ))}
    </span>
  );
}

export function TagChips({ tags }: { tags: string[] }) {
  if (!tags.length) return <span className="text-muted-foreground">—</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {tags.map((tag) => (
        <Badge key={tag} variant="secondary" className="text-[11px]">
          {tag}
        </Badge>
      ))}
    </span>
  );
}

/** Display name, or the anonymized marker. */
export function CustomerName({ customer }: { customer: Customer }) {
  const { t } = useLocale();
  if (customer.anonymized) {
    return <StatusChip label={t("customers.anonymized_badge")} />;
  }
  return <span className="font-medium">{customer.display_name}</span>;
}
