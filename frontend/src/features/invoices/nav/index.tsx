"use client";

import { createNavAdornment } from "@/features/nav-engine";
import type { NavAdornment } from "@/features/nav-engine";
import { useInvoicesNeedsAction } from "@/features/invoices/hooks/use-invoices";
import { useLocale } from "@/providers/locale-provider";

/** Invoices item: sends that failed or wait for a person. */
function useInvoicesNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const facets = useInvoicesNeedsAction();
  if (!facets || facets.needs_action <= 0) return { badges: [] };
  const failed = facets.status.find((f) => f.value === "failed")?.count ?? 0;
  return {
    badges: [{ kind: "count", value: facets.needs_action, variant: "danger" }],
    info: {
      title: t("invoices.nav.info_title"),
      rows: [
        {
          label: t("invoices.status.failed"),
          value: failed,
          tone: "danger" as const,
        },
        {
          label: t("invoices.nav.waiting"),
          value: Math.max(0, facets.needs_action - failed),
          tone: "warning" as const,
        },
      ].filter((r) => r.value > 0),
    },
  };
}

export const InvoicesNavAdornment = createNavAdornment(useInvoicesNavAdornment);
