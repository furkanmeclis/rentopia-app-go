import type { ReactNode } from "react";

import { FeatureGate } from "@/features/tenant/components/feature-gate";

/**
 * E-invoice settings belong to e-document sending, a plan feature (TEC-674);
 * a pure settings form, so it is replaced. Invoice records stay in Finans.
 */
export default function Layout({ children }: { children: ReactNode }) {
  return (
    <FeatureGate feature="einvoice" mode="replace">
      {children}
    </FeatureGate>
  );
}
