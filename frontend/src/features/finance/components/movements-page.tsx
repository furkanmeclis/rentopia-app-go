"use client";

import { ArrowLeftRight } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import {
  ReverseMovementDialog,
  TransferDialog,
} from "@/features/finance/components/cash-dialogs";
import { useCategoryBulkParams } from "@/features/finance/components/category-bulk-params";
import { useMovementColumns } from "@/features/finance/components/movement-columns";
import { MOVEMENTS_RESOURCE } from "@/features/finance/constants";
import {
  useCashAccountsList,
  useCategories,
  useFinanceMeta,
  useMovementsList,
} from "@/features/finance/hooks/use-finance";
import type { CashMovement, ListParams } from "@/features/finance/types";
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

/**
 * "Kasa / banka hareketleri": the movement ledger of every account the
 * member may see. DataTable with account / kind / category facets, a date
 * range, export / import (bank statement lines) and the bulk "set category".
 */
export function MovementsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [transferOpen, setTransferOpen] = useState(false);
  const [reverse, setReverse] = useState<CashMovement | null>(null);
  const listState = useServerListState({
    initialSort: "-movement_date",
    initialPageSize: 30,
    realtime: tenantLiveList(organization, MOVEMENTS_RESOURCE),
  });
  const accounts = useCashAccountsList({ limit: 100, offset: 0, sort: "code" });
  const categories = useCategories(false);

  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      account: join(col("account")),
      kind: join(col("kind")),
      category: join(col("category")),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, to]);

  const query = useMovementsList(params);
  const meta = useFinanceMeta("movements");
  const columns = useMovementColumns({
    selectable: true,
    // The running balance is per account: shown when one account is chosen.
    showBalance:
      typeof params.account === "string" && !params.account.includes(","),
    categories: categories.data?.items,
    accounts: accounts.data?.items,
    onReverse: can(permissions.tenant.finance.cash.write)
      ? setReverse
      : undefined,
  });
  const engineQuery = {
    q: params.q,
    account: params.account as string | undefined,
    kind: params.kind as string | undefined,
    category: params.category as string | undefined,
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
        MOVEMENTS_RESOURCE,
        meta.data?.bulk_actions ?? [],
      ),
    [meta.data?.bulk_actions],
  );
  const bulkParams = useCategoryBulkParams();
  const hasFilters = Boolean(
    params.q ||
    params.account ||
    params.kind ||
    params.category ||
    params.from ||
    params.to,
  );
  const refresh = () => void query.refetch();

  return (
    <EntityPage
      title={t("finance.movements.title")}
      description={t("finance.movements.description")}
      permission={permissions.tenant.finance.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.movements.title") },
      ]}
      actions={
        can(permissions.tenant.finance.cash.write) ? (
          <Button type="button" size="sm" onClick={() => setTransferOpen(true)}>
            <ArrowLeftRight className="size-4" />
            {t("finance.actions.transfer")}
          </Button>
        ) : null
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
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("finance.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("finance.movements.empty_filtered")
            : t("finance.movements.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("finance.movements.empty_description")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 30)),
        )}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-finance-movements-v1",
          rowSelection: true,
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
              resource={MOVEMENTS_RESOURCE}
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
              resource={MOVEMENTS_RESOURCE}
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
      <TransferDialog open={transferOpen} onOpenChange={setTransferOpen} />
      <ReverseMovementDialog
        movement={reverse}
        onOpenChange={(open) => {
          if (!open) setReverse(null);
        }}
      />
    </EntityPage>
  );
}
