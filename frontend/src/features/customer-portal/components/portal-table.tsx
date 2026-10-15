"use client";

import type { ColumnDef } from "@tanstack/react-table";

import { DataTable } from "@/components/tables";
import { useLocale } from "@/providers/locale-provider";

/**
 * The DataTable of the customer portal: a small client-side list (cards
 * below the md breakpoint), no toolbar, selection or column controls.
 */
export function PortalTable<TData>({
  columns,
  data,
  getRowId,
  emptyTitle,
  emptyDescription,
  onRowClick,
  isLoading,
}: {
  columns: ColumnDef<TData, unknown>[];
  data: TData[];
  getRowId: (row: TData) => string;
  emptyTitle?: string;
  emptyDescription?: string;
  onRowClick?: (row: TData) => void;
  isLoading?: boolean;
}) {
  const { t } = useLocale();
  return (
    <DataTable
      columns={columns}
      data={data}
      getRowId={getRowId}
      isLoading={isLoading}
      emptyTitle={emptyTitle ?? t("common.empty_title")}
      emptyDescription={emptyDescription ?? t("common.empty_description")}
      onRowClick={onRowClick}
      initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
      features={{
        sorting: false,
        multiSort: false,
        globalFilter: false,
        columnFilters: false,
        columnVisibility: false,
        columnOrdering: false,
        columnPinning: false,
        columnResizing: false,
        rowSelection: false,
        pagination: false,
        density: false,
        viewMode: false,
        facetedFilters: false,
      }}
    />
  );
}
