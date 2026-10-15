import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Reservation = Schemas["Reservation"];
export type ReservationListItem = Schemas["ReservationListItem"];
export type ReservationPage = Schemas["EnvelopeReservationPage"]["data"];
export type ReservationFacets = Schemas["EnvelopeReservationFacets"]["data"];
export type ReservationFacet = Schemas["ReservationFacet"];
export type ReservationEvent = Schemas["ReservationEvent"];
export type ReservationEventPage =
  Schemas["EnvelopeReservationEventPage"]["data"];
export type ReservationDriver = Schemas["ReservationDriver"];
export type ReservationAssignment = Schemas["ReservationAssignment"];
export type ReservationPlan = Schemas["ReservationPlan"];
export type ReservationConditions = Schemas["ReservationConditions"];
export type ReservationChangeResult = Schemas["ReservationChangeResult"];
export type ReservationExtensionAlternatives =
  Schemas["ReservationExtensionAlternatives"];
export type ReservationCreateRequest = Schemas["ReservationCreateRequest"];
export type ReservationChangeQuoteRequest =
  Schemas["ReservationChangeQuoteRequest"];
export type ReservationApplyChangeRequest =
  Schemas["ReservationApplyChangeRequest"];
export type ReservationPlateRequest = Schemas["ReservationPlateRequest"];
export type ReservationVehicleRef = Schemas["ReservationVehicleRef"];
export type ReservationDriverInput = Schemas["ReservationDriverInput"];
export type ReservationDriversRequest = Schemas["ReservationDriversRequest"];
export type ReservationStatus = Reservation["status"];
export type PaymentStatus = Reservation["payment_status"];
export type CapacityCommitment = Reservation["capacity_commitment"];

export type Quote = Schemas["Quote"];
export type QuoteLine = Schemas["QuoteLine"];
export type QuoteListItem = Schemas["QuoteListItem"];
export type QuotePage = Schemas["EnvelopeQuotePage"]["data"];
export type QuoteRequest = Schemas["QuoteRequest"];
export type QuoteSearchRequest = Schemas["QuoteSearchRequest"];
export type QuoteSearchResult = Schemas["QuoteSearchResult"];
export type QuoteSearchItem = Schemas["QuoteSearchItem"];
export type QuotePurpose = Quote["purpose"];
export type QuoteStatus = Quote["status"];
export type ExtraSelection = Schemas["ExtraSelection"];
export type ManualDiscount = Schemas["ManualDiscount"];

export type ResolutionTask = Schemas["ResolutionTask"];
export type ResolutionTaskPage = Schemas["EnvelopeResolutionTaskPage"]["data"];
export type ResolutionTaskReplan =
  Schemas["EnvelopeResolutionTaskReplan"]["data"];
export type TaskType = ResolutionTask["task_type"];
export type TaskStatus = ResolutionTask["status"];

/** List query of `GET /v1/tenant/reservations` (multi-values comma-joined). */
export type ListReservationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  payment_status?: string;
  channel?: string;
  product?: string;
  customer?: string;
  branch?: string;
  assigned_user?: string;
  starts_from?: string;
  starts_to?: string;
};

/** List query of `GET /v1/tenant/quotes`. */
export type ListQuotesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  purpose?: string;
  status?: string;
  channel?: string;
  product?: string;
  reservation?: string;
};

/** List query of `GET /v1/tenant/resolution-tasks`. */
export type ListTasksParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  task_type?: string;
  reservation?: string;
  assignee?: string;
};
