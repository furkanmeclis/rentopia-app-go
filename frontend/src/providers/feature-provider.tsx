"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";

import { hasFeature, type FeatureKey } from "@/config/features";

type FeatureContextValue = {
  /** Enabled plan features of the current organization; `null` = unknown. */
  features: readonly string[] | null;
  /** True when the feature (or any of several) is enabled. */
  hasFeature: (feature: FeatureKey | readonly FeatureKey[]) => boolean;
};

const ALLOW_ALL: FeatureContextValue = {
  features: null,
  hasFeature: () => true,
};

const FeatureContext = createContext<FeatureContextValue>(ALLOW_ALL);

/**
 * Plan features of the current organization (TEC-674). The tenant shell
 * provides them; outside it (platform, account) nothing is gated.
 */
export function FeatureProvider({
  features,
  children,
}: {
  features: readonly string[] | null;
  children: ReactNode;
}) {
  const value = useMemo<FeatureContextValue>(
    () => ({
      features,
      hasFeature: (feature) => hasFeature(features, feature),
    }),
    [features],
  );
  return (
    <FeatureContext.Provider value={value}>{children}</FeatureContext.Provider>
  );
}

/** Plan feature access of the current organization. */
export function useFeatures(): FeatureContextValue {
  return useContext(FeatureContext);
}

/** Whether the current organization's plan enables the feature. */
export function useFeature(feature: FeatureKey | readonly FeatureKey[]) {
  return useFeatures().hasFeature(feature);
}
