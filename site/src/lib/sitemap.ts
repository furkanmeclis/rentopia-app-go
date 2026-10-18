/**
 * sitemap.xml (TEC-785; pure: node --test imports it). The API lists the
 * published site's public paths (GET /v1/public/site/sitemap); the site
 * owns the XML and the origin: every <loc> is the request's site origin
 * plus a validated relative path.
 */

export type SitemapEntry = { path: string; locale?: string; lastmod: string };

/** Sitemap protocol limit per file. */
export const MAX_URLS = 50_000;

const ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&apos;",
};

export function xmlEscape(value: string): string {
  return value.replace(/[&<>"']/g, (c) => ESCAPES[c]);
}

/** `https://host[:port]` without a path, or null. */
export function sitemapOrigin(raw: string | null | undefined): string | null {
  try {
    const u = new URL(raw ?? "");
    if (u.protocol !== "https:" && u.protocol !== "http:") return null;
    if (u.username || u.password) return null;
    return u.origin;
  } catch {
    return null;
  }
}

/** A root-relative path without control characters or `//`. */
export function sitemapPath(raw: unknown): string | null {
  if (typeof raw !== "string" || !raw.startsWith("/") || raw.startsWith("//"))
    return null;
  if (raw.length > 2000 || /[\\\s\u0000-\u001f\u007f]/.test(raw)) return null;
  return raw;
}

export function buildSitemapXml(
  origin: string,
  entries: SitemapEntry[] | null | undefined,
): string {
  const base = sitemapOrigin(origin);
  const seen = new Set<string>();
  const urls: string[] = [];
  for (const e of Array.isArray(entries) ? entries : []) {
    if (!base || urls.length >= MAX_URLS) break;
    const path = sitemapPath(e?.path);
    if (!path || seen.has(path)) continue;
    seen.add(path);
    const time = typeof e.lastmod === "string" ? Date.parse(e.lastmod) : NaN;
    const lastmod = Number.isNaN(time)
      ? ""
      : `<lastmod>${new Date(time).toISOString()}</lastmod>`;
    urls.push(`<url><loc>${xmlEscape(base + path)}</loc>${lastmod}</url>`);
  }
  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    ...urls,
    "</urlset>",
    "",
  ].join("\n");
}
