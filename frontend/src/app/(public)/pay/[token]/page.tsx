import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { Suspense } from "react";

import { Loading } from "@/components/common/loading";
import { PayPage } from "@/features/payments";
import {
  resolveWidgetReturnSite,
  validateWidgetReturn,
  widgetReturnCookie,
} from "@/lib/widget-payment-return";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ token: string }>;
  searchParams: Promise<{ returned?: string }>;
}) {
  const { token } = await params;
  if ((await searchParams).returned === "1") {
    const handoff = (await cookies()).get(widgetReturnCookie(token))?.value;
    const destination = await validateWidgetReturn(
      handoff,
      token,
      resolveWidgetReturnSite,
    );
    if (destination) {
      // Ask the provider for a verified result before returning to the reservation.
      const api = (process.env.API_URL || "http://127.0.0.1:8080/v1")
        .replace(/\/+$/, "")
        .replace(/\/v1$/, "");
      await fetch(
        `${api}/v1/public/pay/${encodeURIComponent(token)}?verify=1`,
        {
          cache: "no-store",
          redirect: "error",
          signal: AbortSignal.timeout(5000),
        },
      ).catch(() => null);
      redirect(destination);
    }
  }
  return (
    <Suspense fallback={<Loading />}>
      <PayPage token={token} />
    </Suspense>
  );
}
