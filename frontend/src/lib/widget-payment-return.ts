import { createHash, createHmac, timingSafeEqual } from "node:crypto";

type Resolved = { origin: string; organization: { uuid: string } };

export const widgetReturnCookie = (token: string) =>
  `rtp_wr_${createHash("sha256").update(token).digest("hex").slice(0, 24)}`;

/** Signed by the site server, then checked against the current host resolver. */
export async function validateWidgetReturn(
  raw: string | null | undefined,
  token: string,
  resolve: (host: string) => Promise<Resolved | null>,
  secret = process.env.SITE_EMBED_SECRET,
  now = Date.now(),
): Promise<string | null> {
  if (!secret || secret.length < 32 || !raw || raw.length > 12000) return null;
  try {
    const [version, payload, signature, extra] = raw.split(".");
    if (version !== "h1" || !payload || !signature || extra !== undefined)
      return null;
    const want = createHmac("sha256", secret)
      .update(`rentopia/widget-payment/v1:${payload}`)
      .digest();
    const got = Buffer.from(signature, "base64url");
    if (got.length !== want.length || !timingSafeEqual(want, got)) return null;
    const claims = JSON.parse(
      Buffer.from(payload, "base64url").toString("utf8"),
    );
    if (
      claims.token !== token ||
      !Number.isFinite(claims.expires) ||
      claims.expires * 1000 <= now ||
      claims.expires * 1000 > now + 30 * 60_000
    )
      return null;
    const url = new URL(claims.url);
    if (
      url.username ||
      url.password ||
      url.hash ||
      !/^\/reservation\/[A-Za-z0-9-]{1,40}$/.test(url.pathname)
    )
      return null;
    if (
      url.protocol !== "https:" &&
      !(
        url.protocol === "http:" &&
        (url.hostname === "localhost" || url.hostname.endsWith(".localhost"))
      )
    )
      return null;
    const site = await resolve(url.host);
    if (
      !site ||
      site.origin !== url.origin ||
      site.organization.uuid !== claims.organization
    )
      return null;
    return url.toString();
  } catch {
    return null;
  }
}

export async function resolveWidgetReturnSite(
  host: string,
): Promise<Resolved | null> {
  const api = (process.env.API_URL || "http://127.0.0.1:8080/v1")
    .replace(/\/+$/, "")
    .replace(/\/v1$/, "");
  const response = await fetch(
    `${api}/v1/public/site/resolve?host=${encodeURIComponent(host)}`,
    { cache: "no-store", redirect: "error", signal: AbortSignal.timeout(5000) },
  );
  if (!response.ok) return null;
  return (await response.json()).data ?? null;
}
