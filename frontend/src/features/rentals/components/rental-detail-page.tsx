"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  CalendarPlus,
  CheckCircle2,
  ClipboardCheck,
  KeyRound,
  MoreHorizontal,
  Pencil,
  Plus,
  Repeat,
  Smartphone,
  Undo2,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Loading } from "@/components/common/loading";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
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
import { DocumentsSection } from "@/features/contracts/components/documents-section";
import { RentalTollCharges } from "@/features/tolls/components/rental-toll-charges";
import { RentalKabisChip } from "@/features/kabis/components/rental-kabis-chip";
import {
  ChargeStatusChip,
  RentalStatusChip,
  placeText,
} from "@/features/rentals/components/rental-cells";
import {
  ChargeDialog,
  CompleteReturnDialog,
  HandoverDialog,
  RentalExtendDialog,
  ReturnDialog,
  SwapDialog,
} from "@/features/rentals/components/rental-dialogs";
import { OperationTasksTable } from "@/features/rentals/components/operation-tasks-table";
import {
  CHARGE_KIND_LABEL_KEYS,
  RENTAL_STATUS_LABEL_KEYS,
  eventLabel,
  eventReason,
} from "@/features/rentals/constants";
import {
  useCancelRental,
  useDecideCharge,
  useOverrideCharge,
  useRental,
  useRentalEvents,
} from "@/features/rentals/hooks/use-rentals";
import type {
  Rental,
  RentalCharge,
  RentalDriver,
  RentalEvent,
  RentalLeg,
  RentalStatus,
} from "@/features/rentals/types";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import {
  ReferenceText,
  actorName,
} from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import type { AppLocale } from "@/config/i18n";
import { currency, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type DialogKind =
  | "handover"
  | "return"
  | "check"
  | "extend"
  | "swap"
  | "charge"
  | "cancel"
  | null;

function statusLabel(t: (key: string) => string, status: string | null) {
  if (!status) return "—";
  const key = RENTAL_STATUS_LABEL_KEYS[status as RentalStatus];
  return key ? t(key) : status;
}

function money(amount: string, code: string, locale: AppLocale) {
  const n = Number(amount);
  return Number.isFinite(n) ? currency(n, code || "TRY", locale) : amount;
}

function OverviewCard({ rental }: { rental: Rental }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const r = rental;
  const at = (value: string | null) =>
    value ? datetime(value, "d MMM yyyy HH:mm", locale) : "—";
  return (
    <EntitySectionCard title={t("rentals.detail.overview")}>
      <EntityDetail
        sections={[
          {
            id: "planned",
            title: t("rentals.detail.planned"),
            fields: [
              {
                key: "planned_pickup",
                label: t("rentals.fields.pickup"),
                value: `${placeText(r.planned_pickup)} — ${at(r.planned_pickup_at)}`,
              },
              {
                key: "planned_return",
                label: t("rentals.fields.return"),
                value: `${placeText(r.planned_return)} — ${at(r.planned_return_at)}`,
              },
            ],
          },
          {
            id: "actual",
            title: t("rentals.detail.actual"),
            fields: [
              {
                key: "actual_pickup",
                label: t("rentals.fields.pickup"),
                value: r.actual_pickup_at
                  ? `${placeText(r.actual_pickup)} — ${at(r.actual_pickup_at)} · ${actorName(r.delivered_by)}`
                  : "—",
              },
              {
                key: "actual_return",
                label: t("rentals.fields.return"),
                value: r.actual_return_at
                  ? `${placeText(r.actual_return)} — ${at(r.actual_return_at)} · ${actorName(r.returned_by)}`
                  : "—",
              },
              {
                key: "late",
                label: t("rentals.fields.late"),
                value:
                  r.late_minutes > 0
                    ? t("rentals.late_minutes", { count: r.late_minutes })
                    : "—",
              },
              {
                key: "allocation",
                label: t("rentals.fields.allocation_ends_at"),
                value: at(r.allocation_ends_at),
              },
            ],
          },
          {
            id: "rental",
            title: t("rentals.detail.rental"),
            fields: [
              {
                key: "reservation",
                label: t("rentals.fields.reservation"),
                value: can(permissions.tenant.reservations.read) ? (
                  <Link
                    className="text-primary font-medium hover:underline"
                    href={routes.tenant.reservations.detail(
                      slug,
                      r.reservation.uuid,
                    )}
                  >
                    {r.reservation.reference}
                  </Link>
                ) : (
                  r.reservation.reference
                ),
              },
              {
                key: "customer",
                label: t("rentals.fields.customer"),
                value: r.customer.display_name,
              },
              {
                key: "product",
                label: t("rentals.fields.product"),
                value: `${r.product.code ?? ""} · ${r.product.name}`,
              },
              {
                key: "vehicle",
                label: t("rentals.fields.vehicle"),
                value: `${r.vehicle.plate} · ${r.vehicle.label}`,
              },
              {
                key: "total",
                label: t("rentals.fields.total"),
                value: `${money(r.reservation.total_amount, r.reservation.currency, locale)} (${t("rentals.fields.paid")}: ${money(r.reservation.paid_amount, r.reservation.currency, locale)})`,
              },
              {
                key: "contract",
                label: t("rentals.fields.contract"),
                value: !r.contract.required
                  ? t("rentals.contract.not_required")
                  : r.contract.approved
                    ? t("rentals.contract.approved")
                    : r.contract_waiver
                      ? t("rentals.contract.waived")
                      : t("rentals.contract.pending"),
              },
              {
                key: "created",
                label: t("rentals.fields.created"),
                value: `${at(r.created_at)} · ${actorName(r.created_by)}`,
              },
              ...(r.checked_at
                ? [
                    {
                      key: "checked",
                      label: t("rentals.fields.checked_at"),
                      value: at(r.checked_at),
                    },
                  ]
                : []),
              ...(r.cancel_reason
                ? [
                    {
                      key: "cancel_reason",
                      label: t("rentals.fields.cancel_reason"),
                      value: r.cancel_reason,
                    },
                  ]
                : []),
            ],
          },
        ]}
      />
      {r.note || r.return_note || r.damage_note ? (
        <div className="text-muted-foreground mt-4 space-y-1 text-sm whitespace-pre-line">
          {r.note ? <p>{r.note}</p> : null}
          {r.return_note ? (
            <p>
              {t("rentals.fields.return_note")}: {r.return_note}
            </p>
          ) : null}
          {r.damage_note ? (
            <p>
              {t("rentals.fields.damage_note")}: {r.damage_note}
            </p>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

function PolicyCard({ rental }: { rental: Rental }) {
  const { t } = useLocale();
  const p = rental.policy;
  return (
    <EntitySectionCard
      title={t("rentals.policy.title")}
      collapsible
      defaultOpen={false}
    >
      <EntityDetail
        sections={[
          {
            id: "policy",
            description: t("rentals.policy.description", {
              version: p.settings_version,
            }),
            fields: [
              {
                key: "grace",
                label: t("rentals.policy.grace"),
                value:
                  p.late_return_grace_minutes !== null
                    ? t("rentals.policy.minutes", {
                        count: p.late_return_grace_minutes,
                      })
                    : "—",
              },
              {
                key: "extra_hour_fee",
                label: t("rentals.policy.extra_hour_fee"),
                value: p.extra_hour_fee || "—",
              },
              {
                key: "fuel_fee_per_percent",
                label: t("rentals.policy.fuel_fee_per_percent"),
                value: p.fuel_fee_per_percent || "—",
              },
              {
                key: "early_return",
                label: t("rentals.policy.early_return"),
                value: p.early_return || "—",
              },
              {
                key: "extension",
                label: t("rentals.policy.extension"),
                value: p.extension || "—",
              },
              {
                key: "fuel",
                label: t("rentals.policy.fuel"),
                value: p.fuel || "—",
              },
              {
                key: "mileage",
                label: t("rentals.policy.mileage"),
                value: p.mileage || "—",
              },
            ],
          },
        ]}
      />
    </EntitySectionCard>
  );
}

function LegsCard({ rental }: { rental: Rental }) {
  const { t, locale } = useLocale();
  const at = (value: string | null) =>
    value ? datetime(value, "d MMM HH:mm", locale) : "—";
  const columns = useMemo(
    () =>
      [
        createColumn<RentalLeg>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "rentals.legs.columns.vehicle",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-mono text-xs font-medium">
                {row.original.vehicle.plate}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.vehicle.label}
              </span>
            </span>
          ),
        }),
        createColumn<RentalLeg>({
          accessorKey: "reason",
          labelKey: "rentals.legs.columns.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="outline">
              {row.original.reason === "swap"
                ? t("rentals.legs.reasons.swap")
                : t("rentals.legs.reasons.handover")}
            </Badge>
          ),
        }),
        createColumn<RentalLeg>({
          id: "out",
          accessorFn: (row) => row.out_at ?? "",
          labelKey: "rentals.legs.columns.out",
          enableSorting: false,
          cell: ({ row }) =>
            `${at(row.original.out_at)} · ${row.original.out_branch?.code ?? "—"}`,
        }),
        createColumn<RentalLeg>({
          id: "out_reading",
          accessorFn: (row) => row.out_km ?? 0,
          labelKey: "rentals.legs.columns.out_reading",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.out_km !== null
              ? `${row.original.out_km.toLocaleString(locale)} km · %${row.original.out_fuel ?? "—"}`
              : "—",
        }),
        createColumn<RentalLeg>({
          id: "in",
          accessorFn: (row) => row.in_at ?? "",
          labelKey: "rentals.legs.columns.in",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.in_at
              ? `${at(row.original.in_at)} · ${row.original.in_branch?.code ?? "—"}`
              : "—",
        }),
        createColumn<RentalLeg>({
          id: "in_reading",
          accessorFn: (row) => row.in_km ?? 0,
          labelKey: "rentals.legs.columns.in_reading",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.in_km !== null
              ? `${row.original.in_km.toLocaleString(locale)} km · %${row.original.in_fuel ?? "—"}`
              : "—",
        }),
        createColumn<RentalLeg>({
          id: "driven",
          accessorFn: (row) => row.driven_km ?? 0,
          labelKey: "rentals.legs.columns.driven",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.driven_km !== null
              ? `${row.original.driven_km.toLocaleString(locale)} km`
              : "—",
        }),
        createColumn<RentalLeg>({
          accessorKey: "note",
          labelKey: "rentals.legs.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
      ] as ColumnDef<RentalLeg, unknown>[],
    // `at` only depends on the locale.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [locale, t],
  );
  return (
    <EntitySectionCard
      title={t("rentals.legs.title")}
      badge={rental.legs.length}
    >
      <EntityTable
        columns={columns}
        data={rental.legs}
        getRowId={(row) => row.uuid}
        emptyTitle={t("rentals.legs.empty")}
        emptyDescription={t("rentals.legs.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-rental-legs-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

function DriversCard({ rental }: { rental: Rental }) {
  const { t } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<RentalDriver>({
          accessorKey: "name",
          labelKey: "rentals.drivers.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5 font-medium">
              {row.original.name}
              {row.original.role === "primary" ? (
                <Badge variant="secondary" className="px-1 py-0 text-[10px]">
                  {t("rentals.drivers.primary")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<RentalDriver>({
          accessorKey: "licence_hint",
          labelKey: "rentals.drivers.columns.licence",
          enableSorting: false,
          cell: ({ row }) => row.original.licence_hint || "—",
        }),
      ] as ColumnDef<RentalDriver, unknown>[],
    [t],
  );
  return (
    <EntitySectionCard
      title={t("rentals.drivers.title")}
      badge={rental.drivers.length}
    >
      <EntityTable
        columns={columns}
        data={rental.drivers}
        getRowId={(row, index) => row.uuid ?? `driver-${index}`}
        emptyTitle={t("rentals.drivers.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-rental-drivers-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

type Decision = { charge: RentalCharge; decision: "approve" | "waive" } | null;

function ChargesCard({
  rental,
  onAdd,
  canAdd,
}: {
  rental: Rental;
  onAdd: () => void;
  canAdd: boolean;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const decide = useDecideCharge(rental.uuid);
  const [pending, setPending] = useState<Decision>(null);
  const closePerm = can(permissions.tenant.rentals.close);
  const damagePerm = can(permissions.tenant.damage.approve);
  const writePerm = can(permissions.tenant.rentals.write);
  const overridePerm = can(permissions.tenant.price.override);
  const [override, setOverride] = useState<RentalCharge | null>(null);
  // Toll passes have their own section (RentalTollCharges).
  const charges = useMemo(
    () => rental.charges.filter((c) => c.source !== "toll"),
    [rental.charges],
  );
  const columns = useMemo(
    () =>
      [
        createColumn<RentalCharge>({
          accessorKey: "kind",
          labelKey: "rentals.charges.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5">
              <span className="font-medium">
                {t(CHARGE_KIND_LABEL_KEYS[row.original.kind])}
              </span>
              {row.original.source === "policy" ? (
                <Badge variant="outline" className="px-1 py-0 text-[10px]">
                  {t("rentals.charges.policy_badge", {
                    version: row.original.settings_version,
                  })}
                </Badge>
              ) : null}
              {row.original.disputed ? (
                <Badge variant="danger" className="px-1 py-0 text-[10px]">
                  {t("rentals.charges.disputed_badge")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<RentalCharge>({
          accessorKey: "status",
          labelKey: "rentals.charges.columns.status",
          enableSorting: false,
          cell: ({ row }) => <ChargeStatusChip status={row.original.status} />,
        }),
        createColumn<RentalCharge>({
          accessorKey: "amount",
          labelKey: "rentals.charges.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col tabular-nums">
              <span>
                {money(row.original.amount, row.original.currency, locale)}
              </span>
              {row.original.original_amount ? (
                <span className="text-muted-foreground text-xs">
                  {t("rentals.charges.original_amount", {
                    amount: money(
                      row.original.original_amount,
                      row.original.currency,
                      locale,
                    ),
                  })}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<RentalCharge>({
          accessorKey: "reason",
          labelKey: "rentals.charges.columns.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 text-sm">
              {row.original.reason || "—"}
            </span>
          ),
        }),
        createColumn<RentalCharge>({
          id: "decided",
          accessorFn: (row) => row.decided_at ?? "",
          labelKey: "rentals.charges.columns.decided",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.decided_at
              ? `${datetime(row.original.decided_at, "d MMM HH:mm", locale)} · ${actorName(row.original.decided_by)}${row.original.decision_note ? ` — ${row.original.decision_note}` : ""}`
              : "—",
        }),
        createColumn<RentalCharge>({
          accessorKey: "created_at",
          labelKey: "rentals.charges.columns.created_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            `${datetime(row.original.created_at, "d MMM HH:mm", locale)} · ${row.original.created_by ? actorName(row.original.created_by) : t("rentals.system")}`,
        }),
        createColumn<RentalCharge>({
          id: "actions",
          labelKey: "rentals.charges.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const charge = row.original;
            if (charge.status !== "proposed") return null;
            // A disputed charge is approved only by the damage approver.
            const canApprove = charge.disputed
              ? damagePerm
              : closePerm || (charge.kind === "damage" && damagePerm);
            const canOverride =
              writePerm &&
              (charge.source !== "policy" && !charge.original_amount
                ? true
                : overridePerm);
            return (
              <EntityRowActions
                actions={[
                  ...(canOverride
                    ? [
                        {
                          id: "override",
                          label: t("rentals.charges.actions.override"),
                          icon: Pencil,
                          onSelect: () => setOverride(charge),
                        },
                      ]
                    : []),
                  ...(canApprove
                    ? [
                        {
                          id: "approve",
                          label: t("rentals.charges.actions.approve"),
                          icon: CheckCircle2,
                          onSelect: () =>
                            setPending({ charge, decision: "approve" }),
                        },
                      ]
                    : []),
                  ...(closePerm
                    ? [
                        {
                          id: "waive",
                          label: t("rentals.charges.actions.waive"),
                          icon: XCircle,
                          variant: "destructive" as const,
                          onSelect: () =>
                            setPending({ charge, decision: "waive" }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<RentalCharge, unknown>[],
    [closePerm, damagePerm, locale, overridePerm, t, writePerm],
  );
  return (
    <EntitySectionCard
      title={t("rentals.charges.title")}
      badge={charges.length}
      actions={
        canAdd ? (
          <Button type="button" size="sm" variant="outline" onClick={onAdd}>
            <Plus className="size-4" />
            {t("rentals.actions.propose_charge")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={charges}
        getRowId={(row) => row.uuid}
        emptyTitle={t("rentals.charges.empty")}
        emptyDescription={t("rentals.charges.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-rental-charges-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
      <ReasonDialog
        open={Boolean(pending)}
        title={
          pending?.decision === "waive"
            ? t("rentals.charges.waive_title")
            : t("rentals.charges.approve_title")
        }
        description={
          pending
            ? `${t(CHARGE_KIND_LABEL_KEYS[pending.charge.kind])} · ${money(pending.charge.amount, pending.charge.currency, locale)}${pending.charge.disputed ? ` — ${t("rentals.charges.disputed_hint")}` : ""}`
            : undefined
        }
        label={t("rentals.charges.decision_note")}
        confirmLabel={
          pending?.decision === "waive"
            ? t("rentals.charges.actions.waive")
            : t("rentals.charges.actions.approve")
        }
        destructive={pending?.decision === "waive"}
        required={
          pending?.decision === "waive" || Boolean(pending?.charge.disputed)
        }
        pending={decide.isPending}
        onCancel={() => setPending(null)}
        onConfirm={(note) => {
          if (!pending) return;
          decide.mutate(
            {
              charge: pending.charge.uuid,
              decision: pending.decision,
              note,
            },
            { onSuccess: () => setPending(null) },
          );
        }}
      />
      <ChargeOverrideDialog
        rentalUuid={rental.uuid}
        charge={override}
        onClose={() => setOverride(null)}
      />
    </EntitySectionCard>
  );
}

const OVERRIDE_FORM_ID = "rental-charge-override";

/** Changes the amount of a proposed charge with a reason. */
function ChargeOverrideDialog({
  rentalUuid,
  charge,
  onClose,
}: {
  rentalUuid: string;
  charge: RentalCharge | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const override = useOverrideCharge(rentalUuid);
  return (
    <AppDialog
      open={Boolean(charge)}
      onOpenChange={(next) => {
        if (!next && !override.isPending) onClose();
      }}
      title={t("rentals.charges.override_title")}
      description={
        charge
          ? `${t(CHARGE_KIND_LABEL_KEYS[charge.kind])} · ${money(charge.original_amount ?? charge.amount, charge.currency, locale)} — ${t("rentals.charges.override_description")}`
          : undefined
      }
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={OVERRIDE_FORM_ID}
            disabled={override.isPending}
          >
            {t("rentals.actions.save")}
          </Button>
        </>
      }
    >
      {charge ? (
        <AppForm<{ amount: string; reason: string }>
          id={OVERRIDE_FORM_ID}
          className="grid gap-4"
          schema={z.object({
            amount: z
              .string()
              .trim()
              .regex(/^\d+([.,]\d{1,2})?$/, t("rentals.validation.amount")),
            reason: z
              .string()
              .trim()
              .min(3, t("reservations.validation.reason"))
              .max(500, t("reservations.validation.reason")),
          })}
          defaultValues={{ amount: charge.amount, reason: "" }}
          onSubmit={async (values) => {
            await override.mutateAsync({
              charge: charge.uuid,
              amount: values.amount.replace(",", "."),
              reason: values.reason,
            });
            onClose();
          }}
        >
          <AppInput
            name="amount"
            inputMode="decimal"
            label={t("rentals.charges.override_amount")}
          />
          <AppTextarea
            name="reason"
            rows={2}
            label={t("rentals.charges.override_reason")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

function EventsCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 });
  const query = useRentalEvents(uuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const total = query.data?.total ?? 0;
  const columns = useMemo(
    () =>
      [
        createColumn<RentalEvent>({
          accessorKey: "created_at",
          labelKey: "rentals.events.columns.at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<RentalEvent>({
          accessorKey: "action",
          labelKey: "rentals.events.columns.action",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {eventLabel(t, row.original.action)}
            </span>
          ),
        }),
        createColumn<RentalEvent>({
          id: "status",
          accessorFn: (row) => row.to_status ?? "",
          labelKey: "rentals.events.columns.status",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.to_status
              ? `${statusLabel(t, row.original.from_status)} → ${statusLabel(t, row.original.to_status)}`
              : "—",
        }),
        createColumn<RentalEvent>({
          accessorKey: "reason",
          labelKey: "rentals.events.columns.reason",
          enableSorting: false,
          cell: ({ row }) => eventReason(t, row.original.reason),
        }),
        createColumn<RentalEvent>({
          id: "actor",
          accessorFn: (row) => row.actor?.email ?? "",
          labelKey: "rentals.events.columns.actor",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.actor
              ? actorName(row.original.actor)
              : t("rentals.system"),
        }),
      ] as ColumnDef<RentalEvent, unknown>[],
    [locale, t],
  );
  return (
    <EntitySectionCard
      title={t("rentals.events.title")}
      badge={query.data ? total : undefined}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("rentals.events.empty")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          persistKey: "tenant-rental-events-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

/**
 * Rental show page: planned (reservation) vs actual values, handover,
 * return, return check, extension, vehicle swap, cancel; vehicle legs,
 * drivers, charges (policy fees and manual proposals with approval),
 * documents, operation tasks and the timeline.
 */
export function RentalDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useRental(uuid);
  const cancel = useCancelRental(uuid);
  const [dialog, setDialog] = useState<DialogKind>(null);
  const r = query.data;
  const title = r?.reference ?? t("rentals.detail_title");
  const write = can(permissions.tenant.rentals.write);
  const closePerm = can(permissions.tenant.rentals.close);
  const status = r?.status;
  const underway = status === "active";
  const canCharge =
    write &&
    (status === "active" || status === "return_check" || status === "returned");
  const open = (kind: DialogKind) => () => setDialog(kind);
  const onOpenChange = (kind: DialogKind) => (next: boolean) =>
    setDialog(next ? kind : null);

  return (
    <EntityPage
      title={title}
      description={t("rentals.detail_description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("rentals.title"), href: routes.tenant.rentals.root(slug) },
        { label: title },
      ]}
      actions={
        r ? (
          <EntityActions>
            {status === "preparation" && write ? (
              <Button asChild type="button" size="sm" variant="outline">
                <Link href={routes.tenant.field.handover(slug, uuid)}>
                  <Smartphone className="size-4" />
                  {t("rentals.actions.mobile_handover")}
                </Link>
              </Button>
            ) : null}
            {status === "preparation" && write ? (
              <Button type="button" size="sm" onClick={open("handover")}>
                <KeyRound className="size-4" />
                {t("rentals.actions.handover")}
              </Button>
            ) : null}
            {underway && write ? (
              <Button asChild type="button" size="sm" variant="outline">
                <Link href={routes.tenant.field.return(slug, uuid)}>
                  <Smartphone className="size-4" />
                  {t("rentals.actions.mobile_return")}
                </Link>
              </Button>
            ) : null}
            {underway && write ? (
              <Button type="button" size="sm" onClick={open("return")}>
                <Undo2 className="size-4" />
                {t("rentals.actions.return")}
              </Button>
            ) : null}
            {status === "return_check" && closePerm ? (
              <Button type="button" size="sm" onClick={open("check")}>
                <ClipboardCheck className="size-4" />
                {t("rentals.actions.complete_return")}
              </Button>
            ) : null}
            {(status === "preparation" || underway) && write ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={open("extend")}
              >
                <CalendarPlus className="size-4" />
                {t("rentals.actions.extend")}
              </Button>
            ) : null}
            {(underway && write) ||
            canCharge ||
            (status === "preparation" && closePerm) ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    aria-label={t("rentals.actions.more")}
                  >
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {underway && write ? (
                    <DropdownMenuItem onSelect={open("swap")}>
                      <Repeat className="size-4" />
                      {t("rentals.actions.swap")}
                    </DropdownMenuItem>
                  ) : null}
                  {canCharge ? (
                    <DropdownMenuItem onSelect={open("charge")}>
                      <Plus className="size-4" />
                      {t("rentals.actions.propose_charge")}
                    </DropdownMenuItem>
                  ) : null}
                  {status === "preparation" && closePerm ? (
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onSelect={open("cancel")}
                    >
                      <Ban className="size-4" />
                      {t("rentals.actions.cancel")}
                    </DropdownMenuItem>
                  ) : null}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("rentals.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {r ? (
        <>
          <div className="space-y-6">
            <EntityHeader
              title={r.reference}
              subtitle={
                <span className="flex flex-wrap items-center gap-2">
                  <ReferenceText reference={r.reservation.reference} />
                  <span>
                    {r.vehicle.plate} · {r.customer.display_name}
                  </span>
                  <span className="text-sm tabular-nums">
                    {datetime(
                      r.actual_pickup_at ?? r.planned_pickup_at,
                      "d MMM yyyy HH:mm",
                      locale,
                    )}
                    <span className="text-muted-foreground"> → </span>
                    {datetime(
                      r.actual_return_at ?? r.planned_return_at,
                      "d MMM yyyy HH:mm",
                      locale,
                    )}
                  </span>
                </span>
              }
              badges={
                <>
                  <RentalStatusChip status={r.status} />
                  {r.overdue_at ? (
                    <Badge variant="danger">{t("rentals.overdue_badge")}</Badge>
                  ) : null}
                  {r.contract.required && !r.contract.approved ? (
                    <Badge variant="warning">
                      {r.contract_waiver
                        ? t("rentals.contract.waived")
                        : t("rentals.contract.pending")}
                    </Badge>
                  ) : null}
                  <RentalKabisChip rentalUuid={r.uuid} />
                </>
              }
            />
            <Tabs defaultValue="overview" className="space-y-4">
              <TabsList className="flex h-auto flex-wrap">
                <TabsTrigger value="overview">
                  {t("rentals.tabs.overview")}
                </TabsTrigger>
                <TabsTrigger value="vehicles">
                  {t("rentals.tabs.vehicles")}
                </TabsTrigger>
                <TabsTrigger value="charges">
                  {t("rentals.tabs.charges")}
                  {r.charges.some((c) => c.status === "proposed")
                    ? ` (${r.charges.filter((c) => c.status === "proposed").length})`
                    : ""}
                </TabsTrigger>
                {can(permissions.tenant.contracts.read) ? (
                  <TabsTrigger value="documents">
                    {t("rentals.tabs.documents")}
                  </TabsTrigger>
                ) : null}
                <TabsTrigger value="tasks">
                  {t("rentals.tabs.tasks")}
                </TabsTrigger>
                <TabsTrigger value="timeline">
                  {t("rentals.tabs.timeline")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="overview" className="space-y-6">
                <OverviewCard rental={r} />
                <DriversCard rental={r} />
                <PolicyCard rental={r} />
              </TabsContent>
              <TabsContent value="vehicles">
                <LegsCard rental={r} />
              </TabsContent>
              <TabsContent value="charges" className="space-y-6">
                <ChargesCard
                  rental={r}
                  canAdd={canCharge}
                  onAdd={open("charge")}
                />
                <RentalTollCharges
                  rentalUuid={r.uuid}
                  charges={r.charges.filter((c) => c.source === "toll")}
                  currency={r.reservation.currency}
                />
              </TabsContent>
              {can(permissions.tenant.contracts.read) ? (
                <TabsContent value="documents">
                  <DocumentsSection reservationUuid={r.reservation.uuid} />
                </TabsContent>
              ) : null}
              <TabsContent value="tasks">
                <EntitySectionCard title={t("rentals.tabs.tasks")}>
                  <OperationTasksTable
                    rentalUuid={r.uuid}
                    persistKey="tenant-rental-tasks-v1"
                  />
                </EntitySectionCard>
              </TabsContent>
              <TabsContent value="timeline">
                <EventsCard uuid={r.uuid} />
              </TabsContent>
            </Tabs>
          </div>

          <HandoverDialog
            rental={r}
            open={dialog === "handover"}
            onOpenChange={onOpenChange("handover")}
          />
          <ReturnDialog
            rental={r}
            open={dialog === "return"}
            onOpenChange={onOpenChange("return")}
          />
          <CompleteReturnDialog
            rental={r}
            open={dialog === "check"}
            onOpenChange={onOpenChange("check")}
          />
          <RentalExtendDialog
            rental={r}
            open={dialog === "extend"}
            onOpenChange={onOpenChange("extend")}
          />
          <SwapDialog
            rental={r}
            open={dialog === "swap"}
            onOpenChange={onOpenChange("swap")}
          />
          <ChargeDialog
            rental={r}
            open={dialog === "charge"}
            onOpenChange={onOpenChange("charge")}
          />
          <ReasonDialog
            open={dialog === "cancel"}
            title={t("rentals.cancel.title")}
            description={t("rentals.cancel.description", {
              reference: r.reference,
            })}
            label={t("rentals.fields.cancel_reason")}
            confirmLabel={t("rentals.actions.cancel")}
            destructive
            pending={cancel.isPending}
            onCancel={() => setDialog(null)}
            onConfirm={(reason) =>
              cancel.mutate(reason, { onSuccess: () => setDialog(null) })
            }
          />
        </>
      ) : null}
    </EntityPage>
  );
}
