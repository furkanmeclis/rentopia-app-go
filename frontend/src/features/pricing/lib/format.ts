import type { AppLocale } from "@/config/i18n";
import { currency, date } from "@/lib/utils/format";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** "1-3 gün" / "15+ gün" (tr), "1-3 days" / "15+ days" (en). */
export function bandLabel(
  t: Translate,
  min: number,
  max: number | null | undefined,
): string {
  if (max == null) return t("pricing.band.open", { min });
  if (max === min) return t("pricing.band.single", { min });
  return t("pricing.band.range", { min, max });
}

/** Decimal API amount → "₺1.300,00" (tr-TR). Never assembles floats for maths. */
export function moneyText(
  value: string | null | undefined,
  locale: AppLocale,
): string {
  if (value == null || value === "") return "—";
  return currency(Number(value), "TRY", locale);
}

/** Signed rule value: "+20 %" / "+₺100,00" / "-10 %". */
export function ruleValueText(
  kind: string,
  value: string,
  locale: AppLocale,
): string {
  const n = Number(value);
  if (!Number.isFinite(n)) return value;
  const sign = n > 0 ? "+" : n < 0 ? "−" : "";
  const abs = Math.abs(n);
  if (kind === "percent") {
    const pct = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
      maximumFractionDigits: 2,
    }).format(abs);
    return `${sign}%${pct}`;
  }
  return `${sign}${currency(abs, "TRY", locale)}`;
}

/** "1 Haz 2026 – 1 Eyl 2026 (hariç)" — the end day is excluded. */
export function rangeText(
  t: Translate,
  start: string | null | undefined,
  end: string | null | undefined,
  locale: AppLocale,
): string {
  if (!start || !end) return "—";
  return t("pricing.range", {
    start: date(start, "d MMM yyyy", locale),
    end: date(end, "d MMM yyyy", locale),
  });
}

/** Normalises a typed amount: "1.300,50" → "1300.50"; "" stays "". */
export function normalizeAmount(raw: string): string {
  let v = raw.trim().replace(/\s/g, "").replace(/₺/g, "");
  if (v === "") return "";
  if (v.includes(",") && v.includes(".")) {
    // "1.300,50": dots group thousands, comma is the decimal separator.
    v = v.replace(/\./g, "").replace(",", ".");
  } else if (v.includes(",")) {
    v = v.replace(",", ".");
  }
  return v;
}

export const AMOUNT_RE = /^\d{1,12}(\.\d{1,2})?$/;
export const SIGNED_RE = /^-?\d{1,12}(\.\d{1,2})?$/;

/** True when a typed amount is a valid non-negative price. */
export function isAmount(raw: string): boolean {
  return AMOUNT_RE.test(normalizeAmount(raw));
}
