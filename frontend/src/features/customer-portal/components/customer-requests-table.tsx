"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, ExternalLink, XCircle } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
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
  CUSTOMER_REQUESTS_RESOURCE,
  REQUEST_STATUSES,
  REQUEST_STATUS_LABEL_KEYS,
  REQUEST_STATUS_TONES,
  REQUEST_TYPES,
  REQUEST_TYPE_LABEL_KEYS,
} from "@/features/customer-portal/constants";
import {
  useCustomerRequests,
  useResolveCustomerRequest,
} from "@/features/customer-portal/hooks/use-customer-requests";
import type {
  ListCustomerRequestsParams,
  StaffCustomerRequest,
} from "@/features/customer-portal/types";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { moneyText } from "@/features/pricing/lib/format";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import { ReferenceText } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Target = {
  request: StaffCustomerRequest;
  status: "resolved" | "rejected";
};

/**
 * Customer requests ("Müşteri talepleri", TEC-715): change and
 * cancellation requests sent from the customer portal. Staff close them
 * with a note the customer sees; acting on the reservation stays a separate
 * action. `reservationUuid` limits the table to one reservation.
 */
export function CustomerRequestsTable({
  reservationUuid,
  persistKey,
}: {
  reservationUuid?: string;
  persistKey: string;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CUSTOMER_REQUESTS_RESOURCE),
  });
  const [target, setTarget] = useState<Target | null>(null);
  const resolve = useResolveCustomerRequest();
  const write = permissions.tenant.reservations.write;

  const params = useMemo<ListCustomerRequestsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      request_type: joinFilter(column("type")),
      reservation: reservationUuid,
    };
  }, [listState.columnFilters, listState.params, reservationUuid]);
  const query = useCustomerRequests(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<StaffCustomerRequest>({
          accessorKey: "type",
          labelKey: "customer_portal.staff.columns.type",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: REQUEST_TYPES.map((v) => ({
            value: v,
            labelKey: REQUEST_TYPE_LABEL_KEYS[v],
            label: t(REQUEST_TYPE_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(REQUEST_TYPE_LABEL_KEYS[row.original.type])}
            </Badge>
          ),
        }),
        createColumn<StaffCustomerRequest>({
          accessorKey: "status",
          labelKey: "customer_portal.staff.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: REQUEST_STATUSES.map((v) => ({
            value: v,
            labelKey: REQUEST_STATUS_LABEL_KEYS[v],
            label: t(REQUEST_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(REQUEST_STATUS_LABEL_KEYS[row.original.status])}
              tone={REQUEST_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        ...(reservationUuid
          ? []
          : [
              createColumn<StaffCustomerRequest>({
                id: "reservation",
                accessorFn: (row) => row.reservation.reference,
                labelKey: "customer_portal.staff.columns.reservation",
                enableSorting: false,
                gridSecondary: true,
                cell: ({ row }) => (
                  <ReferenceText
                    reference={row.original.reservation.reference}
                  />
                ),
              }),
              createColumn<StaffCustomerRequest>({
                id: "starts_at",
                accessorFn: (row) => row.reservation.starts_at,
                labelKey: "customer_portal.staff.columns.starts_at",
                enableSorting: false,
                cell: ({ row }) =>
                  datetime(
                    row.original.reservation.starts_at,
                    "d MMM HH:mm",
                    locale,
                  ),
              }),
            ]),
        createColumn<StaffCustomerRequest>({
          id: "requester",
          accessorFn: (row) => row.requester_name,
          labelKey: "customer_portal.staff.columns.requester",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>{row.original.requester_name}</span>
              <span className="text-muted-foreground text-xs">
                {t(
                  row.original.requester_role === "driver"
                    ? "customer_portal.roles.driver"
                    : "customer_portal.roles.renter",
                )}{" "}
                · {row.original.contact_masked}
              </span>
            </div>
          ),
        }),
        createColumn<StaffCustomerRequest>({
          accessorKey: "message",
          labelKey: "customer_portal.staff.columns.message",
          enableSorting: false,
          cell: ({ row }) => {
            const r = row.original;
            return (
              <div className="flex max-w-md flex-col gap-1 text-sm">
                {r.message ? (
                  <span className="line-clamp-3">{r.message}</span>
                ) : null}
                {r.requested_starts_at || r.requested_ends_at ? (
                  <span className="text-muted-foreground text-xs">
                    {t("customer_portal.staff.requested_dates", {
                      start: r.requested_starts_at
                        ? datetime(r.requested_starts_at, "d MMM HH:mm", locale)
                        : "—",
                      end: r.requested_ends_at
                        ? datetime(r.requested_ends_at, "d MMM HH:mm", locale)
                        : "—",
                    })}
                  </span>
                ) : null}
                {r.cancellation_fee ? (
                  <span className="text-muted-foreground text-xs">
                    {t("customer_portal.staff.fee", {
                      fee: moneyText(r.cancellation_fee, locale),
                    })}
                  </span>
                ) : null}
                {r.outcome === "self_cancelled" ? (
                  <span className="text-muted-foreground text-xs">
                    {t("customer_portal.staff.self_cancelled")}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<StaffCustomerRequest>({
          accessorKey: "resolution_note",
          labelKey: "customer_portal.staff.columns.resolution_note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.resolution_note || "—",
        }),
        createColumn<StaffCustomerRequest>({
          accessorKey: "created_at",
          labelKey: "customer_portal.staff.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<StaffCustomerRequest>({
          id: "actions",
          labelKey: "customer_portal.staff.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const request = row.original;
            const open = request.status === "open";
            return (
              <EntityRowActions
                actions={[
                  ...(reservationUuid
                    ? []
                    : [
                        {
                          id: "open",
                          label: t("customer_portal.staff.actions.open"),
                          icon: ExternalLink,
                          onSelect: () =>
                            router.push(
                              routes.tenant.reservations.detail(
                                slug,
                                request.reservation.uuid,
                              ),
                            ),
                        },
                      ]),
                  ...(open
                    ? [
                        {
                          id: "resolve",
                          label: t("customer_portal.staff.actions.resolve"),
                          icon: CheckCircle2,
                          permission: write,
                          onSelect: () =>
                            setTarget({ request, status: "resolved" }),
                        },
                        {
                          id: "reject",
                          label: t("customer_portal.staff.actions.reject"),
                          icon: XCircle,
                          permission: write,
                          onSelect: () =>
                            setTarget({ request, status: "rejected" }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<StaffCustomerRequest, unknown>[],
    [locale, reservationUuid, router, slug, t, write],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("customer_portal.staff.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("customer_portal.staff.empty_title")}
        emptyDescription={t("customer_portal.staff.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <ReasonDialog
        open={Boolean(target)}
        title={t(
          target?.status === "rejected"
            ? "customer_portal.staff.reject_title"
            : "customer_portal.staff.resolve_title",
        )}
        description={target?.request.message || undefined}
        label={t("customer_portal.staff.note")}
        confirmLabel={t(
          target?.status === "rejected"
            ? "customer_portal.staff.actions.reject"
            : "customer_portal.staff.actions.resolve",
        )}
        destructive={target?.status === "rejected"}
        required={target?.status === "rejected"}
        pending={resolve.isPending}
        onCancel={() => setTarget(null)}
        onConfirm={(note) => {
          if (!target) return;
          resolve.mutate(
            {
              uuid: target.request.uuid,
              body: { status: target.status, note },
            },
            { onSuccess: () => setTarget(null) },
          );
        }}
      />
    </>
  );
}
