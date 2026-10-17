import { apiFetch } from "./api.ts";
import type { Site } from "./types";

/**
 * Booking sessions (public API, TEC-728): the site server mints a 30-minute
 * session token per visitor with the site's publishable key for the site's
 * origin, and keeps it in an httpOnly cookie of the site host. The cookie
 * records the origin it was minted for; a token is only used for the same
 * origin (the API enforces it as well).
 */
export const SESSION_COOKIE = "rtp_bs";
/** Request header the proxy sets for the current render (never trusted from the client). */
export const SESSION_HEADER = "x-rtp-session";
/** Re-mint when less than this is left. */
const REFRESH_MS = 2 * 60_000;

export type StoredSession = { o: string; t: string; e: number };

export function encodeSession(s: StoredSession): string {
  return Buffer.from(JSON.stringify(s)).toString("base64url");
}

export function decodeSession(raw: string | undefined): StoredSession | null {
  if (!raw || raw.length > 4096) return null;
  try {
    const v = JSON.parse(Buffer.from(raw, "base64url").toString("utf8"));
    if (
      typeof v?.o === "string" &&
      typeof v?.t === "string" &&
      v.t.startsWith("rtp_ps_") &&
      typeof v?.e === "number"
    ) {
      return v as StoredSession;
    }
  } catch {
    // Malformed cookie: mint a new session.
  }
  return null;
}

/** A stored session usable for this site now. */
export function usableSession(
  s: StoredSession | null,
  site: Pick<Site, "origin">,
  now = Date.now(),
): s is StoredSession {
  return !!s && s.o === site.origin && s.e * 1000 - now > REFRESH_MS;
}

export async function mintSession(
  site: Pick<Site, "origin" | "publishable_key">,
  clientIp?: string,
): Promise<StoredSession> {
  const out = await apiFetch<{ token: string; expires_at: string }>(
    "/v1/public/sessions",
    {
      method: "POST",
      body: { publishable_key: site.publishable_key },
      origin: site.origin,
      clientIp,
    },
  );
  return {
    o: site.origin,
    t: out.token,
    e: Math.floor(new Date(out.expires_at).getTime() / 1000),
  };
}

export function sessionCookieOptions(site: Pick<Site, "origin">) {
  return {
    httpOnly: true,
    sameSite: "lax" as const,
    secure: site.origin.startsWith("https:"),
    path: "/",
    maxAge: 30 * 60,
  };
}
