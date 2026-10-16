import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ProfitabilityFigures = Schemas["VehicleProfitabilityFigures"];
export type ProfitabilityRow = Schemas["VehicleProfitabilityRow"];
export type ProfitabilityTrendPoint = Schemas["VehicleProfitabilityTrendPoint"];
export type ProfitabilityMethod = Schemas["VehicleProfitabilityMethod"];
export type ProfitabilityReport = Schemas["VehicleProfitabilityReport"];
export type VehicleProfitability = Schemas["VehicleProfitabilityVehicle"];

export type ProfitabilityBasis = ProfitabilityMethod["basis"];
export type ProfitabilityAllocation = ProfitabilityMethod["allocation"];

/** Query of the report and the vehicle tab (`/v1/tenant/reports/...`). */
export type ProfitabilityParams = {
  from?: string;
  to?: string;
  basis?: ProfitabilityBasis;
  allocation?: ProfitabilityAllocation;
  branch?: string;
  rental_group?: string;
  q?: string;
};
