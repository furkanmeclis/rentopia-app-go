"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { DatePicker } from "@/components/ui/date-picker";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  Amount,
  BalanceStatusChip,
} from "@/features/finance/components/finance-cells";
import { FinanceSummaryCards } from "@/features/finance/components/finance-summary";
import {
  ACCOUNTS_RESOURCE,
  BALANCE_STATUSES,
  BALANCE_STATUS_LABEL_KEYS,
  DUE_BUCKET_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  useAccountsList,
  useFinanceMeta,
  useFinanceSummary,
  useRentalBalances,
} from "@/features/finance/hooks/use-finance";
import type {
  FinanceAccount,
  FinanceRentalBalance,
  FinanceSummary,
  ListParams,
} from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * "Cari hesaplar": organization totals, the accounts DataTable (balance
 * status and branch facets, open debts due before a day, export) and the
 * per-rental / per-branch / due-date breakdowns.
 */
export function AccountsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("finance.accounts.title")}
      description={t("finance.accounts.description")}
      permission={permissions.tenant.finance.accounts.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.accounts.title") },
      ]}
    >
      <div className="space-y-6">
        <FinanceSummaryCards />
        <Tabs defaultValue="accounts" className="space-y-4">
          <div className="-mx-1 overflow-x-auto px-1 pb-1">
            <TabsList className="h-auto w-max">
              <TabsTrigger value="accounts">
                {t("finance.accounts.tab_accounts")}
              </TabsTrigger>
              <TabsTrigger value="rentals">
                {t("finance.accounts.tab_rentals")}
              </TabsTrigger>
              <TabsTrigger value="breakdown">
                {t("finance.accounts.tab_breakdown")}
              </TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="accounts">
            <AccountsTable />
          </TabsContent>
          <TabsContent value="rentals">
            <RentalBalancesTable />
          </TabsContent>
          <TabsContent value="breakdown">
            <Breakdown />
          </TabsContent>
        </Tabs>
      </div>
    </EntityPage>
  );
}

