"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Info } from "lucide-react";
import { useMemo } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  LabelList,
  XAxis,
  YAxis,
} from "recharts";

import { AppChart } from "@/components/charts";
import { StatsCard } from "@/components/common/stats-card";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { Amount } from "@/features/finance/components/finance-cells";
import { amountText } from "@/features/finance/lib/format";
import {
  ALLOCATION_LABEL_KEYS,
  ALLOCATION_NOTE_KEYS,
  BASIS_LABEL_KEYS,
  BASIS_NOTE_KEYS,
  FIELD_LABEL_KEYS,
  GROUP_LABEL_KEYS,
  CHART_COLORS,
  trendChartConfig,
} from "@/features/profitability/constants";
import { monthLabel, num } from "@/features/profitability/lib/period";
import type {
  ProfitabilityFigures,
  ProfitabilityMethod,
  ProfitabilityRow,
  ProfitabilityTrendPoint,
} from "@/features/profitability/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function pct(v: number | null | undefined, locale: string): string {
  if (v == null) return "—";
  return `%${new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: 1,
  }).format(v)}`;
}

function days(v: number, locale: string): string {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: 1,
  }).format(v);
}

export { pct as percentText, days as daysText };

/** KPI tiles: income, costs, overhead, contribution, occupancy, days. */
export function ProfitabilityKpis({
  figures,
  loading,
}: {
  figures?: ProfitabilityFigures;
  loading?: boolean;
}) {
  const { t, locale } = useLocale();
  if (loading || !figures) {
    return (
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }
  const negative = num(figures.contribution) < 0;
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
      <StatsCard
        title={t("profitability.fields.gross_income")}
        value={amountText(figures.gross_income, locale)}
        hint={t("profitability.kpi.commissions", {
          amount: amountText(figures.commissions, locale),
        })}
      />
      <StatsCard
        title={t("profitability.fields.direct_costs")}
        value={amountText(figures.direct_costs, locale)}
      />
      <StatsCard
        title={t("profitability.fields.overhead")}
        value={amountText(figures.overhead, locale)}
      />
      <StatsCard
        title={t("profitability.fields.contribution")}
        value={
          <span className={negative ? "text-destructive" : undefined}>
            {amountText(figures.contribution, locale)}
          </span>
        }
        hint={t("profitability.kpi.margin", {
          value: pct(figures.margin, locale),
        })}
      />
      <StatsCard
        title={t("profitability.fields.occupancy")}
        value={pct(figures.occupancy, locale)}
        hint={t("profitability.kpi.available", {
          days: days(figures.available_days, locale),
        })}
      />
      <StatsCard
        title={t("profitability.fields.rental_days")}
        value={days(figures.rental_days, locale)}
        hint={t("profitability.kpi.rentals", { count: figures.rental_count })}
      />
    </div>
  );
}

/** What the figures mean: period, basis, allocation and the method notes. */
export function MethodNotes({
  method,
  from,
  to,
}: {
  method?: ProfitabilityMethod;
  from?: string;
  to?: string;
}) {
  const { t, locale } = useLocale();
  if (!method || !from || !to) return null;
  const notes = [
    t("profitability.notes.amounts"),
    t(BASIS_NOTE_KEYS[method.basis]),
    t(ALLOCATION_NOTE_KEYS[method.allocation]),
    t("profitability.notes.swap"),
    t("profitability.notes.kept_apart"),
  ];
  if (method.overhead_scope === "branches") {
    notes.push(t("profitability.notes.overhead_branches"));
  }
  if (method.allocation_fallback) notes.push(t("profitability.notes.fallback"));
  if (num(method.unallocated_overhead) !== 0) {
    notes.push(
      t("profitability.notes.unallocated", {
        amount: amountText(method.unallocated_overhead, locale),
      }),
    );
  }
  return (
    <EntitySectionCard title={t("profitability.notes.title")}>
      <div className="space-y-3 text-sm">
        <dl className="grid gap-2 sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("profitability.filters.period")}
            </dt>
            <dd className="font-medium">
              {date(from, "dd.MM.yyyy", locale)} –{" "}
              {date(to, "dd.MM.yyyy", locale)}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("profitability.filters.basis")}
            </dt>
            <dd className="font-medium">{t(BASIS_LABEL_KEYS[method.basis])}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("profitability.filters.allocation")}
            </dt>
            <dd className="font-medium">
              {t(ALLOCATION_LABEL_KEYS[method.allocation])}
            </dd>
          </div>
        </dl>
        <ul className="text-muted-foreground space-y-1.5">
          {notes.map((n) => (
            <li key={n} className="flex gap-2">
              <Info className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              <span>{n}</span>
            </li>
          ))}
        </ul>
      </div>
    </EntitySectionCard>
  );
}

