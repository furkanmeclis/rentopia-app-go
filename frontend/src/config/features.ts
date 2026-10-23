/**
 * Plan features (TEC-674). An organization's plan (Yönetim / Komple) and
 * the platform's overrides decide which of these it may use; `/v1/auth/me`
 * and `/v1/tenant/context` list the enabled keys. The panel hides what is
 * off, the API refuses it with `403 FEATURE_NOT_ENABLED`.
 *
 * Keep FEATURE_KEYS identical (same order) to
 * backend/internal/platform/features (the Go catalog test reads this list).
 */
export const FEATURE_KEYS = [
  "site",
  "widget",
  "public_api",
  "channel.yolcu360",
  "channel.obilet",
  "kabis",
  "hgs",
  "einvoice",
  "whatsapp",
  "sms",
] as const;

export type FeatureKey = (typeof FEATURE_KEYS)[number];

/** Plan codes seeded by the backend (migration 000134). */
export const PLAN_CODES = ["management", "complete"] as const;

export type PlanCode = (typeof PLAN_CODES)[number];

/** Error code of a request outside the organization's plan. */
export const FEATURE_NOT_ENABLED = "FEATURE_NOT_ENABLED";

export const FEATURE_LABEL_KEYS: Record<FeatureKey, string> = {
  site: "features.labels.site",
  widget: "features.labels.widget",
  public_api: "features.labels.public_api",
  "channel.yolcu360": "features.labels.channel_yolcu360",
  "channel.obilet": "features.labels.channel_obilet",
  kabis: "features.labels.kabis",
  hgs: "features.labels.hgs",
  einvoice: "features.labels.einvoice",
  whatsapp: "features.labels.whatsapp",
  sms: "features.labels.sms",
};

export const FEATURE_DESCRIPTION_KEYS: Record<FeatureKey, string> = {
  site: "features.descriptions.site",
  widget: "features.descriptions.widget",
  public_api: "features.descriptions.public_api",
  "channel.yolcu360": "features.descriptions.channel_yolcu360",
  "channel.obilet": "features.descriptions.channel_obilet",
  kabis: "features.descriptions.kabis",
  hgs: "features.descriptions.hgs",
  einvoice: "features.descriptions.einvoice",
  whatsapp: "features.descriptions.whatsapp",
  sms: "features.descriptions.sms",
};

/** What the usage counter of a feature counts (none for the widget). */
export const FEATURE_USAGE_KEYS: Partial<Record<FeatureKey, string>> = {
  site: "features.usage.site",
  public_api: "features.usage.public_api",
  "channel.yolcu360": "features.usage.channel",
  "channel.obilet": "features.usage.channel",
  kabis: "features.usage.kabis",
  hgs: "features.usage.hgs",
  einvoice: "features.usage.einvoice",
  whatsapp: "features.usage.whatsapp",
  sms: "features.usage.sms",
};

export const PLAN_LABEL_KEYS: Record<PlanCode, string> = {
  management: "features.plans.management",
  complete: "features.plans.complete",
};

export function isFeatureKey(value: string): value is FeatureKey {
  return (FEATURE_KEYS as readonly string[]).includes(value);
}

export function isPlanCode(value: string): value is PlanCode {
  return (PLAN_CODES as readonly string[]).includes(value);
}

/**
 * Whether `enabled` (an organization's enabled keys) has the feature, or
 * any of several. `null` / `undefined` means unknown (outside an
 * organization, or a session cached before plans existed): allowed, like
 * the API without a feature source; the server still decides.
 */
export function hasFeature(
  enabled: readonly string[] | null | undefined,
  feature: FeatureKey | readonly FeatureKey[],
): boolean {
  if (!enabled) return true;
  const wanted = typeof feature === "string" ? [feature] : feature;
  return wanted.some((key) => enabled.includes(key));
}
