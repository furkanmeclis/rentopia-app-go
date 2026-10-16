import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type PortalInfo = Schemas["CustomerPortalInfo"];
export type PortalStartRequest = Schemas["CustomerAuthStartRequest"];
export type PortalStartResult = Schemas["CustomerAuthStart"];
export type PortalVerifyRequest = Schemas["CustomerAuthVerifyRequest"];
export type PortalSession = Schemas["CustomerSession"];
export type PortalSessionIssued = Schemas["CustomerSessionIssued"];
export type PortalReservationSummary = Schemas["CustomerReservationSummary"];
export type PortalReservation = Schemas["CustomerReservation"];
export type PortalDocument = Schemas["PortalDocument"];
export type PortalInvoice = Schemas["PortalInvoice"];
export type PortalRequest = Schemas["CustomerRequest"];
export type PortalRequestCreate = Schemas["CustomerRequestCreate"];
export type PortalPayResult = Schemas["CustomerPayResult"];

export type CustomerRequestType = PortalRequest["type"];
export type CustomerRequestStatus = PortalRequest["status"];

export type StaffCustomerRequest = Schemas["StaffCustomerRequest"];
export type CustomerRequestResolve = Schemas["CustomerRequestResolve"];
export type CustomerRequestPage = {
  items: StaffCustomerRequest[];
  total: number;
  limit: number;
  offset: number;
};

export type ListCustomerRequestsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  request_type?: string;
  reservation?: string;
};
