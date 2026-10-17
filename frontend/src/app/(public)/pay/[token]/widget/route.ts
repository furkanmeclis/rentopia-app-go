import { NextRequest, NextResponse } from "next/server";

import {
  resolveWidgetReturnSite,
  validateWidgetReturn,
  widgetReturnCookie,
} from "@/lib/widget-payment-return";

/** Enter the hosted payment page in a top window with a verified return destination. */
export async function GET(
  request: NextRequest,
  context: { params: Promise<{ token: string }> },
) {
  const { token } = await context.params;
  const handoff = request.nextUrl.searchParams.get("handoff");
  const destination = await validateWidgetReturn(
    handoff,
    token,
    resolveWidgetReturnSite,
  );
  if (!destination)
    return new NextResponse("Invalid payment return", {
      status: 400,
      headers: { "Cache-Control": "no-store" },
    });
  const response = NextResponse.redirect(
    new URL(`/pay/${encodeURIComponent(token)}`, request.url),
  );
  response.headers.set("Cache-Control", "private, no-store");
  response.headers.set("Referrer-Policy", "no-referrer");
  response.cookies.set(widgetReturnCookie(token), handoff!, {
    httpOnly: true,
    sameSite: "lax",
    secure:
      process.env.NODE_ENV === "production" ||
      request.nextUrl.protocol === "https:",
    path: `/pay/${encodeURIComponent(token)}`,
    maxAge: 1800,
  });
  return response;
}
