"use client";

import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
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
import { useCollectionColumns } from "@/features/finance/components/collection-columns";
import { COLLECTIONS_RESOURCE } from "@/features/finance/constants";
import {
  useCashAccountsList,
  useCollectionsList,
  useFinanceMeta,
} from "@/features/finance/hooks/use-finance";
import type { ListParams } from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * "Tahsilatlar": every collection the member may see (cash, transfer, POS,
 * online). DataTable with method / branch / status / cash account facets, a
 * date range, export and the bulk "allocate the credit".
 */
export function CollectionsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const { slug, organization } = useTenant();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const listState = useServerListState({
    initialSort: "-collection_date",
    initialPageSize: 30,
    realtime: tenantLiveList(organization, COLLECTIONS_RESOURCE),
  });
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const cashAccounts = useCashAccountsList({
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
      method: join(col("method")),
      branch: join(col("branch")),
      status: join(col("status")),
      allocation_status: join(col("allocation_status")),
      cash_account: join(col("cash_account")),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, to]);

  const query = useCollectionsList(params);
  const meta = useFinanceMeta("collections");
  const columns = useCollectionColumns({
    selectable: true,
    branches: branches.data?.items,
    cashAccounts: cashAccounts.data?.items,
  });
  const engineQuery = {
    q: params.q,
    method: params.method as string | undefined,
    branch: params.branch as string | undefined,
    status: params.status as string | undefined,
    allocation_status: params.allocation_status as string | undefined,
    cash_account: params.cash_account as string | undefined,
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
        COLLECTIONS_RESOURCE,
        meta.data?.bulk_actions ?? [],
      ),
    [meta.data?.bulk_actions],
  );
  const hasFilters = Boolean(
    params.q ||
    params.method ||
    params.branch ||
    params.status ||
    params.allocation_status ||
    params.cash_account ||
    params.from ||
    params.to,
  );
  const refresh = () => void query.refetch();

  return (
    <EntityPage
      title={t("finance.collections.title")}
      description={t("finance.collections.description")}
      permission={permissions.tenant.finance.accounts.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.collections.title") },
      ]}
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
        onRowClick={(row) =>
          router.push(routes.tenant.finance.collection(slug, row.uuid))
        }
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("finance.collections.empty_filtered")
            : t("finance.collections.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("finance.collections.empty_description")
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
          persistKey: "tenant-finance-collections-v1",
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
              resource={COLLECTIONS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={COLLECTIONS_RESOURCE}
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
    </EntityPage>
  );
}
