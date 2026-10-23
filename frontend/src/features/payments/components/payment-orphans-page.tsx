"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Link2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { AppCombobox, AppForm, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import type { ComboboxOption } from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  ORPHAN_STATUSES,
  ORPHAN_STATUS_LABEL_KEYS,
  ORPHAN_STATUS_TONES,
  PAYMENT_ORPHANS_RESOURCE,
} from "@/features/payments/constants";
import {
  useMatchOrphan,
  usePaymentOrphans,
} from "@/features/payments/hooks/use-payments";
import type {
  ListOrphansParams,
  PaymentWebhookOrphan,
} from "@/features/payments/types";
import { ReferenceText } from "@/features/reservations/components/reservation-cells";
import { reservationsService } from "@/features/reservations/services/reservations.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DATE = "d MMM yyyy HH:mm";
const MATCH_FORM = "payment-orphan-match-form";

/**
 * Unmatched provider notifications (TEC-962): a customer paid on the
 * provider for a checkout we have no record of (typically created after
 * the backup the database was restored from). The signature verified with
 * one of the organization's accounts. "Rezervasyonla eşleştir" retrieves
 * the checkout from the provider and records it as a payment of the chosen
 * reservation (confirmed once).
 */
export function PaymentOrphansPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, PAYMENT_ORPHANS_RESOURCE),
  });
  const [target, setTarget] = useState<PaymentWebhookOrphan | null>(null);
  const collect = permissions.tenant.payment.collect;

  const params = useMemo<ListOrphansParams>(
    () => ({
      ...listState.params,
      status: joinFilter(filterValues(listState.columnFilters, "status")),
    }),
    [listState.columnFilters, listState.params],
  );
  const query = usePaymentOrphans(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<PaymentWebhookOrphan>({
          accessorKey: "provider_payment_id",
          labelKey: "payments.orphans.columns.payment",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span className="font-mono text-xs font-medium">
                {row.original.provider_payment_id || "—"}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.account_name || row.original.provider}
              </span>
            </div>
          ),
        }),
        createColumn<PaymentWebhookOrphan>({
          accessorKey: "status",
          labelKey: "payments.orphans.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ORPHAN_STATUSES.map((v) => ({
            value: v,
            labelKey: ORPHAN_STATUS_LABEL_KEYS[v],
            label: t(ORPHAN_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(ORPHAN_STATUS_LABEL_KEYS[row.original.status])}
              tone={ORPHAN_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<PaymentWebhookOrphan>({
          accessorKey: "provider_status",
          labelKey: "payments.orphans.columns.provider_status",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>{row.original.provider_status || "—"}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.event_type}
              </span>
            </div>
          ),
        }),
        createColumn<PaymentWebhookOrphan>({
          accessorKey: "conversation_id",
          labelKey: "payments.orphans.columns.conversation",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs break-all">
              {row.original.conversation_id || "—"}
            </span>
          ),
        }),
        createColumn<PaymentWebhookOrphan>({
          accessorKey: "deliveries",
          labelKey: "payments.orphans.columns.deliveries",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.deliveries,
        }),
        createColumn<PaymentWebhookOrphan>({
          id: "reservation",
          accessorFn: (row) => row.reservation?.reference ?? "",
          labelKey: "payments.orphans.columns.reservation",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.reservation ? (
              <div className="flex flex-col gap-0.5">
                <ReferenceText reference={row.original.reservation.reference} />
                {row.original.match_note ? (
                  <span className="text-muted-foreground line-clamp-2 text-xs">
                    {row.original.match_note}
                  </span>
                ) : null}
              </div>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<PaymentWebhookOrphan>({
          accessorKey: "created_at",
          labelKey: "payments.orphans.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.created_at, DATE, locale),
        }),
        createColumn<PaymentWebhookOrphan>({
          id: "actions",
          labelKey: "payments.orphans.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            row.original.status === "unmatched" ? (
              <EntityRowActions
                actions={[
                  {
                    id: "match",
                    label: t("payments.orphans.actions.match"),
                    icon: Link2,
                    permission: collect,
                    onSelect: () => setTarget(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<PaymentWebhookOrphan, unknown>[],
    [collect, locale, t],
  );

  return (
    <EntityPage
      title={t("payments.orphans.title")}
      description={t("payments.orphans.description")}
      permission={permissions.tenant.payments.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("payments.orphans.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("payments.orphans.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("payments.orphans.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("payments.orphans.empty_title")}
        emptyDescription={t("payments.orphans.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-payment-orphans-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <MatchOrphanDialog orphan={target} onClose={() => setTarget(null)} />
    </EntityPage>
  );
}

type MatchValues = { reservation_uuid: string; note: string };

/** Pick the reservation the money belongs to and say why. */
function MatchOrphanDialog({
  orphan,
  onClose,
}: {
  orphan: PaymentWebhookOrphan | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const match = useMatchOrphan();
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        reservation_uuid: z
          .string()
          .min(1, t("payments.orphans.match.reservation_required")),
        note: z
          .string()
          .trim()
          .min(3, t("payments.orphans.match.note_invalid"))
          .max(500, t("payments.orphans.match.note_invalid")),
      }),
    [t],
  );
  const loadReservations = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      try {
        const page = await reservationsService.list(slug, {
          limit: 20,
          offset: 0,
          sort: "-created_at",
          q: q || undefined,
        });
        return page.items.map((r) => ({
          value: r.uuid,
          label: r.reference,
          description: r.customer.display_name,
        }));
      } catch {
        return [];
      }
    },
    [slug],
  );
  const close = () => {
    setProblem(null);
    onClose();
  };
  return (
    <AppDialog
      open={Boolean(orphan)}
      onOpenChange={(next) => {
        if (!next && !match.isPending) close();
      }}
      title={t("payments.orphans.match.title")}
      description={t("payments.orphans.match.description", {
        payment: orphan?.provider_payment_id || orphan?.conversation_id || "",
      })}
      className="sm:max-w-md"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={close}
            disabled={match.isPending}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={MATCH_FORM} disabled={match.isPending}>
            {t("payments.orphans.match.confirm")}
          </Button>
        </>
      }
    >
      {orphan ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<MatchValues>
            id={MATCH_FORM}
            className="space-y-4"
            schema={schema}
            defaultValues={{ reservation_uuid: "", note: "" }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await match.mutateAsync({
                  uuid: orphan.uuid,
                  reservation_uuid: values.reservation_uuid,
                  note: values.note.trim(),
                });
                close();
              } catch (error) {
                setProblem(
                  error instanceof ApiError
                    ? error.message
                    : t("payments.orphans.match.failed"),
                );
              }
            }}
          >
            <AppCombobox
              name="reservation_uuid"
              label={t("payments.orphans.match.reservation")}
              loadOptions={loadReservations}
              placeholder={t("payments.orphans.match.reservation_placeholder")}
              searchPlaceholder={t("payments.orphans.match.reservation_search")}
              emptyText={t("payments.orphans.match.reservation_empty")}
            />
            <AppTextarea
              name="note"
              label={t("payments.orphans.match.note")}
              rows={3}
            />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}
