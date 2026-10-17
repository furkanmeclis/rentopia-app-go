import { en } from "../messages/en";
import { tr, type MessageKey } from "../messages/tr";
import type { Locale } from "./types";

export const LOCALE_COOKIE = "site_locale";
export const LOCALES: Locale[] = ["tr", "en"];

const dictionaries: Record<Locale, Record<MessageKey, string>> = { tr, en };

export type Translate = (
  key: MessageKey,
  vars?: Record<string, string | number>,
) => string;

export function isLocale(v: string | undefined | null): v is Locale {
  return v === "tr" || v === "en";
}

/** Picks the visitor's locale: their choice if the site offers it, else the site default. */
export function pickLocale(
  chosen: string | undefined,
  offered: string[],
  fallback: string,
): Locale {
  if (isLocale(chosen) && offered.includes(chosen)) return chosen;
  return isLocale(fallback) ? fallback : "tr";
}

export function translator(locale: Locale): Translate {
  const dict = dictionaries[locale];
  return (key, vars) => {
    let s = dict[key] ?? tr[key] ?? key;
    if (vars) {
      for (const [k, v] of Object.entries(vars))
        s = s.replaceAll(`{${k}}`, String(v));
    }
    return s;
  };
}
