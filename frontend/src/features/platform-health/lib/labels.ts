import type { AppLocale } from "@/config/i18n";
import { loadMessages } from "@/lib/i18n/messages";

/**
 * Health sources are pluggable: a module can register a source whose
 * codes (source, metric key, issue kind) have no label here yet. Known
 * codes come from `platform_health.<group>.<code>`; unknown ones are shown
 * humanized ("channel_sync" → "Channel sync") instead of a raw key.
 */
export function healthLabel(
  locale: AppLocale,
  group: "sources" | "metrics" | "kinds" | "job_status",
  code: string,
): string {
  const messages = loadMessages(locale) as Record<
    string,
    Record<string, string> | undefined
  >;
  const fallback = loadMessages("en") as Record<
    string,
    Record<string, string> | undefined
  >;
  const key = `${group}.${code}`;
  return (
    messages.platform_health?.[key] ??
    fallback.platform_health?.[key] ??
    humanize(code)
  );
}

export function healthHint(
  locale: AppLocale,
  code: string,
): string | undefined {
  const messages = loadMessages(locale) as Record<
    string,
    Record<string, string> | undefined
  >;
  return messages.platform_health?.[`sources.${code}_hint`];
}

function humanize(code: string): string {
  const text = code.replace(/[_.]+/g, " ").trim();
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : code;
}

/** Bytes with binary units (B … TB). */
export function formatBytes(bytes: number, locale: AppLocale): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1;
  return `${value.toLocaleString(locale, {
    maximumFractionDigits: digits,
  })} ${units[unit]}`;
}

/** The last 12 calendar months, newest first ("2026-10"). */
export function recentMonths(now = new Date(), count = 12): string[] {
  const out: string[] = [];
  for (let i = 0; i < count; i += 1) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    out.push(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`);
  }
  return out;
}

export function monthLabel(month: string, locale: AppLocale): string {
  const [y, m] = month.split("-").map(Number);
  return new Date(y, m - 1, 1).toLocaleDateString(locale, {
    month: "long",
    year: "numeric",
  });
}
