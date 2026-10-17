import type { Locale } from "./types";

/**
 * Organizations sell in Turkey: dates and times are Europe/Istanbul
 * (UTC+03:00 all year).
 */
export const TIME_ZONE = "Europe/Istanbul";
const OFFSET = "+03:00";

const intlLocale = (l: Locale) => (l === "tr" ? "tr-TR" : "en-GB");

export function money(
  amount: string | null | undefined,
  currency: string,
  locale: Locale,
): string {
  const n = Number(amount ?? 0);
  try {
    return new Intl.NumberFormat(intlLocale(locale), {
      style: "currency",
      currency: currency || "TRY",
    }).format(Number.isFinite(n) ? n : 0);
  } catch {
    return `${amount} ${currency}`;
  }
}

export function dateTime(iso: string, locale: Locale): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(intlLocale(locale), {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: TIME_ZONE,
  }).format(d);
}

/** "2026-10-20" + "10:30" → RFC 3339 in Istanbul time, or null. */
export function toRFC3339(date: string, time: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !/^\d{2}:\d{2}$/.test(time))
    return null;
  const iso = `${date}T${time}:00${OFFSET}`;
  return Number.isNaN(new Date(iso).getTime()) ? null : iso;
}

/** Today (+days) in Istanbul as YYYY-MM-DD. */
export function istanbulDate(days = 0, now = new Date()): string {
  const d = new Date(now.getTime() + days * 86_400_000);
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(d);
}

/** Half-hour slots of a day ("00:00" … "23:30"). */
export const TIME_SLOTS = Array.from({ length: 48 }, (_, i) => {
  const h = String(Math.floor(i / 2)).padStart(2, "0");
  return `${h}:${i % 2 ? "30" : "00"}`;
});
