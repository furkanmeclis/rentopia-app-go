"use client";

import type {
  ColumnDef,
  ColumnFiltersState,
  OnChangeFn,
} from "@tanstack/react-table";
import { Eye, Globe } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
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
import { SiteLeadDrawer } from "@/features/site-leads/components/site-lead-drawer";
import {
  KIND_LABEL_KEYS,
  LEAD_KINDS,
  LEAD_STATUSES,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/site-leads/constants";
import { useSiteLeadsList } from "@/features/site-leads/hooks/use-site-leads";
import type {
  ListSiteLeadsParams,
  SiteLead,
} from "@/features/site-leads/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Organizations sell in Turkey: day filters are Istanbul days (UTC+03:00). */
const OFFSET = "+03:00";

/** `yyyy-MM-dd` → the start of that Istanbul day (RFC 3339). */
function dayStart(day: string): string | undefined {
  return /^\d{4}-\d{2}-\d{2}$/.test(day)
    ? `${day}T00:00:00${OFFSET}`
    : undefined;
}

/** `yyyy-MM-dd` → the start of the next Istanbul day (exclusive end). */
function dayEnd(day: string): string | undefined {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) return undefined;
  const next = new Date(`${day}T00:00:00Z`);
  next.setUTCDate(next.getUTCDate() + 1);
  return `${next.toISOString().slice(0, 10)}T00:00:00${OFFSET}`;
}

/** Filters the API takes one value of: the last choice wins. */
const SINGLE_FILTERS = new Set(["kind", "status"]);

function singleValued(filters: ColumnFiltersState): ColumnFiltersState {
  return filters.map((f) =>
    SINGLE_FILTERS.has(f.id) && Array.isArray(f.value) && f.value.length > 1
      ? { ...f, value: [f.value[f.value.length - 1]] }
      : f,
  );
}

/**
 * Site applications ("Başvurular", TEC-786): contact messages, corporate
 * rental and franchise applications and newsletter subscriptions sent from
 * the branded site. Filters: type, status, date; the drawer shows every
 * field and lets `manage` set the handling status and an internal note.
 */
export function SiteLeadsPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [selected, setSelected] = useState<SiteLead | null>(null);

  const params = useMemo<ListSiteLeadsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      // One type / status at a time (singleValued keeps the last choice).
      kind: joinFilter(column("kind").slice(-1)),
      status: joinFilter(column("status").slice(-1)),
      sort:
        listState.params.sort === "created_at" ? "created_at" : "-created_at",
      date_from: dayStart(from),
      date_to: dayEnd(to),
    };
  }, [from, listState.columnFilters, listState.params, to]);

  const { onColumnFiltersChange } = listState;
  const tableState = useMemo(() => {
    const onChange: OnChangeFn<ColumnFiltersState> = (updater) =>
      onColumnFiltersChange((prev) =>
        singleValued(typeof updater === "function" ? updater(prev) : updater),
      );
    return { ...listState.tableState, onColumnFiltersChange: onChange };
  }, [listState.tableState, onColumnFiltersChange]);

  const query = useSiteLeadsList(params);
  const total = query.data?.total ?? 0;

  const setDay = (set: (v: string) => void) => (value: string) => {
    set(value ?? "");
    listState.setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  };

  const columns = useMemo(
    () =>
      [
        createColumn<SiteLead>({
          accessorKey: "created_at",
          labelKey: "site_leads.columns.created_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<SiteLead>({
          accessorKey: "kind",
          labelKey: "site_leads.columns.kind",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: LEAD_KINDS.map((v) => ({
            value: v,
            labelKey: KIND_LABEL_KEYS[v],
            label: t(KIND_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(KIND_LABEL_KEYS[row.original.kind])}
            </Badge>
          ),
        }),
        createColumn<SiteLead>({
          id: "name",
          accessorFn: (row) => row.name || row.email,
          labelKey: "site_leads.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.name || row.original.email}
            </span>
          ),
        }),
        createColumn<SiteLead>({
          accessorKey: "email",
          labelKey: "site_leads.columns.email",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="break-all">{row.original.email}</span>
          ),
        }),
        createColumn<SiteLead>({
          accessorKey: "phone",
          labelKey: "site_leads.columns.phone",
          enableSorting: false,
          cell: ({ row }) => row.original.phone || "—",
        }),
        createColumn<SiteLead>({
          id: "detail",
          accessorFn: (row) => row.company || row.city,
          labelKey: "site_leads.columns.detail",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.company || row.original.city || "—",
        }),
        createColumn<SiteLead>({
          accessorKey: "status",
          labelKey: "site_leads.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: LEAD_STATUSES.map((v) => ({
            value: v,
            labelKey: STATUS_LABEL_KEYS[v],
            label: t(STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<SiteLead>({
          id: "consent_marketing",
          accessorFn: (row) => row.consent_marketing,
          labelKey: "site_leads.columns.consent_marketing",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.consent_marketing
              ? t("site_leads.detail.yes")
              : t("site_leads.detail.no"),
        }),
        createColumn<SiteLead>({
          accessorKey: "source_host",
          labelKey: "site_leads.columns.source_host",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.source_host}
            </code>
          ),
        }),
        createColumn<SiteLead>({
          id: "actions",
          labelKey: "site_leads.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("site_leads.actions.open"),
                  icon: Eye,
                  onSelect: () => setSelected(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<SiteLead, unknown>[],
    [locale, t],
  );

  const hasFilters = Boolean(
    params.q || params.kind || params.status || from || to,
  );

  return (
    <EntityPage
      title={t("site_leads.title")}
      description={t("site_leads.description")}
      permission={permissions.tenant.siteLeads.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("site_leads.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("layout.section_tenant_settings") },
        { label: t("site_leads.title") },
      ]}
      actions={
        can(permissions.tenant.site.read) ? (
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.settings.site(slug)}>
              <Globe className="size-4" />
              {t("site_leads.actions.site_settings")}
            </Link>
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => setSelected(row)}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("site_leads.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={
          hasFilters
            ? t("site_leads.empty_filtered_title")
            : t("site_leads.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("site_leads.empty_filtered_description")
            : t("site_leads.empty_description")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={tableState}
        features={{
          persistKey: "tenant-site-leads-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <div className="grid w-full grid-cols-2 gap-2 sm:w-auto">
              <DatePicker
                value={from}
                onChange={setDay(setFrom)}
                placeholder={t("site_leads.filters.from")}
              />
              <DatePicker
                value={to}
                onChange={setDay(setTo)}
                placeholder={t("site_leads.filters.to")}
              />
            </div>
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      <SiteLeadDrawer
        lead={selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      />
    </EntityPage>
  );
}
