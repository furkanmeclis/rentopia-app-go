import { COUNTRY_CODES } from "@/features/customers/constants";

/** Localized country name of an ISO 3166-1 alpha-2 code (falls back to the code). */
export function countryName(code: string | null | undefined, locale: string) {
  if (!code) return "";
  try {
    const names = new Intl.DisplayNames([locale === "tr" ? "tr" : "en"], {
      type: "region",
    });
    return names.of(code.toUpperCase()) ?? code;
  } catch {
    return code;
  }
}

/** Options of the nationality / country pickers. */
export function countryOptions(locale: string) {
  return COUNTRY_CODES.map((code) => ({
    value: code,
    label: `${countryName(code, locale)} (${code})`,
  }));
}

/** "VIP, Filo" → ["VIP", "Filo"] (trimmed, unique, case-insensitive). */
export function splitTags(raw: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of raw.split(/[,;]/)) {
    const tag = part.trim();
    if (!tag || seen.has(tag.toLocaleLowerCase("tr"))) continue;
    seen.add(tag.toLocaleLowerCase("tr"));
    out.push(tag);
  }
  return out;
}

/**
 * Turkish phone check of the forms (the API normalizes and validates
 * again): a national number (0532…, 532…) has 10 digits after the 0, an
 * international one starts with + and has 7 to 15 digits.
 */
export function isValidPhone(raw: string): boolean {
  const value = raw.trim();
  if (!value) return true;
  if (!/^[+0-9 ()-]+$/.test(value)) return false;
  const digits = value.replace(/\D/g, "");
  if (value.startsWith("+") || digits.startsWith("00")) {
    const intl = value.startsWith("+") ? digits : digits.slice(2);
    if (intl.startsWith("90")) return intl.length === 12;
    return intl.length >= 7 && intl.length <= 15;
  }
  if (digits.startsWith("90") && digits.length === 12) return true;
  if (digits.startsWith("0")) return digits.length === 11;
  return digits.length === 10;
}
