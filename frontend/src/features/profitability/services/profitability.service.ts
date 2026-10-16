import type {
  ProfitabilityParams,
  ProfitabilityReport,
  VehicleProfitability,
} from "@/features/profitability/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/reports/vehicle-profitability";

/** Vehicle profitability report (TEC-727) of one organization. */
export const profitabilityService = {
  report(slug: string, params: ProfitabilityParams) {
    return tenantRequest<ProfitabilityReport>(slug, "GET", BASE, {
      query: params,
    });
  },
  vehicle(slug: string, uuid: string, params: ProfitabilityParams) {
    return tenantRequest<VehicleProfitability>(
      slug,
      "GET",
      `${BASE}/vehicles/${uuid}`,
      { query: params },
    );
  },
};
