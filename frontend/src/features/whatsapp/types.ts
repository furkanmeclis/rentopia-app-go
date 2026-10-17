import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type WhatsAppSession = Schemas["WhatsAppSession"];
export type WhatsAppSessionStatus = WhatsAppSession["status"];
export type WhatsAppProvider = Schemas["WhatsAppProvider"];
export type WhatsAppConnectInput = Schemas["WhatsAppConnectInput"];
export type WhatsAppSettingsInput = Schemas["WhatsAppSettingsInput"];
export type WhatsAppTestInput = Schemas["WhatsAppTestInput"];
export type WhatsAppMessage = Schemas["WhatsAppMessage"];
export type WhatsAppMessageStatus = WhatsAppMessage["status"];
export type WhatsAppQR = Schemas["EnvelopeWhatsAppQR"]["data"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** Query of the message log (comma-joined multi-value filters). */
export type MessageListParams = {
  limit: number;
  offset: number;
  q?: string;
  sort?: string;
  status?: string;
  kind?: string;
  created_from?: string;
  created_to?: string;
};
