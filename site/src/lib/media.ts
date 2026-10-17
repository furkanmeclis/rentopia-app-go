/**
 * Same-origin URLs of site library media (pure: node --test imports it).
 * The resolve answer maps each referenced media UUID to its API-owned
 * stream path (`/v1/public/site/media/{site}/{media}`, plus `?preview=` in
 * a draft preview). The browser gets `/media/site/media/{site}/{media}`;
 * a preview adds `draft=1` and the media route forwards the visitor's
 * preview cookie, so the token never appears in the page.
 */
const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const PATH = new RegExp(
  `^/v1/public/site/media/(${UUID})/(${UUID})(?:\\?preview=[A-Za-z0-9._-]+)?$`,
);

export function siteMediaUrl(
  site: { site_uuid: string; media?: Record<string, string> },
  id: string | null | undefined,
): string | null {
  if (!id) return null;
  const path = site.media?.[id];
  if (typeof path !== "string") return null;
  const m = PATH.exec(path);
  if (!m || m[1] !== site.site_uuid || m[2] !== id) return null;
  return `/media/site/media/${m[1]}/${m[2]}${path.includes("?preview=") ? "?draft=1" : ""}`;
}
