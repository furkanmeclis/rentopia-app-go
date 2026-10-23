import type { ReactNode } from "react";

import { FeatureGate } from "@/features/tenant/components/feature-gate";

/**
 * The channel manager needs a channel plan feature (TEC-674): without it the page stays
 * readable under a banner; changes are refused.
 */
export default function Layout({ children }: { children: ReactNode }) {
  return (
    <FeatureGate feature={["channel.yolcu360", "channel.obilet"]}>
      {children}
    </FeatureGate>
  );
}
