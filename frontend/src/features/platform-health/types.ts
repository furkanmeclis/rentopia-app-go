import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type OrganizationUsage = Schemas["PlatformOrganizationUsage"];
export type OrganizationUsagePage =
  Schemas["EnvelopePlatformUsagePage"]["data"];
export type UsageSummary = Schemas["PlatformUsageSummary"];
export type HealthSummary = Schemas["PlatformHealthSummary"];
export type HealthSource = Schemas["PlatformHealthSource"];
export type HealthMetric = Schemas["PlatformHealthMetric"];
export type HealthAlertState = Schemas["PlatformHealthAlertState"];
export type HealthQueue = Schemas["PlatformHealthQueue"];
export type HealthJobStatusCount = Schemas["PlatformHealthJobStatusCount"];
export type HealthIssue = Schemas["PlatformHealthIssue"];
export type HealthIssuePage =
  Schemas["EnvelopePlatformHealthIssuePage"]["data"];
export type HealthStatus = HealthSummary["status"];
export type HealthSeverity = HealthIssue["severity"];

/** Query of `GET /v1/platform/usage` (multi values comma-joined). */
export type ListUsageParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  month?: string;
};

/** Query of `GET /v1/platform/health/issues`. */
export type ListIssuesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  source?: string;
  severity?: string;
};
