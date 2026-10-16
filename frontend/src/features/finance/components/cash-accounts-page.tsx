"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowLeftRight, Eye, Pencil } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

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
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CashAccountDialog,
  TransferDialog,
} from "@/features/finance/components/cash-dialogs";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  ACCOUNT_STATUS_LABEL_KEYS,
  CASH_ACCOUNTS_RESOURCE,
  CASH_KIND_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  useCashAccountsList,
  useFinanceMeta,
} from "@/features/finance/hooks/use-finance";
import type { CashAccount, ListParams } from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** "Kasa ve banka hesapları": tills and bank accounts with balances. */
export function CashAccountsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const [editing, setEditing] = useState<CashAccount | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [transfer, setTransfer] = useState<CashAccount | null | undefined>(
    undefined,
  );
  const listState = useServerListState({
    initialSort: "code",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CASH_ACCOUNTS_RESOURCE),
  });
  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const kind = col("kind");
    const status = col("status");
    return {
      ...listState.params,
      kind: kind.length ? kind.join(",") : undefined,
      status: status.length ? status.join(",") : undefined,
    };
  }, [listState.columnFilters, listState.params]);
  const query = useCashAccountsList(params);
  const meta = useFinanceMeta("cash");
  const write = can(permissions.tenant.finance.cash.write);
  const open = useCallback(
    (a: CashAccount) =>
      router.push(routes.tenant.finance.cashAccount(slug, a.uuid)),
    [router, slug],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<CashAccount>({
          accessorKey: "code",
          labelKey: "finance.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
              {row.original.code}
            </code>
          ),
        }),
        createColumn<CashAccount>({
          accessorKey: "name",
          labelKey: "finance.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <div className="font-medium">{row.original.name}</div>
              {row.original.iban ? (
                <div className="text-muted-foreground truncate font-mono text-xs">
                  {row.original.iban}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<CashAccount>({
          accessorKey: "kind",
          labelKey: "finance.columns.cash_kind",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (["cash", "bank"] as const).map((k) => ({
            value: k,
            labelKey: CASH_KIND_LABEL_KEYS[k],
            label: t(CASH_KIND_LABEL_KEYS[k]),
          })),
          cell: ({ row }) => t(CASH_KIND_LABEL_KEYS[row.original.kind]),
        }),
        createColumn<CashAccount>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "finance.columns.branch",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.branch?.name ?? t("finance.head_office"),
        }),
        createColumn<CashAccount>({
          accessorKey: "balance",
          labelKey: "finance.columns.balance",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.balance}
              currency={row.original.currency}
              className="font-semibold"
            />
          ),
        }),
        createColumn<CashAccount>({
          accessorKey: "total_in",
          labelKey: "finance.columns.total_in",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => <Amount value={row.original.total_in} />,
        }),
        createColumn<CashAccount>({
          accessorKey: "total_out",
          labelKey: "finance.columns.total_out",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => <Amount value={row.original.total_out} />,
        }),
        createColumn<CashAccount>({
          accessorKey: "status",
          labelKey: "finance.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (["active", "archived"] as const).map((s) => ({
            value: s,
            labelKey: ACCOUNT_STATUS_LABEL_KEYS[s],
            label: t(ACCOUNT_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(ACCOUNT_STATUS_LABEL_KEYS[row.original.status])}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<CashAccount>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("finance.actions.open"),
                  icon: Eye,
                  onSelect: () => open(row.original),
                },
                {
                  id: "edit",
                  label: t("finance.actions.edit"),
                  icon: Pencil,
                  permission: permissions.tenant.finance.cash.write,
                  onSelect: () => setEditing(row.original),
                },
                {
                  id: "transfer",
                  label: t("finance.actions.transfer"),
                  icon: ArrowLeftRight,
                  permission: permissions.tenant.finance.cash.write,
                  disabled: row.original.status !== "active",
                  onSelect: () => setTransfer(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CashAccount, unknown>[],
    [open, t],
  );

  const total = query.data?.total ?? 0;
  const engineQuery = {
    q: params.q,
    kind: params.kind as string | undefined,
    status: params.status as string | undefined,
    sort: params.sort,
  };
  const hasFilters = Boolean(params.q || params.kind || params.status);
  const refresh = () => void query.refetch();

  return (
    <EntityPage
      title={t("finance.cash.title")}
      description={t("finance.cash.description")}
      permission={permissions.tenant.finance.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.cash.title") },
      ]}
      actions={
        <>
          {write ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setTransfer(null)}
            >
              <ArrowLeftRight className="size-4" />
              {t("finance.actions.transfer")}
            </Button>
          ) : null}
          <EntityCreateButton
            onClick={() => setCreateOpen(true)}
            label={t("finance.cash.create")}
            permission={permissions.tenant.finance.cash.write}
          />
        </>
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={open}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("finance.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("finance.cash.empty_filtered")
            : t("finance.cash.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("finance.cash.empty_description")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-finance-cash-accounts-v1",
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={CASH_ACCOUNTS_RESOURCE}
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
      <CashAccountDialog
        account={editing}
        open={createOpen || Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) {
            setCreateOpen(false);
            setEditing(null);
          }
        }}
      />
      <TransferDialog
        from={transfer ?? null}
        open={transfer !== undefined}
        onOpenChange={(next) => {
          if (!next) setTransfer(undefined);
        }}
      />
    </EntityPage>
  );
}
