import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type NotificationChannelInfo = Schemas["NotificationChannelInfo"];
export type NotificationChannel = NotificationChannelInfo["name"];
export type NotificationEvent = Schemas["NotificationEvent"];
export type NotificationEvents = Schemas["NotificationEvents"];
export type NotificationRuleInput = Schemas["NotificationRuleInput"];
export type NotificationTemplate = Schemas["NotificationTemplate"];
export type NotificationTemplateInput = Schemas["NotificationTemplateInput"];
export type NotificationTemplateKey = Schemas["NotificationTemplateKey"];
export type NotificationPreviewInput = Schemas["NotificationPreviewInput"];
export type NotificationPreview = Schemas["NotificationPreview"];
export type NotificationTestInput = Schemas["NotificationTestInput"];
export type NotificationDelivery = Schemas["NotificationDelivery"];
export type NotificationDeliveryStatus = NotificationDelivery["status"];
export type NotificationDispatch = Schemas["NotificationDispatch"];
export type NotificationDispatchStatus = NotificationDispatch["status"];
export type NotificationDeliveryFacets = Schemas["NotificationDeliveryFacets"];
export type NotificationLanguage = NotificationTemplate["language"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** Query of the delivery history (comma-joined multi-value filters). */
export type DeliveryListParams = {
  limit: number;
  offset: number;
  q?: string;
  sort?: string;
  channel?: string;
  status?: string;
  event?: string;
  audience?: string;
  created_from?: string;
  created_to?: string;
};
