"use client";

import { useQuery } from "@tanstack/react-query";

import { profitabilityService } from "@/features/profitability/services/profitability.service";
import type { ProfitabilityParams } from "@/features/profitability/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";

export const profitabilityKeys = {
  all: (slug: string) => ["tenant", slug, "profitability"] as const,
  report: (slug: string, params: ProfitabilityParams) =>
    [...profitabilityKeys.all(slug), "report", params] as const,
  vehicle: (slug: string, uuid: string, params: ProfitabilityParams) =>
    [...profitabilityKeys.all(slug), "vehicle", uuid, params] as const,
};

/** The fleet report; keeps the previous answer while filters change. */
export function useProfitabilityReport(params: ProfitabilityParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: profitabilityKeys.report(slug, params),
    queryFn: () => profitabilityService.report(slug, params),
    placeholderData: (previous) => previous,
  });
}

/** One vehicle (vehicle detail "Kârlılık" tab). */
export function useVehicleProfitability(
  uuid: string,
  params: ProfitabilityParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: profitabilityKeys.vehicle(slug, uuid, params),
    queryFn: () => profitabilityService.vehicle(slug, uuid, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}
