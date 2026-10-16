import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ChannelAccount = Schemas["ChannelAccount"];
export type ChannelAccountSettings = Schemas["ChannelAccountSettings"];
export type ChannelSettingsInput = Schemas["ChannelSettingsInput"];
export type ChannelMapping = Schemas["ChannelMapping"];
export type ChannelMappingInput = Schemas["ChannelMappingInput"];
export type ChannelMappingPage = Schemas["EnvelopeChannelMappingPage"]["data"];
export type ChannelMappingFacets = Schemas["ChannelMappingFacets"];
export type ChannelMessage = Schemas["ChannelMessage"];
export type ChannelMessageDetail = Schemas["ChannelMessageDetail"];
export type ChannelMessagePage = Schemas["EnvelopeChannelMessagePage"]["data"];
export type ChannelMessageFacets = Schemas["ChannelMessageFacets"];
export type ChannelMessageStatus = ChannelMessage["status"];
export type ChannelMessageType = ChannelMessage["type"];
export type ChannelSyncRun = Schemas["ChannelSyncRun"];
export type ChannelSyncRunPage = Schemas["EnvelopeChannelSyncRunPage"]["data"];
export type ChannelSimulateInput = Schemas["ChannelSimulateInput"];
export type ChannelReconciliation = Schemas["ChannelReconciliation"];
export type ChannelReconciliationLine = Schemas["ChannelReconciliationLine"];
export type ChannelEntityType = ChannelMapping["entity_type"];
export type ChannelHealth = ChannelAccount["sync"]["health"];

/** List query of `GET /v1/tenant/channel-mappings`. */
export type ListChannelMappingsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  account?: string;
  entity_type?: string;
  active?: string;
  stop_sale?: string;
};

/** List query of `GET /v1/tenant/channel-messages`. */
export type ListChannelMessagesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  account?: string;
  status?: string;
  type?: string;
};

/** List query of `GET /v1/tenant/channel-sync-runs`. */
export type ListChannelSyncRunsParams = {
  limit: number;
  offset: number;
  sort?: string;
  account?: string;
  direction?: string;
  status?: string;
};
