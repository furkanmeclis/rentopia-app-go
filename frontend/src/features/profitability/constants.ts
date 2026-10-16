import type { ChartConfig } from "@/components/charts";
import type {
  ProfitabilityAllocation,
  ProfitabilityBasis,
} from "@/features/profitability/types";

export const BASES: ProfitabilityBasis[] = ["accrual", "cash"];

export const ALLOCATIONS: ProfitabilityAllocation[] = [
  "rental_days",
  "vehicle_count",
  "revenue",
];

export const BASIS_LABEL_KEYS: Record<ProfitabilityBasis, string> = {
  accrual: "profitability.basis.accrual",
  cash: "profitability.basis.cash",
};

export const ALLOCATION_LABEL_KEYS: Record<ProfitabilityAllocation, string> = {
  rental_days: "profitability.allocation.rental_days",
  vehicle_count: "profitability.allocation.vehicle_count",
  revenue: "profitability.allocation.revenue",
};

export const PERIOD_PRESETS = [
  "this_month",
  "last_month",
  "last_3_months",
  "last_12_months",
  "this_year",
  "custom",
] as const;

export type PeriodPreset = (typeof PERIOD_PRESETS)[number];

export const PERIOD_LABEL_KEYS: Record<PeriodPreset, string> = {
  this_month: "profitability.period.this_month",
  last_month: "profitability.period.last_month",
  last_3_months: "profitability.period.last_3_months",
  last_12_months: "profitability.period.last_12_months",
  this_year: "profitability.period.this_year",
  custom: "profitability.period.custom",
};

/**
 * Chart colours (validated pairs, light / dark): income blue vs cost orange
 * in the trend; the contribution chart is diverging (blue = positive,
 * red = negative). Red and orange never share a chart.
 */
export const CHART_COLORS = {
  income: { light: "#2a78d6", dark: "#3987e5" },
  cost: { light: "#eb6834", dark: "#d95926" },
  positive: { light: "#2a78d6", dark: "#3987e5" },
  negative: { light: "#e34948", dark: "#e66767" },
} as const;

export function trendChartConfig(t: (key: string) => string): ChartConfig {
  return {
    income: {
      label: t("profitability.fields.gross_income"),
      theme: CHART_COLORS.income,
    },
    cost: {
      label: t("profitability.chart.total_cost"),
      theme: CHART_COLORS.cost,
    },
  };
}

/** Static label keys of the figures (no template keys). */
export const FIELD_LABEL_KEYS: Record<string, string> = {
  vehicle: "profitability.fields.vehicle",
  home_branch: "profitability.fields.home_branch",
  rental_days: "profitability.fields.rental_days",
  available_days: "profitability.fields.available_days",
  occupancy: "profitability.fields.occupancy",
  gross_income: "profitability.fields.gross_income",
  commissions: "profitability.fields.commissions",
  payment_commission: "profitability.fields.payment_commission",
  channel_commission: "profitability.fields.channel_commission",
  direct_costs: "profitability.fields.direct_costs",
  expenses: "profitability.fields.expenses",
  maintenance: "profitability.fields.maintenance",
  transfers: "profitability.fields.transfers",
  overhead: "profitability.fields.overhead",
  contribution: "profitability.fields.contribution",
  margin: "profitability.fields.margin",
  rent: "profitability.fields.rent",
  extras: "profitability.fields.extras",
  charges: "profitability.fields.charges",
  toll_fees: "profitability.fields.toll_fees",
  refunded: "profitability.fields.refunded",
  deposits_held: "profitability.fields.deposits_held",
  deposits_returned: "profitability.fields.deposits_returned",
  pass_through: "profitability.fields.pass_through",
  capex: "profitability.fields.capex",
};

export const GROUP_LABEL_KEYS: Record<string, string> = {
  income: "profitability.breakdown.groups.income",
  deduction: "profitability.breakdown.groups.deduction",
  cost: "profitability.breakdown.groups.cost",
  apart: "profitability.breakdown.groups.apart",
  total: "profitability.breakdown.groups.total",
};

export const BASIS_NOTE_KEYS: Record<ProfitabilityBasis, string> = {
  accrual: "profitability.notes.basis_accrual",
  cash: "profitability.notes.basis_cash",
};

export const ALLOCATION_NOTE_KEYS: Record<ProfitabilityAllocation, string> = {
  rental_days: "profitability.notes.allocation_rental_days",
  vehicle_count: "profitability.notes.allocation_vehicle_count",
  revenue: "profitability.notes.allocation_revenue",
};
