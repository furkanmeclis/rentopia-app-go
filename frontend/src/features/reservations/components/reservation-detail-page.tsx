"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  CalendarClock,
  CalendarPlus,
  Car,
  CheckCircle2,
  Eye,
  MoreHorizontal,
  Pencil,
  Send,
  UserCheck,
  UserX,
  Users,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
  EntityUnavailable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CustomerRevealProvider,
  RevealBanner,
  SensitiveValue,
  useCustomerReveal,
} from "@/features/customers/components/customer-reveal";
import { DocumentsSection } from "@/features/contracts";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import {
  ConditionsList,
  PriceLinesTable,
  PriceTotals,
} from "@/features/reservations/components/price-breakdown";
import { ReservationRentalCard } from "@/features/rentals";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import {
  PaymentStatusChip,
  RangeText,
  ReservationStatusChip,
  actorName,
} from "@/features/reservations/components/reservation-cells";
import {
  AssignDialog,
  ChangeDialog,
  DriversDialog,
  ExtendDialog,
  PlateDialog,
} from "@/features/reservations/components/reservation-dialogs";
import { TasksTable } from "@/features/reservations/components/tasks-table";
import { CustomerRequestsTable } from "@/features/customer-portal/components/customer-requests-table";
import {
  ASSIGNMENT_SOURCE_LABEL_KEYS,
  COMMITMENT_LABEL_KEYS,
  DRIVER_REASON_KEYS,
  EVENT_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  reasonText,
} from "@/features/reservations/constants";
import {
  useReservation,
  useReservationEvents,
  useReservationTransition,
  type TransitionAction,
} from "@/features/reservations/hooks/use-reservations";
import type {
  Reservation,
  ReservationAssignment,
  ReservationDriver,
  ReservationEvent,
  ReservationStatus,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { AcceptDialog } from "@/features/payments/components/accept-dialog";
import { ReservationPayments } from "@/features/payments/components/reservation-payments";
import { useRejectReservation } from "@/features/payments/hooks/use-payments";

type DialogKind =
  | "change"
  | "extend"
  | "plate"
  | "drivers"
  | "assign"
  | "cancel"
  | "no_show"
  | "submit"
  | "approve"
  | "confirm"
  | null;

function statusLabel(t: (key: string) => string, status: string | null) {
  if (!status) return "—";
  const key = STATUS_LABEL_KEYS[status as ReservationStatus];
  return key ? t(key) : status;
}

function DriversCard({ reservation }: { reservation: Reservation }) {
  const { t } = useLocale();
  const { revealed } = useCustomerReveal();
  const columns = useMemo(
    () =>
      [
        createColumn<ReservationDriver>({
          accessorKey: "name",
          labelKey: "reservations.drivers.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5 font-medium">
              {row.original.name}
              {row.original.role === "primary" ? (
                <Badge variant="secondary" className="px-1 py-0 text-[10px]">
                  {t("reservations.drivers.primary_short")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<ReservationDriver>({
          id: "licence",
          accessorFn: (row) => row.licence_class,
          labelKey: "reservations.drivers.columns.licence",
          enableSorting: false,
          cell: ({ row }) => {
            const plain = revealed?.drivers.find(
              (d) => d.uuid === row.original.driver_uuid,
            )?.licence_number;
            return (
              <span className="flex items-center gap-1.5">
                {row.original.licence_class || "—"}
                <SensitiveValue
                  masked={row.original.licence_masked}
                  plain={plain}
                />
              </span>
            );
          },
        }),
        createColumn<ReservationDriver>({
          id: "eligible",
          accessorFn: (row) => String(row.eligible),
          labelKey: "reservations.drivers.columns.eligible",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.eligible ? (
              <StatusChip
                label={t("reservations.drivers.eligible")}
                tone="success"
              />
            ) : (
              <span className="flex flex-col gap-0.5">
                <StatusChip
                  label={
                    row.original.override_reason
                      ? t("reservations.drivers.overridden")
                      : t("reservations.drivers.not_eligible")
                  }
                  tone={row.original.override_reason ? "warning" : "danger"}
                />
                <span className="text-muted-foreground text-xs">
                  {row.original.reasons
                    .map((r) => reasonText(t, DRIVER_REASON_KEYS, r))
                    .join(", ")}
                </span>
                {row.original.override_reason ? (
                  <span className="text-muted-foreground text-xs">
                    {row.original.override_reason}
                  </span>
                ) : null}
              </span>
            ),
        }),
      ] as ColumnDef<ReservationDriver, unknown>[],
    [revealed, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={reservation.drivers}
      getRowId={(row, index) => row.driver_uuid ?? String(index)}
      emptyTitle={t("reservations.drivers.none")}
      emptyDescription={t("reservations.drivers.none_description")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-reservation-drivers-v1",
        rowSelection: false,
        globalFilter: false,
        sorting: false,
      }}
    />
  );
}

function CustomerCard({ reservation }: { reservation: Reservation }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { revealed, open } = useCustomerReveal();
  const c = reservation.customer;
  return (
    <EntitySectionCard
      title={t("reservations.detail.customer")}
      actions={
        can(permissions.tenant.customers.reveal) ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={Boolean(revealed)}
            onClick={open}
          >
            <Eye className="size-4" />
            {t("customers.reveal.button")}
          </Button>
        ) : null
      }
    >
      <div className="space-y-4">
        <RevealBanner />
        <EntityDetail
          sections={[
            {
              id: "customer",
              fields: [
                {
                  key: "name",
                  label: t("reservations.fields.customer"),
                  value: can(permissions.tenant.customers.read) ? (
                    <Link
                      className="text-primary font-medium hover:underline"
                      href={routes.tenant.customers.detail(slug, c.uuid)}
                    >
                      {c.display_name}
                    </Link>
                  ) : (
                    c.display_name
                  ),
                },
                {
                  key: "identity",
                  label: t("reservations.fields.identity"),
                  value: (
                    <SensitiveValue
                      masked={c.identity_masked}
                      plain={revealed?.identity_number}
                    />
                  ),
                },
                {
                  key: "phone",
                  label: t("reservations.fields.phone"),
                  value: (
                    <SensitiveValue
                      masked={c.phone_masked}
                      plain={revealed?.phone}
                    />
                  ),
                },
                {
                  key: "email",
                  label: t("reservations.fields.email"),
                  value: (
                    <SensitiveValue
                      masked={c.email_masked}
                      plain={revealed?.email}
                    />
                  ),
                },
                ...(reservation.invoice_customer
                  ? [
                      {
                        key: "invoice",
                        label: t("reservations.fields.invoice_customer"),
                        value: reservation.invoice_customer.display_name,
                      },
                    ]
                  : []),
                ...(reservation.agreement
                  ? [
                      {
                        key: "agreement",
                        label: t("reservations.fields.agreement"),
                        value: (
                          <span className="flex flex-wrap items-center gap-1.5">
                            <Link
                              className="font-medium underline-offset-2 hover:underline"
                              href={routes.tenant.agreements.detail(
                                slug,
                                reservation.agreement.uuid,
                              )}
                            >
                              {reservation.agreement.code} ·{" "}
                              {reservation.agreement.name}
                            </Link>
                            {reservation.open_account ? (
                              <Badge variant="outline">
                                {t("reservations.fields.open_account")}
                              </Badge>
                            ) : null}
                          </span>
                        ),
                      },
                    ]
                  : []),
              ],
            },
          ]}
        />
        <div className="space-y-2">
          <h4 className="text-sm font-semibold">
            {t("reservations.drivers.title")}
          </h4>
          <DriversCard reservation={reservation} />
        </div>
      </div>
    </EntitySectionCard>
  );
}

function PlanCard({ reservation }: { reservation: Reservation }) {
  const { t, locale } = useLocale();
  const plan = reservation.plan;
  const upgraded = plan.sold_product.uuid !== plan.product.uuid;
  const columns = useMemo(
    () =>
      [
        createColumn<ReservationAssignment>({
          accessorKey: "started_at",
          labelKey: "reservations.plan.columns.at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.started_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<ReservationAssignment>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle?.plate ?? "",
          labelKey: "reservations.plan.columns.vehicle",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono">
              {row.original.vehicle?.plate ?? "—"}
            </span>
          ),
        }),
        createColumn<ReservationAssignment>({
          accessorKey: "kind",
          labelKey: "reservations.plan.columns.kind",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.kind === "fixed"
              ? t("reservations.plan.fixed")
              : t("reservations.plan.planned"),
        }),
        createColumn<ReservationAssignment>({
          accessorKey: "source",
          labelKey: "reservations.plan.columns.source",
          enableSorting: false,
          cell: ({ row }) =>
            reasonText(t, ASSIGNMENT_SOURCE_LABEL_KEYS, row.original.source),
        }),
        createColumn<ReservationAssignment>({
          id: "actor",
          accessorFn: (row) => row.actor?.email ?? "",
          labelKey: "reservations.plan.columns.actor",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.actor
              ? actorName(row.original.actor)
              : t("reservations.plan.system"),
        }),
        createColumn<ReservationAssignment>({
          accessorKey: "note",
          labelKey: "reservations.plan.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
      ] as ColumnDef<ReservationAssignment, unknown>[],
    [locale, t],
  );
  return (
    <div className="space-y-6">
      <EntitySectionCard title={t("reservations.plan.title")}>
        <div className="space-y-4">
          <EntityDetail
            sections={[
              {
                id: "plan",
                fields: [
                  {
                    key: "sold",
                    label: t("reservations.plan.sold_product"),
                    value: `${plan.sold_product.code} · ${plan.sold_product.title} (v${plan.sold_product.version})`,
                  },
                  ...(upgraded
                    ? [
                        {
                          key: "current",
                          label: t("reservations.plan.current_product"),
                          value: `${plan.product.code} · ${plan.product.title} (v${plan.product.version})`,
                        },
                      ]
                    : []),
                  {
                    key: "guarantee",
                    label: t("reservations.conditions.guarantee"),
                    value:
                      plan.conditions.guarantee_label || plan.guarantee_type,
                  },
                  {
                    key: "planned",
                    label: t("reservations.plan.planned_vehicle"),
                    value: plan.planned_vehicle ? (
                      <span className="font-mono">
                        {plan.planned_vehicle.plate}{" "}
                        <span className="text-muted-foreground font-sans text-xs">
                          {plan.planned_vehicle.label}
                        </span>
                      </span>
                    ) : (
                      "—"
                    ),
                  },
                  {
                    key: "assigned",
                    label: t("reservations.plan.assigned_vehicle"),
                    value: plan.assigned_vehicle ? (
                      <span className="font-mono">
                        {plan.assigned_vehicle.plate}
                      </span>
                    ) : (
                      <span className="text-muted-foreground text-sm">
                        {t("reservations.plan.not_fixed")}
                      </span>
                    ),
                  },
                  {
                    key: "allocation",
                    label: t("reservations.plan.allocation"),
                    value: plan.allocation_uuid
                      ? `${plan.allocation_kind} · ${plan.allocation_status}`
                      : t("reservations.plan.no_capacity"),
                  },
                  ...(plan.hold_expires_at
                    ? [
                        {
                          key: "hold",
                          label: t("reservations.plan.hold_expires_at"),
                          value: datetime(
                            plan.hold_expires_at,
                            "d MMM yyyy HH:mm",
                            locale,
                          ),
                        },
                      ]
                    : []),
                ],
              },
            ]}
          />
          <p className="text-muted-foreground text-xs">
            {t("reservations.plan.help")}
          </p>
        </div>
      </EntitySectionCard>
      <EntitySectionCard title={t("reservations.conditions.title")}>
        <ConditionsList
          conditions={plan.conditions}
          previous={upgraded ? plan.sold_conditions : null}
        />
      </EntitySectionCard>
      <EntitySectionCard
        title={t("reservations.plan.history")}
        badge={plan.history.length}
      >
        <EntityTable
          columns={columns}
          data={plan.history}
          getRowId={(row, index) => `${row.started_at}-${index}`}
          emptyTitle={t("reservations.plan.history_empty")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-reservation-plan-history-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}

function PriceCard({ reservation }: { reservation: Reservation }) {
  const { t } = useLocale();
  const price = reservation.price;
  const snap = price.snapshot;
  return (
    <div className="space-y-6">
      <EntitySectionCard title={t("reservations.price.title")}>
        <div className="space-y-4">
          {snap.kept_price ? (
            <Badge variant="warning">{t("reservations.price.kept")}</Badge>
          ) : null}
          <PriceLinesTable
            lines={snap.lines}
            persistKey="tenant-reservation-price-lines-v1"
          />
          <PriceTotals
            rows={[
              {
                key: "base",
                label: t("reservations.price.base_total"),
                value: snap.base_total,
              },
              {
                key: "vat",
                label: t("reservations.price.vat_total"),
                value: snap.vat_total,
              },
              {
                key: "total",
                label: t("reservations.price.total"),
                value: price.total,
                strong: true,
              },
              {
                key: "prepayment",
                label: t("reservations.price.prepayment"),
                value: price.prepayment,
              },
              {
                key: "paid",
                label: t("reservations.price.paid"),
                value: price.paid,
              },
              {
                key: "balance",
                label: t("reservations.price.balance"),
                value: price.balance,
              },
              ...(Number(price.commission) > 0
                ? [
                    {
                      key: "commission",
                      label: t("reservations.price.commission"),
                      value: price.commission,
                    },
                    {
                      key: "net",
                      label: t("reservations.price.net_receivable"),
                      value: price.net_receivable,
                    },
                  ]
                : []),
            ]}
          />
          {snap.coupon_code ? (
            <p className="text-sm">
              {t("reservations.price.coupon", { code: snap.coupon_code })}
            </p>
          ) : null}
          {snap.manual_discount ? (
            <p className="text-sm">
              {t("reservations.price.manual_discount", {
                reason: snap.manual_discount.reason,
              })}
            </p>
          ) : null}
        </div>
      </EntitySectionCard>
      {snap.extensions.length > 0 ? (
        <EntitySectionCard
          title={t("reservations.price.extensions")}
          badge={snap.extensions.length}
        >
          <ExtensionsTable reservation={reservation} />
        </EntitySectionCard>
      ) : null}
    </div>
  );
}

type Extension = Reservation["price"]["snapshot"]["extensions"][number];

function ExtensionsTable({ reservation }: { reservation: Reservation }) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<Extension>({
          accessorKey: "ends_at",
          labelKey: "reservations.price.columns.extension_end",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.ends_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<Extension>({
          accessorKey: "added_days",
          labelKey: "reservations.price.columns.added_days",
          enableSorting: false,
        }),
        createColumn<Extension>({
          id: "band",
          accessorFn: (row) => row.billing_days,
          labelKey: "reservations.price.columns.band",
          enableSorting: false,
          cell: ({ row }) =>
            t("reservations.price.band_of", {
              days: row.original.billing_days,
            }),
        }),
        createColumn<Extension>({
          accessorKey: "amount",
          labelKey: "reservations.price.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.amount, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<Extension, unknown>[],
    [locale, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={reservation.price.snapshot.extensions}
      getRowId={(row, index) => `${row.ends_at}-${index}`}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-reservation-extensions-v1",
        rowSelection: false,
        globalFilter: false,
        sorting: false,
      }}
    />
  );
}

function EventsCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 });
  const query = useReservationEvents(uuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const total = query.data?.total ?? 0;
  const columns = useMemo(
    () =>
      [
        createColumn<ReservationEvent>({
          accessorKey: "created_at",
          labelKey: "reservations.events.columns.at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<ReservationEvent>({
          accessorKey: "action",
          labelKey: "reservations.events.columns.action",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {reasonText(t, EVENT_LABEL_KEYS, row.original.action)}
            </span>
          ),
        }),
        createColumn<ReservationEvent>({
          id: "status",
          accessorFn: (row) => row.to_status ?? "",
          labelKey: "reservations.events.columns.status",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.to_status
              ? `${statusLabel(t, row.original.from_status)} → ${statusLabel(t, row.original.to_status)}`
              : "—",
        }),
        createColumn<ReservationEvent>({
          accessorKey: "reason",
          labelKey: "reservations.events.columns.reason",
          enableSorting: false,
          cell: ({ row }) => row.original.reason || "—",
        }),
        createColumn<ReservationEvent>({
          id: "actor",
          accessorFn: (row) => row.actor?.email ?? "",
          labelKey: "reservations.events.columns.actor",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.actor
              ? actorName(row.original.actor)
              : t("reservations.plan.system"),
        }),
      ] as ColumnDef<ReservationEvent, unknown>[],
    [locale, t],
  );
  return (
    <EntitySectionCard
      title={t("reservations.events.title")}
      badge={query.data ? total : undefined}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("reservations.events.empty")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          persistKey: "tenant-reservation-events-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

function OverviewCard({ reservation }: { reservation: Reservation }) {
  const { t, locale } = useLocale();
  const r = reservation;
  const at = (value: string | null) =>
    value ? datetime(value, "d MMM yyyy HH:mm", locale) : "—";
  return (
    <EntitySectionCard title={t("reservations.detail.rental")}>
      <EntityDetail
        sections={[
          {
            id: "rental",
            fields: [
              {
                key: "pickup",
                label: t("reservations.fields.pickup"),
                value: `${r.pickup_branch.code} · ${r.pickup_branch.name}${r.pickup_point ? ` (${r.pickup_point.name})` : ""} — ${at(r.starts_at)}`,
              },
              {
                key: "return",
                label: t("reservations.fields.return"),
                value: `${r.return_branch.code} · ${r.return_branch.name}${r.return_point ? ` (${r.return_point.name})` : ""} — ${at(r.ends_at)}`,
              },
              {
                key: "billing_days",
                label: t("reservations.fields.billing_days"),
                value: String(r.billing_days),
              },
              {
                key: "product",
                label: t("reservations.fields.product"),
                value: `${r.plan.product.code} · ${r.plan.product.title}`,
              },
              {
                key: "channel",
                label: t("reservations.fields.channel"),
                value: channelLabel(t, r.channel),
              },
              {
                key: "external",
                label: t("reservations.fields.external_reference"),
                value: r.external_reference || "—",
              },
              {
                key: "commitment",
                label: t("reservations.fields.capacity_commitment"),
                value: t(COMMITMENT_LABEL_KEYS[r.capacity_commitment]),
              },
              {
                key: "deadline",
                label: t("reservations.fields.deadline"),
                value: at(r.review_deadline ?? r.payment_deadline),
              },
              {
                key: "assigned",
                label: t("reservations.fields.assigned_user"),
                value: actorName(r.assigned_user),
              },
              {
                key: "created",
                label: t("reservations.fields.created"),
                value: `${at(r.created_at)} · ${actorName(r.created_by)}`,
              },
              ...(r.cancel_reason
                ? [
                    {
                      key: "cancel_reason",
                      label: t("reservations.fields.cancel_reason"),
                      value: r.cancel_reason,
                    },
                  ]
                : []),
            ],
          },
        ]}
      />
      {r.note ? (
        <p className="text-muted-foreground mt-4 text-sm whitespace-pre-line">
          {r.note}
        </p>
      ) : null}
    </EntitySectionCard>
  );
}

/**
 * Reservation show page: status and actions (approve, confirm, cancel,
 * no-show, change / upgrade, extend, plate, drivers, staff), customer and
 * drivers (masked, reveal through the customers API), price snapshot,
 * vehicle plan (sold product vs planned / promised vehicle), resolution
 * tasks and the timeline. Payments and documents arrive with M3.
 */
export function ReservationDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useReservation(uuid);
  const transition = useReservationTransition();
  const [dialog, setDialog] = useState<DialogKind>(null);
  const r = query.data;
  const title = r?.reference ?? t("reservations.detail_title");
  const write = can(permissions.tenant.reservations.write);
  const confirmPerm = can(permissions.tenant.reservations.confirm);
  const cancelPerm = can(permissions.tenant.reservations.cancel);
  const approvePerm = can(permissions.tenant.reservations.approve);
  const [acceptOpen, setAcceptOpen] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const reject = useRejectReservation();

  const run = (action: TransitionAction, reason?: string) => {
    if (!r) return;
    transition.mutate(
      { uuid: r.uuid, action, reason },
      { onSuccess: () => setDialog(null) },
    );
  };

  const confirmCopy: Record<
    "submit" | "approve" | "confirm",
    { title: string; description: string }
  > = {
    submit: {
      title: t("reservations.submit.title"),
      description: t("reservations.submit.description"),
    },
    approve: {
      title: t("reservations.approve.title"),
      description: t("reservations.approve.description"),
    },
    confirm: {
      title: t("reservations.confirm.title"),
      description: t("reservations.confirm.description"),
    },
  };

  return (
    <EntityPage
      title={title}
      description={t("reservations.detail_description")}
      permission={permissions.tenant.reservations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: title },
      ]}
      actions={
        r ? (
          <EntityActions>
            {r.actions.submit && write ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog("submit")}
              >
                <Send className="size-4" />
                {t("reservations.actions.submit")}
              </Button>
            ) : null}
            {r.actions.approve && approvePerm ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setAcceptOpen(true)}
              >
                <CheckCircle2 className="size-4" />
                {t("reservations.actions.approve")}
              </Button>
            ) : null}
            {r.actions.reject && approvePerm ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setRejectOpen(true)}
              >
                <XCircle className="size-4" />
                {t("payments.approvals.reject")}
              </Button>
            ) : null}
            {r.actions.confirm && confirmPerm ? (
              <Button
                type="button"
                size="sm"
                variant={r.actions.approve ? "outline" : "default"}
                onClick={() => setDialog("confirm")}
              >
                <CheckCircle2 className="size-4" />
                {t("reservations.actions.confirm")}
              </Button>
            ) : null}
            {r.actions.change && write ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("change")}
              >
                <Pencil className="size-4" />
                {t("reservations.actions.change")}
              </Button>
            ) : null}
            {r.actions.extend && write ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("extend")}
              >
                <CalendarPlus className="size-4" />
                {t("reservations.actions.extend")}
              </Button>
            ) : null}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  aria-label={t("reservations.actions.more")}
                >
                  <MoreHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {r.actions.plate && write ? (
                  <DropdownMenuItem onSelect={() => setDialog("plate")}>
                    <Car className="size-4" />
                    {t("reservations.actions.plate")}
                  </DropdownMenuItem>
                ) : null}
                {write ? (
                  <DropdownMenuItem onSelect={() => setDialog("drivers")}>
                    <Users className="size-4" />
                    {t("reservations.actions.drivers")}
                  </DropdownMenuItem>
                ) : null}
                {write ? (
                  <DropdownMenuItem onSelect={() => setDialog("assign")}>
                    <UserCheck className="size-4" />
                    {t("reservations.actions.assign")}
                  </DropdownMenuItem>
                ) : null}
                {r.actions.no_show && cancelPerm ? (
                  <DropdownMenuItem onSelect={() => setDialog("no_show")}>
                    <UserX className="size-4" />
                    {t("reservations.actions.no_show")}
                  </DropdownMenuItem>
                ) : null}
                {r.actions.cancel && cancelPerm ? (
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onSelect={() => setDialog("cancel")}
                  >
                    <Ban className="size-4" />
                    {t("reservations.actions.cancel")}
                  </DropdownMenuItem>
                ) : null}
              </DropdownMenuContent>
            </DropdownMenu>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("reservations.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {r ? (
        <CustomerRevealProvider customerUuid={r.customer.uuid}>
          <div className="space-y-6">
            <EntityHeader
              title={r.reference}
              subtitle={
                <span className="flex flex-wrap items-center gap-2">
                  <span>
                    {r.plan.product.code} · {r.customer.display_name}
                  </span>
                  <RangeText startsAt={r.starts_at} endsAt={r.ends_at} />
                </span>
              }
              badges={
                <>
                  <ReservationStatusChip status={r.status} />
                  <PaymentStatusChip status={r.payment_status} />
                  <Badge variant="outline">
                    {t(COMMITMENT_LABEL_KEYS[r.capacity_commitment])}
                  </Badge>
                  {r.open_tasks > 0 ? (
                    <Badge variant="warning">
                      {t("reservations.open_tasks_badge", {
                        count: r.open_tasks,
                      })}
                    </Badge>
                  ) : null}
                  {r.plan.hold_expires_at ? (
                    <Badge variant="outline">
                      <CalendarClock className="size-3" />
                      {t("reservations.detail.hold_until", {
                        time: datetime(
                          r.plan.hold_expires_at,
                          "d MMM HH:mm",
                          locale,
                        ),
                      })}
                    </Badge>
                  ) : null}
                </>
              }
            />
            <Tabs defaultValue="overview" className="space-y-4">
              <TabsList className="flex h-auto flex-wrap">
                <TabsTrigger value="overview">
                  {t("reservations.tabs.overview")}
                </TabsTrigger>
                <TabsTrigger value="plan">
                  {t("reservations.tabs.plan")}
                </TabsTrigger>
                <TabsTrigger value="price">
                  {t("reservations.tabs.price")}
                </TabsTrigger>
                {can(permissions.tenant.resolutionTasks.read) ? (
                  <TabsTrigger value="tasks">
                    {t("reservations.tabs.tasks")}
                    {r.open_tasks > 0 ? ` (${r.open_tasks})` : ""}
                  </TabsTrigger>
                ) : null}
                <TabsTrigger value="requests">
                  {t("reservations.tabs.customer_requests")}
                </TabsTrigger>
                <TabsTrigger value="payments">
                  {t("reservations.tabs.payments")}
                </TabsTrigger>
                <TabsTrigger value="documents">
                  {t("reservations.tabs.documents")}
                </TabsTrigger>
                <TabsTrigger value="timeline">
                  {t("reservations.tabs.timeline")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="overview" className="space-y-6">
                <OverviewCard reservation={r} />
                <ReservationRentalCard
                  reservationUuid={r.uuid}
                  reservationStatus={r.status}
                  plateLabel={
                    r.plan.assigned_vehicle?.plate ??
                    r.plan.planned_vehicle?.plate ??
                    null
                  }
                />
                <CustomerCard reservation={r} />
              </TabsContent>
              <TabsContent value="plan">
                <PlanCard reservation={r} />
              </TabsContent>
              <TabsContent value="price">
                <PriceCard reservation={r} />
              </TabsContent>
              {can(permissions.tenant.resolutionTasks.read) ? (
                <TabsContent value="tasks">
                  <EntitySectionCard title={t("reservations.tabs.tasks")}>
                    <TasksTable
                      reservationUuid={r.uuid}
                      persistKey="tenant-reservation-tasks-v1"
                    />
                  </EntitySectionCard>
                </TabsContent>
              ) : null}
              <TabsContent value="requests">
                <EntitySectionCard
                  title={t("reservations.tabs.customer_requests")}
                >
                  <CustomerRequestsTable
                    reservationUuid={r.uuid}
                    persistKey="tenant-reservation-customer-requests-v1"
                  />
                </EntitySectionCard>
              </TabsContent>
              <TabsContent value="payments">
                <ReservationPayments reservation={r} />
              </TabsContent>
              <TabsContent value="documents">
                {can(permissions.tenant.contracts.read) ? (
                  <DocumentsSection reservationUuid={r.uuid} />
                ) : (
                  <EntitySectionCard title={t("reservations.tabs.documents")}>
                    <EntityUnavailable
                      title={t("reservations.documents.title")}
                      description={t("reservations.documents.description")}
                    />
                  </EntitySectionCard>
                )}
              </TabsContent>
              <TabsContent value="timeline">
                <EventsCard uuid={r.uuid} />
              </TabsContent>
            </Tabs>
          </div>

          <ChangeDialog
            reservation={r}
            open={dialog === "change"}
            onOpenChange={(open) => setDialog(open ? "change" : null)}
          />
          <ExtendDialog
            reservation={r}
            open={dialog === "extend"}
            onOpenChange={(open) => setDialog(open ? "extend" : null)}
          />
          <PlateDialog
            reservation={r}
            open={dialog === "plate"}
            onOpenChange={(open) => setDialog(open ? "plate" : null)}
          />
          <DriversDialog
            key={r.updated_at}
            reservation={r}
            open={dialog === "drivers"}
            onOpenChange={(open) => setDialog(open ? "drivers" : null)}
          />
          <AssignDialog
            reservation={r}
            open={dialog === "assign"}
            onOpenChange={(open) => setDialog(open ? "assign" : null)}
          />
          <AcceptDialog
            reservation={
              acceptOpen
                ? { uuid: r.uuid, reference: r.reference, total: r.price.total }
                : null
            }
            onClose={() => setAcceptOpen(false)}
          />
          <ReasonDialog
            open={rejectOpen}
            title={t("payments.approvals.reject_title", {
              reference: r.reference,
            })}
            description={t("payments.approvals.reject_description")}
            label={t("payments.approvals.reject_reason")}
            confirmLabel={t("payments.approvals.reject")}
            destructive
            pending={reject.isPending}
            onCancel={() => setRejectOpen(false)}
            onConfirm={(reason) =>
              reject.mutate(
                { uuid: r.uuid, reason },
                { onSuccess: () => setRejectOpen(false) },
              )
            }
          />
          <ReasonDialog
            open={dialog === "cancel" || dialog === "no_show"}
            title={
              dialog === "no_show"
                ? t("reservations.no_show.title")
                : t("reservations.cancel.title")
            }
            description={
              dialog === "no_show"
                ? t("reservations.no_show.description")
                : t("reservations.cancel.description", {
                    reference: r.reference,
                  })
            }
            label={t("reservations.fields.cancel_reason")}
            confirmLabel={
              dialog === "no_show"
                ? t("reservations.actions.no_show")
                : t("reservations.actions.cancel")
            }
            destructive
            pending={transition.isPending}
            onCancel={() => setDialog(null)}
            onConfirm={(reason) =>
              run(dialog === "no_show" ? "no-show" : "cancel", reason)
            }
          />
          <ConfirmDialog
            open={
              dialog === "submit" ||
              dialog === "approve" ||
              dialog === "confirm"
            }
            title={
              dialog === "submit" ||
              dialog === "approve" ||
              dialog === "confirm"
                ? confirmCopy[dialog].title
                : ""
            }
            description={
              dialog === "submit" ||
              dialog === "approve" ||
              dialog === "confirm"
                ? confirmCopy[dialog].description
                : undefined
            }
            confirmLabel={t("reservations.actions.continue")}
            isPending={transition.isPending}
            onCancel={() => setDialog(null)}
            onConfirm={() => {
              if (
                dialog === "submit" ||
                dialog === "approve" ||
                dialog === "confirm"
              ) {
                run(dialog);
              }
            }}
          />
        </CustomerRevealProvider>
      ) : null}
    </EntityPage>
  );
}
