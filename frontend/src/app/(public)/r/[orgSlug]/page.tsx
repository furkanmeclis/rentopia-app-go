import type { Metadata } from "next";
import { Suspense } from "react";

import { Loading } from "@/components/common/loading";
import { PortalHome } from "@/features/customer-portal";

// Customer portal (TEC-715): personal data behind a customer session; keep
// it out of indexes and referrers.
export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function Page({
  params,
}: {
  params: Promise<{ orgSlug: string }>;
}) {
  const { orgSlug } = await params;
  return (
    <Suspense fallback={<Loading />}>
      <PortalHome orgSlug={orgSlug} />
    </Suspense>
  );
}
