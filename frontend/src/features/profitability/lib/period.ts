import { todayISO } from "@/features/finance/lib/format";
import type { PeriodPreset } from "@/features/profitability/constants";

function iso(y: number, m: number, d: number): string {
  // Date.UTC normalises month / day overflow (month 0-based).
  const dt = new Date(Date.UTC(y, m, d));
  return dt.toISOString().slice(0, 10);
}

/** [from, to] of a preset, Europe/Istanbul days; `custom` returns null. */
export function presetRange(
  preset: PeriodPreset,
  today = todayISO(),
): { from: string; to: string } | null {
  const y = Number(today.slice(0, 4));
  const m = Number(today.slice(5, 7)) - 1;
  switch (preset) {
    case "this_month":
      return { from: iso(y, m, 1), to: today };
    case "last_month":
      return { from: iso(y, m - 1, 1), to: iso(y, m, 0) };
    case "last_3_months":
      return { from: iso(y, m - 2, 1), to: today };
    case "last_12_months":
      return { from: iso(y, m - 11, 1), to: today };
    case "this_year":
      return { from: iso(y, 0, 1), to: today };
    default:
      return null;
  }
}

/** "2026-03" → "Mar 2026" / "Mar 2026" in the locale. */
export function monthLabel(month: string, locale: string): string {
  const [y, m] = month.split("-").map(Number);
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-US", {
    month: "short",
    year: "2-digit",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, 1)));
}

/** Decimal API string → number for charts (display only, never maths). */
export function num(v: string | null | undefined): number {
  const n = Number(v ?? 0);
  return Number.isFinite(n) ? n : 0;
}
