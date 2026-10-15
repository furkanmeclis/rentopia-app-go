"use client";

import { StatusChip } from "@/components/common/status-chip";
import { STATE_LABEL_KEYS, STATE_TONES } from "@/features/campaigns/constants";
import type { Campaign, CampaignState } from "@/features/campaigns/types";
import { moneyText, ruleValueText } from "@/features/pricing/lib/format";
import { useLocale } from "@/providers/locale-provider";

export function CampaignStateChip({ state }: { state: CampaignState }) {
  const { t } = useLocale();
  return (
    <StatusChip label={t(STATE_LABEL_KEYS[state])} tone={STATE_TONES[state]} />
  );
}

/** "%20 (en fazla ₺500)" / "₺150". */
export function DiscountText({ campaign }: { campaign: Campaign }) {
  const { t, locale } = useLocale();
  const value = ruleValueText(
    campaign.discount_kind,
    campaign.discount_value,
    locale,
  ).replace(/^\+/, "");
  return (
    <span className="tabular-nums">
      {value}
      {campaign.max_discount ? (
        <span className="text-muted-foreground text-xs">
          {" "}
          {t("campaigns.max_discount_suffix", {
            amount: moneyText(campaign.max_discount, locale),
          })}
        </span>
      ) : null}
    </span>
  );
}

/** "3 / 100" or "3" without a limit. */
export function UsageText({ campaign }: { campaign: Campaign }) {
  return (
    <span className="tabular-nums">
      {campaign.used_count}
      {campaign.usage_limit != null ? ` / ${campaign.usage_limit}` : ""}
    </span>
  );
}
