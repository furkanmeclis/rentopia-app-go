/**
 * Pure helpers of the classic renderer (node --test imports them).
 */

/**
 * The monogram tile of the header: the first two letters or digits of the
 * organization's name (Latin, including Turkish letters), upper-cased in
 * the visitor's locale; "R" when the name has none.
 */
export function monogram(name: string, locale: string): string {
  const letters = Array.from(name)
    .filter((ch) => /[0-9A-Za-zÀ-ɏ]/.test(ch))
    .slice(0, 2);
  return letters.join("").toLocaleUpperCase(locale) || "R";
}

/** Home fleet cards: two full rows of the chosen column count. */
export function homeFleetLimit(columns: 2 | 3): number {
  return columns * 2;
}
