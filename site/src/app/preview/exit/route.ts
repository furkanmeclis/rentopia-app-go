import { NextResponse } from "next/server";

import { PREVIEW_COOKIE } from "@/lib/preview";

/** Leaves a draft preview: drops the token cookie and opens the live site. */
export function GET() {
  const res = new NextResponse(null, {
    status: 303,
    headers: { Location: "/" },
  });
  res.cookies.set(PREVIEW_COOKIE, "", {
    httpOnly: true,
    secure: true,
    sameSite: "none",
    partitioned: true,
    path: "/",
    maxAge: 0,
  });
  return res;
}
