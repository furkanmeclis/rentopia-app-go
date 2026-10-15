"use client";

import { createNavAdornment } from "@/features/nav-engine";
import type { NavAdornment } from "@/features/nav-engine";
import { REVIEW_REASON_LABEL_KEYS } from "@/features/tolls/constants";
import { useTollReviewCount } from "@/features/tolls/hooks/use-tolls";
import type { TollReviewReason } from "@/features/tolls/types";
import { useLocale } from "@/providers/locale-provider";

/** Review queue item: passes waiting for a person, split by reason. */
function useTollReviewNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const facets = useTollReviewCount();
  if (!facets) return { badges: [] };
  const reasons = facets.review_reason
    .filter((r) => r.count > 0)
    .sort((a, b) => b.count - a.count)
    .slice(0, 5);
  return {
    badges: [{ kind: "count", value: facets.review, variant: "warning" }],
    info:
      facets.review > 0
        ? {
            title: t("tolls.nav.info_title"),
            rows: reasons.map((r) => ({
              label: t(
                REVIEW_REASON_LABEL_KEYS[
                  r.value as Exclude<TollReviewReason, "">
                ] ?? "tolls.nav.other_reason",
              ),
              value: r.count,
              tone: "warning",
            })),
          }
        : null,
  };
}

export const TollReviewNavAdornment = createNavAdornment(
  useTollReviewNavAdornment,
);