function AccountsTable() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const [dueBefore, setDueBefore] = useState("");
  const listState = useServerListState({
    initialSort: "-balance",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, ACCOUNTS_RESOURCE),
  });
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });

  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const status = col("balance_status");
    const branch = col("branch");
    return {
      ...listState.params,
      balance_status: status.length ? status.join(",") : undefined,
      branch: branch.length ? branch.join(",") : undefined,
      due_before: dueBefore || undefined,
    };
  }, [dueBefore, listState.columnFilters, listState.params]);

  const query = useAccountsList(params);
  const meta = useFinanceMeta("accounts");
  const open = useCallback(
    (a: FinanceAccount) =>
      router.push(routes.tenant.finance.account(slug, a.uuid)),
    [router, slug],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<FinanceAccount>({
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
        createColumn<FinanceAccount>({
          accessorKey: "name",
          labelKey: "finance.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.name || t("finance.accounts.anonymous")}
            </span>
          ),
        }),
        createColumn<FinanceAccount>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "finance.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (branches.data?.items ?? []).map((b) => ({
            value: b.uuid,
            label: `${b.code} · ${b.name}`,
          })),
          cell: ({ row }) => row.original.branch?.name ?? "—",
        }),
        createColumn<FinanceAccount>({
          id: "balance",
          accessorFn: (row) => row.balance.balance,
          labelKey: "finance.columns.balance",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.balance.balance}
              currency={row.original.currency}
              signed
              className="font-semibold"
            />
          ),
        }),
        createColumn<FinanceAccount>({
          id: "overdue",
          accessorFn: (row) => row.balance.overdue,
          labelKey: "finance.columns.overdue",
          enableSorting: true,
          cell: ({ row }) =>
            Number(row.original.balance.overdue) > 0 ? (
              <Amount
                value={row.original.balance.overdue}
                className="text-destructive"
              />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<FinanceAccount>({
          id: "due_soon",
          accessorFn: (row) => row.balance.due_soon,
          labelKey: "finance.columns.due_soon",
          enableSorting: false,
          cell: ({ row }) =>
            Number(row.original.balance.due_soon) > 0 ? (
              <Amount value={row.original.balance.due_soon} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<FinanceAccount>({
          id: "deposit_held",
          accessorFn: (row) => row.balance.deposit_held,
          labelKey: "finance.columns.deposit_held",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <Amount value={row.original.balance.deposit_held} />
          ),
        }),
        createColumn<FinanceAccount>({
          id: "balance_status",
          accessorFn: (row) => row.balance.status,
          labelKey: "finance.columns.balance_status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: BALANCE_STATUSES.map((s) => ({
            value: s,
            labelKey: BALANCE_STATUS_LABEL_KEYS[s],
            label: t(BALANCE_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <BalanceStatusChip status={row.original.balance.status} />
          ),
        }),
        createColumn<FinanceAccount>({
          accessorKey: "payment_term_days",
          labelKey: "finance.columns.payment_term_days",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            t("finance.accounts.term_days", {
              count: row.original.payment_term_days,
            }),
        }),
        createColumn<FinanceAccount>({
          accessorKey: "last_entry_at",
          labelKey: "finance.columns.last_entry_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.last_entry_at
              ? datetime(row.original.last_entry_at)
              : "—",
        }),
        createColumn<FinanceAccount>({
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
                  label: t("finance.actions.open_statement"),
                  icon: Eye,
                  onSelect: () => open(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<FinanceAccount, unknown>[],
    [branches.data?.items, open, t],
  );

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const engineQuery = {
    q: params.q,
    balance_status: params.balance_status as string | undefined,
    branch: params.branch as string | undefined,
    due_before: params.due_before as string | undefined,
    sort: params.sort,
  };
  const hasFilters = Boolean(
    params.q || params.balance_status || params.branch || params.due_before,
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={open}
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("finance.load_failed")}
      onRetry={() => void query.refetch()}
      realtime={listState.realtimeFor(() => query.refetch())}
      emptyTitle={
        hasFilters
          ? t("finance.accounts.empty_filtered")
          : t("finance.accounts.empty")
      }
      emptyDescription={
        hasFilters ? undefined : t("finance.accounts.empty_description")
      }
      pageCount={pageCount}
      state={listState.tableState}
      features={{ persistKey: "tenant-finance-accounts-v1", viewMode: true }}
      toolbarExtra={
        <>
          <div className="w-full sm:w-52">
            <DatePicker
              value={dueBefore}
              onChange={setDueBefore}
              placeholder={t("finance.accounts.due_before")}
            />
          </div>
          <ResourceIOToolbar
            resource={ACCOUNTS_RESOURCE}
            query={engineQuery}
            capabilities={meta.data?.capabilities}
          />
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        </>
      }
    />
  );
}

function RentalBalancesTable() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
  });
  const params = listState.params as ListParams;
  const query = useRentalBalances({ ...params, open: "true", sort: undefined });
  const columns = useMemo(
    () =>
      [
        createColumn<FinanceRentalBalance>({
          accessorKey: "reference",
          labelKey: "finance.columns.reference",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs font-semibold">
              {row.original.reference || "—"}
            </span>
          ),
        }),
        createColumn<FinanceRentalBalance>({
          id: "account",
          accessorFn: (row) => row.account?.name ?? "",
          labelKey: "finance.columns.account",
          enableSorting: false,
          cell: ({ row }) => row.original.account?.name ?? "—",
        }),
        createColumn<FinanceRentalBalance>({
          accessorKey: "debit",
          labelKey: "finance.columns.debit",
          enableSorting: false,
          cell: ({ row }) => <Amount value={row.original.debit} />,
        }),
        createColumn<FinanceRentalBalance>({
          accessorKey: "credit",
          labelKey: "finance.columns.credit",
          enableSorting: false,
          cell: ({ row }) => <Amount value={row.original.credit} />,
        }),
        createColumn<FinanceRentalBalance>({
          accessorKey: "balance",
          labelKey: "finance.columns.balance",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.balance}
              signed
              className="font-semibold"
            />
          ),
        }),
        createColumn<FinanceRentalBalance>({
          accessorKey: "deposit_held",
          labelKey: "finance.columns.deposit_held",
          enableSorting: false,
          cell: ({ row }) => <Amount value={row.original.deposit_held} />,
        }),
      ] as ColumnDef<FinanceRentalBalance, unknown>[],
    [],
  );
  const total = query.data?.total ?? 0;
  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.reservation_uuid}
      onRowClick={(row) =>
        row.rental_uuid
          ? router.push(routes.tenant.rentals.detail(slug, row.rental_uuid))
          : undefined
      }
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("finance.load_failed")}
      onRetry={() => void query.refetch()}
      emptyTitle={t("finance.rental_balances.empty")}
      pageCount={Math.max(
        1,
        Math.ceil(total / (listState.pagination.pageSize || 20)),
      )}
      state={listState.tableState}
      features={{
        persistKey: "tenant-finance-rental-balances-v1",
        sorting: false,
      }}
    />
  );
}

