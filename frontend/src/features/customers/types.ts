import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Customer = Schemas["Customer"];
export type CustomerPage = Schemas["CustomerPage"];
export type CustomerFacets = Schemas["CustomerFacets"];
export type CustomerCreateRequest = Schemas["CustomerCreateRequest"];
export type CustomerPatchRequest = Schemas["CustomerPatchRequest"];
export type CustomerRevealed = Schemas["CustomerRevealed"];
export type CustomerContact = Schemas["CustomerContact"];
export type CustomerContactCreateRequest =
  Schemas["CustomerContactCreateRequest"];
export type CustomerContactPatchRequest =
  Schemas["CustomerContactPatchRequest"];
export type CustomerConsent = Schemas["CustomerConsent"];
export type CustomerConsents = Schemas["CustomerConsents"];
export type CustomerConsentRequest = Schemas["CustomerConsentRequest"];
export type CustomerDriver = Schemas["CustomerDriver"];
export type CustomerDriverCreateRequest =
  Schemas["CustomerDriverCreateRequest"];
export type CustomerDriverPatchRequest = Schemas["CustomerDriverPatchRequest"];
export type DriverEligibility = Schemas["DriverEligibility"];
export type CustomerDocument = Schemas["CustomerDocument"];
export type CustomerDocumentCreateRequest =
  Schemas["CustomerDocumentCreateRequest"];

export type CustomerType = Customer["customer_type"];
export type IdentityType = NonNullable<Customer["identity_type"]>;
export type ConsentChannel = CustomerConsent["channel"];
export type ConsentSource = CustomerConsent["source"];
export type CustomerDocumentType = CustomerDocument["doc_type"];

/** List query of `GET /v1/tenant/customers` (multi-values comma-joined). */
export type ListCustomersParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  customer_type?: string;
  nationality?: string;
  tag?: string;
  consent?: string;
  anonymized?: "true" | "false";
};

export type ListResponse<T> = { items: T[] };
