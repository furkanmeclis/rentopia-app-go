"use client";

import type { ColumnDef } from "@tanstack/react-table";
import type { ReactNode } from "react";

import { DataTable } from "@/components/tables";
import { useLocale } from "@/providers/locale-provider";

type CatalogLocalTableProps<TData> = {
  columns: ColumnDef<TData, unknown>[];
  data: TData[];
  getRowId: (row: TData, index: number) => string;
  emptyTitle?: string;
  emptyDescription?: string;
  /** Rows per page; pagination and search only show for longer lists. */
  pageSize?: number;
  toolbar?: ReactNode;
  onRowClick?: (row: TData) => void;
  className?: string;
};

/**
 * Client-side DataTable for small, already loaded catalog lists (valuation
 * grids, import diff samples): sorting, and search + pagination once the
 * list is long enough. Same table / card rendering as the server lists.
 */
export function CatalogLocalTable<TData>({
  columns,
  data,
  getRowId,
  emptyTitle,
  emptyDescription,
  pageSize = 10,
  toolbar,
  onRowClick,
  className,
}: CatalogLocalTableProps<TData>) {
  const { t } = useLocale();
  const long = data.length > pageSize;
  return (
    <DataTable
      className={className}
      columns={columns}
      data={data}
      getRowId={getRowId}
      emptyTitle={emptyTitle ?? t("common.empty_title")}
      emptyDescription={emptyDescription ?? t("common.empty_description")}
      toolbar={toolbar}
      onRowClick={onRowClick}
      initialState={{ pagination: { pageIndex: 0, pageSize } }}
      pageSizeOptions={[pageSize, pageSize * 2, pageSize * 5]}
      features={{
        sorting: true,
        multiSort: false,
        globalFilter: long,
        columnFilters: false,
        columnVisibility: false,
        columnOrdering: false,
        columnPinning: false,
        columnResizing: false,
        rowSelection: false,
        pagination: long,
        density: false,
        viewMode: false,
        facetedFilters: false,
      }}
    />
  );
}
