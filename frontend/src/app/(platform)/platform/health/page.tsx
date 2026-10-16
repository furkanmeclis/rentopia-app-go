import { Suspense } from "react";

import { PlatformHealthPage } from "@/features/platform-health";

export default function PlatformHealthRoute() {
  return (
    <Suspense>
      <PlatformHealthPage />
    </Suspense>
  );
}
