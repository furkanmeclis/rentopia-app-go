"use client";

import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { EntityPage } from "@/components/entity";
import { FEATURE_LABEL_KEYS, type FeatureKey } from "@/config/features";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useFeatures } from "@/providers/feature-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Plan-gated screen (TEC-674). With the feature on it renders the page as
 * is. Without it:
 *
 * - `readonly` (default): the page still renders, so nothing a company
 *   recorded becomes invisible after a downgrade, under a banner saying the
 *   feature is not in the plan; the pages hide their create / send / sync
 *   actions through `useFeature` and the API refuses changes (403
 *   FEATURE_NOT_ENABLED, same message in toasts).
 * - `replace`: a pure settings form is replaced by an empty state.
 */
export function FeatureGate({
  feature,
  mode = "readonly",
  children,
}: {
  feature: FeatureKey | FeatureKey[];
  mode?: "readonly" | "replace";
  children: ReactNode;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { hasFeature } = useFeatures();
  if (hasFeature(feature)) return <>{children}</>;

  const keys = typeof feature === "string" ? [feature] : feature;
  const label = keys.map((key) => t(FEATURE_LABEL_KEYS[key])).join(" / ");

  if (mode === "readonly") {
    return (
      <>
        <EmptyState
          className="mb-6 py-6"
          title={t("features.gate.readonly_title", { feature: label })}
          description={t("features.gate.readonly_description")}
        />
        {children}
      </>
    );
  }

  return (
    <EntityPage
      title={label}
      description={t("features.gate.page_description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label },
      ]}
    >
      <EmptyState
        title={t("features.gate.title")}
        description={t("errors.codes.FEATURE_NOT_ENABLED")}
      />
    </EntityPage>
  );
}
