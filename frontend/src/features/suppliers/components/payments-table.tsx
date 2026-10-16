"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import { useCashAccountsList } from "@/features/finance/hooks/use-finance";
import { ResourceIOToolbar } from "@/features/io";
import {
  PaymentDialog,
  VoidPaymentDialog,
} from "@/features/suppliers/components/payment-dialogs";
import {
  PAYMENT_METHODS,
  PAYMENT_METHOD_LABEL_KEYS,
  PAYMENT_STATUS_LABEL_KEYS,
  SUPPLIER_PAYMENTS_RESOURCE,
} from "@/features/suppliers/constants";
import {
  useSupplierOptions,
  useSupplierPayments,
  useSuppliersMeta,
} from "@/features/suppliers/hooks/use-suppliers";
import type { ListParams, SupplierPayment } from "@/features/suppliers/types";
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
 * Supplier payments DataTable (the "Tedarikçi ödemeleri" page and the
 * supplier detail tab). `supplierUuid` fixes the supplier.
 */
export function SupplierPaymentsTable({
  supplierUuid,
  persistKey,
  createOpen = false,
  onCreateClose,
}: {
  supplierUuid?: string;
  persistKey: string;
  createOpen?: boolean;
  onCreateClose?: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [voiding, setVoiding] = useState<SupplierPayment | null>(null);
  const listState = useServerListState({
    initialSort: "-payment_date",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, SUPPLIER_PAYMENTS_RESOURCE),
  });
  const suppliers = useSupplierOptions(!supplierUuid);
  const cash = useCashAccountsList({
    limit: 100,
    offset: 0,
    sort: "code",
  });
  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      supplier: supplierUuid ?? join(col("supplier")),
      cash_account: join(col("cash_account")),
      method: join(col("method")),
      status: join(col("status")),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, supplierUuid, to]);
  const query = useSupplierPayments(params);
  const meta = useSuppliersMeta("payments");

  const columns = useMemo(
    () =>
      [
        createColumn<SupplierPayment>({
          accessorKey: "payment_date",
          labelKey: "suppliers.columns.payment_date",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {date(row.original.payment_date, "dd.MM.yyyy", locale)}
            </span>
          ),
        }),
        ...(supplierUuid
          ? []
          : [
              createColumn<SupplierPayment>({
                id: "supplier",
                accessorFn: (row) => row.supplier.name ?? "",
                labelKey: "suppliers.columns.supplier",
                enableSorting: false,
                gridPrimary: true,
                filterVariant: "faceted",
                filterOptions: suppliers,
                cell: ({ row }) => (
                  <Link
                    href={routes.tenant.finance.supplier(
                      slug,
                      row.original.supplier.uuid,
                    )}
                    className="font-medium hover:underline"
                  >
                    {row.original.supplier.name}
                  </Link>
                ),
              }),
            ]),
        createColumn<SupplierPayment>({
          id: "cash_account",
          accessorFn: (row) => row.cash_account.code ?? "",
          labelKey: "suppliers.columns.cash_account",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (cash.data?.items ?? []).map((a) => ({
            value: a.uuid,
            label: `${a.code} · ${a.name}`,
          })),
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {row.original.cash_account.code} ·{" "}
              {row.original.cash_account.name}
            </span>
          ),
        }),
        createColumn<SupplierPayment>({
          accessorKey: "method",
          labelKey: "suppliers.columns.method",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: PAYMENT_METHODS.map((m) => ({
            value: m,
            labelKey: PAYMENT_METHOD_LABEL_KEYS[m],
            label: t(PAYMENT_METHOD_LABEL_KEYS[m]),
          })),
          cell: ({ row }) => t(PAYMENT_METHOD_LABEL_KEYS[row.original.method]),
        }),
        createColumn<SupplierPayment>({
          id: "invoices",
          accessorFn: (row) =>
            row.allocations.map((a) => a.invoice_no).join(", "),
          labelKey: "suppliers.columns.invoices",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.allocations.length > 0 ? (
              <span className="text-muted-foreground block max-w-[14rem] truncate text-xs">
                {row.original.allocations.map((a) => a.invoice_no).join(", ")}
              </span>
            ) : (
              <span className="text-muted-foreground text-xs">
                {t("suppliers.payments.advance_only")}
              </span>
            ),
        }),
        createColumn<SupplierPayment>({
          accessorKey: "amount",
          labelKey: "suppliers.columns.amount",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.amount}
              currency={row.original.currency}
              className={
                row.original.status === "void"
                  ? "text-muted-foreground line-through"
                  : "font-semibold"
              }
            />
          ),
        }),
        createColumn<SupplierPayment>({
          accessorKey: "unallocated_amount",
          labelKey: "suppliers.columns.unallocated_amount",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.unallocated_amount}
              currency={row.original.currency}
              muted={Number(row.original.unallocated_amount) === 0}
            />
          ),
        }),
        createColumn<SupplierPayment>({
          accessorKey: "reference",
          labelKey: "suppliers.columns.reference",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.reference || "—",
        }),
        createColumn<SupplierPayment>({
          accessorKey: "status",
          labelKey: "suppliers.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (["posted", "void"] as const).map((s) => ({
            value: s,
            labelKey: PAYMENT_STATUS_LABEL_KEYS[s],
            label: t(PAYMENT_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(PAYMENT_STATUS_LABEL_KEYS[row.original.status])}
              tone={row.original.status === "void" ? "danger" : "success"}
            />
          ),
        }),
        createColumn<SupplierPayment>({
          id: "actions",
          labelKey: "suppliers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "void",
                  label: t("suppliers.actions.void"),
                  icon: Ban,
                  permission: permissions.tenant.finance.cash.write,
                  variant: "destructive" as const,
                  disabled: row.original.status === "void",
                  onSelect: () => setVoiding(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<SupplierPayment, unknown>[],
    [cash.data?.items, locale, slug, supplierUuid, suppliers, t],
  );

  const engineQuery = {
    q: params.q,
    supplier: params.supplier as string | undefined,
    cash_account: params.cash_account as string | undefined,
    method: params.method as string | undefined,
    status: params.status as string | undefined,
    from: params.from as string | undefined,
    to: params.to as string | undefined,
    sort: params.sort,
  };
  const total = query.data?.total ?? 0;
  const hasFilters = Boolean(
    params.q ||
    (!supplierUuid && params.supplier) ||
    params.cash_account ||
    params.method ||
    params.status ||
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
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("suppliers.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("suppliers.payments.empty_filtered")
            : t("suppliers.payments.empty")
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
              resource={SUPPLIER_PAYMENTS_RESOURCE}
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
      <PaymentDialog
        supplierUuid={supplierUuid}
        open={createOpen}
        onOpenChange={(next) => {
          if (!next) onCreateClose?.();
        }}
      />
      <VoidPaymentDialog
        payment={voiding}
        onOpenChange={(next) => {
          if (!next) setVoiding(null);
        }}
      />
    </>
  );
}
