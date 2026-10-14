import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type VehicleBlock = Schemas["VehicleBlock"];
export type VehicleBlockPage = Schemas["VehicleBlockPage"];
export type VehicleBlockResult = Schemas["VehicleBlockResult"];
export type VehicleBlockRequest = Schemas["VehicleBlockRequest"];
export type VehicleBlockPatchRequest = Schemas["VehicleBlockPatchRequest"];
export type VehicleBlockReleaseRequest = Schemas["VehicleBlockReleaseRequest"];
export type BlockKind = VehicleBlock["kind"];
export type BlockState = VehicleBlock["state"];

export type Transfer = Schemas["Transfer"];
export type TransferPage = Schemas["TransferPage"];
export type TransferFacets = Schemas["TransferFacets"];
export type TransferCreateRequest = Schemas["TransferCreateRequest"];
export type TransferPatchRequest = Schemas["TransferPatchRequest"];
export type TransferActionRequest = Schemas["TransferActionRequest"];
export type TransferAssignees = Schemas["TransferAssignees"];
export type TransferStatus = Transfer["status"];

export type OneWayRule = Schemas["OneWayRule"];
export type OneWayRulePage = Schemas["OneWayRulePage"];
export type OneWayRuleFacets = Schemas["OneWayRuleFacets"];
export type OneWayRuleCreateRequest = Schemas["OneWayRuleCreateRequest"];
export type OneWayRulePatchRequest = Schemas["OneWayRulePatchRequest"];

/** List query of `GET /v1/tenant/vehicles/{uuid}/blocks`. */
export type ListBlocksParams = {
  limit: number;
  offset: number;
  state?: string;
  kind?: string;
};

/** List query of `GET /v1/tenant/transfers` (multi-values comma-joined). */
export type ListTransfersParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  from_branch?: string;
  to_branch?: string;
  vehicle?: string;
};

/** List query of `GET /v1/tenant/one-way-rules`. */
export type ListOneWayRulesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  from_branch?: string;
  to_branch?: string;
  allowed?: "true" | "false";
  is_active?: "true" | "false";
};

export type TransferAction = "start" | "complete" | "cancel";
