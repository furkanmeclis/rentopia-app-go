import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Extra = Schemas["Extra"];
export type ExtraPage = Schemas["ExtraPage"];
export type ExtraFacets = Schemas["ExtraFacets"];
export type ExtraCreateRequest = Schemas["ExtraCreateRequest"];
export type ExtraPatchRequest = Schemas["ExtraPatchRequest"];
export type ExtraType = Extra["extra_type"];
export type ChargeType = Extra["charge_type"];
export type ExtraStatus = Extra["status"];

export type ExtraPriceMatrix = Schemas["ExtraPriceMatrix"];
export type ExtraProductPrice = Schemas["ExtraProductPrice"];
export type ExtraPriceRow = Schemas["ExtraPriceRow"];
export type ExtraPricesRequest = Schemas["ExtraPricesRequest"];
export type ExtraPriceHistoryPage = Schemas["ExtraPriceHistoryPage"];
export type ExtraPriceHistoryEntry = ExtraPriceHistoryPage["items"][number];

export type ExtraStockRow = Schemas["ExtraStockRow"];
export type ExtraStockRequest = Schemas["ExtraStockRequest"];
export type ExtraStockResult = Schemas["ExtraStockResult"];

export type EquipmentAllocation = Schemas["EquipmentAllocation"];
export type EquipmentAllocationPage = Schemas["EquipmentAllocationPage"];
export type EquipmentReserveRequest = Schemas["EquipmentReserveRequest"];
export type AllocationKind = EquipmentAllocation["kind"];
export type AllocationStatus = EquipmentAllocation["status"];

export type ExtrasAvailability = Schemas["ExtrasAvailability"];
export type ExtrasAvailabilityRequest = Schemas["ExtrasAvailabilityRequest"];

export type ExtraActivityPage = Schemas["FleetActivityPage"];

/** List query of `GET /v1/tenant/extras` (multi-values comma-joined). */
export type ListExtrasParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  extra_type?: string;
  charge_type?: string;
  status?: string;
  is_physical?: "true" | "false";
  channel?: string;
};

/** List query of `GET /v1/tenant/equipment-allocations`. */
export type ListAllocationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  extra?: string;
  branch?: string;
  status?: string;
  kind?: string;
};
