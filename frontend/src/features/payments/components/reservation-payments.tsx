"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Banknote, Link2, RefreshCw, Undo2 } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { AppForm, AppInput, AppSelect, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  LINK_STATUS_LABEL_KEYS,
  LINK_STATUS_TONES,
  MANUAL_METHODS,
  METHOD_LABEL_KEYS,
  OUTCOME_LABEL_KEYS,
  PAYMENTS_RESOURCE,
  PAYMENT_LINKS_RESOURCE,
  PAYMENT_STATUS_LABEL_KEYS,
  PAYMENT_STATUS_TONES,
  VERIFIED_VIA_LABEL_KEYS,
} from "@/features/payments/constants";
import { PaymentLinkBox } from "@/features/payments/components/payment-link-box";
import {
  useCancelPaymentLink,
  useCollectPayment,
  useCreatePaymentLink,
  usePaymentLinks,
  usePaymentsList,
  useRefundPayment,
  useVerifyPayment,
} from "@/features/payments/hooks/use-payments";
import type { Payment, PaymentLink } from "@/features/payments/types";
import { moneyText } from "@/features/pricing/lib/format";
import { PriceTotals } from "@/features/reservations/components/price-breakdown";
import { PaymentStatusChip } from "@/features/reservations/components/reservation-cells";
import { reasonText } from "@/features/reservations/constants";
import type { Reservation } from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

const LINK_FORM = "payment-link-form";
const COLLECT_FORM = "payment-collect-form";
const REFUND_FORM = "payment-refund-form";

const amountSchema = (message: string) =>
  z
    .string()
    .trim()
    .refine((v) => v === "" || /^\d+([.,]\d{1,2})?$/.test(v), message);

const normalize = (v: string) => v.trim().replace(",", ".");

function errorText(error: unknown, fallback: string) {
  return error instanceof ApiError ? error.message : fallback;
}

/**
 * Payments tab of a reservation (TEC-707 / TEC-708): payment status and the
 * remaining rent shown separately, payments (DataTable) with verify /
 * refund, pay-by-link links with cancel, and branch collections.
 */
