"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, ExternalLink, XCircle } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
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
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { AcceptDialog } from "@/features/payments/components/accept-dialog";
import { useRejectReservation } from "@/features/payments/hooks/use-payments";
import { moneyText } from "@/features/pricing/lib/format";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import {
  RangeText,
  ReferenceText,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-cells";
import {
  RESERVATIONS_RESOURCE,
  STATUS_LABEL_KEYS,
} from "@/features/reservations/constants";
import { useReservationsList } from "@/features/reservations/hooks/use-reservations";
import type {
  ListReservationsParams,
  ReservationListItem,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

const QUEUE_STATUSES = ["request", "awaiting_review"] as const;

/** Minute clock for the SLA countdowns. */
function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);
  return now;
}

/** Remaining review time (SLA from the staff review window). */
function SlaCell({ deadline, now }: { deadline: string | null; now: number }) {
  const { t } = useLocale();
  if (!deadline) return <span className="text-muted-foreground">—</span>;
  const left = Math.round((new Date(deadline).getTime() - now) / 60_000);
  if (left <= 0) {
    return (
      <Badge variant="danger">
        {t("payments.approvals.sla_overdue", { minutes: Math.abs(left) })}
      </Badge>
    );
  }
  const hours = Math.floor(left / 60);
  const minutes = left % 60;
  return (
    <Badge variant={left <= 15 ? "warning" : "secondary"}>
      {hours > 0
        ? t("payments.approvals.sla_left_hours", { hours, minutes })
        : t("payments.approvals.sla_left_minutes", { minutes })}
    </Badge>
  );
}

/**
 * Staff approval queue ("Onay kuyruğu", TEC-708): requests without stock
 * and reviews holding an option, by SLA deadline. Accept reserves at the
 * current availability / price (payment window, optional payment link);
 * reject needs a reason.
 */
export function ApprovalQueuePage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const now = useNow();
  const listState = useServerListState({
    initialSort: "review_deadline",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RESERVATIONS_RESOURCE),
  });
  const [acceptTarget, setAcceptTarget] = useState<ReservationListItem | null>(
    null,
  );
  const [rejectTarget, setRejectTarget] = useState<ReservationListItem | null>(
    null,
  );
  const reject = useRejectReservation();
  const approve = permissions.tenant.reservations.approve;

  const params = useMemo<ListReservationsParams>(() => {
    const chosen = filterValues(listState.columnFilters, "status").filter((s) =>
      (QUEUE_STATUSES as readonly string[]).includes(s),
    );
    return {
      ...listState.params,
      status: joinFilter(chosen.length ? chosen : [...QUEUE_STATUSES]),
    };
  }, [listState.columnFilters, listState.params]);
  const query = useReservationsList(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ReservationListItem>({
          accessorKey: "reference",
          labelKey: "payments.approvals.columns.reference",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <ReferenceText reference={row.original.reference} />
          ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "status",
          labelKey: "payments.approvals.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: QUEUE_STATUSES.map((v) => ({
            value: v,
            labelKey: STATUS_LABEL_KEYS[v],
            label: t(STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <ReservationStatusChip status={row.original.status} />
          ),
        }),
        createColumn<ReservationListItem>({
          id: "review_deadline",
          accessorFn: (row) => row.review_deadline ?? "",
          labelKey: "payments.approvals.columns.sla",
          enableSorting: true,
          cell: ({ row }) => (
            <SlaCell deadline={row.original.review_deadline} now={now} />
          ),
        }),
        createColumn<ReservationListItem>({
          id: "customer",
          accessorFn: (row) => row.customer.display_name,
          labelKey: "payments.approvals.columns.customer",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => row.original.customer.display_name,
        }),
        createColumn<ReservationListItem>({
          id: "product",
          accessorFn: (row) => row.product.title,
          labelKey: "payments.approvals.columns.product",
          enableSorting: false,
          cell: ({ row }) => row.original.product.title,
        }),
        createColumn<ReservationListItem>({
          accessorKey: "starts_at",
          labelKey: "payments.approvals.columns.range",
          enableSorting: true,
          cell: ({ row }) => (
            <RangeText
              startsAt={row.original.starts_at}
              endsAt={row.original.ends_at}
            />
          ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "channel",
          labelKey: "payments.approvals.columns.channel",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="outline">{row.original.channel}</Badge>
          ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "total_amount",
          labelKey: "payments.approvals.columns.total",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.total_amount, locale)}
            </span>
          ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "prepayment_amount",
          labelKey: "payments.approvals.columns.prepayment",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.prepayment_amount, locale)}
            </span>
          ),
        }),
        createColumn<ReservationListItem>({
          id: "actions",
          labelKey: "payments.approvals.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "accept",
                  label: t("payments.approvals.accept"),
                  icon: CheckCircle2,
                  permission: approve,
                  onSelect: () => setAcceptTarget(row.original),
                },
                {
                  id: "reject",
                  label: t("payments.approvals.reject"),
                  icon: XCircle,
                  permission: approve,
                  variant: "destructive",
                  onSelect: () => setRejectTarget(row.original),
                },
                {
                  id: "open",
                  label: t("payments.approvals.open"),
                  icon: ExternalLink,
                  separatorBefore: true,
                  onSelect: () =>
                    router.push(
                      routes.tenant.reservations.detail(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<ReservationListItem, unknown>[],
    [approve, locale, now, router, slug, t],
  );

  return (
    <EntityPage
      title={t("payments.approvals.title")}
      description={t("payments.approvals.description")}
      permission={permissions.tenant.reservations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("payments.approvals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("payments.approvals.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("payments.approvals.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("payments.approvals.empty_title")}
        emptyDescription={t("payments.approvals.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-approval-queue-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <AcceptDialog
        reservation={
          acceptTarget
            ? {
                uuid: acceptTarget.uuid,
                reference: acceptTarget.reference,
                total: acceptTarget.total_amount,
              }
            : null
        }
        onClose={() => setAcceptTarget(null)}
      />
      <ReasonDialog
        open={Boolean(rejectTarget)}
        title={t("payments.approvals.reject_title", {
          reference: rejectTarget?.reference ?? "",
        })}
        description={t("payments.approvals.reject_description")}
        label={t("payments.approvals.reject_reason")}
        confirmLabel={t("payments.approvals.reject")}
        destructive
        pending={reject.isPending}
        onCancel={() => setRejectTarget(null)}
        onConfirm={(reason) => {
          if (!rejectTarget) return;
          reject.mutate(
            { uuid: rejectTarget.uuid, reason },
            { onSuccess: () => setRejectTarget(null) },
          );
        }}
      />
    </EntityPage>
  );
}
