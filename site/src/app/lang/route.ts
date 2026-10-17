import { NextResponse, type NextRequest } from "next/server";

import { LOCALE_COOKIE, isLocale } from "@/lib/i18n";
import { safeNext } from "@/lib/request";

/** GET /lang?l=en&next=/search… : stores the language and goes back. */
export function GET(request: NextRequest) {
  const l = request.nextUrl.searchParams.get("l");
  const next = safeNext(request.nextUrl.searchParams.get("next"));
  const res = new NextResponse(null, {
    status: 303,
    headers: { Location: next },
  });
  if (isLocale(l)) {
    res.cookies.set(LOCALE_COOKIE, l, {
      path: "/",
      sameSite: "lax",
      maxAge: 60 * 60 * 24 * 365,
    });
  }
  return res;
}
