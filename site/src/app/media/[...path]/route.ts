import type { NextRequest } from "next/server";

import { apiOrigin } from "@/lib/api";
import { clientIpFrom } from "@/lib/request";
import { resolveSite } from "@/lib/resolve";

/**
 * Same-origin images for the site: only this host's site images and its
 * organization's product images are passed through from the API. Media
 * URLs stay API-owned; the browser never sees a storage key or the API.
 */
const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const site = await resolveSite(
    request.headers.get("host"),
    clientIpFrom(request.headers),
  ).catch(() => null);
  if (!site) return new Response(null, { status: 404 });
  const { path } = await params;
  const rel = path.join("/");
  const allowed = [
    new RegExp(`^site/assets/${site.site_uuid}/(logo|favicon|cover)$`),
    new RegExp(
      `^media/rental-products/${site.organization.uuid}/images/${UUID}$`,
    ),
  ];
  if (!allowed.some((re) => re.test(rel)))
    return new Response(null, { status: 404 });

  const v = request.nextUrl.searchParams.get("v");
  const query = v && /^\d{1,10}$/.test(v) ? `?v=${v}` : "";
  let upstream: Response;
  try {
    upstream = await fetch(`${apiOrigin()}/v1/public/${rel}${query}`, {
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
  } catch {
    return new Response(null, { status: 502 });
  }
  if (!upstream.ok || !upstream.body)
    return new Response(null, { status: upstream.status === 404 ? 404 : 502 });
  const type = upstream.headers.get("content-type") ?? "";
  if (!type.startsWith("image/")) return new Response(null, { status: 404 });
  return new Response(upstream.body, {
    headers: {
      "Content-Type": type,
      "Cache-Control":
        upstream.headers.get("cache-control") ?? "public, max-age=3600",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
