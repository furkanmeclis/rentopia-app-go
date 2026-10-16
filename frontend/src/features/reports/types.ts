import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ReportMethod = Schemas["ReportMethod"];
export type ReportRef = Schemas["ReportRef"];

export type OccupancyReport = Schemas["ReportOccupancy"];
export type OccupancyRow = Schemas["ReportOccupancyRow"];
export type OccupancyFigures = Schemas["ReportOccupancyFigures"];
export type OccupancyGroup = OccupancyReport["group_by"];

export type IncomeExpenseReport = Schemas["ReportIncomeExpense"];
export type IncomeExpenseRow = Schemas["ReportIncomeExpenseRow"];
export type IncomeExpenseGroup = IncomeExpenseReport["group_by"];

export type ChannelReport = Schemas["ReportChannels"];
export type ChannelRow = Schemas["ReportChannelRow"];
export type ChannelFigures = Schemas["ReportChannelFigures"];
export type ChannelDateBasis = ChannelReport["date_basis"];

export type AgingReport = Schemas["ReportAging"];
export type AgingRow = Schemas["ReportAgingRow"];
export type AgingParty = AgingReport["party"];

export type BranchReport = Schemas["ReportBranches"];
export type BranchRow = Schemas["ReportBranchRow"];
export type BranchFigures = Schemas["ReportBranchFigures"];

/** Query of every report (each report reads its own keys). */
export type ReportParams = {
  from?: string;
  to?: string;
  branch?: string;
  group_by?: string;
  rental_group?: string;
  date_basis?: string;
  as_of?: string;
  party?: string;
  basis?: string;
  allocation?: string;
};

export type ReportKind =
  "occupancy" | "income-expense" | "channels" | "aging" | "branches";
