"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { CustomerQuickEditDrawer } from "@/features/customers/components/customer-quick-edit-drawer";
import {
  SensitiveExportButton,
  useCustomerBulkParams,
} from "@/features/customers/components/customer-toolbar-extras";
import {
  useCustomersColumns,
  type CustomerRowActionHandlers,
} from "@/features/customers/components/customers-columns";
import { CUSTOMERS_RESOURCE } from "@/features/customers/constants";
import {
  useCustomerFacets,
  useCustomersList,
  useCustomersMeta,
  useDeleteCustomer,
} from "@/features/customers/hooks/use-customers";
import type { Customer, ListCustomersParams } from "@/features/customers/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Customers list (tenant list standard): server-side pagination / sort /
 * search (names, phone digits, exact identity number), faceted filters
 * (type, nationality, tag, consent), masked identity / phone / e-mail
 * columns, row actions, quick edit, export (masked; a separate sensitive
 * export with reveal), İdes import, tag bulk actions and live updates.
 */
export function CustomersPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CUSTOMERS_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Customer | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Customer | null>(null);
  const bulkParams = useCustomerBulkParams();

  const listParams = useMemo<ListCustomersParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const join = (id: string) => {
      const values = column(id);
      return values.length ? values.join(",") : undefined;
    };
    const anonymized = column("anonymized");
    return {
      ...listState.params,
      customer_type: join("customer_type"),
      nationality: join("nationality"),
      tag: join("tag"),
      consent: join("consent"),
      // Both values selected = no filter.
      anonymized:
        anonymized.length === 1
          ? (anonymized[0] as "true" | "false")
          : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useCustomersList(listParams);
  const metaQuery = useCustomersMeta();
  const facetsQuery = useCustomerFacets();
  const deleteCustomer = useDeleteCustomer();

  const openDetail = useCallback(
    (customer: Customer) =>
      router.push(routes.tenant.customers.detail(slug, customer.uuid)),
    [router, slug],
  );

  const handlers = useMemo<CustomerRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (customer) =>
        router.push(routes.tenant.customers.edit(slug, customer.uuid)),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug],
  );

  const baseColumns = useCustomersColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Customer>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      customer_type: listParams.customer_type,
      nationality: listParams.nationality,
      tag: listParams.tag,
      consent: listParams.consent,
      anonymized: listParams.anonymized,
      sort: listParams.sort,
    }),
    [
      listParams.anonymized,
      listParams.consent,
      listParams.customer_type,
      listParams.nationality,
      listParams.q,
      listParams.sort,
      listParams.tag,
    ],
  );

  const total = listQuery.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery: engineQuery,
    total,
  });

  const meta = metaQuery.data;
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(CUSTOMERS_RESOURCE, meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && Boolean(value),
  );

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("customers.title")}
      description={t("customers.description")}
      permission={permissions.tenant.customers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("customers.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.customers.create(slug))}
          label={t("customers.actions.create")}
          permission={permissions.tenant.customers.write}
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
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("customers.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("customers.empty_filtered_title")
            : t("customers.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("customers.empty_filtered_description")
            : t("customers.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-customers-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={CUSTOMERS_RESOURCE}
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
              resource={CUSTOMERS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
              onImportComplete={refresh}
            />
            {meta?.capabilities?.export ? (
              <SensitiveExportButton query={engineQuery} />
            ) : null}
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      {bulkParams.dialog}

      <CustomerQuickEditDrawer
        customer={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget
            ? deleteTarget.display_name || t("customers.anonymized_badge")
            : ""
        }
        isPending={deleteCustomer.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteCustomer.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
