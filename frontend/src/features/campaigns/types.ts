import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Campaign = Schemas["Campaign"];
export type CampaignPage = Schemas["CampaignPage"];
export type CampaignFacets = Schemas["CampaignFacets"];
export type CampaignCreateRequest = Schemas["CampaignCreateRequest"];
export type CampaignPatchRequest = Schemas["CampaignPatchRequest"];
export type CampaignRedemption = Schemas["CampaignRedemption"];
export type CampaignRedemptionPage = Schemas["CampaignRedemptionPage"];
export type CampaignType = Campaign["campaign_type"];
export type DiscountKind = Campaign["discount_kind"];
export type CampaignState = Campaign["state"];
export type CampaignScope = Campaign["product_scope"];
export type RedemptionStatus = CampaignRedemption["status"];
export type CampaignActivityPage = Schemas["FleetActivityPage"];
/** Website card of a campaign (TEC-784): editorial fields only. */
export type CampaignStorefront = Schemas["CampaignStorefront"];
export type CampaignStorefrontPatch = Schemas["CampaignStorefrontPatch"];

/** List query of `GET /v1/tenant/campaigns` (multi-values comma-joined). */
export type ListCampaignsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  campaign_type?: string;
  discount_kind?: string;
  state?: string;
  channel?: string;
};
