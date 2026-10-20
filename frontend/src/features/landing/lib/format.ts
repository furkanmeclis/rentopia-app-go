import type { AppLocale } from "@/config/i18n";

/** Lira amounts in the mockups, formatted for the active locale. */
export function formatTry(locale: AppLocale, amount: number) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    style: "currency",
    currency: "TRY",
    maximumFractionDigits: 0,
  }).format(amount);
}
