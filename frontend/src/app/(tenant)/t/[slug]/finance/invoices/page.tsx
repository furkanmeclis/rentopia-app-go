import { Suspense } from "react";

import { InvoicesPage } from "@/features/invoices";

export default function Page() {
  return (
    <Suspense fallback={null}>
      <InvoicesPage />
    </Suspense>
  );
}
