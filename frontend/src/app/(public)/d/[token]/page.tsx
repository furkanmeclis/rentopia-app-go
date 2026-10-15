import type { Metadata } from "next";
import { Suspense } from "react";

import { Loading } from "@/components/common/loading";
import { RemoteApprovalPage } from "@/features/contracts/components/remote-approval-page";

// The token is a bearer credential: keep the page out of indexes and do
// not leak it through the Referer header.
export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function Page({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  return (
    <Suspense fallback={<Loading />}>
      <RemoteApprovalPage token={token} />
    </Suspense>
  );
}