export function ReservationPayments({
  reservation,
}: {
  reservation: Reservation;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.payments.read);
  const canLink = can(permissions.tenant.payments.createLink);
  const canCollect = can(permissions.tenant.payment.collect);
  const canRefund = can(permissions.tenant.payment.refund);
  const price = reservation.price;
  const payableByLink =
    reservation.status === "awaiting_payment" ||
    reservation.status === "confirmed";
  const collectable = !["request", "cancelled", "expired", "no_show"].includes(
    reservation.status,
  );

  const paymentsState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 10,
    realtime: tenantLiveList(organization, PAYMENTS_RESOURCE),
  });
  const linksState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 10,
    realtime: tenantLiveList(organization, PAYMENT_LINKS_RESOURCE),
  });
  const payments = usePaymentsList(
    { ...paymentsState.params, reservation: reservation.uuid },
    canRead,
  );
  const links = usePaymentLinks(
    {
      limit: linksState.params.limit,
      offset: linksState.params.offset,
      reservation: reservation.uuid,
    },
    canRead,
  );

  const createLink = useCreatePaymentLink(reservation.uuid);
  const cancelLink = useCancelPaymentLink();
  const collect = useCollectPayment(reservation.uuid);
  const refund = useRefundPayment();
  const verify = useVerifyPayment();

  const [linkOpen, setLinkOpen] = useState(false);
  const [createdLink, setCreatedLink] = useState<PaymentLink | null>(null);
  const [collectOpen, setCollectOpen] = useState(false);
  const [refundTarget, setRefundTarget] = useState<Payment | null>(null);
  const [cancelTarget, setCancelTarget] = useState<PaymentLink | null>(null);
  const [problem, setProblem] = useState<string | null>(null);

  const paymentColumns = useMemo(
    () =>
      [
        createColumn<Payment>({
          accessorKey: "created_at",
          labelKey: "payments.columns.created_at",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<Payment>({
          accessorKey: "status",
          labelKey: "payments.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(PAYMENT_STATUS_LABEL_KEYS[row.original.status])}
              tone={PAYMENT_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<Payment>({
          accessorKey: "method",
          labelKey: "payments.columns.method",
          enableSorting: false,
          cell: ({ row }) => t(METHOD_LABEL_KEYS[row.original.method]),
        }),
        createColumn<Payment>({
          accessorKey: "amount",
          labelKey: "payments.columns.amount",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.amount, locale)}
            </span>
          ),
        }),
        createColumn<Payment>({
          accessorKey: "gross_amount",
          labelKey: "payments.columns.gross",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => moneyText(row.original.gross_amount, locale),
        }),
        createColumn<Payment>({
          accessorKey: "commission_amount",
          labelKey: "payments.columns.commission",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.commission_amount, locale),
        }),
        createColumn<Payment>({
          accessorKey: "net_amount",
          labelKey: "payments.columns.net",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.net_amount, locale),
        }),
        createColumn<Payment>({
          accessorKey: "refunded_amount",
          labelKey: "payments.columns.refunded",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.refunded_amount, locale),
        }),
        createColumn<Payment>({
          accessorKey: "verified_via",
          labelKey: "payments.columns.verified_via",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.verified_via
              ? reasonText(
                  t,
                  VERIFIED_VIA_LABEL_KEYS,
                  row.original.verified_via,
                )
              : "—",
        }),
        createColumn<Payment>({
          accessorKey: "outcome",
          labelKey: "payments.columns.outcome",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.outcome
              ? reasonText(t, OUTCOME_LABEL_KEYS, row.original.outcome)
              : row.original.failure_reason || "—",
        }),
        createColumn<Payment>({
          id: "actions",
          labelKey: "payments.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const p = row.original;
            return (
              <EntityRowActions
                actions={[
                  ...(p.status === "pending" && p.provider !== "manual"
                    ? [
                        {
                          id: "verify",
                          label: t("payments.actions.verify"),
                          icon: RefreshCw,
                          permission: permissions.tenant.payments.createLink,
                          onSelect: () => verify.mutate(p.uuid),
                        },
                      ]
                    : []),
                  ...(Number(p.refundable) > 0
                    ? [
                        {
                          id: "refund",
                          label: t("payments.actions.refund"),
                          icon: Undo2,
                          permission: permissions.tenant.payment.refund,
                          variant: "destructive" as const,
                          onSelect: () => {
                            setProblem(null);
                            setRefundTarget(p);
                          },
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<Payment, unknown>[],
    [locale, t, verify],
  );

  const linkColumns = useMemo(
    () =>
      [
        createColumn<PaymentLink>({
          accessorKey: "created_at",
          labelKey: "payments.links.columns.created_at",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<PaymentLink>({
          accessorKey: "status",
          labelKey: "payments.links.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(LINK_STATUS_LABEL_KEYS[row.original.status])}
              tone={LINK_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<PaymentLink>({
          accessorKey: "amount",
          labelKey: "payments.links.columns.amount",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.amount, locale),
        }),
        createColumn<PaymentLink>({
          accessorKey: "expires_at",
          labelKey: "payments.links.columns.expires_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.expires_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<PaymentLink>({
          accessorKey: "url",
          labelKey: "payments.links.columns.url",
          enableSorting: false,
          cell: ({ row }) => (
            <code className="line-clamp-1 max-w-56 text-xs">
              {row.original.url}
            </code>
          ),
        }),
        createColumn<PaymentLink>({
          id: "actions",
          labelKey: "payments.links.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={
                row.original.status === "active"
                  ? [
                      {
                        id: "copy",
                        label: t("payments.links.copy"),
                        icon: Link2,
                        onSelect: () =>
                          void navigator.clipboard.writeText(row.original.url),
                      },
                      {
                        id: "cancel",
                        label: t("payments.links.cancel"),
                        icon: Ban,
                        permission: permissions.tenant.payments.createLink,
                        variant: "destructive",
                        onSelect: () => setCancelTarget(row.original),
                      },
                    ]
                  : []
              }
            />
          ),
        }),
      ] as ColumnDef<PaymentLink, unknown>[],
    [locale, t],
  );

  return (
    <div className="space-y-6">
      <EntitySectionCard
        title={t("payments.summary.title")}
        actions={
          <div className="flex flex-wrap gap-2">
            {canLink && payableByLink && Number(price.balance) > 0 ? (
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  setProblem(null);
                  setCreatedLink(null);
                  setLinkOpen(true);
                }}
              >
                <Link2 className="size-4" />
                {t("payments.actions.create_link")}
              </Button>
            ) : null}
            {canCollect && collectable && Number(price.balance) > 0 ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setProblem(null);
                  setCollectOpen(true);
                }}
              >
                <Banknote className="size-4" />
                {t("payments.actions.collect")}
              </Button>
            ) : null}
          </div>
        }
      >
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">
              {t("payments.summary.payment_status")}
            </span>
            <PaymentStatusChip status={reservation.payment_status} />
          </div>
          <PriceTotals
            rows={[
              {
                key: "total",
                label: t("payments.summary.total"),
                value: price.total,
                strong: true,
              },
              {
                key: "prepayment",
                label: t("payments.summary.prepayment"),
                value: price.prepayment,
              },
              {
                key: "paid",
                label: t("payments.summary.paid"),
                value: price.paid,
              },
              {
                key: "due",
                label: t("payments.summary.prepayment_due"),
                value: price.prepayment_due,
              },
              {
                key: "balance",
                label: t("payments.summary.balance"),
                value: price.balance,
                strong: true,
              },
            ]}
          />
          {reservation.status === "awaiting_payment" &&
          reservation.payment_deadline ? (
            <p className="text-muted-foreground text-xs">
              {t("payments.summary.payment_deadline", {
                deadline: datetime(
                  reservation.payment_deadline,
                  "d MMM yyyy HH:mm",
                  locale,
                ),
              })}
            </p>
          ) : null}
        </div>
      </EntitySectionCard>

      {canRead ? (
        <>
          <EntitySectionCard
            title={t("payments.list_title")}
            badge={payments.data?.total ?? 0}
          >
            <EntityTable
              columns={paymentColumns}
              data={payments.data?.items ?? []}
              getRowId={(row) => row.uuid}
              isLoading={payments.isLoading}
              isError={payments.isError}
              errorDescription={t("payments.load_failed")}
              onRetry={() => void payments.refetch()}
              realtime={paymentsState.realtimeFor(() => payments.refetch())}
              emptyTitle={t("payments.empty_title")}
              emptyDescription={t("payments.empty_description")}
              pageCount={Math.max(
                1,
                Math.ceil((payments.data?.total ?? 0) / 10),
              )}
              state={paymentsState.tableState}
              features={{
                persistKey: "tenant-reservation-payments-v1",
                rowSelection: false,
              }}
            />
          </EntitySectionCard>
          <EntitySectionCard
            title={t("payments.links.title")}
            badge={links.data?.total ?? 0}
          >
            <EntityTable
              columns={linkColumns}
              data={links.data?.items ?? []}
              getRowId={(row) => row.uuid}
              isLoading={links.isLoading}
              isError={links.isError}
              errorDescription={t("payments.links.load_failed")}
              onRetry={() => void links.refetch()}
              realtime={linksState.realtimeFor(() => links.refetch())}
              emptyTitle={t("payments.links.empty_title")}
              emptyDescription={t("payments.links.empty_description")}
              pageCount={Math.max(1, Math.ceil((links.data?.total ?? 0) / 10))}
              state={linksState.tableState}
              features={{
                persistKey: "tenant-reservation-payment-links-v1",
                rowSelection: false,
              }}
            />
          </EntitySectionCard>
        </>
      ) : null}

      <AppDialog
        open={linkOpen}
        onOpenChange={(open) => {
          if (!open) setLinkOpen(false);
        }}
        title={t("payments.links.create_title")}
        description={t("payments.links.create_description")}
        className="sm:max-w-lg"
        footer={
          createdLink ? (
            <Button type="button" onClick={() => setLinkOpen(false)}>
              {t("payments.accept.done")}
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => setLinkOpen(false)}
              >
                {t("form.cancel")}
              </Button>
              <Button
                type="submit"
                form={LINK_FORM}
                disabled={createLink.isPending}
              >
                {t("payments.actions.create_link")}
              </Button>
            </>
          )
        }
      >
        {createdLink ? (
          <PaymentLinkBox
            url={createdLink.url}
            amount={createdLink.amount}
            expiresAt={createdLink.expires_at}
          />
        ) : linkOpen ? (
          <div className="space-y-4">
            {problem ? (
              <Alert variant="destructive">
                <AlertDescription>{problem}</AlertDescription>
              </Alert>
            ) : null}
            <AppForm<{ amount: string }>
              id={LINK_FORM}
              schema={z.object({
                amount: amountSchema(t("payments.validation.amount")),
              })}
              defaultValues={{
                amount:
                  reservation.status === "awaiting_payment"
                    ? price.prepayment_due
                    : price.balance,
              }}
              onSubmit={async (values) => {
                try {
                  const link = await createLink.mutateAsync({
                    amount: values.amount
                      ? normalize(values.amount)
                      : undefined,
                  });
                  setCreatedLink(link);
                } catch (error) {
                  setProblem(
                    errorText(error, t("payments.links.create_failed")),
                  );
                }
              }}
            >
              <AppInput
                name="amount"
                label={t("payments.fields.amount")}
                description={t("payments.links.amount_hint", {
                  balance: moneyText(price.balance, locale),
                })}
                inputMode="decimal"
              />
            </AppForm>
          </div>
        ) : null}
      </AppDialog>

      <AppDialog
        open={collectOpen}
        onOpenChange={(open) => {
          if (!open) setCollectOpen(false);
        }}
        title={t("payments.collect.title")}
        description={t("payments.collect.description")}
        className="sm:max-w-lg"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setCollectOpen(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={COLLECT_FORM}
              disabled={collect.isPending}
            >
              {t("payments.actions.collect")}
            </Button>
          </>
        }
      >
        {collectOpen ? (
          <div className="space-y-4">
            {problem ? (
              <Alert variant="destructive">
                <AlertDescription>{problem}</AlertDescription>
              </Alert>
            ) : null}
            <AppForm<{ amount: string; method: string; note: string }>
              id={COLLECT_FORM}
              schema={z.object({
                amount: amountSchema(t("payments.validation.amount")),
                method: z.string().min(1),
                note: z.string().max(500),
              })}
              defaultValues={{
                amount: price.balance,
                method: "cash",
                note: "",
              }}
              onSubmit={async (values) => {
                try {
                  await collect.mutateAsync({
                    amount: values.amount
                      ? normalize(values.amount)
                      : undefined,
                    method: values.method,
                    note: values.note.trim() || undefined,
                  });
                  setCollectOpen(false);
                } catch (error) {
                  setProblem(errorText(error, t("payments.collect.failed")));
                }
              }}
            >
              <div className="space-y-4">
                <AppInput
                  name="amount"
                  label={t("payments.fields.amount")}
                  inputMode="decimal"
                />
                <AppSelect
                  name="method"
                  label={t("payments.fields.method")}
                  options={MANUAL_METHODS.map((m) => ({
                    value: m,
                    label: t(METHOD_LABEL_KEYS[m]),
                  }))}
                />
                <AppTextarea name="note" label={t("payments.fields.note")} />
              </div>
            </AppForm>
          </div>
        ) : null}
      </AppDialog>

      <AppDialog
        open={Boolean(refundTarget)}
        onOpenChange={(open) => {
          if (!open) setRefundTarget(null);
        }}
        title={t("payments.refund.title")}
        description={t("payments.refund.description")}
        className="sm:max-w-lg"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setRefundTarget(null)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={REFUND_FORM}
              variant="destructive"
              disabled={refund.isPending || !canRefund}
            >
              {t("payments.actions.refund")}
            </Button>
          </>
        }
      >
        {refundTarget ? (
          <div className="space-y-4">
            {problem ? (
              <Alert variant="destructive">
                <AlertDescription>{problem}</AlertDescription>
              </Alert>
            ) : null}
            <AppForm<{ amount: string; reason: string }>
              id={REFUND_FORM}
              schema={z.object({
                amount: amountSchema(t("payments.validation.amount")),
                reason: z
                  .string()
                  .trim()
                  .min(3, t("payments.validation.reason"))
                  .max(500, t("payments.validation.reason")),
              })}
              defaultValues={{ amount: refundTarget.refundable, reason: "" }}
              onSubmit={async (values) => {
                try {
                  await refund.mutateAsync({
                    uuid: refundTarget.uuid,
                    amount: values.amount
                      ? normalize(values.amount)
                      : undefined,
                    reason: values.reason.trim(),
                  });
                  setRefundTarget(null);
                } catch (error) {
                  setProblem(errorText(error, t("payments.refund.failed")));
                }
              }}
            >
              <div className="space-y-4">
                <AppInput
                  name="amount"
                  label={t("payments.fields.amount")}
                  description={t("payments.refund.refundable", {
                    amount: moneyText(refundTarget.refundable, locale),
                  })}
                  inputMode="decimal"
                />
                <AppTextarea
                  name="reason"
                  label={t("payments.fields.reason")}
                />
              </div>
            </AppForm>
          </div>
        ) : null}
      </AppDialog>

      <ConfirmDialog
        open={Boolean(cancelTarget)}
        title={t("payments.links.cancel_title")}
        description={t("payments.links.cancel_description")}
        confirmLabel={t("payments.links.cancel")}
        variant="destructive"
        isPending={cancelLink.isPending}
        onCancel={() => setCancelTarget(null)}
        onConfirm={() => {
          if (!cancelTarget) return;
          cancelLink.mutate(cancelTarget.uuid, {
            onSuccess: () => setCancelTarget(null),
          });
        }}
      />
    </div>
  );
}
