"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { AlertTriangle, ArrowRight, Calculator } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
} from "@/components/forms";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { CAPACITY_REASON_LABEL_KEYS } from "@/features/availability/constants";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  isoToLocalInput,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import { moneyText } from "@/features/pricing/lib/format";
import { MemberPickerField } from "@/features/reservations/components/member-picker";
import {
  ConditionsList,
  PriceLinesTable,
  PriceTotals,
} from "@/features/reservations/components/price-breakdown";
import {
  CustomerDriversPicker,
  type WizardDrivers,
} from "@/features/reservations/components/wizard-customer-step";
import {
  DRIVER_REASON_KEYS,
  PURPOSE_LABEL_KEYS,
  WARNING_KEYS,
  reasonText,
} from "@/features/reservations/constants";
import {
  useApplyChange,
  useAssignReservation,
  useChangePlate,
  useChangeQuote,
  useExtendReservation,
  usePlateCandidates,
  useQuoteSearch,
  useUpdateDrivers,
} from "@/features/reservations/hooks/use-reservations";
import {
  reservationProblem,
  type ReservationProblem,
} from "@/features/reservations/lib/errors";
import type {
  Quote,
  Reservation,
  ReservationVehicleRef,
} from "@/features/reservations/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type DialogProps = {
  reservation: Reservation;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/** Generic answer of a change attempt (capacity, conflicts, other). */
function ChangeProblem({ problem }: { problem: ReservationProblem }) {
  const { t } = useLocale();
  let body: string;
  switch (problem.kind) {
    case "capacity":
      body = t(
        CAPACITY_REASON_LABEL_KEYS[problem.reason] ??
          CAPACITY_REASON_LABEL_KEYS.no_vehicle,
      );
      break;
    case "drivers":
      body = problem.drivers
        .filter((d) => !d.eligible)
        .map(
          (d) =>
            `${d.name}: ${d.reasons.map((r) => reasonText(t, DRIVER_REASON_KEYS, r)).join(", ")}`,
        )
        .join(" · ");
      break;
    case "coupon":
      body = problem.message;
      break;
    default:
      body = "message" in problem ? problem.message : "";
  }
  return (
    <Alert variant="destructive">
      <AlertTriangle className="size-4" />
      <AlertTitle>{t("reservations.problems.change_failed")}</AlertTitle>
      <AlertDescription>{body}</AlertDescription>
    </Alert>
  );
}

