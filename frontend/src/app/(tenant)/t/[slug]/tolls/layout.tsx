import type { ReactNode } from "react";

import { FeatureGate } from "@/features/tenant/components/feature-gate";

/**
 * Toll passes (HGS) are a plan feature (TEC-674): without it the page stays
 * readable under a banner; changes are refused.
 */
export default function Layout({ children }: { children: ReactNode }) {
  return <FeatureGate feature={"hgs"}>{children}</FeatureGate>;
}
