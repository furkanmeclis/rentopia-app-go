import { KNOWN_CHANNELS } from "@/features/reports/constants";

/** "%12,5" / "%12.5"; "—" when null. */
export function percentText(
  v: number | null | undefined,
  locale: string,
): string {
  if (v == null) return "—";
  return `%${new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: 1,
  }).format(v)}`;
}

export function numberText(v: number, locale: string, digits = 1): string {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: digits,
  }).format(v);
}

/** Compact axis numbers (12,5 B / 12.5K). */
export function compactText(v: number, locale: string): string {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(v);
}

/** Decimal API string → number (charts and sorting only, never maths). */
export function num(v: string | null | undefined): number {
  const n = Number(v ?? 0);
  return Number.isFinite(n) ? n : 0;
}

/**
 * Label of a sales channel: a known code is translated, a channel added
 * later (channel manager) shows its code.
 */
export function channelLabel(code: string, t: (key: string) => string): string {
  return (KNOWN_CHANNELS as readonly string[]).includes(code)
    ? t(`reports.channel.${code}`)
    : code;
}

/** Income category codes the client translates (rent, charge.<kind> …). */
const INCOME_CODES = new Set([
  "rent",
  "other",
  "charge.late_return",
  "charge.early_return",
  "charge.damage",
  "charge.fuel",
  "charge.mileage",
  "charge.equipment",
  "charge.toll",
  "charge.toll_fee",
  "charge.other",
]);

export function incomeCategoryLabel(
  code: string,
  t: (key: string) => string,
): string {
  return INCOME_CODES.has(code) ? t(`reports.income_category.${code}`) : code;
}
