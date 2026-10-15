import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Vehicle = Schemas["Vehicle"];
export type VehiclePage = Schemas["VehiclePage"];
export type VehicleFacets = Schemas["VehicleFacets"];
export type VehicleCreateRequest = Schemas["VehicleCreateRequest"];
export type VehiclePatchRequest = Schemas["VehiclePatchRequest"];
export type VehicleStatusRequest = Schemas["VehicleStatusRequest"];
export type VehicleEvent = Schemas["VehicleEvent"];
export type VehicleEventPage = Schemas["VehicleEventPage"];
export type VehicleGroups = Schemas["VehicleGroups"];
export type VehicleMembership = Schemas["VehicleMembership"];
export type VehicleGroupSuggestion = Schemas["VehicleGroupSuggestion"];
export type VehicleKasko = Schemas["VehicleKasko"];
export type VehicleValuation = Schemas["VehicleValuation"];
export type ProductVehicles = Schemas["ProductVehicles"];
export type RentalGroupMember = Schemas["RentalGroupMember"];
export type RentalGroupMembers = Schemas["RentalGroupMembers"];
export type RentalGroupAddEligible = Schemas["RentalGroupAddEligible"];
export type MembershipBlockingAllocation =
  Schemas["MembershipBlockingAllocation"];
export type VehicleStatus = Vehicle["status"];
export type VehicleOwnership = Vehicle["ownership"];

/** List query of `GET /v1/tenant/vehicles` (multi-values comma-joined). */
export type ListVehiclesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  rental_group?: string;
  segment?: string;
  home_branch?: string;
  current_branch?: string;
  ownership?: string;
  pool?: string;
  is_active?: "true" | "false";
  /** Next document to renew: expired / expiring / valid / none. */
  document_status?: string;
};
