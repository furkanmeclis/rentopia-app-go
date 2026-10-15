import type { Metadata } from "next";
import { Suspense } from "react";

import { Loading } from "@/components/common/loading";
import { PortalReservationPage } from "@/features/customer-portal";

export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function Page({
  params,
}: {
  params: Promise<{ orgSlug: string; uuid: string }>;
}) {
  const { orgSlug, uuid } = await params;
  return (
    <Suspense fallback={<Loading />}>
      <PortalReservationPage orgSlug={orgSlug} uuid={uuid} />
    </Suspense>
  );
}
