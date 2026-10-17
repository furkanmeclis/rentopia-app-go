import { localized } from "./links.ts";
import type { Locale, Site, SiteCampaign } from "./types";

/**
 * Storefront campaign cards (TEC-784; pure: node --test imports it). The
 * resolve answer carries the organization's live campaigns (active,
 * visible, valid today, web channel, ordered by the panel's sort): the site
 * renders them server-side without another API call. Dates and codes come
 * from the campaign itself; a code is present only for public coupons.
 */

const ID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const UUID = new RegExp(`^${ID}$`, "i");
const MEDIA = new RegExp(`^/v1/public/site/media/(${ID})/(${ID})$`, "i");
const DATE = /^\d{4}-\d{2}-\d{2}$/;
const CODE = /^[A-Za-z0-9_-]{1,64}$/;

/** At most this many cards anywhere on the site. */
export const MAX_CAMPAIGNS = 24;

export type CampaignCard = {
  uuid: string;
  title: string;
  summary: string;
  cta: string;
  image: string | null;
  code: string | null;
  start: string | null;
  end: string | null;
};

/**
 * Same-origin image of a campaign: the API-owned stream path of this
 * site's library (`/v1/public/site/media/{site}/{media}`) through the
 * site's media route; anything else → null (token gradient instead).
 */
export function campaignMediaUrl(
  site: Pick<Site, "site_uuid">,
  path: string | null | undefined,
): string | null {
  const m = typeof path === "string" ? MEDIA.exec(path) : null;
  if (!m || m[1].toLowerCase() !== site.site_uuid.toLowerCase()) return null;
  return `/media/site/media/${m[1]}/${m[2]}`;
}

/** The visitor's campaigns: well-formed entries, in the API's order. */
export function campaignCards(
  site: Pick<Site, "site_uuid" | "campaigns">,
  locale: Locale,
): CampaignCard[] {
  const out: CampaignCard[] = [];
  const seen = new Set<string>();
  for (const c of Array.isArray(site.campaigns) ? site.campaigns : []) {
    if (!c || typeof c.uuid !== "string" || !UUID.test(c.uuid)) continue;
    if (seen.has(c.uuid)) continue;
    seen.add(c.uuid);
    out.push(toCard(site, c, locale));
    if (out.length >= MAX_CAMPAIGNS) break;
  }
  return out;
}

export function campaignCard(
  site: Pick<Site, "site_uuid" | "campaigns">,
  uuid: string,
  locale: Locale,
): CampaignCard | null {
  if (!UUID.test(uuid)) return null;
  return (
    campaignCards(site, locale).find(
      (c) => c.uuid.toLowerCase() === uuid.toLowerCase(),
    ) ?? null
  );
}

function toCard(
  site: Pick<Site, "site_uuid">,
  c: SiteCampaign,
  locale: Locale,
): CampaignCard {
  const date = (v: string | null | undefined) =>
    typeof v === "string" && DATE.test(v) ? v : null;
  return {
    uuid: c.uuid.toLowerCase(),
    title: localized(c.title, locale),
    summary: localized(c.summary, locale),
    cta: localized(c.cta_label, locale),
    image: campaignMediaUrl(site, c.media_url),
    code: typeof c.code === "string" && CODE.test(c.code) ? c.code : null,
    start: date(c.start_date),
    end: date(c.end_date),
  };
}

/** "2026-10-31" → "31 Eki 2026" / "31 Oct 2026" (a calendar date, no zone shift). */
export function campaignDate(value: string, locale: Locale): string {
  if (!DATE.test(value)) return value;
  const d = new Date(`${value}T12:00:00Z`);
  if (Number.isNaN(d.getTime())) return value;
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  }).format(d);
}

/** Which validity sentence a card shows (the caller translates it). */
export function campaignValidity(
  card: Pick<CampaignCard, "start" | "end">,
  today: string,
): {
  kind: "until" | "from" | "between" | "open";
  start?: string;
  end?: string;
} {
  const startsLater = Boolean(card.start && card.start > today);
  if (card.end && startsLater)
    return { kind: "between", start: card.start!, end: card.end };
  if (card.end) return { kind: "until", end: card.end };
  if (startsLater) return { kind: "from", start: card.start! };
  return { kind: "open" };
}
