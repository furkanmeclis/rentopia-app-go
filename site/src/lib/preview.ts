/**
 * Draft preview (TEC-730). The panel opens the site's own address with
 * `?preview=<token>` (POST /v1/tenant/site/preview-token) in a frame. The
 * proxy keeps the token in an httpOnly cookie for the next navigations and
 * resolves the host with it: the API then answers the draft, and only for
 * a valid, unexpired token of this host's site. The site never trusts the
 * token itself; it only checks the shape before forwarding it.
 *
 * Pure helpers (no Next imports) so node --test can run them.
 */

/** Cookie that carries the preview token between navigations. */
export const PREVIEW_COOKIE = "rtp_preview";

/** Internal request header the proxy sets for server components. */
export const PREVIEW_HEADER = "x-rtp-preview";

/** Query parameter of the panel's preview link. */
export const PREVIEW_PARAM = "preview";

const TOKEN_RE =
  /^sp1\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.\d{1,12}\.[A-Za-z0-9_-]{20,100}$/;

/** The token when it has the API's shape, else "" (never forwarded). */
export function previewToken(raw: string | null | undefined): string {
  const v = (raw ?? "").trim();
  return v.length <= 200 && TOKEN_RE.test(v) ? v : "";
}

/** Unverified expiry of a well-formed token (cookie lifetime only). */
export function previewExpiry(token: string): Date | null {
  if (!previewToken(token)) return null;
  const sec = Number(token.split(".")[2]);
  return Number.isFinite(sec) ? new Date(sec * 1000) : null;
}

/**
 * Seconds the preview cookie lives: until the token expires, at most 30
 * minutes, 0 when it already expired.
 */
export function previewCookieMaxAge(token: string, now = Date.now()): number {
  const exp = previewExpiry(token);
  if (!exp) return 0;
  return Math.max(0, Math.min(1800, Math.floor((exp.getTime() - now) / 1000)));
}

/** Cookie attributes: sent inside the panel's frame (cross-site). */
export function previewCookieOptions(token: string, now = Date.now()) {
  return {
    httpOnly: true,
    secure: true,
    sameSite: "none" as const,
    partitioned: true,
    path: "/",
    maxAge: previewCookieMaxAge(token, now),
  };
}

/** An http(s) origin, or "" (only such a value reaches frame-ancestors). */
export function safeOrigin(raw: string | null | undefined): string {
  try {
    const u = new URL(raw ?? "");
    if (u.protocol !== "https:" && u.protocol !== "http:") return "";
    if (u.username || u.password) return "";
    return u.origin;
  } catch {
    return "";
  }
}

/**
 * Framing headers of a page response. Site pages are never framed, except
 * a preview page, which only the panel's origin may frame.
 */
export function frameHeaders(
  preview: { panelOrigin: string } | null,
  production = process.env.NODE_ENV === "production",
): Record<string, string> {
  const formAction = `form-action 'self' https:${production ? "" : " http://localhost:*"}`;
  const panel = preview ? safeOrigin(preview.panelOrigin) : "";
  if (panel) {
    return {
      "Content-Security-Policy": `frame-ancestors ${panel}; base-uri 'self'; ${formAction}`,
    };
  }
  return {
    "Content-Security-Policy": `frame-ancestors 'none'; base-uri 'self'; ${formAction}`,
    "X-Frame-Options": "DENY",
  };
}