/** Monthly income vs total cost (commissions + direct + overhead). */
export function ProfitabilityTrendChart({
  trend,
  loading,
}: {
  trend: ProfitabilityTrendPoint[];
  loading?: boolean;
}) {
  const { t, locale } = useLocale();
  const data = useMemo(
    () =>
      trend.map((p) => ({
        label: p.month,
        income: num(p.gross_income),
        cost: num(p.commissions) + num(p.direct_costs) + num(p.overhead),
      })),
    [trend],
  );
  const compact = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    notation: "compact",
    maximumFractionDigits: 1,
  });
  return (
    <AppChart
      type="bar"
      title={t("profitability.chart.trend_title")}
      description={t("profitability.chart.trend_description")}
      data={data}
      config={trendChartConfig(t)}
      valueFormatter={(v) => compact.format(v)}
      categoryFormatter={(v) => monthLabel(v, locale)}
      height={260}
      loading={loading}
      emptyTitle={t("profitability.empty")}
    />
  );
}

/**
 * Contribution by vehicle: the five best and the five worst (diverging:
 * positive blue, negative red), value at the bar end.
 */
export function ContributionChart({
  rows,
  loading,
}: {
  rows: ProfitabilityRow[];
  loading?: boolean;
}) {
  const { t, locale } = useLocale();
  const data = useMemo(() => {
    const sorted = [...rows]
      .map((r) => ({ label: r.vehicle.plate, value: num(r.contribution) }))
      .sort((a, b) => b.value - a.value);
    if (sorted.length <= 10) return sorted;
    return [...sorted.slice(0, 5), ...sorted.slice(-5)];
  }, [rows]);
  const config: ChartConfig = {
    value: {
      label: t("profitability.fields.contribution"),
      theme: CHART_COLORS.positive,
    },
    negative: {
      label: t("profitability.fields.contribution"),
      theme: CHART_COLORS.negative,
    },
  };
  const compact = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    notation: "compact",
    maximumFractionDigits: 1,
  });
  const height = Math.max(160, data.length * 32 + 40);
  return (
    <EntitySectionCard title={t("profitability.chart.vehicles_title")}>
      <p className="text-muted-foreground mb-3 text-xs">
        {t("profitability.chart.vehicles_description")}
      </p>
      {loading ? (
        <Skeleton className="h-[220px] w-full rounded-lg" />
      ) : data.length === 0 ? (
        <p className="text-muted-foreground py-10 text-center text-sm">
          {t("profitability.empty")}
        </p>
      ) : (
        <ChartContainer
          config={config}
          className="aspect-auto w-full"
          style={{ height }}
        >
          <BarChart
            data={data}
            layout="vertical"
            margin={{ top: 4, right: 56, bottom: 4, left: 4 }}
          >
            <CartesianGrid horizontal={false} />
            <XAxis
              type="number"
              tickLine={false}
              axisLine={false}
              tickFormatter={(v) => compact.format(Number(v))}
            />
            <YAxis
              type="category"
              dataKey="label"
              tickLine={false}
              axisLine={false}
              width={96}
            />
            <ChartTooltip
              cursor={false}
              content={
                <ChartTooltipContent
                  hideIndicator
                  formatter={(value) => amountText(String(value), locale)}
                />
              }
            />
            <Bar dataKey="value" radius={4} maxBarSize={24}>
              {data.map((d) => (
                <Cell
                  key={d.label}
                  fill={
                    d.value < 0 ? "var(--color-negative)" : "var(--color-value)"
                  }
                />
              ))}
              <LabelList
                dataKey="value"
                position="right"
                className="fill-foreground"
                fontSize={11}
                formatter={(v) => compact.format(Number(v))}
              />
            </Bar>
          </BarChart>
        </ChartContainer>
      )}
    </EntitySectionCard>
  );
}

