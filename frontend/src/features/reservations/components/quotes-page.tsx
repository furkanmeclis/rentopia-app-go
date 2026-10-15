"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, ExternalLink } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import {
  QuoteStatusChip,
  RangeText,
  ReferenceText,
} from "@/features/reservations/components/reservation-cells";
import { QuoteSummary } from "@/features/reservations/components/reservation-wizard";
import {
  PURPOSE_LABEL_KEYS,
  QUOTES_RESOURCE,
  QUOTE_PURPOSES,
  QUOTE_STATUSES,
  QUOTE_STATUS_LABEL_KEYS,
} from "@/features/reservations/constants";
import {
  useQuote,
  useQuotesList,
} from "@/features/reservations/hooks/use-reservations";
import type {
  ListQuotesParams,
  QuoteListItem,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function QuoteDialog({
  uuid,
  onOpenChange,
}: {
  uuid: string | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const query = useQuote(uuid);
  return (
    <AppDialog
      open={Boolean(uuid)}
      onOpenChange={onOpenChange}
      title={t("reservations.quotes.detail_title")}
      className="sm:max-w-3xl"
    >
      <div className="max-h-[70vh] overflow-y-auto">
        {query.isLoading ? <Loading label={t("common.loading")} /> : null}
        {query.data ? <QuoteSummary quote={query.data} /> : null}
      </div>
    </AppDialog>
  );
}

/**
 * Quotes ("Teklifler"): priced searches with their validity. A quote is
 * a snapshot: the reservation is created from it and re-validated then.
 */
export function QuotesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const [openUuid, setOpenUuid] = useState<string | null>(null);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, QUOTES_RESOURCE),
  });
  const params = useMemo<ListQuotesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      purpose: joinFilter(column("purpose")),
      status: joinFilter(column("status")),
    };
  }, [listState.columnFilters, listState.params]);
  const query = useQuotesList(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<QuoteListItem>({
          id: "product",
          accessorFn: (row) => row.product.code,
          labelKey: "reservations.quotes.columns.product",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.product.code}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.product.title}
              </span>
            </span>
          ),
        }),
        createColumn<QuoteListItem>({
          id: "customer",
          accessorFn: (row) => row.customer?.display_name ?? "",
          labelKey: "reservations.quotes.columns.customer",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => row.original.customer?.display_name ?? "—",
        }),
        createColumn<QuoteListItem>({
          id: "reservation",
          accessorFn: (row) => row.reservation?.reference ?? "",
          labelKey: "reservations.quotes.columns.reservation",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reservation ? (
              <ReferenceText reference={row.original.reservation.reference} />
            ) : (
              "—"
            ),
        }),
        createColumn<QuoteListItem>({
          accessorKey: "starts_at",
          labelKey: "reservations.quotes.columns.starts_at",
          enableSorting: true,
          cell: ({ row }) => (
            <RangeText
              startsAt={row.original.starts_at}
              endsAt={row.original.ends_at}
            />
          ),
        }),
        createColumn<QuoteListItem>({
          accessorKey: "total",
          labelKey: "reservations.quotes.columns.total",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {moneyText(row.original.total, locale)}
            </span>
          ),
        }),
        createColumn<QuoteListItem>({
          accessorKey: "difference",
          labelKey: "reservations.quotes.columns.difference",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.difference
              ? moneyText(row.original.difference, locale)
              : "—",
        }),
        createColumn<QuoteListItem>({
          accessorKey: "purpose",
          labelKey: "reservations.quotes.columns.purpose",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: QUOTE_PURPOSES.map((v) => ({
            value: v,
            labelKey: PURPOSE_LABEL_KEYS[v],
            label: t(PURPOSE_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(PURPOSE_LABEL_KEYS[row.original.purpose])}
            </Badge>
          ),
        }),
        createColumn<QuoteListItem>({
          accessorKey: "status",
          labelKey: "reservations.quotes.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: QUOTE_STATUSES.map((v) => ({
            value: v,
            labelKey: QUOTE_STATUS_LABEL_KEYS[v],
            label: t(QUOTE_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <QuoteStatusChip
              status={row.original.status}
              expired={row.original.expired}
            />
          ),
        }),
        createColumn<QuoteListItem>({
          accessorKey: "channel",
          labelKey: "reservations.quotes.columns.channel",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => channelLabel(t, row.original.channel),
        }),
        createColumn<QuoteListItem>({
          accessorKey: "valid_until",
          labelKey: "reservations.quotes.columns.valid_until",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.valid_until, "d MMM HH:mm", locale),
        }),
        ...auditColumns<QuoteListItem>(
          {
            createdAt: "reservations.quotes.columns.created_at",
            uuid: "reservations.quotes.columns.uuid",
          },
          locale,
        ),
        createColumn<QuoteListItem>({
          id: "actions",
          labelKey: "reservations.quotes.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("reservations.quotes.actions.open"),
                  icon: Eye,
                  onSelect: () => setOpenUuid(row.original.uuid),
                },
                ...(row.original.reservation
                  ? [
                      {
                        id: "reservation",
                        label: t("reservations.quotes.actions.reservation"),
                        icon: ExternalLink,
                        onSelect: () =>
                          router.push(
                            routes.tenant.reservations.detail(
                              slug,
                              row.original.reservation!.uuid,
                            ),
                          ),
                      },
                    ]
                  : []),
              ]}
            />
          ),
        }),
      ] as ColumnDef<QuoteListItem, unknown>[],
    [locale, router, slug, t],
  );

  return (
    <EntityPage
      title={t("reservations.quotes.title")}
      description={t("reservations.quotes.description")}
      permission={permissions.tenant.quotes.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.quotes.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("reservations.quotes.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => setOpenUuid(row.uuid)}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("reservations.quotes.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("reservations.quotes.empty_title")}
        emptyDescription={t("reservations.quotes.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey: "tenant-quotes-v1", rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <QuoteDialog
        uuid={openUuid}
        onOpenChange={(open) => {
          if (!open) setOpenUuid(null);
        }}
      />
    </EntityPage>
  );
}
