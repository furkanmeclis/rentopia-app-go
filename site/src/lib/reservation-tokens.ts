import "server-only";

import { cookies, headers } from "next/headers";

import { embedContext } from "./embed-server";
import { openEmbed, sealEmbed } from "./embed";
import { PATH_HEADER } from "./request";
import type { Site } from "./types";

/**
 * Reservation access tokens (public API: shown once at booking, needed for
 * status and payment) kept in an httpOnly cookie of the site host, so the
 * booker can come back to /reservation/{reference} on this device. Other
 * devices use the customer portal (reference + one-time code).
 */
const COOKIE = "rtp_rt";
const MAX = 5;

type Stored = { o: string; r: Record<string, string> };

async function read(site: Site): Promise<Record<string, string>> {
  const raw = (await cookies()).get(COOKIE)?.value;
  if (!raw || raw.length > 4096) return {};
  try {
    const v = JSON.parse(
      Buffer.from(raw, "base64url").toString("utf8"),
    ) as Stored;
    if (v?.o !== site.origin || typeof v.r !== "object" || !v.r) return {};
    return Object.fromEntries(
      Object.entries(v.r).filter(
        ([k, t]) => typeof t === "string" && /^[A-Za-z0-9-]{1,40}$/.test(k),
      ),
    );
  } catch {
    return {};
  }
}

export async function reservationToken(
  site: Site,
  reference: string,
): Promise<string | null> {
  const path = (await headers()).get(PATH_HEADER) || "/";
  const state = openEmbed(
    new URL(path, site.origin).searchParams.get("s"),
    site.origin,
  );
  if (state?.reservation?.reference === reference)
    return state.reservation.token;
  if (await embedContext()) return null;
  return (await read(site))[reference] ?? null;
}

export async function deviceReservations(site: Site): Promise<string[]> {
  return Object.keys(await read(site)).reverse();
}

/** Server actions only (sets a cookie). */
export async function rememberReservation(
  site: Site,
  reference: string,
  token: string,
) {
  const embed = await embedContext();
  if (embed) {
    const state = openEmbed(embed.fields.s, site.origin);
    if (!state) throw new Error("Embed session expired");
    return sealEmbed({ ...state, reservation: { reference, token } });
  }
  const all = await read(site);
  delete all[reference];
  all[reference] = token;
  const kept = Object.fromEntries(Object.entries(all).slice(-MAX));
  (await cookies()).set(
    COOKIE,
    Buffer.from(
      JSON.stringify({ o: site.origin, r: kept } satisfies Stored),
    ).toString("base64url"),
    {
      httpOnly: true,
      sameSite: "lax",
      secure: site.origin.startsWith("https:"),
      path: "/",
      maxAge: 60 * 60 * 24 * 90,
    },
  );
}
