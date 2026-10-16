import type { AppLocale } from "@/config/i18n";
import {
  ENTRY_KIND_LABEL_KEYS,
  PURPOSE_LABEL_KEYS,
  SYSTEM_REASON_LABEL_KEYS,
} from "@/features/finance/constants";
import type { LedgerEntry } from "@/features/finance/types";
import { ApiError } from "@/lib/api";
import { currency } from "@/lib/utils/format";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Decimal API amount → "₺1.300,00" (never maths on floats). */
export function amountText(
  value: string | null | undefined,
  locale: AppLocale,
  code = "TRY",
): string {
  if (value == null || value === "") return "—";
  return currency(Number(value), code || "TRY", locale);
}

/** Tone of a signed balance: receivable (positive) / payable (negative). */
export function balanceTone(value: string): string {
  const n = Number(value);
  if (n > 0) return "text-foreground";
  if (n < 0) return "text-emerald-600 dark:text-emerald-400";
  return "text-muted-foreground";
}

/** Typed amount "1.300,50" / "1300,5" → "1300.50"-style API decimal. */
export function normalizeAmount(raw: string): string {
  let v = raw.trim().replace(/\s/g, "").replace(/₺/g, "");
  if (v === "") return "";
  if (v.includes(",") && v.includes(".")) {
    v = v.replace(/\./g, "").replace(",", ".");
  } else if (v.includes(",")) {
    v = v.replace(",", ".");
  }
  return v;
}

export const AMOUNT_RE = /^\d{1,12}(\.\d{1,2})?$/;

/** True when the typed text is a positive amount. */
export function isPositiveAmount(raw: string): boolean {
  const v = normalizeAmount(raw);
  return AMOUNT_RE.test(v) && Number(v) > 0;
}

/** API decimal ("1300.50") → integer kuruş (previews only; the API decides). */
export function centsOf(value: string | null | undefined): number {
  if (!value) return 0;
  const n = Number(value);
  return Number.isFinite(n) ? Math.round(n * 100) : 0;
}

/** Integer kuruş → API decimal ("1300.50"). */
export function fromCents(cents: number): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(Math.round(cents));
  return `${sign}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
}

/** "Kira tahakkuku · ACME-2026-000012" style label of an entry. */
export function entryTitle(t: Translate, entry: LedgerEntry): string {
  const kind = t(ENTRY_KIND_LABEL_KEYS[entry.kind]);
  const purpose =
    entry.purpose !== "general" ? t(PURPOSE_LABEL_KEYS[entry.purpose]) : "";
  return [kind, purpose].filter(Boolean).join(" · ");
}

/** Reason text: system reasons are translated. */
export function reasonText(t: Translate, reason: string): string {
  const key = SYSTEM_REASON_LABEL_KEYS[reason];
  return key ? t(key) : reason;
}

/** Message of an API error for an inline alert. */
export function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    const fields = Object.values(error.fieldErrors());
    if (fields.length > 0) return fields.join(" · ");
    return error.message || fallback;
  }
  return fallback;
}

/** Today in Istanbul as yyyy-MM-dd. */
export function todayISO(): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
}

/** An API timestamp (UTC) as "dd.MM.yyyy HH:mm" in Istanbul time. */
export function istanbulDateTime(value: string, locale: AppLocale): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat(locale === "en" ? "en-GB" : "tr-TR", {
    timeZone: "Europe/Istanbul",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
    .format(d)
    .replace(",", "");
}

/** First day of the current month (Istanbul) as yyyy-MM-dd. */
export function monthStartISO(): string {
  return `${todayISO().slice(0, 8)}01`;
}
