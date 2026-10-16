import { clientIpFromHeaders, fetchUpstream } from "@/lib/server/upstream";

/**
 * Public booking API (TEC-728, `/api/v1/public/{sessions,locations,…}`):
 * called by the branded site, the embeddable widget on other origins and
 * partner servers. The caller's own credential (a secret key or a booking
 * session token) is the bearer; the staff session is never attached and no
 * cookie is involved, so cross-site calls are allowed here and the API
 * checks the origin against the key (CORS answers come from the API).
 */
const PUBLIC_BOOKING_ROOTS = new Set([
  "sessions",
  "locations",
  "vehicle-subgroups",
  "extras",
  "availability",
  "quotes",
  "reservations",
]);

/** Request headers forwarded to the API (allow-list). */
const FORWARDED_REQUEST_HEADERS = [
  "accept",
  "content-type",
  "authorization",
  "idempotency-key",
  "x-reservation-token",
  "access-control-request-method",
  "access-control-request-headers",
  "user-agent",
] as const;

/** Answer headers passed back to the caller (allow-list). */
const FORWARDED_RESPONSE_HEADERS = [
  "content-type",
  "cache-control",
  "retry-after",
  "x-request-id",
  "x-ratelimit-limit",
  "idempotent-replayed",
  "vary",
  "access-control-allow-origin",
  "access-control-allow-methods",
  "access-control-allow-headers",
  "access-control-expose-headers",
  "access-control-max-age",
] as const;

export function isPublicBookingPath(segments: string[]): boolean {
  return (
    segments[0] === "public" &&
    segments.length >= 2 &&
    PUBLIC_BOOKING_ROOTS.has(segments[1]!)
  );
}

export async function proxyPublicBooking(
  pathSegments: string[],
  request: Request,
): Promise<Response> {
  const url = new URL(request.url);
  const method = request.method.toUpperCase();
  const headers = new Headers();
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  if (!headers.has("accept")) headers.set("accept", "application/json");
  // The upstream helper strips Origin; the API reads it from here.
  const origin = request.headers.get("origin");
  if (origin) headers.set("X-Forwarded-Origin", origin);
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) headers.set("X-Forwarded-For", clientIp);

  const body =
    method === "GET" || method === "HEAD" || method === "OPTIONS"
      ? null
      : await request.arrayBuffer();
  const result = await fetchUpstream(`${pathSegments.join("/")}${url.search}`, {
    method,
    headers,
    body: body && body.byteLength > 0 ? body : null,
  });
  const out = new Headers();
  for (const name of FORWARDED_RESPONSE_HEADERS) {
    const value = result.headers.get(name);
    if (value) out.set(name, value);
  }
  const empty = result.status === 204 || result.status === 304;
  return new Response(empty ? null : result.body, {
    status: result.status,
    headers: out,
  });
}
