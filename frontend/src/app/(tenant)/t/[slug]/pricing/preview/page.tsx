import { Suspense } from "react";

import { PricePreviewPage } from "@/features/pricing";

export default function Page() {
  return (
    <Suspense>
      <PricePreviewPage />
    </Suspense>
  );
}
