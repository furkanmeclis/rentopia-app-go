"use client";

import { ArrowRight } from "lucide-react";
import Link from "next/link";

import { StatsCard } from "@/components/common/stats-card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { amountText } from "@/features/finance/lib/format";
import { usePayablesSummary } from "@/features/suppliers/hooks/use-suppliers";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Vadesi yaklaşan ödemeler": open supplier invoices overdue and due within
 * the window (caller's branch scope), with a link to the payables view.
 * Hidden without `tenant.finance.expenses.read`.
 */
export function PayablesCard({ showLink = true }: { showLink?: boolean }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = usePayablesSummary();
  if (!query.allowed) return null;
  const s = query.data;
  if (query.isLoading || !s) {
    return (
      <div className="grid gap-3 sm:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }
  return (
    <div className="space-y-2">
      <div className="grid gap-3 sm:grid-cols-3">
        <StatsCard
          title={t("suppliers.payables.overdue")}
          value={
            <span
              className={
                Number(s.overdue_amount) > 0 ? "text-destructive" : undefined
              }
            >
              {amountText(s.overdue_amount, locale, s.currency)}
            </span>
          }
          hint={t("suppliers.payables.invoices", { count: s.overdue_count })}
        />
        <StatsCard
          title={t("suppliers.payables.due_soon", { days: s.window_days })}
          value={
            <span
              className={
                Number(s.due_soon_amount) > 0
                  ? "text-amber-600 dark:text-amber-400"
                  : undefined
              }
            >
              {amountText(s.due_soon_amount, locale, s.currency)}
            </span>
          }
          hint={t("suppliers.payables.invoices", { count: s.due_soon_count })}
        />
        <StatsCard
          title={t("suppliers.payables.open")}
          value={amountText(s.open_amount, locale, s.currency)}
          hint={t("suppliers.payables.invoices", { count: s.open_count })}
        />
      </div>
      {showLink && s.attention_count > 0 ? (
        <div className="flex justify-end">
          <Button asChild variant="link" size="sm" className="px-0">
            <Link href={routes.tenant.finance.payables(slug)}>
              {t("suppliers.payables.view", { count: s.attention_count })}
              <ArrowRight className="size-4" />
            </Link>
          </Button>
        </div>
      ) : null}
    </div>
  );
}
