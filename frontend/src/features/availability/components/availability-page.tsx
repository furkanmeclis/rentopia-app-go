"use client";

import { useCallback, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AvailabilityResultsTable } from "@/features/availability/components/availability-results-table";
import { AvailabilitySearchForm } from "@/features/availability/components/availability-search-form";
import { HoldDialog } from "@/features/availability/components/hold-dialog";
import { HoldsTable } from "@/features/availability/components/holds-table";
import { useAvailabilitySearch } from "@/features/availability/hooks/use-availability";
import type {
  AvailabilityProduct,
  AvailabilitySearchRequest,
} from "@/features/availability/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Müsaitlik": capacity per rentable product for a pick-up / return branch,
 * a date-time range and a sales channel (the availability engine's
 * explainable breakdown), "Opsiyon koy" on a product, and the active holds.
 */
export function AvailabilityPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [request, setRequest] = useState<AvailabilitySearchRequest | null>(
    null,
  );
  const [holdTarget, setHoldTarget] = useState<AvailabilityProduct | null>(
    null,
  );
  const search = useAvailabilitySearch(request);
  const items = search.data?.items ?? [];
  const echo = search.data?.request;

  const onHold = useCallback((product: AvailabilityProduct) => {
    setHoldTarget(product);
  }, []);

  return (
    <EntityPage
      title={t("availability.title")}
      description={t("availability.description")}
      permission={permissions.tenant.availability.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("availability.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("availability.title") },
      ]}
    >
      <EntitySectionCard title={t("availability.search_title")}>
        <AvailabilitySearchForm
          pending={search.isFetching}
          onSearch={setRequest}
        />
      </EntitySectionCard>

      <EntitySectionCard
        title={t("availability.results_title")}
        badge={request ? items.length : undefined}
      >
        {request ? (
          <div className="space-y-3">
            {echo ? (
              <p className="text-muted-foreground text-sm">
                {t("availability.results_summary", {
                  pickup: echo.pickup_branch.code,
                  return: echo.return_branch.code,
                  ready: datetime(echo.ready_at, undefined, locale),
                  prep: echo.prep_minutes,
                })}
              </p>
            ) : null}
            <AvailabilityResultsTable
              items={items}
              isLoading={search.isLoading}
              isError={search.isError}
              onRetry={() => void search.refetch()}
              onHold={onHold}
            />
          </div>
        ) : (
          <EmptyState
            title={t("availability.start_title")}
            description={t("availability.start_description")}
          />
        )}
      </EntitySectionCard>

      <EntitySectionCard title={t("availability.holds.title")}>
        <HoldsTable />
      </EntitySectionCard>

      <HoldDialog
        product={holdTarget}
        request={request}
        onOpenChange={(open) => {
          if (!open) setHoldTarget(null);
        }}
        onPickProduct={(productUuid) => {
          const next = items.find((item) => item.product.uuid === productUuid);
          setHoldTarget(next ?? null);
        }}
        onPickRange={(startsAt, endsAt) => {
          setHoldTarget(null);
          setRequest((current) =>
            current
              ? { ...current, starts_at: startsAt, ends_at: endsAt }
              : current,
          );
        }}
      />
    </EntityPage>
  );
}
