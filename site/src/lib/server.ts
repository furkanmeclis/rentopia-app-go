import "server-only";

import { cookies, headers } from "next/headers";
import { notFound } from "next/navigation";
import { cache } from "react";

import { ApiError, apiFetch, type ApiRequest } from "./api";
import { LOCALE_COOKIE, pickLocale, translator, type Translate } from "./i18n";
import { PREVIEW_HEADER, previewToken } from "./preview";
import { clientIpFrom } from "./request";
import { resolveSite } from "./resolve";
import {
  SESSION_COOKIE,
  SESSION_HEADER,
  decodeSession,
  encodeSession,
  mintSession,
  sessionCookieOptions,
  usableSession,
} from "./session";
import type { Locale, Site, SiteContent } from "./types";

/** The site of the current request host (404 when none). */
export const getSite = cache(async (): Promise<Site> => {
  const h = await headers();
  // Set by the proxy only (client copies are dropped there).
  const preview = previewToken(h.get(PREVIEW_HEADER));
  const site = await resolveSite(h.get("host"), clientIpFrom(h), preview);
  if (!site) notFound();
  return site;
});

export const getClientIp = cache(async () => clientIpFrom(await headers()));

export type Ctx = {
  site: Site;
  locale: Locale;
  t: Translate;
  content: SiteContent;
};

const EMPTY_CONTENT: SiteContent = {
  hero_title: "",
  hero_subtitle: "",
  about: "",
  faq: [],
  terms: "",
  privacy: "",
};

/** Site, visitor locale, translator and the site's texts in that locale. */
export const getCtx = cache(async (): Promise<Ctx> => {
  const site = await getSite();
  const jar = await cookies();
  const locale = pickLocale(
    jar.get(LOCALE_COOKIE)?.value,
    site.locales,
    site.default_locale,
  );
  return {
    site,
    locale,
    t: translator(locale),
    content: site.config.content?.[locale] ?? EMPTY_CONTENT,
  };
});

async function currentToken(site: Site): Promise<string | null> {
  const fromProxy = (await headers()).get(SESSION_HEADER);
  if (fromProxy) return fromProxy;
  const stored = decodeSession((await cookies()).get(SESSION_COOKIE)?.value);
  return usableSession(stored, site) ? stored.t : null;
}

async function refreshToken(site: Site): Promise<string> {
  const fresh = await mintSession(site, await getClientIp());
  try {
    // Allowed in server actions; a page render keeps the proxy's cookie.
    (await cookies()).set(
      SESSION_COOKIE,
      encodeSession(fresh),
      sessionCookieOptions(site),
    );
  } catch {
    // Rendering: the next request's proxy pass stores a fresh session.
  }
  return fresh.t;
}

/**
 * Calls the public booking API as this visitor: the booking session of the
 * site's origin (minted again once when it expired).
 */
export async function bookingApi<T>(
  path: string,
  req: Omit<ApiRequest, "token" | "origin" | "clientIp"> = {},
): Promise<T> {
  const site = await getSite();
  if (!site.booking_enabled || !site.publishable_key) {
    throw new ApiError(403, "BOOKING_DISABLED", "Online booking is off");
  }
  // A draft preview browses the catalog and prices but never books or pays.
  if (
    site.preview &&
    req.method === "POST" &&
    path.startsWith("/v1/public/reservations")
  ) {
    throw new ApiError(403, "PREVIEW_READ_ONLY", "Previews cannot book");
  }
  const base = { ...req, origin: site.origin, clientIp: await getClientIp() };
  let token = await currentToken(site);
  if (!token) token = await refreshToken(site);
  try {
    return await apiFetch<T>(path, { ...base, token });
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      return apiFetch<T>(path, { ...base, token: await refreshToken(site) });
    }
    throw err;
  }
}

/** Same-site URL of an API media path (proxied by /media/...). */
export function mediaUrl(apiPath: string | null | undefined): string | null {
  if (!apiPath || !apiPath.startsWith("/v1/public/")) return null;
  return `/media${apiPath.slice("/v1/public".length)}`;
}
