import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SupportGrant = Schemas["SupportGrant"];
export type SupportGrantPage = Schemas["EnvelopeSupportGrantPage"]["data"];
export type SupportGrantOpenRequest = Schemas["SupportGrantOpenRequest"];
export type SupportGrantScope = SupportGrant["scope"];
export type SupportGrantStatus = SupportGrant["status"];

export type ListSupportGrantsParams = {
  limit?: number;
  offset?: number;
  /** Comma-separated statuses. */
  status?: string;
};
