import { Suspense } from "react";

import { DepositsPage } from "@/features/deposits";

export default function TenantDepositsPage() {
  return (
    <Suspense>
      <DepositsPage />
    </Suspense>
  );
}
