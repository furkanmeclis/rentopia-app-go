"use client";

import { createNavAdornment } from "@/features/nav-engine";
import type { NavAdornment } from "@/features/nav-engine";
import { amountText } from "@/features/finance/lib/format";
import { usePayablesSummary } from "@/features/suppliers/hooks/use-suppliers";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Vadesi yaklaşan ödemeler" item: count of overdue + due-soon supplier
 * invoices (danger when something is overdue), the amounts in the info.
 */
function usePayablesNavAdornment(): NavAdornment {
  const { t, locale } = useLocale();
  const { data } = usePayablesSummary();
  if (!data || data.attention_count === 0) return { badges: [] };
  return {
    badges: [
      {
        kind: "count",
        value: data.attention_count,
        variant: data.overdue_count > 0 ? "danger" : "warning",
      },
    ],
    info: {
      title: t("suppliers.nav.info_title"),
      rows: [
        {
          label: t("suppliers.payables.overdue"),
          value: amountText(data.overdue_amount, locale, data.currency),
          tone: "danger" as const,
        },
        {
          label: t("suppliers.payables.due_soon", { days: data.window_days }),
          value: amountText(data.due_soon_amount, locale, data.currency),
          tone: "warning" as const,
        },
      ],
    },
  };
}

export const PayablesNavAdornment = createNavAdornment(usePayablesNavAdornment);
