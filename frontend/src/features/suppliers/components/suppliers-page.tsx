"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { Amount } from "@/features/finance/components/finance-cells";
import { ResourceIOToolbar } from "@/features/io";
import { PayablesCard } from "@/features/suppliers/components/payables-card";
import { SupplierDueChip } from "@/features/suppliers/components/supplier-cells";
import { SupplierDialog } from "@/features/suppliers/components/supplier-dialog";
import {
  SUPPLIERS_RESOURCE,
  SUPPLIER_DUE_LABEL_KEYS,
  SUPPLIER_DUE_STATES,
  SUPPLIER_STATUS_LABEL_KEYS,
  SUPPLIER_TYPES,
  SUPPLIER_TYPE_LABEL_KEYS,
} from "@/features/suppliers/constants";
import {
  useSupplierFacets,
  useSuppliersList,
  useSuppliersMeta,
} from "@/features/suppliers/hooks/use-suppliers";
import type { ListParams, Supplier } from "@/features/suppliers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * "Tedarikçiler": supplier cards with what we owe them, type / status /
 * city / due facets, export / import, bulk archive and the due payables
 * card on top.
 */
export function SuppliersPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const [editing, setEditing] = useState<Supplier | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const listState = useServerListState({
    initialSort: "title",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, SUPPLIERS_RESOURCE),
  });
  const facets = useSupplierFacets();
  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      supplier_type: join(col("supplier_type")),
      status: join(col("status")),
      city: join(col("city")),
      due_state: join(col("due_state")),
    };
  }, [listState.columnFilters, listState.params]);
  const query = useSuppliersList(params);
  const meta = useSuppliersMeta("suppliers");
  const open = (s: Supplier) =>
    router.push(routes.tenant.finance.supplier(slug, s.uuid));

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<Supplier>(),
        createColumn<Supplier>({
          accessorKey: "code",
          labelKey: "suppliers.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
              {row.original.code}
            </code>
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "title",
          labelKey: "suppliers.columns.title",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="max-w-[18rem] space-y-0.5">
              <div className="truncate font-medium">{row.original.title}</div>
              {row.original.city || row.original.contacts[0] ? (
                <div className="text-muted-foreground truncate text-xs">
                  {[row.original.city, row.original.contacts[0]?.name]
                    .filter(Boolean)
                    .join(" · ")}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "supplier_type",
          labelKey: "suppliers.columns.supplier_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: SUPPLIER_TYPES.map((type) => ({
            value: type,
            labelKey: SUPPLIER_TYPE_LABEL_KEYS[type],
            label: t(SUPPLIER_TYPE_LABEL_KEYS[type]),
            count: facets.data?.supplier_type.find((f) => f.value === type)
              ?.count,
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(SUPPLIER_TYPE_LABEL_KEYS[row.original.supplier_type])}
            </Badge>
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "tax_number",
          labelKey: "suppliers.columns.tax_number",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.tax_number ? (
              <span className="font-mono text-xs">
                {row.original.tax_number}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Supplier>({
          accessorKey: "phone",
          labelKey: "suppliers.columns.phone",
          enableSorting: false,
          cell: ({ row }) => row.original.phone || "—",
        }),
        createColumn<Supplier>({
          id: "city",
          accessorFn: (row) => row.city,
          labelKey: "suppliers.columns.city",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: (facets.data?.city ?? []).map((f) => ({
            value: f.value,
            label: f.value,
            count: f.count,
          })),
          cell: ({ row }) => row.original.city || "—",
        }),
        createColumn<Supplier>({
          accessorKey: "payable",
          labelKey: "suppliers.columns.payable",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.payable}
              className={
                Number(row.original.payable) > 0 ? "font-semibold" : undefined
              }
              muted={Number(row.original.payable) === 0}
            />
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "overdue_amount",
          labelKey: "suppliers.columns.overdue_amount",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.overdue_amount}
              className={
                Number(row.original.overdue_amount) > 0
                  ? "text-destructive font-semibold"
                  : undefined
              }
              muted={Number(row.original.overdue_amount) === 0}
            />
          ),
        }),
        createColumn<Supplier>({
          accessorKey: "next_due_date",
          labelKey: "suppliers.columns.next_due_date",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.next_due_date ? (
              <span className="whitespace-nowrap tabular-nums">
                {date(row.original.next_due_date, "dd.MM.yyyy", locale)}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Supplier>({
          accessorKey: "due_state",
          labelKey: "suppliers.columns.due_state",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: SUPPLIER_DUE_STATES.map((s) => ({
            value: s,
            labelKey: SUPPLIER_DUE_LABEL_KEYS[s],
            label: t(SUPPLIER_DUE_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => <SupplierDueChip state={row.original.due_state} />,
        }),
        createColumn<Supplier>({
          accessorKey: "status",
          labelKey: "suppliers.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (["active", "archived"] as const).map((s) => ({
            value: s,
            labelKey: SUPPLIER_STATUS_LABEL_KEYS[s],
            label: t(SUPPLIER_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(SUPPLIER_STATUS_LABEL_KEYS[row.original.status])}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<Supplier>({
          id: "actions",
          labelKey: "suppliers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("suppliers.actions.view"),
                  icon: Eye,
                  onSelect: () => open(row.original),
                },
                {
                  id: "edit",
                  label: t("suppliers.actions.edit"),
                  icon: Pencil,
                  permission: permissions.tenant.suppliers.write,
                  onSelect: () => setEditing(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Supplier, unknown>[],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [facets.data, locale, slug, t],
  );

  const engineQuery = {
    q: params.q,
    supplier_type: params.supplier_type as string | undefined,
    status: params.status as string | undefined,
    city: params.city as string | undefined,
    due_state: params.due_state as string | undefined,
    sort: params.sort,
  };
  const total = query.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery: engineQuery,
    total,
  });
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        SUPPLIERS_RESOURCE,
        meta.data?.bulk_actions ?? [],
      ),
    [meta.data?.bulk_actions],
  );
  const hasFilters = Boolean(
    params.q ||
    params.supplier_type ||
    params.status ||
    params.city ||
    params.due_state,
  );
  const refresh = () => void query.refetch();

  return (
    <EntityPage
      title={t("suppliers.title")}
      description={t("suppliers.description")}
      permission={permissions.tenant.suppliers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("suppliers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("suppliers.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("suppliers.create")}
          permission={permissions.tenant.suppliers.write}
        />
      }
    >
      <PayablesCard />
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={open}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("suppliers.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters ? t("suppliers.empty_filtered") : t("suppliers.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("suppliers.empty_description")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-suppliers-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={SUPPLIERS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={SUPPLIERS_RESOURCE}
              query={engineQuery}
              capabilities={meta.data?.capabilities}
              onImportComplete={refresh}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      <SupplierDialog
        supplier={editing}
        open={createOpen || Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) {
            setCreateOpen(false);
            setEditing(null);
          }
        }}
        onSaved={(saved) => {
          if (!editing) open(saved);
        }}
      />
    </EntityPage>
  );
}
