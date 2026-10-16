"use client";

import { ErrorState } from "@/components/common/error-state";
import {
  ProfitabilityFilters,
  useProfitabilityFilters,
} from "@/features/profitability/components/profitability-filters";
import {
  MethodNotes,
  ProfitabilityBreakdown,
  ProfitabilityKpis,
  ProfitabilityTrendChart,
} from "@/features/profitability/components/profitability-sections";
import { useVehicleProfitability } from "@/features/profitability/hooks/use-profitability";
import { useLocale } from "@/providers/locale-provider";

/**
 * Vehicle detail "Kârlılık" tab (TEC-727): KPIs, the monthly trend, the
 * income and cost breakdown and the method notes of one vehicle. Defaults
 * to the last 12 months.
 */
export function VehicleProfitabilityTab({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { state, setState, params } = useProfitabilityFilters("last_12_months");
  const query = useVehicleProfitability(uuid, params);
  const data = query.data;

  return (
    <div className="space-y-6">
      <ProfitabilityFilters state={state} onChange={setState} />
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("profitability.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <>
          <ProfitabilityKpis
            figures={data?.figures}
            loading={query.isLoading}
          />
          <ProfitabilityTrendChart
            trend={data?.trend ?? []}
            loading={query.isLoading}
          />
          <ProfitabilityBreakdown
            figures={data?.figures}
            categories={data?.categories}
            loading={query.isLoading}
          />
          <MethodNotes method={data?.method} from={data?.from} to={data?.to} />
        </>
      )}
    </div>
  );
}
