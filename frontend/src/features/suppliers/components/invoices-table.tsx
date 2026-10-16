"use client";

import type { ColumnDef, ColumnFiltersState } from "@tanstack/react-table";
import { Ban, Banknote, Download, Pencil } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useCategories } from "@/features/finance/hooks/use-finance";
import {
  Amount,
  AttachmentBadge,
} from "@/features/finance/components/finance-cells";
import { ResourceIOToolbar } from "@/features/io";
import {
  InvoiceDialog,
  VoidInvoiceDialog,
} from "@/features/suppliers/components/invoice-dialogs";
import { PaymentDialog } from "@/features/suppliers/components/payment-dialogs";
import {
  InvoiceDueChip,
  InvoiceStatusChip,
} from "@/features/suppliers/components/supplier-cells";
import {
  INVOICE_DUE_LABEL_KEYS,
  INVOICE_DUE_STATES,
  INVOICE_STATUSES,
  INVOICE_STATUS_LABEL_KEYS,
  SUPPLIER_INVOICES_RESOURCE,
} from "@/features/suppliers/constants";
import {
  useSupplierInvoices,
  useSupplierOptions,
  useSuppliersMeta,
} from "@/features/suppliers/hooks/use-suppliers";
import { suppliersService } from "@/features/suppliers/services/suppliers.service";
import type { ListParams, SupplierInvoice } from "@/features/suppliers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date, download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Supplier invoices DataTable (the "Gider faturaları" page, the payables
 * view and the supplier detail tab). `supplierUuid` fixes the supplier;
 * `initialDueStates` pre-filters the due state.
 */
