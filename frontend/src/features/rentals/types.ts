import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Rental = Schemas["Rental"];
export type RentalListItem = Schemas["RentalListItem"];
export type RentalPage = Schemas["EnvelopeRentalPage"]["data"];
export type RentalFacets = Schemas["RentalFacets"];
export type RentalFacetValue = Schemas["RentalFacetValue"];
export type RentalEvent = Schemas["RentalEvent"];
export type RentalEventPage = Schemas["EnvelopeRentalEventPage"]["data"];
export type RentalLeg = Schemas["RentalLeg"];
export type RentalDriver = Schemas["RentalDriver"];
export type RentalCharge = Schemas["RentalCharge"];
export type RentalStatus = Rental["status"];
export type ChargeKind = RentalCharge["kind"];
export type ChargeStatus = RentalCharge["status"];

export type RentalCreateRequest = Schemas["RentalCreateRequest"];
export type RentalHandoverRequest = Schemas["RentalHandoverRequest"];
export type RentalReturnRequest = Schemas["RentalReturnRequest"];
export type RentalCompleteReturnRequest =
  Schemas["RentalCompleteReturnRequest"];
export type RentalExtendRequest = Schemas["RentalExtendRequest"];
export type RentalSwapRequest = Schemas["RentalSwapRequest"];
export type RentalChargeRequest = Schemas["RentalChargeRequest"];
export type RentalChargeOverrideRequest =
  Schemas["RentalChargeOverrideRequest"];

export type OperationTask = Schemas["OperationTask"];
export type OperationTaskPage = Schemas["EnvelopeOperationTaskPage"]["data"];
export type OperationTaskFacets =
  Schemas["EnvelopeOperationTaskFacets"]["data"];
export type OperationTaskType = OperationTask["task_type"];
export type OperationTaskStatus = OperationTask["status"];

/** List query of `GET /v1/tenant/rentals` (multi-values comma-joined). */
export type ListRentalsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  branch?: string;
  product?: string;
  vehicle?: string;
  customer?: string;
  reservation?: string;
  overdue?: string;
};

/** List query of `GET /v1/tenant/operation-tasks`. */
export type ListOperationTasksParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  task_type?: string;
  branch?: string;
  rental?: string;
  vehicle?: string;
  assignee?: string;
};
