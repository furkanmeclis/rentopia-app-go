"use client";

import { StatsCard } from "@/components/common/stats-card";
import { Skeleton } from "@/components/ui/skeleton";
import { useFinanceSummary } from "@/features/finance/hooks/use-finance";
import { amountText } from "@/features/finance/lib/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Organization totals in the caller's branch scope: receivable, payable,
 * overdue, due soon and deposits held.
 */
export function FinanceSummaryCards() {
  const { t, locale } = useLocale();
  const query = useFinanceSummary();
  const s = query.data;
  if (query.isLoading || !s) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }
  const by = s.accounts_by_status;
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <StatsCard
        title={t("finance.summary.receivable")}
        value={amountText(s.receivable, locale, s.currency)}
        hint={t("finance.summary.net", {
          amount: amountText(s.net, locale, s.currency),
        })}
      />
      <StatsCard
        title={t("finance.summary.overdue")}
        value={
          <span
            className={Number(s.overdue) > 0 ? "text-destructive" : undefined}
          >
            {amountText(s.overdue, locale, s.currency)}
          </span>
        }
        hint={t("finance.summary.accounts", { count: by.overdue ?? 0 })}
      />
      <StatsCard
        title={t("finance.summary.due_soon")}
        value={amountText(s.due_soon, locale, s.currency)}
        hint={t("finance.summary.accounts", { count: by.due_soon ?? 0 })}
      />
      <StatsCard
        title={t("finance.summary.payable")}
        value={amountText(s.payable, locale, s.currency)}
        hint={t("finance.summary.accounts", { count: by.payable ?? 0 })}
      />
      <StatsCard
        title={t("finance.summary.deposit_held")}
        value={amountText(s.deposit_held, locale, s.currency)}
        hint={t("finance.summary.closed", { count: by.closed ?? 0 })}
      />
    </div>
  );
}