type BranchRow = FinanceSummary["by_branch"][number];
type DueRow = FinanceSummary["by_due"][number];

function Breakdown() {
  const { t } = useLocale();
  const query = useFinanceSummary();
  const branchColumns = useMemo(
    () =>
      [
        createColumn<BranchRow>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "finance.columns.branch",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.branch
              ? `${row.original.branch.code} · ${row.original.branch.name}`
              : t("finance.head_office"),
        }),
        createColumn<BranchRow>({
          accessorKey: "debit",
          labelKey: "finance.columns.debit",
          enableSorting: false,
          cell: ({ row }) => <Amount value={row.original.debit} />,
        }),
        createColumn<BranchRow>({
          accessorKey: "credit",
          labelKey: "finance.columns.credit",
          enableSorting: false,
          cell: ({ row }) => <Amount value={row.original.credit} />,
        }),
        createColumn<BranchRow>({
          accessorKey: "balance",
          labelKey: "finance.columns.balance",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.balance}
              signed
              className="font-semibold"
            />
          ),
        }),
      ] as ColumnDef<BranchRow, unknown>[],
    [t],
  );
  const dueColumns = useMemo(
    () =>
      [
        createColumn<DueRow>({
          accessorKey: "key",
          labelKey: "finance.columns.due_bucket",
          enableSorting: false,
          cell: ({ row }) =>
            t(DUE_BUCKET_LABEL_KEYS[row.original.key] ?? row.original.key),
        }),
        createColumn<DueRow>({
          accessorKey: "amount",
          labelKey: "finance.columns.open_amount",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount value={row.original.amount} className="font-semibold" />
          ),
        }),
      ] as ColumnDef<DueRow, unknown>[],
    [t],
  );
  const manual = { filtering: false, sorting: false, pagination: false };
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <EntitySectionCard title={t("finance.breakdown.by_branch")}>
        <EntityTable
          columns={branchColumns}
          data={query.data?.by_branch ?? []}
          getRowId={(row, i) => row.branch?.uuid ?? `head-${i}`}
          isLoading={query.isLoading}
          emptyTitle={t("finance.breakdown.empty")}
          manual={manual}
          features={{
            persistKey: "tenant-finance-by-branch-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
      </EntitySectionCard>
      <EntitySectionCard title={t("finance.breakdown.by_due")}>
        <EntityTable
          columns={dueColumns}
          data={query.data?.by_due ?? []}
          getRowId={(row) => row.key}
          isLoading={query.isLoading}
          emptyTitle={t("finance.breakdown.empty")}
          manual={manual}
          features={{
            persistKey: "tenant-finance-by-due-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}