/** Totals of a change / extension quote with its difference. */
function ChangeQuoteSummary({ quote }: { quote: Quote }) {
  const { t } = useLocale();
  const isExtension = quote.purpose === "extension";
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="secondary">
          {t(PURPOSE_LABEL_KEYS[quote.purpose])}
        </Badge>
        <span className="text-muted-foreground">
          {quote.product.code} · {quote.product.title}
        </span>
      </div>
      <PriceLinesTable
        lines={quote.lines}
        persistKey="tenant-reservation-change-lines-v1"
      />
      <PriceTotals
        rows={[
          {
            key: "total",
            label: isExtension
              ? t("reservations.price.extension_total")
              : t("reservations.price.new_total"),
            value: quote.total,
            strong: true,
          },
          {
            key: "difference",
            label: t("reservations.price.difference"),
            value: quote.difference,
          },
        ]}
      />
      {quote.breakdown.conditions && quote.breakdown.previous_conditions ? (
        <div className="space-y-2">
          <h4 className="text-sm font-semibold">
            {t("reservations.conditions.difference_title")}
          </h4>
          <ConditionsList
            conditions={quote.breakdown.conditions}
            previous={quote.breakdown.previous_conditions}
          />
        </div>
      ) : null}
      {quote.breakdown.warnings.length > 0 ? (
        <Alert>
          <AlertDescription>
            {quote.breakdown.warnings
              .map((w) => reasonText(t, WARNING_KEYS, w))
              .join(" · ")}
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}

const CHANGE_FORM_ID = "reservation-change-form";
const KEEP = "keep";

type ChangeValues = {
  pickup_branch_uuid: string;
  return_branch_uuid: string;
  starts_at: string;
  ends_at: string;
  product_uuid: string;
};

/**
 * "Değiştir / Yükselt": new dates, places or product are priced as a
 * change quote first (difference, condition changes for an upgrade); the
 * change is applied with that quote atomically (capacity, plan and price
 * together) or refused as a whole.
 */
export function ChangeDialog({ reservation, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );
  const quoteMutation = useChangeQuote(reservation.uuid);
  const apply = useApplyChange(reservation.uuid);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [problem, setProblem] = useState<ReservationProblem | null>(null);
  const [reason, setReason] = useState("");
  const [keepPrice, setKeepPrice] = useState(false);
  const products = useQuoteSearch(
    open
      ? {
          pickup_branch_uuid: reservation.pickup_branch.uuid,
          return_branch_uuid: reservation.return_branch.uuid,
          starts_at: reservation.starts_at,
          ends_at: reservation.ends_at,
          channel: reservation.channel,
        }
      : null,
  );
  const schema = useMemo(
    () =>
      z
        .object({
          pickup_branch_uuid: z.string().min(1),
          return_branch_uuid: z.string().min(1),
          starts_at: z.string().min(1, t("reservations.validation.required")),
          ends_at: z.string().min(1, t("reservations.validation.required")),
          product_uuid: z.string().min(1),
        })
        .refine((v) => new Date(v.ends_at) > new Date(v.starts_at), {
          path: ["ends_at"],
          message: t("reservations.validation.ends_after_start"),
        }),
    [t],
  );
  const branchOptions = (branches.data?.items ?? []).map((b) => ({
    value: b.uuid,
    label: `${b.code} · ${b.name}`,
  }));
  const productOptions = [
    {
      value: KEEP,
      label: t("reservations.change.keep_product", {
        code: reservation.plan.product.code,
      }),
    },
    ...(products.data?.items ?? [])
      .filter(
        (i) =>
          i.product.uuid !== reservation.plan.product.uuid && i.sellable > 0,
      )
      .map((i) => ({
        value: i.product.uuid,
        label: `${i.product.code} · ${i.product.title} (${i.sellable})`,
      })),
  ];
  const close = () => {
    setQuote(null);
    setProblem(null);
    setReason("");
    setKeepPrice(false);
    onOpenChange(false);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("reservations.change.title")}
      description={t("reservations.change.description")}
      className="sm:max-w-3xl"
      footer={
        quote ? (
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setQuote(null)}
            >
              {t("reservations.wizard.back")}
            </Button>
            <Button
              type="button"
              disabled={apply.isPending}
              onClick={async () => {
                setProblem(null);
                try {
                  await apply.mutateAsync({
                    quote_uuid: quote.uuid,
                    reason: reason.trim() || undefined,
                    keep_price: keepPrice,
                  });
                  close();
                } catch (error) {
                  setProblem(reservationProblem(error));
                }
              }}
            >
              <ArrowRight className="size-4" />
              {t("reservations.change.apply")}
            </Button>
          </>
        ) : (
          <>
            <Button type="button" variant="outline" onClick={close}>
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={CHANGE_FORM_ID}
              disabled={quoteMutation.isPending}
            >
              <Calculator className="size-4" />
              {t("reservations.change.price")}
            </Button>
          </>
        )
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        {problem ? <ChangeProblem problem={problem} /> : null}
        {quote ? (
          <>
            <ChangeQuoteSummary quote={quote} />
            <div className="grid gap-1.5">
              <Label htmlFor="change-reason">
                {t("reservations.change.reason")}
              </Label>
              <Input
                id="change-reason"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </div>
            {can(permissions.tenant.price.override) ? (
              <div className="flex items-start gap-3">
                <Switch
                  id="keep-price"
                  checked={keepPrice}
                  onCheckedChange={setKeepPrice}
                />
                <div className="grid gap-0.5">
                  <Label htmlFor="keep-price">
                    {t("reservations.change.keep_price")}
                  </Label>
                  <span className="text-muted-foreground text-xs">
                    {t("reservations.change.keep_price_hint")}
                  </span>
                </div>
              </div>
            ) : null}
          </>
        ) : (
          open && (
            <AppForm<ChangeValues>
              id={CHANGE_FORM_ID}
              className="grid gap-4 sm:grid-cols-2"
              schema={schema}
              defaultValues={{
                pickup_branch_uuid: reservation.pickup_branch.uuid,
                return_branch_uuid: reservation.return_branch.uuid,
                starts_at: isoToLocalInput(reservation.starts_at),
                ends_at: isoToLocalInput(reservation.ends_at),
                product_uuid: KEEP,
              }}
              onSubmit={async (values) => {
                setProblem(null);
                try {
                  const q = await quoteMutation.mutateAsync({
                    pickup_branch_uuid: values.pickup_branch_uuid,
                    return_branch_uuid: values.return_branch_uuid,
                    starts_at: localInputToISO(values.starts_at),
                    ends_at: localInputToISO(values.ends_at),
                    product_uuid:
                      values.product_uuid === KEEP ? null : values.product_uuid,
                  });
                  setQuote(q);
                } catch (error) {
                  setProblem(reservationProblem(error));
                }
              }}
            >
              <AppSelect
                name="pickup_branch_uuid"
                label={t("reservations.fields.pickup_branch")}
                options={branchOptions}
              />
              <AppSelect
                name="return_branch_uuid"
                label={t("reservations.fields.return_branch")}
                options={branchOptions}
              />
              <AppDatePicker
                name="starts_at"
                withTime
                label={t("reservations.fields.starts_at")}
              />
              <AppDatePicker
                name="ends_at"
                withTime
                label={t("reservations.fields.ends_at")}
              />
              <AppSelect
                name="product_uuid"
                className="sm:col-span-2"
                label={t("reservations.fields.product")}
                description={t("reservations.change.product_hint")}
                options={productOptions}
              />
            </AppForm>
          )
        )}
      </div>
    </AppDialog>
  );
}

const EXTEND_FORM_ID = "reservation-extend-form";

/**
 * "Uzat": a later return. The added days are priced with the band of the
 * NEW total length; when another commitment blocks the planned vehicle the
 * API answers EXTENSION_CONFLICT with alternatives (another vehicle, an
 * upgrade) and opens a resolution task (RT-A31).
 */
export function ExtendDialog({ reservation, open, onOpenChange }: DialogProps) {
  const { t, locale } = useLocale();
  const extend = useExtendReservation(reservation.uuid);
  const apply = useApplyChange(reservation.uuid);
  const [problem, setProblem] = useState<ReservationProblem | null>(null);
  const schema = useMemo(
    () =>
      z
        .object({
          ends_at: z.string().min(1, t("reservations.validation.required")),
          reason: z.string().max(500),
        })
        .refine((v) => new Date(v.ends_at) > new Date(reservation.ends_at), {
          path: ["ends_at"],
          message: t("reservations.validation.extend_later"),
        }),
    [reservation.ends_at, t],
  );
  const close = () => {
    setProblem(null);
    onOpenChange(false);
  };
  const applyAlternative = async (quote: Quote) => {
    try {
      await apply.mutateAsync({
        quote_uuid: quote.uuid,
        reason: t("reservations.extend.alternative_reason"),
      });
      close();
    } catch (error) {
      setProblem(reservationProblem(error));
    }
  };
  const alternatives =
    problem?.kind === "extension" ? problem.alternatives : null;

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("reservations.extend.title")}
      description={t("reservations.extend.description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={EXTEND_FORM_ID}
            disabled={extend.isPending}
          >
            {t("reservations.actions.extend")}
          </Button>
        </>
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        {problem && problem.kind !== "extension" ? (
          <ChangeProblem problem={problem} />
        ) : null}
        {problem?.kind === "extension" ? (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertTitle>{t("reservations.extend.conflict_title")}</AlertTitle>
            <AlertDescription className="space-y-3">
              <p>{t("reservations.extend.conflict_description")}</p>
              {alternatives && alternatives.vehicles.length > 0 ? (
                <div className="space-y-1">
                  <p className="font-medium">
                    {t("reservations.extend.other_vehicles")}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {alternatives.vehicles.map((alt) => (
                      <Button
                        key={alt.vehicle.uuid}
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={apply.isPending}
                        onClick={() => void applyAlternative(alt.quote)}
                      >
                        {alt.vehicle.plate} ·{" "}
                        {moneyText(alt.quote.total, locale)}
                      </Button>
                    ))}
                  </div>
                </div>
              ) : null}
              {alternatives && alternatives.products.length > 0 ? (
                <div className="space-y-1">
                  <p className="font-medium">
                    {t("reservations.extend.other_products")}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {alternatives.products.map((alt) => (
                      <Button
                        key={alt.product.uuid}
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={apply.isPending}
                        onClick={() => void applyAlternative(alt.quote)}
                      >
                        {alt.product.code} · +
                        {moneyText(alt.quote.difference, locale)}
                      </Button>
                    ))}
                  </div>
                </div>
              ) : null}
              {alternatives &&
              alternatives.vehicles.length === 0 &&
              alternatives.products.length === 0 ? (
                <p>{t("reservations.extend.no_alternatives")}</p>
              ) : null}
              <p className="text-xs">{t("reservations.extend.task_opened")}</p>
            </AlertDescription>
          </Alert>
        ) : null}
        {open ? (
          <AppForm<{ ends_at: string; reason: string }>
            id={EXTEND_FORM_ID}
            className="grid gap-4 sm:grid-cols-2"
            schema={schema}
            defaultValues={{
              ends_at: isoToLocalInput(reservation.ends_at),
              reason: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await extend.mutateAsync({
                  ends_at: localInputToISO(values.ends_at) ?? values.ends_at,
                  reason: values.reason.trim() || undefined,
                });
                close();
              } catch (error) {
                setProblem(reservationProblem(error));
              }
            }}
          >
            <AppDatePicker
              name="ends_at"
              withTime
              label={t("reservations.extend.new_end")}
            />
            <AppInput name="reason" label={t("reservations.change.reason")} />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

const PLATE_FORM_ID = "reservation-plate-form";

/**
 * "Plaka değiştir / ata": the vehicles the engine can move the plan to
 * (keeping the sold guarantee). `fix` promises the plate to the customer.
 */
export function PlateDialog({ reservation, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const candidates = usePlateCandidates(reservation.uuid, open);
  const plate = useChangePlate();
  const [selection, setSelection] = useState<RowSelectionState>({});
  const selected = Object.keys(selection).find((id) => selection[id]) ?? null;
  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<ReservationVehicleRef>(),
        createColumn<ReservationVehicleRef>({
          accessorKey: "plate",
          labelKey: "reservations.plate.columns.plate",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono font-medium">{row.original.plate}</span>
          ),
        }),
        createColumn<ReservationVehicleRef>({
          accessorKey: "label",
          labelKey: "reservations.plate.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => row.original.label || "—",
        }),
      ] as ColumnDef<ReservationVehicleRef, unknown>[],
    [],
  );
  const close = () => {
    setSelection({});
    onOpenChange(false);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("reservations.plate.title")}
      description={t("reservations.plate.description", {
        plate:
          reservation.plan.assigned_vehicle?.plate ??
          reservation.plan.planned_vehicle?.plate ??
          "—",
      })}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={PLATE_FORM_ID}
            disabled={plate.isPending || !selected}
          >
            {t("reservations.actions.save")}
          </Button>
        </>
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        <EntityTable
          columns={columns}
          data={candidates.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={candidates.isLoading}
          isError={candidates.isError}
          onRetry={() => void candidates.refetch()}
          emptyTitle={t("reservations.plate.empty")}
          emptyDescription={t("reservations.plate.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          state={{
            rowSelection: selection,
            onRowSelectionChange: (updater) => {
              const next =
                typeof updater === "function" ? updater(selection) : updater;
              // One vehicle: keep only the latest pick.
              const ids = Object.keys(next).filter(
                (id) => next[id] && !selection[id],
              );
              setSelection(ids.length ? { [ids[0]]: true } : next);
            },
          }}
          features={{
            persistKey: "tenant-reservation-plate-candidates-v1",
            rowSelection: true,
            globalFilter: false,
            sorting: false,
          }}
        />
        {open ? (
          <AppForm<{ fix: boolean; reason: string }>
            id={PLATE_FORM_ID}
            className="grid gap-4"
            schema={z.object({ fix: z.boolean(), reason: z.string().max(500) })}
            defaultValues={{ fix: reservation.plan.plate_fixed, reason: "" }}
            onSubmit={async (values) => {
              if (!selected) return;
              await plate.mutateAsync({
                uuid: reservation.uuid,
                body: {
                  vehicle_uuid: selected,
                  fix: values.fix,
                  reason: values.reason.trim() || undefined,
                  source: "staff",
                },
              });
              close();
            }}
          >
            <AppSwitch
              name="fix"
              label={t("reservations.plate.fix")}
              description={t("reservations.plate.fix_hint")}
            />
            <AppInput name="reason" label={t("reservations.change.reason")} />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

/**
 * Drivers of the reservation: checked against the sold version's driver
 * rules on the pick-up day; an ineligible driver needs the override
 * permission and a reason.
 */
export function DriversDialog({
  reservation,
  open,
  onOpenChange,
}: DialogProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const update = useUpdateDrivers(reservation.uuid);
  const initial = useMemo<WizardDrivers>(() => {
    const selected: RowSelectionState = {};
    let primary = "";
    for (const d of reservation.drivers) {
      if (!d.driver_uuid) continue;
      selected[d.driver_uuid] = true;
      if (d.role === "primary") primary = d.driver_uuid;
    }
    return { selected, primary };
  }, [reservation.drivers]);
  const [drivers, setDrivers] = useState<WizardDrivers>(initial);
  const [problem, setProblem] = useState<ReservationProblem | null>(null);
  const [override, setOverride] = useState("");
  const ids = Object.keys(drivers.selected).filter(
    (id) => drivers.selected[id],
  );
  const save = async (withOverride: boolean) => {
    setProblem(null);
    try {
      await update.mutateAsync({
        drivers: ids.map((id) => ({
          driver_uuid: id,
          role: id === drivers.primary ? "primary" : "additional",
        })),
        eligibility_override: withOverride ? { reason: override.trim() } : null,
      });
      onOpenChange(false);
    } catch (error) {
      setProblem(reservationProblem(error));
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setDrivers(initial);
          setProblem(null);
        }
        onOpenChange(next);
      }}
      title={t("reservations.drivers.edit_title")}
      description={t("reservations.drivers.help")}
      className="sm:max-w-3xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={update.isPending}
            onClick={() => void save(false)}
          >
            {t("reservations.actions.save")}
          </Button>
        </>
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        {problem ? <ChangeProblem problem={problem} /> : null}
        {problem?.kind === "drivers" &&
        can(permissions.tenant.reservations.overrideEligibility) ? (
          <div className="flex flex-wrap items-end gap-2">
            <div className="grid min-w-64 flex-1 gap-1.5">
              <Label htmlFor="drivers-override">
                {t("reservations.problems.override_reason")}
              </Label>
              <Input
                id="drivers-override"
                value={override}
                onChange={(e) => setOverride(e.target.value)}
              />
            </div>
            <Button
              type="button"
              variant="destructive"
              disabled={override.trim().length < 3 || update.isPending}
              onClick={() => void save(true)}
            >
              {t("reservations.problems.override")}
            </Button>
          </div>
        ) : null}
        {open ? (
          <CustomerDriversPicker
            customerUuid={reservation.customer.uuid}
            value={drivers}
            onChange={setDrivers}
          />
        ) : null}
      </div>
    </AppDialog>
  );
}

const ASSIGN_FORM_ID = "reservation-assign-form";

/** Staff member responsible for the reservation (empty clears). */
export function AssignDialog({ reservation, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const assign = useAssignReservation();
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("reservations.assign.title")}
      className="sm:max-w-md"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={ASSIGN_FORM_ID}
            disabled={assign.isPending}
          >
            {t("reservations.actions.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<{ user_uuid: string }>
          id={ASSIGN_FORM_ID}
          schema={z.object({ user_uuid: z.string() })}
          defaultValues={{ user_uuid: reservation.assigned_user?.uuid ?? "" }}
          onSubmit={async (values) => {
            await assign.mutateAsync({
              uuid: reservation.uuid,
              userUuid: values.user_uuid || null,
            });
            onOpenChange(false);
          }}
        >
          <MemberPickerField
            name="user_uuid"
            label={t("reservations.fields.assigned_user")}
            description={t("reservations.assign.clear_hint")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
