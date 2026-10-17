import type { NextRequest } from "next/server";

import { apiOrigin } from "@/lib/api";
import { PREVIEW_COOKIE, previewToken } from "@/lib/preview";
import { clientIpFrom } from "@/lib/request";
import { resolveSite } from "@/lib/resolve";

/**
 * Same-origin images for the site: only this host's site images, its
 * library media and its organization's product images are passed through from the API. Media
 * URLs stay API-owned; the browser never sees a storage key or the API.
 */
const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  // Draft image of a preview (TEC-730): resolve with this visitor's
  // preview token and forward it; the API checks it belongs to this site.
  const preview =
    request.nextUrl.searchParams.get("draft") === "1"
      ? previewToken(request.cookies.get(PREVIEW_COOKIE)?.value)
      : "";
  const site = await resolveSite(
    request.headers.get("host"),
    clientIpFrom(request.headers),
    preview,
  ).catch(() => null);
  if (!site) return new Response(null, { status: 404 });
  const { path } = await params;
  const rel = path.join("/");
  const allowed = [
    new RegExp(`^site/assets/${site.site_uuid}/(logo|favicon|cover)$`),
    // Site library media (TEC-781): the API serves only media the active
    // publication (or the previewed draft) references.
    new RegExp(`^site/media/${site.site_uuid}/${UUID}$`),
    new RegExp(
      `^media/rental-products/${site.organization.uuid}/images/${UUID}$`,
    ),
  ];
  if (!allowed.some((re) => re.test(rel)))
    return new Response(null, { status: 404 });

  const v = request.nextUrl.searchParams.get("v");
  const qs = new URLSearchParams();
  if (v && /^\d{1,10}$/.test(v)) qs.set("v", v);
  if (preview && rel.startsWith("site/")) qs.set("preview", preview);
  const query = qs.size ? `?${qs.toString()}` : "";
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
      "Cache-Control": preview
        ? "private, no-store"
        : (upstream.headers.get("cache-control") ?? "public, max-age=3600"),
      "X-Content-Type-Options": "nosniff",
    },
  });
}
