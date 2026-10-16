import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Agreement = Schemas["Agreement"];
export type AgreementListItem = Schemas["AgreementListItem"];
export type AgreementPage = Schemas["EnvelopeAgreementPage"]["data"];
export type AgreementFacets = Schemas["AgreementFacets"];
export type AgreementInput = Schemas["AgreementInput"];
export type AgreementStatusRequest = Schemas["AgreementStatusRequest"];
export type AgreementContactRequest = Schemas["AgreementContactRequest"];
export type AgreementDriverRequest = Schemas["AgreementDriverRequest"];
export type AgreementContact = Schemas["AgreementContact"];
export type AgreementDriver = Schemas["AgreementDriver"];
export type AgreementRentalRow = Schemas["AgreementRentalRow"];
export type AgreementPeriod = Schemas["AgreementPeriod"];
export type AgreementBillingPlan = Schemas["AgreementBillingPlan"];
export type AgreementStatement = Schemas["AgreementStatement"];
export type AgreementStatementRow = Schemas["AgreementStatementRow"];
export type AgreementPricing = NonNullable<Schemas["AgreementPricing"]>;
export type BillingPeriodPlan = Schemas["BillingPeriodPlan"];
export type AgreementTariff = Schemas["RatePlan"];

export type AgreementStatus = Agreement["status"];
export type AgreementValidity = Agreement["validity"];
export type InvoicePeriod = Agreement["invoice_period"];
export type ContactRole = AgreementContact["role"];
export type PeriodStatus = AgreementPeriod["status"];

/** List query of `GET /v1/tenant/agreements` (multi-values comma-joined). */
export type ListAgreementsParams = {
  limit?: number;
  offset?: number;
  q?: string;
  sort?: string;
  status?: string;
  customer?: string;
  branch?: string;
  invoice_period?: string;
  validity?: string;
  valid_from?: string;
  valid_to?: string;
};
