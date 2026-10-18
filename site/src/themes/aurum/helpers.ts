/**
 * Pure helpers of the Aurum renderer (node --test imports them).
 */

/**
 * The hero title with its last two words emphasized in the accent (the
 * studio's `<em>`): [lead, emphasis]. One or two words are all emphasis.
 */
export function splitTitle(title: string): [string, string] {
  const words = title.trim().split(/\s+/).filter(Boolean);
  if (words.length <= 2) return ["", words.join(" ")];
  return [words.slice(0, -2).join(" "), words.slice(-2).join(" ")];
}

/** Home fleet cards: one row (three columns) or two rows of two. */
export function homeFleetLimit(columns: 2 | 3): number {
  return columns === 2 ? 4 : 3;
}
