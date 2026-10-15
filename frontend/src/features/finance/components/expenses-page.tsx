"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Download, Pencil } from "lucide-react";
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
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { useCategoryBulkParams } from "@/features/finance/components/category-bulk-params";
import {
  ExpenseDialog,
  VoidExpenseDialog,
} from "@/features/finance/components/expense-dialogs";
import {
  Amount,
  AttachmentBadge,
} from "@/features/finance/components/finance-cells";
import {
  EXPENSES_RESOURCE,
  EXPENSE_STATUS_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  useCategories,
  useExpensesList,
  useFinanceMeta,
} from "@/features/finance/hooks/use-finance";
import { financeService } from "@/features/finance/services/finance.service";
import type { Expense, ListParams } from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date, download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * "Giderler": expenses with category / branch / status facets and a date
 * range, receipts, export / import and the bulk "set category".
 */
export function ExpensesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [editing, setEditing] = useState<Expense | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [voiding, setVoiding] = useState<Expense | null>(null);
  const listState = useServerListState({
    initialSort: "-expense_date",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, EXPENSES_RESOURCE),
  });
  const categories = useCategories(false);
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      category: join(col("category")),
      branch: join(col("branch")),
      status: join(col("status")),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, to]);
  const query = useExpensesList(params);
  const meta = useFinanceMeta("expenses");

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<Expense>(),
        createColumn<Expense>({
          accessorKey: "expense_date",
          labelKey: "finance.columns.date",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {date(row.original.expense_date, "dd.MM.yyyy", locale)}
            </span>
          ),
        }),
        createColumn<Expense>({
          id: "category",
          accessorFn: (row) => row.category.name,
          labelKey: "finance.columns.category",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (categories.data?.items ?? []).map((c) => ({
            value: c.uuid,
            label: c.name,
          })),
          cell: ({ row }) => (
            <Badge variant="outline">{row.original.category.name}</Badge>
          ),
        }),
        createColumn<Expense>({
          accessorKey: "description",
          labelKey: "finance.columns.description",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="max-w-[20rem] space-y-0.5">
              <div className="truncate font-medium">
                {row.original.description || "—"}
              </div>
              {row.original.supplier_name || row.original.document_no ? (
                <div className="text-muted-foreground truncate text-xs">
                  {[row.original.supplier_name, row.original.document_no]
                    .filter(Boolean)
                    .join(" · ")}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<Expense>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "finance.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (branches.data?.items ?? []).map((b) => ({
            value: b.uuid,
            label: `${b.code} · ${b.name}`,
          })),
          cell: ({ row }) =>
            row.original.branch?.name ?? t("finance.head_office"),
        }),
        createColumn<Expense>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle?.code ?? "",
          labelKey: "finance.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.vehicle ? (
              <span className="font-mono text-xs">
                {row.original.vehicle.code}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Expense>({
          id: "cash_account",
          accessorFn: (row) => row.cash_account?.code ?? "",
          labelKey: "finance.columns.paid_from",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.cash_account?.name ?? t("finance.fields.unpaid"),
        }),
        createColumn<Expense>({
          accessorKey: "vat_amount",
          labelKey: "finance.columns.vat_amount",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => <Amount value={row.original.vat_amount} />,
        }),
        createColumn<Expense>({
          accessorKey: "total_amount",
          labelKey: "finance.columns.total_amount",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.total_amount}
              currency={row.original.currency}
              className={
                row.original.status === "void"
                  ? "text-muted-foreground line-through"
                  : "font-semibold"
              }
            />
          ),
        }),
        createColumn<Expense>({
          id: "attachment",
          accessorFn: (row) => (row.attachment ? "1" : ""),
          labelKey: "finance.columns.attachment",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.attachment ? (
              <AttachmentBadge name={row.original.attachment.name} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Expense>({
          accessorKey: "status",
          labelKey: "finance.columns.status",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: (["recorded", "void"] as const).map((s) => ({
            value: s,
            labelKey: EXPENSE_STATUS_LABEL_KEYS[s],
            label: t(EXPENSE_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(EXPENSE_STATUS_LABEL_KEYS[row.original.status])}
              tone={row.original.status === "void" ? "danger" : "success"}
            />
          ),
        }),
        createColumn<Expense>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const e = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("finance.actions.edit"),
                    icon: Pencil,
                    permission: permissions.tenant.finance.expenses.write,
                    disabled: e.status === "void",
                    onSelect: () => setEditing(e),
                  },
                  ...(e.attachment
                    ? [
                        {
                          id: "download",
                          label: t("finance.actions.download"),
                          icon: Download,
                          onSelect: async () => {
                            const out = await financeService.downloadAttachment(
                              slug,
                              e.uuid,
                            );
                            download(
                              out.blob,
                              out.filename ?? e.attachment?.name ?? "receipt",
                            );
                          },
                        },
                      ]
                    : []),
                  {
                    id: "void",
                    label: t("finance.actions.void"),
                    icon: Ban,
                    permission: permissions.tenant.finance.expenses.write,
                    variant: "destructive" as const,
                    disabled: e.status === "void",
                    onSelect: () => setVoiding(e),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<Expense, unknown>[],
    [branches.data?.items, categories.data?.items, locale, slug, t],
  );

  const engineQuery = {
    q: params.q,
    category: params.category as string | undefined,
    branch: params.branch as string | undefined,
    status: params.status as string | undefined,
    from: params.from as string | undefined,
    to: params.to as string | undefined,
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
        EXPENSES_RESOURCE,
        meta.data?.bulk_actions ?? [],
      ),
    [meta.data?.bulk_actions],
  );
  const bulkParams = useCategoryBulkParams();
  const hasFilters = Boolean(
    params.q ||
    params.category ||
    params.branch ||
    params.status ||
    params.from ||
    params.to,
  );
  const refresh = () => void query.refetch();

  return (
    <EntityPage
      title={t("finance.expenses.title")}
      description={t("finance.expenses.description")}
      permission={permissions.tenant.finance.expenses.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.expenses.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("finance.expenses.create")}
          permission={permissions.tenant.finance.expenses.write}
        />
      }
    >
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
        onRowClick={(row) =>
          row.status === "recorded" ? setEditing(row) : undefined
        }
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("finance.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("finance.expenses.empty_filtered")
            : t("finance.expenses.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("finance.expenses.empty_description")
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
          persistKey: "tenant-finance-expenses-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <div className="grid w-full grid-cols-2 gap-2 sm:w-auto">
              <DatePicker
                value={from}
                onChange={setFrom}
                placeholder={t("finance.filters.from")}
              />
              <DatePicker
                value={to}
                onChange={setTo}
                placeholder={t("finance.filters.to")}
              />
            </div>
            <BulkActionMenu
              resource={EXPENSES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              resolveParams={bulkParams.resolveParams}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={EXPENSES_RESOURCE}
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
      {bulkParams.dialog}
      <ExpenseDialog
        expense={editing}
        open={createOpen || Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) {
            setCreateOpen(false);
            setEditing(null);
          }
        }}
      />
      <VoidExpenseDialog
        expense={voiding}
        onOpenChange={(next) => {
          if (!next) setVoiding(null);
        }}
      />
    </EntityPage>
  );
}
