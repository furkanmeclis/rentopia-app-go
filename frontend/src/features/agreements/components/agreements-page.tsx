"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityActions,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AgreementStatusChip,
  ValidityChip,
  ValidityRange,
} from "@/features/agreements/components/agreement-cells";
import { AgreementDrawer } from "@/features/agreements/components/agreement-drawer";
import {
  AGREEMENTS_RESOURCE,
  AGREEMENT_STATUSES,
  AGREEMENT_STATUS_LABEL_KEYS,
  AGREEMENT_VALIDITIES,
  INVOICE_PERIODS,
  INVOICE_PERIOD_LABEL_KEYS,
  VALIDITY_LABEL_KEYS,
} from "@/features/agreements/constants";
import {
  useAgreement,
  useAgreementFacets,
  useAgreementsList,
  useAgreementsMeta,
} from "@/features/agreements/hooks/use-agreements";
import type {
  AgreementListItem,
  ListAgreementsParams,
} from "@/features/agreements/types";
import {
  CodeBadge,
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { moneyText } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

type FacetValue = { value: string; label?: string; count: number };

function countOf(facets: FacetValue[] | undefined, value: string) {
  return facets?.find((f) => f.value === value)?.count ?? 0;
}

function facetOptions(facets: FacetValue[] | undefined) {
  return (facets ?? []).map((f) => ({
    value: f.value,
    label: `${f.label || f.value} (${f.count})`,
  }));
}

/**
 * Corporate agreements ("Kurumsal anlaşmalar"): status, customer,
 * validity, branch and invoice period facets, a validity window filter,
 * live updates and export.
 */
export function AgreementsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const [createOpen, setCreateOpen] = useState(false);
  const [editUuid, setEditUuid] = useState<string | null>(null);
  const [validFrom, setValidFrom] = useState("");
  const [validTo, setValidTo] = useState("");
  const editing = useAgreement(editUuid);
  const urlDefaults = useUrlListDefaults(
    ["status", "customer", "branch", "invoice_period", "validity"],
    ["code", "name", "customer", "status", "valid_from", "valid_to"],
  );
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-created_at",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, AGREEMENTS_RESOURCE),
  });

  const listParams = useMemo<ListAgreementsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      customer: joinFilter(column("customer")),
      branch: joinFilter(column("branch")),
      invoice_period: joinFilter(column("invoice_period")),
      validity: joinFilter(column("validity")),
      valid_from: validFrom || undefined,
      valid_to: validTo || undefined,
    };
  }, [listState.columnFilters, listState.params, validFrom, validTo]);

  const listQuery = useAgreementsList(listParams);
  const metaQuery = useAgreementsMeta();
  const facetsQuery = useAgreementFacets();
  const facets = facetsQuery.data;

  const columns = useMemo(
    () =>
      [
        createColumn<AgreementListItem>({
          accessorKey: "code",
          labelKey: "agreements.columns.code",
          enableSorting: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<AgreementListItem>({
          accessorKey: "name",
          labelKey: "agreements.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<AgreementListItem>({
          id: "customer",
          accessorFn: (row) => row.customer.display_name,
          labelKey: "agreements.columns.customer",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.customer),
          cell: ({ row }) => row.original.customer.display_name,
        }),
        createColumn<AgreementListItem>({
          accessorKey: "status",
          labelKey: "agreements.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: AGREEMENT_STATUSES.map((v) => ({
            value: v,
            labelKey: AGREEMENT_STATUS_LABEL_KEYS[v],
            label: `${t(AGREEMENT_STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => (
            <AgreementStatusChip status={row.original.status} />
          ),
        }),
        createColumn<AgreementListItem>({
          accessorKey: "validity",
          labelKey: "agreements.columns.validity",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: AGREEMENT_VALIDITIES.map((v) => ({
            value: v,
            labelKey: VALIDITY_LABEL_KEYS[v],
            label: `${t(VALIDITY_LABEL_KEYS[v])} (${countOf(facets?.validity, v)})`,
          })),
          cell: ({ row }) => <ValidityChip validity={row.original.validity} />,
        }),
        createColumn<AgreementListItem>({
          accessorKey: "valid_from",
          labelKey: "agreements.columns.valid_from",
          enableSorting: true,
          cell: ({ row }) => (
            <ValidityRange
              from={row.original.valid_from}
              to={row.original.valid_to}
            />
          ),
        }),
        createColumn<AgreementListItem>({
          id: "valid_to",
          accessorFn: (row) => row.valid_to ?? "",
          labelKey: "agreements.columns.valid_to",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.valid_to ?? t("agreements.open_ended"),
        }),
        createColumn<AgreementListItem>({
          accessorKey: "invoice_period",
          labelKey: "agreements.columns.invoice_period",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INVOICE_PERIODS.map((v) => ({
            value: v,
            labelKey: INVOICE_PERIOD_LABEL_KEYS[v],
            label: `${t(INVOICE_PERIOD_LABEL_KEYS[v])} (${countOf(facets?.invoice_period, v)})`,
          })),
          cell: ({ row }) =>
            t(INVOICE_PERIOD_LABEL_KEYS[row.original.invoice_period]),
        }),
        createColumn<AgreementListItem>({
          accessorKey: "payment_term_days",
          labelKey: "agreements.columns.payment_term_days",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            t("agreements.days", { count: row.original.payment_term_days }),
        }),
        createColumn<AgreementListItem>({
          accessorKey: "open_account",
          labelKey: "agreements.columns.open_account",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.open_account ? (
              <Badge variant="outline">
                {moneyText(row.original.credit_limit, locale)}
              </Badge>
            ) : (
              "—"
            ),
        }),
        createColumn<AgreementListItem>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "agreements.columns.branch",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.branch),
          cell: ({ row }) =>
            row.original.branch?.name ?? t("agreements.head_office"),
        }),
        createColumn<AgreementListItem>({
          accessorKey: "reservation_count",
          labelKey: "agreements.columns.reservation_count",
          enableSorting: false,
          cell: ({ row }) => row.original.reservation_count,
        }),
        createColumn<AgreementListItem>({
          accessorKey: "tariff_count",
          labelKey: "agreements.columns.tariff_count",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.tariff_count,
        }),
        ...auditColumns<AgreementListItem>(
          {
            createdAt: "agreements.columns.created_at",
            uuid: "agreements.columns.uuid",
          },
          locale,
        ),
        createColumn<AgreementListItem>({
          id: "actions",
          labelKey: "agreements.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("agreements.actions.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.agreements.detail(slug, row.original.uuid),
                    ),
                },
                {
                  id: "edit",
                  label: t("agreements.actions.edit"),
                  icon: Pencil,
                  permission: permissions.tenant.agreements.write,
                  disabled: row.original.status === "terminated",
                  onSelect: () => setEditUuid(row.original.uuid),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AgreementListItem, unknown>[],
    [facets, locale, router, slug, t],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      customer: listParams.customer,
      branch: listParams.branch,
      invoice_period: listParams.invoice_period,
      validity: listParams.validity,
      valid_from: listParams.valid_from,
      valid_to: listParams.valid_to,
      sort: listParams.sort,
    }),
    [listParams],
  );
  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && Boolean(value),
  );
  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("agreements.title")}
      description={t("agreements.description")}
      permission={permissions.tenant.agreements.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("agreements.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("agreements.title") },
      ]}
      actions={
        <EntityActions>
          <PermissionGuard permission={permissions.tenant.agreements.write}>
            <Button type="button" size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              {t("agreements.actions.create")}
            </Button>
          </PermissionGuard>
        </EntityActions>
      }
    >
      <div className="mb-3 flex flex-wrap items-end gap-3">
        <div className="grid gap-1 text-sm">
          <span className="text-muted-foreground text-xs">
            {t("agreements.filters.valid_from")}
          </span>
          <DatePicker
            id="agreements-valid-from"
            value={validFrom}
            onChange={setValidFrom}
            placeholder={t("agreements.filters.any_day")}
          />
        </div>
        <div className="grid gap-1 text-sm">
          <span className="text-muted-foreground text-xs">
            {t("agreements.filters.valid_to")}
          </span>
          <DatePicker
            id="agreements-valid-to"
            value={validTo}
            onChange={setValidTo}
            placeholder={t("agreements.filters.any_day")}
          />
        </div>
        {validFrom || validTo ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setValidFrom("");
              setValidTo("");
            }}
          >
            {t("agreements.filters.clear_dates")}
          </Button>
        ) : null}
      </div>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(r) =>
          router.push(routes.tenant.agreements.detail(slug, r.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("agreements.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("agreements.empty_filtered_title")
            : t("agreements.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("agreements.empty_filtered_description")
            : t("agreements.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "tenant-agreements-v1", rowSelection: false }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={AGREEMENTS_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <AgreementDrawer
        open={createOpen}
        agreement={null}
        onOpenChange={setCreateOpen}
        onCreated={(a) =>
          router.push(routes.tenant.agreements.detail(slug, a.uuid))
        }
      />
      <AgreementDrawer
        open={editUuid !== null && editing.data !== undefined}
        agreement={editing.data ?? null}
        onOpenChange={(o) => {
          if (!o) setEditUuid(null);
        }}
      />
    </EntityPage>
  );
}
