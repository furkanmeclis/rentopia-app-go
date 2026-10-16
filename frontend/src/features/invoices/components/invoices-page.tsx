"use client";

import { FilePlus2, Settings2 } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
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
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { useInvoiceColumns } from "@/features/invoices/components/invoice-columns";
import { RentalsInvoiceDialog } from "@/features/invoices/components/invoice-dialogs";
import { INVOICES_RESOURCE } from "@/features/invoices/constants";
import {
  useInvoiceFacets,
  useInvoicesList,
  useInvoicesMeta,
} from "@/features/invoices/hooks/use-invoices";
import type { ListInvoicesParams } from "@/features/invoices/types";
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
 * "Faturalar": e-Fatura / e-Arşiv invoices of the member's branches.
 * DataTable with status / profile / branch facets, a date range, "needs
 * action" (failed and held sends), export, bulk send of drafts and the
 * bulk "invoice the selected rentals".
 */
export function InvoicesPage() {
  const { t } = useLocale();
  const router = useRouter();
  const search = useSearchParams();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [needsAction, setNeedsAction] = useState(
    search.get("needs_action") === "true",
  );
  const [picker, setPicker] = useState(false);
  const listState = useServerListState({
    initialSort: "-issue_date",
    initialPageSize: 30,
    realtime: tenantLiveList(organization, INVOICES_RESOURCE),
  });

  const params = useMemo<ListInvoicesParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      status: join(col("status")),
      profile: join(col("profile")),
      branch: join(col("branch")),
      from: from || undefined,
      to: to || undefined,
      needs_action: needsAction ? "true" : undefined,
    };
  }, [from, listState.columnFilters, listState.params, needsAction, to]);

  const query = useInvoicesList(params);
  const facets = useInvoiceFacets();
  const meta = useInvoicesMeta();
  const columns = useInvoiceColumns({
    selectable: true,
    facets: facets.data,
    withFilters: true,
  });
  const engineQuery = {
    q: params.q,
    status: params.status,
    profile: params.profile,
    branch: params.branch,
    from: params.from,
    to: params.to,
    needs_action: params.needs_action,
    sort: params.sort,
  };
  const total = query.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery: engineQuery,
    total,
  });
  // The list selection holds invoices: only "send" applies to it; the
  // rentals action runs from its own picker.
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        INVOICES_RESOURCE,
        (meta.data?.bulk_actions ?? []).filter((a) => a.id === "send"),
      ),
    [meta.data?.bulk_actions],
  );
  const hasFilters = Boolean(
    params.q ||
    params.status ||
    params.profile ||
    params.branch ||
    params.from ||
    params.to ||
    params.needs_action,
  );
  const refresh = () => {
    void query.refetch();
    void facets.refetch();
  };

  return (
    <EntityPage
      title={t("invoices.title")}
      description={t("invoices.description")}
      permission={permissions.tenant.invoices.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("invoices.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("invoices.title") },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          {can(permissions.tenant.invoices.settings) ? (
            <Button asChild size="sm" variant="outline">
              <Link href={routes.tenant.invoices.settings(slug)}>
                <Settings2 className="size-4" />
                {t("invoices.settings.short")}
              </Link>
            </Button>
          ) : null}
          {can(permissions.tenant.invoices.bulkCreateForRentals) ? (
            <Button type="button" size="sm" onClick={() => setPicker(true)}>
              <FilePlus2 className="size-4" />
              {t("invoices.bulk.open")}
            </Button>
          ) : null}
        </div>
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
        errorDescription={t("invoices.load_failed")}
        onRetry={refresh}
        onRowClick={(row) =>
          router.push(routes.tenant.invoices.detail(slug, row.uuid))
        }
        realtime={listState.realtimeFor(() =>
          Promise.all([query.refetch(), facets.refetch()]),
        )}
        emptyTitle={
          hasFilters ? t("invoices.empty_filtered") : t("invoices.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("invoices.empty_description")
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
        features={{ persistKey: "tenant-invoices-v1", rowSelection: true }}
        toolbarExtra={
          <>
            <div className="grid w-full grid-cols-2 gap-2 sm:w-auto">
              <DatePicker
                value={from}
                onChange={(v) => setFrom(v ?? "")}
                placeholder={t("invoices.filters.from")}
              />
              <DatePicker
                value={to}
                onChange={(v) => setTo(v ?? "")}
                placeholder={t("invoices.filters.to")}
              />
            </div>
            <div className="flex h-8 items-center gap-2">
              <Switch
                id="invoices-needs-action"
                checked={needsAction}
                onCheckedChange={setNeedsAction}
              />
              <Label htmlFor="invoices-needs-action" className="text-sm">
                {t("invoices.filters.needs_action", {
                  count: facets.data?.needs_action ?? 0,
                })}
              </Label>
            </div>
            <BulkActionMenu
              resource={INVOICES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={INVOICES_RESOURCE}
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
      <RentalsInvoiceDialog
        open={picker}
        onClose={() => {
          setPicker(false);
          refresh();
        }}
      />
    </EntityPage>
  );
}
