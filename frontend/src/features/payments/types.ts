import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Payment = Schemas["Payment"];
export type PaymentRefund = Schemas["PaymentRefund"];
export type PaymentPage = Schemas["EnvelopePaymentPage"]["data"];
export type PaymentLink = Schemas["PaymentLink"];
export type PaymentLinkPage = Schemas["EnvelopePaymentLinkPage"]["data"];
export type PaymentStatus = Payment["status"];
export type PaymentMethod = Payment["method"];
export type PaymentLinkStatus = PaymentLink["status"];

export type IntegrationAccount = Schemas["IntegrationAccount"];
export type IntegrationAccountPage =
  Schemas["EnvelopeIntegrationAccountPage"]["data"];
export type IntegrationProvider = Schemas["IntegrationProvider"];
export type IntegrationsMeta = Schemas["EnvelopeIntegrationsMeta"]["data"];
export type IntegrationJobPage = Schemas["EnvelopeIntegrationJobPage"]["data"];
export type IntegrationJob = Schemas["IntegrationJob"];
export type IntegrationJobStatus = Schemas["IntegrationJobStatus"];
export type IntegrationJobDetail = Schemas["IntegrationJobDetail"];
export type IntegrationAttempt = Schemas["IntegrationAttempt"];
export type IntegrationJobRetry =
  Schemas["EnvelopeIntegrationJobRetry"]["data"];
export type SmsSent = Schemas["SmsSent"];
export type IntegrationProviderCode = IntegrationAccount["provider"];

export type ReservationAcceptResult = Schemas["ReservationAcceptResult"];
export type PublicPayLink = Schemas["PublicPayLink"];
export type PublicPayState = PublicPayLink["state"];

export type ConnectIntegrationRequest = {
  kind?: "payment" | "sms";
  provider: IntegrationProviderCode;
  display_name: string;
  mode: "sandbox" | "live";
  credentials: Record<string, string>;
};

export type UpdateIntegrationRequest = {
  display_name?: string | null;
  mode?: "sandbox" | "live" | null;
  credentials?: Record<string, string> | null;
};

export type ListPaymentsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  provider?: string;
  reservation?: string;
};

export type ListLinksParams = {
  limit: number;
  offset: number;
  status?: string;
  reservation?: string;
};

export type ListIntegrationJobsParams = {
  limit: number;
  offset: number;
  sort?: string;
  status?: string;
  kind?: string;
};

export type ListIntegrationsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
};
