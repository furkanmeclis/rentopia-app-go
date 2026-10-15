import { cookies } from "next/headers";

import { fetchUpstream } from "@/lib/server/upstream";

/**
 * Customer portal session (TEC-715). The Go API issues an opaque token on
 * `POST /v1/customer/{orgSlug}/auth/verify`; the BFF keeps it in an
 * HttpOnly, Secure, SameSite=Lax cookie scoped to that organization's API
 * path (`/api/v1/customer/{orgSlug}`), strips it from the browser answer and
 * forwards it upstream as `X-Customer-Session`. It is never the staff
 * session: customer paths get no staff Bearer token.
 */
export const CUSTOMER_SESSION_COOKIE = "customer_session";
const CUSTOMER_SESSION_HEADER = "X-Customer-Session";

const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,62}$/;

/** `customer/{orgSlug}/...` paths. */
export function isCustomerPath(segments: string[]): boolean {
  return segments[0] === "customer" && SLUG_RE.test(segments[1] ?? "");
}

function cookiePath(slug: string) {
  return `/api/v1/customer/${slug}`;
}

function cookieOptions(slug: string, maxAge: number) {
  return {
    httpOnly: true,
    // Browsers treat http://localhost as a secure context, so Secure works
    // in local development too.
    secure: true,
    sameSite: "lax" as const,
    path: cookiePath(slug),
    maxAge,
  };
}

/** Forwards the organization's customer session token, if any. */
export async function attachCustomerSession(headers: Headers) {
  const jar = await cookies();
  const token = jar.get(CUSTOMER_SESSION_COOKIE)?.value;
  if (token) headers.set(CUSTOMER_SESSION_HEADER, token);
}

async function clearCustomerSession(slug: string) {
  const jar = await cookies();
  jar.set(CUSTOMER_SESSION_COOKIE, "", cookieOptions(slug, 0));
}

type VerifyEnvelope = {
  success?: boolean;
  data?: {
    session_token?: string;
    session?: { hard_expires_at?: string };
  } & Record<string, unknown>;
  meta?: unknown;
};

function parse(body: ArrayBuffer): VerifyEnvelope | null {
  try {
    const text = new TextDecoder().decode(body);
    return text ? (JSON.parse(text) as VerifyEnvelope) : null;
  } catch {
    return null;
  }
}

/** Response headers forwarded on customer routes. */
function responseHeaders(upstream: Headers): Headers {
  const out = new Headers();
  for (const key of [
    "content-type",
    "cache-control",
    "retry-after",
    "x-request-id",
    "referrer-policy",
  ]) {
    const value = upstream.get(key);
    if (value) out.set(key, value);
  }
  return out;
}

/**
 * Buffered proxy of a customer route: moves a new session token into the
 * cookie (and out of the body), clears the cookie on sign-out and on 401.
 */
export async function proxyCustomer(
  segments: string[],
  pathWithQuery: string,
  method: string,
  headers: Headers,
  body: BodyInit | null,
): Promise<Response> {
  const slug = segments[1]!;
  const tail = segments.slice(2).join("/");
  await attachCustomerSession(headers);
  const result = await fetchUpstream(pathWithQuery, { method, headers, body });

  let responseBody: ArrayBuffer | string = result.body;
  if (tail === "auth/verify" && result.status === 200) {
    const envelope = parse(result.body);
    const token = envelope?.data?.session_token;
    if (envelope?.data && token) {
      const hard = Date.parse(envelope.data.session?.hard_expires_at ?? "");
      const maxAge = Number.isFinite(hard)
        ? Math.max(60, Math.floor((hard - Date.now()) / 1000))
        : 2 * 60 * 60;
      const jar = await cookies();
      jar.set(CUSTOMER_SESSION_COOKIE, token, cookieOptions(slug, maxAge));
      const data = { ...envelope.data };
      delete data.session_token;
      responseBody = JSON.stringify({ ...envelope, data });
    }
  }
  if (
    result.status === 401 ||
    (tail === "auth/logout" && result.status < 400)
  ) {
    await clearCustomerSession(slug);
  }
  return new Response(responseBody, {
    status: result.status,
    headers: responseHeaders(result.headers),
  });
}

/** Approval / pay evidence on sign-in: the device's user agent. */
export function wantsCustomerUserAgent(segments: string[], method: string) {
  const tail = segments.slice(2).join("/");
  return (
    method.toUpperCase() === "POST" &&
    (tail === "auth/start" || tail === "auth/verify")
  );
}