export function SupplierInvoicesTable({
  supplierUuid,
  initialDueStates,
  initialSort = "-invoice_date",
  persistKey,
  createOpen = false,
  onCreateClose,
  emptyTitle,
}: {
  supplierUuid?: string;
  initialDueStates?: string[];
  initialSort?: string;
  persistKey: string;
  /** The page's create button opens the invoice dialog. */
  createOpen?: boolean;
  onCreateClose?: () => void;
  emptyTitle?: string;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [voiding, setVoiding] = useState<SupplierInvoice | null>(null);
  const [paying, setPaying] = useState<SupplierInvoice | null>(null);
  const initialColumnFilters = useMemo<ColumnFiltersState>(
    () =>
      initialDueStates?.length
        ? [{ id: "due_state", value: initialDueStates }]
        : [],
    [initialDueStates],
  );
  const listState = useServerListState({
    initialSort,
    initialPageSize: 20,
    initialColumnFilters,
    realtime: tenantLiveList(organization, SUPPLIER_INVOICES_RESOURCE),
  });
  const categories = useCategories(false);
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const suppliers = useSupplierOptions(!supplierUuid);
  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      supplier: supplierUuid ?? join(col("supplier")),
      status: join(col("status")),
      due_state: join(col("due_state")),
      category: join(col("category")),
      branch: join(col("branch")),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, supplierUuid, to]);
  const query = useSupplierInvoices(params);
  const meta = useSuppliersMeta("invoices");
  const canPay = can(permissions.tenant.finance.cash.write);

  const columns = useMemo(
    () =>
      [
        createColumn<SupplierInvoice>({
          accessorKey: "invoice_no",
          labelKey: "suppliers.columns.invoice_no",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="space-y-0.5">
              <div className="font-medium">{row.original.invoice_no}</div>
              {row.original.description ? (
                <div className="text-muted-foreground max-w-[16rem] truncate text-xs">
                  {row.original.description}
                </div>
              ) : null}
            </div>
          ),
        }),
        ...(supplierUuid
          ? []
          : [
              createColumn<SupplierInvoice>({
                id: "supplier",
                accessorFn: (row) => row.supplier.name ?? "",
                labelKey: "suppliers.columns.supplier",
                enableSorting: false,
                filterVariant: "faceted",
                filterOptions: suppliers,
                cell: ({ row }) => (
                  <Link
                    href={routes.tenant.finance.supplier(
                      slug,
                      row.original.supplier.uuid,
                    )}
                    className="hover:underline"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {row.original.supplier.name}
                  </Link>
                ),
              }),
            ]),
        createColumn<SupplierInvoice>({
          accessorKey: "invoice_date",
          labelKey: "suppliers.columns.invoice_date",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {date(row.original.invoice_date, "dd.MM.yyyy", locale)}
            </span>
          ),
        }),
        createColumn<SupplierInvoice>({
          accessorKey: "due_date",
          labelKey: "suppliers.columns.due_date",
          enableSorting: true,
          cell: ({ row }) => (
            <span
              className={
                row.original.due_state === "overdue"
                  ? "text-destructive font-medium whitespace-nowrap tabular-nums"
                  : "whitespace-nowrap tabular-nums"
              }
            >
              {date(row.original.due_date, "dd.MM.yyyy", locale)}
            </span>
          ),
        }),
        createColumn<SupplierInvoice>({
          id: "category",
          accessorFn: (row) => row.category.name ?? "",
          labelKey: "suppliers.columns.category",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: (categories.data?.items ?? []).map((c) => ({
            value: c.uuid,
            label: c.name,
          })),
          cell: ({ row }) => (
            <Badge variant="outline">{row.original.category.name}</Badge>
          ),
        }),
        createColumn<SupplierInvoice>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "suppliers.columns.branch",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: (branches.data?.items ?? []).map((b) => ({
            value: b.uuid,
            label: `${b.code} · ${b.name}`,
          })),
          cell: ({ row }) =>
            row.original.branch?.name ?? t("finance.head_office"),
        }),
        createColumn<SupplierInvoice>({
          accessorKey: "total_amount",
          labelKey: "suppliers.columns.total_amount",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.total_amount}
              currency={row.original.currency}
              className={
                row.original.status === "void"
                  ? "text-muted-foreground line-through"
                  : undefined
              }
            />
          ),
        }),
        createColumn<SupplierInvoice>({
          accessorKey: "open_amount",
          labelKey: "suppliers.columns.open_amount",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.open_amount}
              currency={row.original.currency}
              muted={Number(row.original.open_amount) === 0}
              className={
                Number(row.original.open_amount) > 0
                  ? "font-semibold"
                  : undefined
              }
            />
          ),
        }),
        createColumn<SupplierInvoice>({
          accessorKey: "due_state",
          labelKey: "suppliers.columns.due_state",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INVOICE_DUE_STATES.map((s) => ({
            value: s,
            labelKey: INVOICE_DUE_LABEL_KEYS[s],
            label: t(INVOICE_DUE_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => <InvoiceDueChip state={row.original.due_state} />,
        }),
        createColumn<SupplierInvoice>({
          accessorKey: "status",
          labelKey: "suppliers.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INVOICE_STATUSES.map((s) => ({
            value: s,
            labelKey: INVOICE_STATUS_LABEL_KEYS[s],
            label: t(INVOICE_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => <InvoiceStatusChip status={row.original.status} />,
        }),
        createColumn<SupplierInvoice>({
          id: "attachment",
          accessorFn: (row) => (row.attachment ? "1" : ""),
          labelKey: "suppliers.columns.attachment",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.attachment ? (
              <AttachmentBadge name={row.original.attachment.name} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<SupplierInvoice>({
          id: "actions",
          labelKey: "suppliers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const inv = row.original;
            const open =
              inv.status === "open" || inv.status === "partially_paid";
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("suppliers.actions.edit"),
                    icon: Pencil,
                    permission: permissions.tenant.finance.expenses.write,
                    disabled: inv.status === "void",
                    onSelect: () => setEditing(inv.uuid),
                  },
                  ...(canPay && open
                    ? [
                        {
                          id: "pay",
                          label: t("suppliers.actions.pay"),
                          icon: Banknote,
                          onSelect: () => setPaying(inv),
                        },
                      ]
                    : []),
                  ...(inv.attachment
                    ? [
                        {
                          id: "download",
                          label: t("suppliers.actions.download"),
                          icon: Download,
                          onSelect: async () => {
                            const out =
                              await suppliersService.downloadInvoiceFile(
                                slug,
                                inv.uuid,
                              );
                            download(
                              out.blob,
                              out.filename ?? inv.attachment?.name ?? "invoice",
                            );
                          },
                        },
                      ]
                    : []),
                  {
                    id: "void",
                    label: t("suppliers.actions.void"),
                    icon: Ban,
                    permission: permissions.tenant.finance.expenses.write,
                    variant: "destructive" as const,
                    disabled: inv.status !== "open",
                    onSelect: () => setVoiding(inv),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<SupplierInvoice, unknown>[],
    [
      branches.data?.items,
      canPay,
      categories.data?.items,
      locale,
      slug,
      supplierUuid,
      suppliers,
      t,
    ],
  );

  const engineQuery = {
    q: params.q,
    supplier: params.supplier as string | undefined,
    status: params.status as string | undefined,
    due_state: params.due_state as string | undefined,
    category: params.category as string | undefined,
    branch: params.branch as string | undefined,
    from: params.from as string | undefined,
    to: params.to as string | undefined,
    sort: params.sort,
  };
  const total = query.data?.total ?? 0;
  const hasFilters = Boolean(
    params.q ||
    (!supplierUuid && params.supplier) ||
    params.status ||
    params.due_state ||
    params.category ||
    params.branch ||
    params.from ||
    params.to,
  );
  const refresh = () => void query.refetch();

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          row.status !== "void" ? setEditing(row.uuid) : undefined
        }
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("suppliers.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          emptyTitle ??
          (hasFilters
            ? t("suppliers.invoices.empty_filtered")
            : t("suppliers.invoices.empty"))
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey, viewMode: true }}
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
            <ResourceIOToolbar
              resource={SUPPLIER_INVOICES_RESOURCE}
              query={engineQuery}
              capabilities={meta.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      <InvoiceDialog
        invoiceUuid={editing}
        supplierUuid={supplierUuid}
        open={createOpen || Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) {
            setEditing(null);
            onCreateClose?.();
          }
        }}
      />
      <VoidInvoiceDialog
        invoice={voiding}
        onOpenChange={(next) => {
          if (!next) setVoiding(null);
        }}
      />
      <PaymentDialog
        invoice={paying}
        open={Boolean(paying)}
        onOpenChange={(next) => {
          if (!next) setPaying(null);
        }}
      />
    </>
  );
}
