"use client";

import { useQuery } from "@tanstack/react-query";

import { platformHealthService } from "@/features/platform-health/services/platform-health.service";
import type {
  ListIssuesParams,
  ListUsageParams,
} from "@/features/platform-health/types";

export const platformHealthKeys = {
  all: ["platform", "health"] as const,
  usage: (params: ListUsageParams) =>
    [...platformHealthKeys.all, "usage", params] as const,
  usageSummary: (month?: string) =>
    [...platformHealthKeys.all, "usage-summary", month ?? ""] as const,
  summary: () => [...platformHealthKeys.all, "summary"] as const,
  issues: (params: ListIssuesParams) =>
    [...platformHealthKeys.all, "issues", params] as const,
};

/** Health is read live from every source: refresh it every minute. */
const HEALTH_REFRESH_MS = 60_000;

export function useUsageList(params: ListUsageParams) {
  return useQuery({
    queryKey: platformHealthKeys.usage(params),
    queryFn: () => platformHealthService.usage(params),
    placeholderData: (previous) => previous,
  });
}

export function useUsageSummary(month?: string) {
  return useQuery({
    queryKey: platformHealthKeys.usageSummary(month),
    queryFn: () => platformHealthService.usageSummary(month),
  });
}

export function useHealthSummary() {
  return useQuery({
    queryKey: platformHealthKeys.summary(),
    queryFn: () => platformHealthService.health(),
    refetchInterval: HEALTH_REFRESH_MS,
  });
}

export function useHealthIssues(params: ListIssuesParams) {
  return useQuery({
    queryKey: platformHealthKeys.issues(params),
    queryFn: () => platformHealthService.issues(params),
    placeholderData: (previous) => previous,
    refetchInterval: HEALTH_REFRESH_MS,
  });
}
