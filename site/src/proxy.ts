import { NextResponse, type NextRequest } from "next/server";

import { ApiError } from "@/lib/api";
import {
  PREVIEW_COOKIE,
  PREVIEW_HEADER,
  PREVIEW_PARAM,
  frameHeaders,
  previewCookieOptions,
  previewToken,
} from "@/lib/preview";
import { PATH_HEADER, clientIpFrom } from "@/lib/request";
import { resolveSite } from "@/lib/resolve";
import {
  SESSION_COOKIE,
  SESSION_HEADER,
  decodeSession,
  encodeSession,
  mintSession,
  sessionCookieOptions,
  usableSession,
} from "@/lib/session";

/** Internal headers the proxy owns; client copies are always dropped. */
const INTERNAL_HEADERS = [SESSION_HEADER, PATH_HEADER, PREVIEW_HEADER];

const plainPage = (status: number, title: string, body: string) => {
  const res = new NextResponse(
    `<!doctype html><html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>${title}</title><style>body{font-family:system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;padding:16px;color:#0f172a;background:#f8fafc;text-align:center}h1{font-size:1.25rem}p{color:#475569}</style></head><body><main><h1>${title}</h1><p>${body}</p></main></body></html>`,
    {
      status,
      headers: {
        "Content-Type": "text/html; charset=utf-8",
        "Cache-Control": "no-store",
      },
    },
  );
  for (const [k, v] of Object.entries(frameHeaders(null)))
    res.headers.set(k, v);
  return res;
};

/**
 * Every page request: the Host names the site (unknown host → a neutral
 * 404 that shows no organization), and visitors get a booking session for
 * this site's origin when booking is on. A preview token (query or cookie)
 * renders the draft instead; only the panel may frame such a page.
 */
export async function proxy(request: NextRequest) {
  const ip = clientIpFrom(request.headers);
  const fromQuery = previewToken(
    request.nextUrl.searchParams.get(PREVIEW_PARAM),
  );
  const preview =
    fromQuery || previewToken(request.cookies.get(PREVIEW_COOKIE)?.value);
  let site;
  try {
    site = await resolveSite(request.headers.get("host"), ip, preview);
  } catch (err) {
    if (preview && err instanceof ApiError && err.status === 401) {
      const res = plainPage(
        401,
        "Önizleme bağlantısının süresi doldu / Preview link expired",
        "Panelden yeni bir önizleme açın. Open a new preview from the panel.",
      );
      res.cookies.delete(PREVIEW_COOKIE);
      return res;
    }
    return plainPage(
      503,
      "Geçici bir sorun var / Temporarily unavailable",
      "Lütfen biraz sonra tekrar deneyin. Please try again shortly.",
    );
  }
  if (!site) {
    return plainPage(
      404,
      "Site bulunamadı / Site not found",
      "Bu adreste yayında bir site yok. There is no site at this address.",
    );
  }

  const headers = new Headers(request.headers);
  for (const h of INTERNAL_HEADERS) headers.delete(h);
  headers.set(PATH_HEADER, request.nextUrl.pathname + request.nextUrl.search);
  if (site.preview && preview) headers.set(PREVIEW_HEADER, preview);

  let fresh: ReturnType<typeof decodeSession> = null;
  if (site.booking_enabled && site.publishable_key) {
    const stored = decodeSession(request.cookies.get(SESSION_COOKIE)?.value);
    if (usableSession(stored, site)) {
      headers.set(SESSION_HEADER, stored.t);
    } else {
      try {
        fresh = await mintSession(site, ip);
        headers.set(SESSION_HEADER, fresh.t);
      } catch {
        // Booking pages show their own error; content pages still render.
      }
    }
  }

  const response = NextResponse.next({ request: { headers } });
  for (const [k, v] of Object.entries(
    frameHeaders(
      site.preview ? { panelOrigin: site.panel_origin ?? "" } : null,
    ),
  )) {
    response.headers.set(k, v);
  }
  if (site.preview) {
    response.headers.set("Cache-Control", "private, no-store");
    response.headers.set("X-Robots-Tag", "noindex");
    if (fromQuery) {
      response.cookies.set(
        PREVIEW_COOKIE,
        fromQuery,
        previewCookieOptions(fromQuery),
      );
    }
  }
  if (fresh) {
    response.cookies.set(
      SESSION_COOKIE,
      encodeSession(fresh),
      sessionCookieOptions(site),
    );
  }
  return response;
}

export const config = {
  matcher: ["/((?!_next/|health|media/|favicon.ico).*)"],
};
