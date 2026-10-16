import type {
  AgingReport,
  BranchReport,
  ChannelReport,
  IncomeExpenseReport,
  OccupancyReport,
  ReportKind,
  ReportParams,
} from "@/features/reports/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/reports";

export type ReportData = {
  occupancy: OccupancyReport;
  "income-expense": IncomeExpenseReport;
  channels: ChannelReport;
  aging: AgingReport;
  branches: BranchReport;
};

/** Reports (TEC-738) of one organization. */
export const reportsService = {
  get<K extends ReportKind>(slug: string, kind: K, params: ReportParams) {
    return tenantRequest<ReportData[K]>(slug, "GET", `${BASE}/${kind}`, {
      query: params,
    });
  },
};