type Line = {
  key: string;
  label: string;
  amount: string;
  group: "income" | "deduction" | "cost" | "apart" | "total";
};

/** Income, deductions, costs and the classes kept apart, as one table. */
export function ProfitabilityBreakdown({
  figures,
  categories,
  loading,
}: {
  figures?: ProfitabilityFigures;
  categories?: { code: string; name: string; amount: string }[];
  loading?: boolean;
}) {
  const { t } = useLocale();
  const lines = useMemo<Line[]>(() => {
    if (!figures) return [];
    const f = figures;
    const l = (
      key: string,
      amount: string,
      group: Line["group"],
      label = t(FIELD_LABEL_KEYS[key] ?? key),
    ): Line => ({ key, label, amount, group });
    const out: Line[] = [
      l("rent", f.rent, "income"),
      l("extras", f.extras, "income"),
      l("charges", f.charges, "income"),
      l("toll_fees", f.toll_fees, "income"),
      l("gross_income", f.gross_income, "total"),
      l("payment_commission", f.payment_commission, "deduction"),
      l("channel_commission", f.channel_commission, "deduction"),
    ];
    if (categories?.length) {
      for (const c of categories) {
        out.push(l(`category_${c.code}`, c.amount, "cost", c.name));
      }
    } else {
      out.push(l("expenses", f.expenses, "cost"));
    }
    out.push(
      l("maintenance", f.maintenance, "cost"),
      l("transfers", f.transfers, "cost"),
      l("overhead", f.overhead, "cost"),
      l("contribution", f.contribution, "total"),
      l("refunded", f.refunded, "apart"),
      l("deposits_held", f.deposits_held, "apart"),
      l("deposits_returned", f.deposits_returned, "apart"),
      l("pass_through", f.pass_through, "apart"),
      l("capex", f.capex, "apart"),
    );
    return out;
  }, [figures, categories, t]);

  const columns = useMemo(
    () =>
      [
        createColumn<Line>({
          accessorKey: "label",
          labelKey: "profitability.breakdown.line",
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className={
                row.original.group === "total" ? "font-semibold" : undefined
              }
            >
              {row.original.label}
            </span>
          ),
        }),
        createColumn<Line>({
          accessorKey: "group",
          labelKey: "profitability.breakdown.class",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {t(GROUP_LABEL_KEYS[row.original.group] ?? row.original.group)}
            </span>
          ),
        }),
        createColumn<Line>({
          accessorKey: "amount",
          labelKey: "profitability.breakdown.amount",
          cell: ({ row }) => (
            <Amount
              value={row.original.amount}
              muted={row.original.group === "apart"}
              className={
                row.original.group === "total" ? "font-semibold" : undefined
              }
            />
          ),
        }),
      ] as ColumnDef<Line, unknown>[],
    [t],
  );

  return (
    <EntitySectionCard title={t("profitability.breakdown.title")}>
      <EntityTable
        columns={columns}
        data={lines}
        getRowId={(row) => row.key}
        isLoading={loading}
        emptyTitle={t("profitability.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-profitability-breakdown-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          columnFilters: false,
          facetedFilters: false,
        }}
      />
    </EntitySectionCard>
  );
}
