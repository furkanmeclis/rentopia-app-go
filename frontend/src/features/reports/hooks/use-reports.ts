"use client";

import { useQuery } from "@tanstack/react-query";

import { reportsService } from "@/features/reports/services/reports.service";
import type { ReportKind, ReportParams } from "@/features/reports/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";

export const reportKeys = {
  all: (slug: string) => ["tenant", slug, "reports"] as const,
  report: (slug: string, kind: ReportKind, params: ReportParams) =>
    [...reportKeys.all(slug), kind, params] as const,
};

/** One report; keeps the previous answer while the filters change. */
export function useReport<K extends ReportKind>(kind: K, params: ReportParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reportKeys.report(slug, kind, params),
    queryFn: () => reportsService.get(slug, kind, params),
    placeholderData: (previous) => previous,
  });
}
