import type { NextRequest } from "next/server";

import { ApiError, apiFetch } from "@/lib/api";
import { clientIpFrom } from "@/lib/request";
import { resolveSite } from "@/lib/resolve";
import { buildSitemapXml, type SitemapEntry } from "@/lib/sitemap";

export const dynamic = "force-dynamic";

/**
 * sitemap.xml of the request host's published site (TEC-785): the API
 * lists the public paths, the site writes absolute URLs on the site's
 * origin. Unknown hosts and unpublished sites answer 404.
 */
export async function GET(request: NextRequest) {
  const ip = clientIpFrom(request.headers);
  const site = await resolveSite(request.headers.get("host"), ip).catch(
    () => null,
  );
  if (!site) return new Response(null, { status: 404 });
  let entries: SitemapEntry[];
  try {
    entries = await apiFetch<SitemapEntry[]>(
      `/v1/public/site/sitemap?host=${encodeURIComponent(site.host)}`,
      { clientIp: ip },
    );
  } catch (err) {
    return new Response(null, {
      status: err instanceof ApiError && err.status === 404 ? 404 : 503,
    });
  }
  return new Response(buildSitemapXml(site.origin, entries), {
    headers: {
      "Content-Type": "application/xml; charset=utf-8",
      "Cache-Control": "public, max-age=300",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
