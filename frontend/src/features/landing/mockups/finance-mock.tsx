"use client";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatTry } from "../lib/format";

// Sample figures for the mockup, not results from any company.
const ROWS = [
  { plate: "35 RNT 907", income: 142000, cost: 61000 },
  { plate: "34 RTP 482", income: 118500, cost: 57500 },
  { plate: "16 RPA 351", income: 96000, cost: 58800 },
  { plate: "06 RTY 118", income: 71000, cost: 64200 },
];

const MAX_INCOME = Math.max(...ROWS.map((r) => r.income));

/** Vehicle profitability summary. Decorative; the caption describes it. */
export function FinanceMock({ className }: { className?: string }) {
  const { t, locale } = useLocale();

  return (
    <div
      aria-hidden
      className={cn(
        "bg-card text-card-foreground w-full rounded-2xl border p-5 shadow-[0_1px_0_rgba(20,21,39,0.04),0_24px_48px_-24px_rgba(20,21,39,0.25)] sm:p-6",
        className,
      )}
    >
      <div className="flex items-baseline justify-between gap-3">
        <p className="font-semibold">{t("landing.finance.mock_title")}</p>
        <p className="text-muted-foreground text-xs">
          {t("landing.finance.mock_period")}
        </p>
      </div>

      <div className="text-muted-foreground mt-5 grid grid-cols-[minmax(0,1.2fr)_repeat(3,minmax(0,1fr))] gap-3 border-b pb-2 text-xs font-medium">
        <span>{t("landing.finance.mock_vehicle")}</span>
        <span className="text-right">{t("landing.finance.mock_income")}</span>
        <span className="text-right">{t("landing.finance.mock_cost")}</span>
        <span className="text-right">{t("landing.finance.mock_margin")}</span>
      </div>

      <ul>
        {ROWS.map((row) => {
          const profit = row.income - row.cost;
          return (
            <li key={row.plate} className="border-b py-3 last:border-b-0">
              <div className="grid grid-cols-[minmax(0,1.2fr)_repeat(3,minmax(0,1fr))] items-baseline gap-3 text-xs sm:text-sm">
                <span className="truncate font-medium tabular-nums">
                  {row.plate}
                </span>
                <span className="text-right tabular-nums">
                  {formatTry(locale, row.income)}
                </span>
                <span className="text-muted-foreground text-right tabular-nums">
                  {formatTry(locale, row.cost)}
                </span>
                <span className="text-right font-semibold tabular-nums">
                  {formatTry(locale, profit)}
                </span>
              </div>
              <div className="bg-muted mt-2 flex h-1.5 overflow-hidden rounded-full">
                <span
                  className="bg-primary/35"
                  style={{ width: `${(row.cost / MAX_INCOME) * 100}%` }}
                />
                <span
                  className="bg-primary"
                  style={{ width: `${(profit / MAX_INCOME) * 100}%` }}
                />
              </div>
            </li>
          );
        })}
      </ul>

      <p className="text-muted-foreground mt-3 text-[11px]">
        {t("landing.finance.mock_note")}
      </p>
    </div>
  );
}
