import {
  createCipheriv,
  createDecipheriv,
  createHash,
  randomBytes,
} from "node:crypto";

import type { StoredSession } from "./session.ts";

export type EmbedState = {
  origin: string;
  expires: number;
  session: StoredSession;
  reservation?: { reference: string; token: string };
  payment?: string;
};
export const EMBED_HEADER = "x-rtp-embed";
export const EMBED_TTL = 30 * 60;

function key(secret = process.env.SITE_EMBED_SECRET): Buffer {
  if (!secret || secret.length < 32)
    throw new Error("SITE_EMBED_SECRET must contain at least 32 characters");
  return createHash("sha256").update(`rentopia/embed/v1:${secret}`).digest();
}

/** Authenticated encryption keeps API bearer tokens out of the browser URL. */
export function sealEmbed(state: EmbedState, secret?: string): string {
  const iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", key(secret), iv);
  const encrypted = Buffer.concat([
    cipher.update(JSON.stringify(state), "utf8"),
    cipher.final(),
  ]);
  return `e1.${Buffer.concat([iv, cipher.getAuthTag(), encrypted]).toString("base64url")}`;
}

export function openEmbed(
  raw: string | null,
  origin: string,
  secret?: string,
  now = Date.now(),
): EmbedState | null {
  if (!raw || !/^e1\.[A-Za-z0-9_-]+$/.test(raw) || raw.length > 6000)
    return null;
  try {
    const bytes = Buffer.from(raw.slice(3), "base64url");
    const decipher = createDecipheriv(
      "aes-256-gcm",
      key(secret),
      bytes.subarray(0, 12),
    );
    decipher.setAuthTag(bytes.subarray(12, 28));
    const state = JSON.parse(
      Buffer.concat([
        decipher.update(bytes.subarray(28)),
        decipher.final(),
      ]).toString("utf8"),
    ) as EmbedState;
    if (
      state.origin !== origin ||
      !Number.isFinite(state.expires) ||
      state.expires * 1000 <= now ||
      state.expires * 1000 > now + EMBED_TTL * 1000 ||
      state.session?.o !== origin ||
      typeof state.session.t !== "string" ||
      !state.session.t.startsWith("rtp_ps_") ||
      !Number.isFinite(state.session.e) ||
      state.expires > state.session.e ||
      state.session.e * 1000 <= now
    )
      return null;
    return state;
  } catch {
    return null;
  }
}

export { allowedParent, embedFrameHeaders, widgetOrigin } from "./widget.ts";

export type EmbedContext = { fields: Record<string, string> };
export function embedPath(path: string, embed?: EmbedContext | null): string {
  if (!embed) return path;
  const url = new URL(path, "https://site.invalid");
  if (url.origin !== "https://site.invalid") return path;
  if (!url.pathname.startsWith("/embed/") && url.pathname !== "/embed")
    url.pathname = `/embed${url.pathname === "/" ? "" : url.pathname}`;
  for (const [k, v] of Object.entries(embed.fields))
    if (v && !url.searchParams.has(k)) url.searchParams.set(k, v);
  return url.pathname + url.search;
}
