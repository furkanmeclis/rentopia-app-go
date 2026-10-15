import { Suspense } from "react";

import { Loading } from "@/components/common/loading";
import { PayPage } from "@/features/payments";

export default async function Page({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  return (
    <Suspense fallback={<Loading />}>
      <PayPage token={token} />
    </Suspense>
  );
}
