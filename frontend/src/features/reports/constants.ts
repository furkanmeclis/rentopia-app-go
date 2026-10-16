import type { ChartConfig } from "@/components/charts";

/**
 * Categorical slots (validated light / dark pairs, fixed order). A channel
 * keeps its slot whatever the filter: colour follows the entity.
 */
export const SERIES_COLORS = [
  { light: "#2a78d6", dark: "#3987e5" },
  { light: "#eb6834", dark: "#d95926" },
  { light: "#1baf7a", dark: "#199e70" },
  { light: "#eda100", dark: "#c98500" },
  { light: "#e87ba4", dark: "#d55181" },
  { light: "#008300", dark: "#008300" },
  { light: "#6250d6", dark: "#9085e9" },
] as const;

/** Neutral slot for "other" (folded series). */
export const OTHER_COLOR = { light: "#8a8984", dark: "#6f6e69" } as const;

/** Known sales channels in their fixed colour order. */
export const KNOWN_CHANNELS = [
  "web",
  "widget",
  "api",
  "branch",
  "yolcu360",
  "obilet",
] as const;

/** Sequential blue (light → dark) for the aging buckets (older = darker). */
export const AGING_COLORS = [
  { light: "#9ec5f4", dark: "#184f95" },
  { light: "#6da7ec", dark: "#1c5cab" },
  { light: "#3987e5", dark: "#256abf" },
  { light: "#256abf", dark: "#5598e7" },
  { light: "#104281", dark: "#9ec5f4" },
] as const;

export const AGING_BUCKETS = [
  "current",
  "days_1_30",
  "days_31_60",
  "days_61_90",
  "days_90_plus",
] as const;

export const INCOME_COLOR = SERIES_COLORS[0];
export const EXPENSE_COLOR = SERIES_COLORS[1];

export function incomeExpenseChartConfig(
  t: (key: string) => string,
): ChartConfig {
  return {
    income: { label: t("reports.fields.income"), theme: INCOME_COLOR },
    expense: { label: t("reports.fields.expense"), theme: EXPENSE_COLOR },
  };
}

export const OCCUPANCY_GROUPS = ["rental_group", "branch", "month"] as const;
export const INCOME_EXPENSE_GROUPS = ["category", "branch", "month"] as const;
export const DATE_BASES = ["booking", "pickup"] as const;
export const PARTIES = ["customer", "supplier"] as const;
