"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityDrawer,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import {
  DateRuleForm,
  dateRuleBody,
  dateRuleDefaults,
} from "@/features/pricing/components/date-rule-form";
import {
  DATE_RULES_RESOURCE,
  KIND_LABEL_KEYS,
  PRICING_STATUSES,
  PRODUCT_SCOPE_LABEL_KEYS,
  RULE_KINDS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/pricing/constants";
import {
  useCreateDateRule,
  useDateRuleFacets,
  useDateRulesList,
  useDateRulesMeta,
  useDeleteDateRule,
} from "@/features/pricing/hooks/use-pricing";
import { rangeText, ruleValueText } from "@/features/pricing/lib/format";
import type {
  DateRule,
  DateRuleFacets,
  DateRuleKind,
  ListDateRulesParams,
  ProductScope,
} from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "date-rule-create-form";

type FacetList = DateRuleFacets[keyof DateRuleFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

/**
 * Named date rules ("Tarih kuralları"): YAZ, BAYRAM … adjust the daily price
 * of the days that start inside `[start, end)`. Status / kind facets and a
 * date window filter; live updates.
 */
export function DateRulesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "start_date",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, DATE_RULES_RESOURCE),
  });
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DateRule | null>(null);
  const write = permissions.tenant.pricing.write;
  const create = useCreateDateRule();

  const listParams = useMemo<ListDateRulesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      kind: joinFilter(column("kind")),
      date_from: dateFrom || undefined,
      date_to: dateTo || undefined,
    };
  }, [dateFrom, dateTo, listState.columnFilters, listState.params]);

  const listQuery = useDateRulesList(listParams);
  const metaQuery = useDateRulesMeta();
  const facetsQuery = useDateRuleFacets();
  const remove = useDeleteDateRule();
  const facets = facetsQuery.data;

  const columns = useMemo(
    () =>
      [
        createColumn<DateRule>({
          accessorKey: "name",
          labelKey: "pricing.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<DateRule>({
          id: "start_date",
          accessorFn: (row) => row.start_date ?? "",
          labelKey: "pricing.columns.start_date",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.start_date
              ? date(row.original.start_date, "d MMM yyyy", locale)
              : "—",
        }),
        createColumn<DateRule>({
          id: "end_date",
          accessorFn: (row) => row.end_date ?? "",
          labelKey: "pricing.columns.end_date",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.end_date ? (
              <span>
                {date(row.original.end_date, "d MMM yyyy", locale)}{" "}
                <span className="text-muted-foreground text-xs">
                  {t("pricing.excluded")}
                </span>
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<DateRule>({
          accessorKey: "kind",
          labelKey: "pricing.columns.kind",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: RULE_KINDS.map((k) => ({
            value: k,
            labelKey: KIND_LABEL_KEYS[k],
            label: `${t(KIND_LABEL_KEYS[k])} (${countOf(facets?.kind, k)})`,
          })),
          cell: ({ row }) =>
            row.original.kind
              ? t(KIND_LABEL_KEYS[row.original.kind as DateRuleKind])
              : "—",
        }),
        createColumn<DateRule>({
          accessorKey: "value",
          labelKey: "pricing.columns.value",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.kind
                ? ruleValueText(row.original.kind, row.original.value, locale)
                : "—"}
            </span>
          ),
        }),
        createColumn<DateRule>({
          id: "scope",
          accessorFn: (row) => row.product_scope,
          labelKey: "pricing.columns.rule_scope",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-wrap gap-1">
              {row.original.product_scope ? (
                <Badge variant="secondary">
                  {t(
                    PRODUCT_SCOPE_LABEL_KEYS[
                      row.original.product_scope as ProductScope
                    ],
                  )}
                </Badge>
              ) : null}
              {!row.original.all_branches ? (
                <Badge variant="outline">
                  {t("pricing.selected_branches")}
                </Badge>
              ) : null}
              {!row.original.all_channels ? (
                <Badge variant="outline">
                  {t("pricing.selected_channels")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<DateRule>({
          accessorKey: "priority",
          labelKey: "pricing.columns.priority",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.priority}</span>
          ),
        }),
        createColumn<DateRule>({
          accessorKey: "status",
          labelKey: "pricing.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: PRICING_STATUSES.map((s) => ({
            value: s,
            labelKey: STATUS_LABEL_KEYS[s],
            label: `${t(STATUS_LABEL_KEYS[s])} (${countOf(facets?.status, s)})`,
          })),
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1">
              <StatusChip
                label={t(STATUS_LABEL_KEYS[row.original.status])}
                tone={STATUS_TONES[row.original.status]}
              />
              {row.original.has_draft && row.original.status !== "draft" ? (
                <StatusChip label={t("pricing.pending_draft")} tone="warning" />
              ) : null}
            </span>
          ),
        }),
        createColumn<DateRule>({
          id: "version",
          accessorFn: (row) => row.current_version,
          labelKey: "pricing.columns.version",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => `v${row.original.current_version}`,
        }),
        createColumn<DateRule>({
          accessorKey: "updated_at",
          labelKey: "pricing.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.updated_at, undefined, locale),
        }),
        createColumn<DateRule>({
          accessorKey: "uuid",
          labelKey: "pricing.columns.uuid",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.uuid}</code>
          ),
        }),
        createColumn<DateRule>({
          id: "actions",
          labelKey: "pricing.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("pricing.actions.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.pricing.rules.detail(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
                {
                  id: "delete",
                  label: t("pricing.actions.delete"),
                  icon: Trash2,
                  permission: write,
                  variant: "destructive",
                  onSelect: () => setDeleteTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<DateRule, unknown>[],
    [facets, locale, router, slug, t, write],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      kind: listParams.kind,
      date_from: listParams.date_from,
      date_to: listParams.date_to,
      sort: listParams.sort,
    }),
    [
      listParams.date_from,
      listParams.date_to,
      listParams.kind,
      listParams.q,
      listParams.sort,
      listParams.status,
    ],
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
      title={t("pricing.rules.title")}
      description={t("pricing.rules.description")}
      permission={permissions.tenant.pricing.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("pricing.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("layout.section_pricing") },
        { label: t("pricing.rules.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("pricing.rules.create")}
          permission={write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(rule) =>
          router.push(routes.tenant.pricing.rules.detail(slug, rule.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("pricing.rules.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("pricing.empty_filtered_title")
            : t("pricing.rules.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("pricing.empty_filtered_description")
            : t("pricing.rules.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-pricing-date-rules-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <div className="flex items-center gap-1">
              <DatePicker
                value={dateFrom}
                onChange={(v) => setDateFrom(v ?? "")}
                placeholder={t("pricing.filters.date_from")}
                className="h-8 w-40"
              />
              <DatePicker
                value={dateTo}
                onChange={(v) => setDateTo(v ?? "")}
                placeholder={t("pricing.filters.date_to")}
                className="h-8 w-40"
              />
              {dateFrom || dateTo ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setDateFrom("");
                    setDateTo("");
                  }}
                >
                  {t("pricing.filters.clear_dates")}
                </Button>
              ) : null}
            </div>
            <ResourceIOToolbar
              resource={DATE_RULES_RESOURCE}
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

      <EntityDrawer
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={t("pricing.rules.create_title")}
        description={t("pricing.rules.create_description")}
        size="lg"
        footer={
          <div className="flex w-full justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setCreateOpen(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={create.isPending}>
              {t("pricing.actions.save_draft")}
            </Button>
          </div>
        }
      >
        {createOpen ? (
          <div className="px-4">
            <DateRuleForm
              id={FORM_ID}
              defaultValues={dateRuleDefaults(null)}
              onSubmit={async (values) => {
                const created = await create.mutateAsync(dateRuleBody(values));
                setCreateOpen(false);
                router.push(
                  routes.tenant.pricing.rules.detail(slug, created.uuid),
                );
              }}
            />
          </div>
        ) : null}
      </EntityDrawer>

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget
            ? `${deleteTarget.name} · ${rangeText(t, deleteTarget.start_date, deleteTarget.end_date, locale)}`
            : ""
        }
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
