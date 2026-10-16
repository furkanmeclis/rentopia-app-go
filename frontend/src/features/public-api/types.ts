import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type PublicApiKey = Schemas["PublicAPIKey"];
export type PublicApiKeyPage = Schemas["EnvelopePublicAPIKeyPage"]["data"];
export type IssuedPublicApiKey = Schemas["EnvelopePublicAPIIssuedKey"]["data"];
export type PublicApiKeyKind = PublicApiKey["kind"];
export type PublicApiChannel = PublicApiKey["channel"];
export type PublicApiScope = PublicApiKey["scopes"][number];
export type PublicApiKeyCreateRequest = Schemas["PublicAPIKeyCreateRequest"];
export type PublicApiKeyPatchRequest = Schemas["PublicAPIKeyPatchRequest"];

export type PublicWebhook = Schemas["PublicWebhook"];
export type PublicWebhookPage = Schemas["EnvelopePublicWebhookPage"]["data"];
export type IssuedPublicWebhook =
  Schemas["EnvelopePublicWebhookIssued"]["data"];
export type PublicWebhookCreateRequest = Schemas["PublicWebhookCreateRequest"];
export type PublicWebhookPatchRequest = Schemas["PublicWebhookPatchRequest"];
export type PublicWebhookEvent = NonNullable<
  PublicWebhookCreateRequest["events"]
>[number];

export type PublicWebhookDelivery = Schemas["PublicWebhookDelivery"];
export type PublicWebhookDeliveryPage =
  Schemas["EnvelopePublicWebhookDeliveryPage"]["data"];
export type PublicWebhookDeliveryStatus = PublicWebhookDelivery["status"];

export type PublicApiOverview = Schemas["PublicAPIOverview"];

export type ListKeysParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  kind?: string;
  channel?: string;
  status?: string;
};

export type ListWebhooksParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
};

export type ListDeliveriesParams = {
  limit: number;
  offset: number;
  sort?: string;
  endpoint?: string;
  status?: string;
  event_type?: string;
};
