"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  ERROR_CODE_LABEL_KEYS,
  MESSAGE_STATUS_LABEL_KEYS,
  MESSAGE_STATUS_TONES,
  RUN_STATUS_LABEL_KEYS,
  RUN_STATUS_TONES,
} from "@/features/channels/constants";
import type {
  ChannelMessageStatus,
  ChannelSyncRun,
} from "@/features/channels/types";
import { INTEGRATION_PROVIDER_LABEL_KEYS } from "@/features/payments/constants";
import { ApiError } from "@/lib/api";
import { money } from "@/lib/utils/format";
import type { AppLocale } from "@/config/i18n";
import { useLocale } from "@/providers/locale-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

export function MessageStatusChip({
  status,
}: {
  status: ChannelMessageStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(MESSAGE_STATUS_LABEL_KEYS[status])}
      tone={MESSAGE_STATUS_TONES[status]}
    />
  );
}

export function RunStatusChip({ status }: { status: string }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={
        RUN_STATUS_LABEL_KEYS[status]
          ? t(RUN_STATUS_LABEL_KEYS[status])
          : status
      }
      tone={RUN_STATUS_TONES[status] ?? "default"}
    />
  );
}

/** Human text of a message error code ("" for none). */
export function errorCodeText(t: T, code: string): string {
  if (!code) return "";
  const key = ERROR_CODE_LABEL_KEYS[code];
  return key ? t(key) : code;
}

/** Provider label of an account (adapter code as fallback). */
export function providerText(t: T, provider: string): string {
  const key = INTEGRATION_PROVIDER_LABEL_KEYS[provider];
  return key ? t(key) : provider;
}

/** A money string from the API ("1234.50") in the locale, with currency. */
export function amountText(
  value: string | null | undefined,
  locale: AppLocale,
  currency = "TRY",
): string {
  if (value === null || value === undefined || value === "") return "—";
  const n = Number(value);
  if (Number.isNaN(n)) return value;
  return `${money(n, locale)} ${currency}`;
}

/** Text of a refused channel action: the code first, then the details. */
export function channelProblemText(t: T, error: unknown): string {
  if (!(error instanceof ApiError)) return t("channels.problems.generic");
  switch (error.code) {
    case "CHANNEL_SYNC_BUSY":
      return t("channels.problems.busy");
    case "CHANNEL_ACCOUNT_INACTIVE":
      return t("channels.problems.inactive");
    case "CHANNEL_ADAPTER_UNAVAILABLE":
      return t("channels.problems.adapter");
    case "CHANNEL_NOT_SIMULATOR":
      return t("channels.problems.not_simulator");
    case "CHANNEL_MESSAGE_NOT_RETRYABLE":
      return t("channels.problems.not_retryable");
    case "CHANNEL_MAPPING_TAKEN":
      return t("channels.problems.mapping_taken");
    case "CHANNEL_UNSUPPORTED":
      return t("channels.problems.unsupported");
  }
  if (error.details.length > 0) {
    return error.details
      .map((d) => (d.field ? `${d.field}: ${d.message}` : d.message))
      .join(" · ");
  }
  return error.message;
}

/** Summary line of a sync run ("12 / 14 · 2 hata"). */
export function runCountsText(t: T, run: ChannelSyncRun): string {
  return t("channels.runs.counts", {
    ok: run.items_ok,
    total: run.items_total,
    failed: run.items_failed,
  });
}
