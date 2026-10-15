"use client";

import { AlertTriangle } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCheckbox,
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  type ComboboxOption,
} from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  isoToLocalInput,
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import {
  CHARGE_KINDS,
  CHARGE_KIND_LABEL_KEYS,
} from "@/features/rentals/constants";
import {
  useAddCharge,
  useCompleteReturn,
  useExtendRental,
  useHandover,
  useReturnRental,
  useSwapVehicle,
} from "@/features/rentals/hooks/use-rentals";
import { rentalProblemText } from "@/features/rentals/lib/errors";
import type { ChargeKind, Rental } from "@/features/rentals/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { vehiclesService } from "@/features/vehicles/services/vehicles.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type DialogProps = {
  rental: Rental;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function Problem({ text }: { text: string }) {
  const { t } = useLocale();
  return (
    <Alert variant="destructive">
      <AlertTriangle className="size-4" />
      <AlertTitle>{t("rentals.problems.title")}</AlertTitle>
      <AlertDescription>{text}</AlertDescription>
    </Alert>
  );
}

function DialogFooter({
  formId,
  label,
  pending,
  onCancel,
  destructive = false,
}: {
  formId: string;
  label: string;
  pending: boolean;
  onCancel: () => void;
  destructive?: boolean;
}) {
  const { t } = useLocale();
  return (
    <>
      <Button type="button" variant="outline" onClick={onCancel}>
        {t("form.cancel")}
      </Button>
      <Button
        type="submit"
        form={formId}
        variant={destructive ? "destructive" : "default"}
        disabled={pending}
      >
        {label}
      </Button>
    </>
  );
}

/** Branch options of the organization (code · name). */
function useBranchOptions(open: boolean) {
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );
  return (branches.data?.items ?? []).map((b) => ({
    value: b.uuid,
    label: `${b.code} · ${b.name}`,
  }));
}

/** Last known odometer of the rental's current vehicle, if any. */
function lastKm(rental: Rental): number | null {
  const legs = rental.legs;
  const leg = legs[legs.length - 1];
  if (!leg) return null;
  return leg.in_km ?? leg.out_km ?? null;
}

const kmSchema = (t: (key: string) => string) =>
  z.string().trim().regex(/^\d+$/, t("rentals.validation.km"));
const fuelSchema = (t: (key: string) => string) =>
  z
    .string()
    .trim()
    .regex(/^\d+$/, t("rentals.validation.fuel"))
    .refine((v) => Number(v) <= 100, t("rentals.validation.fuel"));

// ------------------------------------------------------------ handover

const HANDOVER_FORM_ID = "rental-handover-form";

type HandoverValues = {
  at: string;
  branch_uuid: string;
  km: string;
  fuel: string;
  note: string;
  waiver_reason: string;
};

/**
 * "Teslim et": km / fuel out, the actual time and branch. Without an
 * approved rental contract the handover needs `signature.waive` and a
 * reason (recorded on the rental).
 */
export function HandoverDialog({ rental, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const handover = useHandover(rental.uuid);
  const branchOptions = useBranchOptions(open);
  const [problem, setProblem] = useState<string | null>(null);
  const needsWaiver = rental.contract.required && !rental.contract.approved;
  const canWaive = can(permissions.tenant.signature.waive);
  const schema = useMemo(
    () =>
      z
        .object({
          at: z.string().min(1, t("rentals.validation.required")),
          branch_uuid: z.string().min(1, t("rentals.validation.required")),
          km: kmSchema(t),
          fuel: fuelSchema(t),
          note: z.string().max(1000),
          waiver_reason: z.string().max(500),
        })
        .refine((v) => !needsWaiver || v.waiver_reason.trim().length >= 3, {
          path: ["waiver_reason"],
          message: t("rentals.validation.reason"),
        }),
    [needsWaiver, t],
  );
  const close = () => {
    setProblem(null);
    onOpenChange(false);
  };
  const km = lastKm(rental);
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("rentals.handover.title")}
      description={t("rentals.handover.description", {
        plate: rental.vehicle.plate,
      })}
      className="sm:max-w-2xl"
      footer={
        <DialogFooter
          formId={HANDOVER_FORM_ID}
          label={t("rentals.actions.handover")}
          pending={handover.isPending || (needsWaiver && !canWaive)}
          onCancel={close}
        />
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        {needsWaiver ? (
          <Alert variant={canWaive ? "default" : "destructive"}>
            <AlertTriangle className="size-4" />
            <AlertTitle>{t("rentals.handover.contract_missing")}</AlertTitle>
            <AlertDescription>
              {canWaive
                ? t("rentals.handover.contract_waive_hint")
                : t("rentals.handover.contract_no_permission")}
            </AlertDescription>
          </Alert>
        ) : null}
        {problem ? <Problem text={problem} /> : null}
        {open ? (
          <AppForm<HandoverValues>
            id={HANDOVER_FORM_ID}
            className="grid gap-4 sm:grid-cols-2"
            schema={schema}
            defaultValues={{
              at: localInputFromNow(0),
              branch_uuid: rental.planned_pickup.branch?.uuid ?? "",
              km: km !== null ? String(km) : "",
              fuel: "100",
              note: "",
              waiver_reason: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await handover.mutateAsync({
                  at: localInputToISO(values.at) ?? null,
                  branch_uuid: values.branch_uuid || null,
                  km: Number(values.km),
                  fuel: Number(values.fuel),
                  note: values.note.trim() || undefined,
                  waiver_reason: needsWaiver
                    ? values.waiver_reason.trim()
                    : undefined,
                });
                close();
              } catch (error) {
                setProblem(rentalProblemText(t, error));
              }
            }}
          >
            <AppDatePicker
              name="at"
              withTime
              label={t("rentals.fields.handover_at")}
            />
            <AppSelect
              name="branch_uuid"
              label={t("rentals.fields.handover_branch")}
              options={branchOptions}
            />
            <AppInput
              name="km"
              type="number"
              min={0}
              label={t("rentals.fields.km_out")}
              description={t("rentals.fields.km_hint")}
            />
            <AppInput
              name="fuel"
              type="number"
              min={0}
              max={100}
              label={t("rentals.fields.fuel_out")}
              description={t("rentals.fields.fuel_hint")}
            />
            <AppTextarea
              name="note"
              rows={2}
              label={t("rentals.fields.note")}
              className="sm:col-span-2"
            />
            {needsWaiver && canWaive ? (
              <AppTextarea
                name="waiver_reason"
                rows={2}
                label={t("rentals.fields.waiver_reason")}
                className="sm:col-span-2"
              />
            ) : null}
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

// ------------------------------------------------------------ return

const RETURN_FORM_ID = "rental-return-form";

type ReturnValues = {
  at: string;
  branch_uuid: string;
  km: string;
  fuel: string;
  note: string;
  damage_note: string;
};

/**
 * "İade al": km / fuel in, the actual time and branch. A late return
 * proposes the policy's late fee; an early return releases the rest of
 * the plan.
 */
export function ReturnDialog({ rental, open, onOpenChange }: DialogProps) {
  const { t, locale } = useLocale();
  const ret = useReturnRental(rental.uuid);
  const branchOptions = useBranchOptions(open);
  const [problem, setProblem] = useState<string | null>(null);
  const minKm = lastKm(rental) ?? 0;
  const schema = useMemo(
    () =>
      z
        .object({
          at: z.string().min(1, t("rentals.validation.required")),
          branch_uuid: z.string().min(1, t("rentals.validation.required")),
          km: kmSchema(t),
          fuel: fuelSchema(t),
          note: z.string().max(1000),
          damage_note: z.string().max(2000),
        })
        .refine((v) => Number(v.km) >= minKm, {
          path: ["km"],
          message: t("rentals.validation.km_lower"),
        }),
    [minKm, t],
  );
  const close = () => {
    setProblem(null);
    onOpenChange(false);
  };
  const grace = rental.policy.late_return_grace_minutes;
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("rentals.return.title")}
      description={t("rentals.return.description", {
        time: isoToLocalInput(rental.planned_return_at).replace("T", " "),
      })}
      className="sm:max-w-2xl"
      footer={
        <DialogFooter
          formId={RETURN_FORM_ID}
          label={t("rentals.actions.return")}
          pending={ret.isPending}
          onCancel={close}
        />
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        <p className="text-muted-foreground text-sm">
          {grace !== null && rental.policy.extra_hour_fee
            ? t("rentals.return.policy_hint", {
                grace,
                fee: rental.policy.extra_hour_fee,
              })
            : t("rentals.return.policy_none")}
        </p>
        {problem ? <Problem text={problem} /> : null}
        {open ? (
          <AppForm<ReturnValues>
            id={RETURN_FORM_ID}
            className="grid gap-4 sm:grid-cols-2"
            schema={schema}
            defaultValues={{
              at: localInputFromNow(0),
              branch_uuid: rental.planned_return.branch?.uuid ?? "",
              km: minKm ? String(minKm) : "",
              fuel: "100",
              note: "",
              damage_note: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await ret.mutateAsync({
                  at: localInputToISO(values.at) ?? null,
                  branch_uuid: values.branch_uuid || null,
                  km: Number(values.km),
                  fuel: Number(values.fuel),
                  note: values.note.trim() || undefined,
                  damage_note: values.damage_note.trim() || undefined,
                });
                close();
              } catch (error) {
                setProblem(rentalProblemText(t, error));
              }
            }}
          >
            <AppDatePicker
              name="at"
              withTime
              label={t("rentals.fields.return_at")}
            />
            <AppSelect
              name="branch_uuid"
              label={t("rentals.fields.return_branch")}
              options={branchOptions}
            />
            <AppInput
              name="km"
              type="number"
              min={minKm}
              label={t("rentals.fields.km_in")}
              description={
                minKm
                  ? t("rentals.fields.km_min", {
                      km: minKm.toLocaleString(locale),
                    })
                  : undefined
              }
            />
            <AppInput
              name="fuel"
              type="number"
              min={0}
              max={100}
              label={t("rentals.fields.fuel_in")}
              description={t("rentals.fields.fuel_hint")}
            />
            <AppTextarea
              name="note"
              rows={2}
              label={t("rentals.fields.return_note")}
              className="sm:col-span-2"
            />
            <AppTextarea
              name="damage_note"
              rows={2}
              label={t("rentals.fields.damage_note")}
              className="sm:col-span-2"
            />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

// ------------------------------------------------------- complete return

const CHECK_FORM_ID = "rental-check-form";

/**
 * "İade kontrolünü tamamla": closes the rental; the vehicle goes to
 * cleaning (a cleaning task opens) or straight back to available.
 */
export function CompleteReturnDialog({
  rental,
  open,
  onOpenChange,
}: DialogProps) {
  const { t } = useLocale();
  const complete = useCompleteReturn(rental.uuid);
  const proposed = rental.charges.filter((c) => c.status === "proposed").length;
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !complete.isPending) onOpenChange(false);
      }}
      title={t("rentals.check.title")}
      description={t("rentals.check.description")}
      className="sm:max-w-md"
      footer={
        <DialogFooter
          formId={CHECK_FORM_ID}
          label={t("rentals.actions.complete_return")}
          pending={complete.isPending}
          onCancel={() => onOpenChange(false)}
        />
      }
    >
      <div className="space-y-4">
        {proposed > 0 ? (
          <Alert>
            <AlertTriangle className="size-4" />
            <AlertDescription>
              {t("rentals.check.open_charges", { count: proposed })}
            </AlertDescription>
          </Alert>
        ) : null}
        {open ? (
          <AppForm<{
            vehicle_status:
              "cleaning" | "available" | "maintenance" | "preparing";
            note: string;
            deposit_task: boolean;
          }>
            id={CHECK_FORM_ID}
            className="grid gap-4"
            schema={z.object({
              vehicle_status: z.enum([
                "cleaning",
                "available",
                "maintenance",
                "preparing",
              ]),
              note: z.string().max(1000),
              deposit_task: z.boolean(),
            })}
            defaultValues={{
              vehicle_status: "cleaning",
              note: "",
              deposit_task: false,
            }}
            onSubmit={async (values) => {
              await complete.mutateAsync({
                vehicle_status: values.vehicle_status,
                note: values.note.trim() || undefined,
                deposit_task: values.deposit_task || undefined,
              });
              onOpenChange(false);
            }}
          >
            <AppSelect
              name="vehicle_status"
              label={t("rentals.check.vehicle_status")}
              options={[
                {
                  value: "cleaning",
                  label: t("rentals.check.status_cleaning"),
                },
                {
                  value: "available",
                  label: t("rentals.check.status_available"),
                },
                {
                  value: "maintenance",
                  label: t("rentals.check.status_maintenance"),
                },
                {
                  value: "preparing",
                  label: t("rentals.check.status_preparing"),
                },
              ]}
            />
            <AppCheckbox
              name="deposit_task"
              label={t("rentals.check.deposit_task")}
            />
            <AppTextarea
              name="note"
              rows={2}
              label={t("rentals.fields.note")}
            />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

// ------------------------------------------------------------ extend

const EXTEND_FORM_ID = "rental-extend-form";

/**
 * "Uzat": a later return through the reservation's extension rule (price
 * difference, same vehicle); a conflict answers with alternatives on the
 * reservation.
 */
export function RentalExtendDialog({
  rental,
  open,
  onOpenChange,
}: DialogProps) {
  const { t } = useLocale();
  const extend = useExtendRental(rental.uuid);
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z
        .object({
          ends_at: z.string().min(1, t("rentals.validation.required")),
          reason: z.string().max(500),
        })
        .refine(
          (v) => new Date(v.ends_at) > new Date(rental.planned_return_at),
          {
            path: ["ends_at"],
            message: t("rentals.validation.extend_later"),
          },
        ),
    [rental.planned_return_at, t],
  );
  const close = () => {
    setProblem(null);
    onOpenChange(false);
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("rentals.extend.title")}
      description={t("rentals.extend.description")}
      className="sm:max-w-lg"
      footer={
        <DialogFooter
          formId={EXTEND_FORM_ID}
          label={t("rentals.actions.extend")}
          pending={extend.isPending}
          onCancel={close}
        />
      }
    >
      <div className="space-y-4">
        {problem ? <Problem text={problem} /> : null}
        {open ? (
          <AppForm<{ ends_at: string; reason: string }>
            id={EXTEND_FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              ends_at: isoToLocalInput(rental.planned_return_at),
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
                setProblem(rentalProblemText(t, error));
              }
            }}
          >
            <AppDatePicker
              name="ends_at"
              withTime
              label={t("rentals.extend.new_end")}
            />
            <AppInput name="reason" label={t("rentals.fields.reason")} />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

// ------------------------------------------------------------ swap

const SWAP_FORM_ID = "rental-swap-form";

type SwapValues = {
  vehicle_uuid: string;
  at: string;
  branch_uuid: string;
  in_km: string;
  in_fuel: string;
  out_km: string;
  out_fuel: string;
  reason: string;
};

/**
 * "Araç değiştir": the current vehicle comes back (km / fuel in, cleaning
 * task) and the new one goes out (km / fuel out) from the swap time; the
 * engine moves the rest of the rental to the new vehicle.
 */
export function SwapDialog({ rental, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const swap = useSwapVehicle(rental.uuid);
  const branchOptions = useBranchOptions(open);
  const [problem, setProblem] = useState<string | null>(null);
  const minKm = lastKm(rental) ?? 0;
  const schema = useMemo(
    () =>
      z
        .object({
          vehicle_uuid: z.string().min(1, t("rentals.validation.required")),
          at: z.string().min(1, t("rentals.validation.required")),
          branch_uuid: z.string().min(1, t("rentals.validation.required")),
          in_km: kmSchema(t),
          in_fuel: fuelSchema(t),
          out_km: kmSchema(t),
          out_fuel: fuelSchema(t),
          reason: z
            .string()
            .trim()
            .min(3, t("rentals.validation.reason"))
            .max(500, t("rentals.validation.reason")),
        })
        .refine((v) => Number(v.in_km) >= minKm, {
          path: ["in_km"],
          message: t("rentals.validation.km_lower"),
        })
        .refine((v) => v.vehicle_uuid !== rental.vehicle.uuid, {
          path: ["vehicle_uuid"],
          message: t("rentals.validation.same_vehicle"),
        }),
    [minKm, rental.vehicle.uuid, t],
  );
  const loadVehicles = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await vehiclesService.list(slug, {
        limit: 20,
        offset: 0,
        sort: "plate",
        q: q || undefined,
        is_active: "true",
      });
      return page.items
        .filter((v) => v.uuid !== rental.vehicle.uuid)
        .map((v) => ({
          value: v.uuid,
          label: v.plate,
          description: `${v.label} · ${v.current_branch.name}`,
        }));
    },
    [rental.vehicle.uuid, slug],
  );
  const close = () => {
    setProblem(null);
    onOpenChange(false);
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("rentals.swap.title")}
      description={t("rentals.swap.description", {
        plate: rental.vehicle.plate,
      })}
      className="sm:max-w-2xl"
      footer={
        <DialogFooter
          formId={SWAP_FORM_ID}
          label={t("rentals.actions.swap")}
          pending={swap.isPending}
          onCancel={close}
        />
      }
    >
      <div className="max-h-[70vh] space-y-4 overflow-y-auto">
        {problem ? <Problem text={problem} /> : null}
        {open ? (
          <AppForm<SwapValues>
            id={SWAP_FORM_ID}
            className="grid gap-4 sm:grid-cols-2"
            schema={schema}
            defaultValues={{
              vehicle_uuid: "",
              at: localInputFromNow(0),
              branch_uuid:
                rental.actual_pickup.branch?.uuid ??
                rental.planned_pickup.branch?.uuid ??
                "",
              in_km: minKm ? String(minKm) : "",
              in_fuel: "",
              out_km: "",
              out_fuel: "100",
              reason: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await swap.mutateAsync({
                  vehicle_uuid: values.vehicle_uuid,
                  at: localInputToISO(values.at) ?? null,
                  branch_uuid: values.branch_uuid || null,
                  in_km: Number(values.in_km),
                  in_fuel: Number(values.in_fuel),
                  out_km: Number(values.out_km),
                  out_fuel: Number(values.out_fuel),
                  reason: values.reason.trim(),
                });
                close();
              } catch (error) {
                setProblem(rentalProblemText(t, error));
              }
            }}
          >
            <AppCombobox
              name="vehicle_uuid"
              label={t("rentals.swap.new_vehicle")}
              className="sm:col-span-2"
              loadOptions={loadVehicles}
              placeholder={t("rentals.swap.vehicle_placeholder")}
              searchPlaceholder={t("rentals.swap.vehicle_search")}
              emptyText={t("rentals.swap.vehicle_empty")}
            />
            <AppDatePicker
              name="at"
              withTime
              label={t("rentals.fields.swap_at")}
            />
            <AppSelect
              name="branch_uuid"
              label={t("rentals.fields.swap_branch")}
              options={branchOptions}
            />
            <AppInput
              name="in_km"
              type="number"
              min={minKm}
              label={t("rentals.swap.in_km", { plate: rental.vehicle.plate })}
            />
            <AppInput
              name="in_fuel"
              type="number"
              min={0}
              max={100}
              label={t("rentals.swap.in_fuel", {
                plate: rental.vehicle.plate,
              })}
            />
            <AppInput
              name="out_km"
              type="number"
              min={0}
              label={t("rentals.swap.out_km")}
            />
            <AppInput
              name="out_fuel"
              type="number"
              min={0}
              max={100}
              label={t("rentals.swap.out_fuel")}
            />
            <AppTextarea
              name="reason"
              rows={2}
              label={t("rentals.fields.reason")}
              className="sm:col-span-2"
            />
          </AppForm>
        ) : null}
      </div>
    </AppDialog>
  );
}

// ------------------------------------------------------------ charges

const CHARGE_FORM_ID = "rental-charge-form";

/**
 * "Ücret öner": a manual charge (damage, fuel, mileage, other) proposed
 * for approval; damage charges are approved with `damage.approve`.
 */
export function ChargeDialog({ rental, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const add = useAddCharge(rental.uuid);
  const schema = useMemo(
    () =>
      z.object({
        kind: z.enum(CHARGE_KINDS as [ChargeKind, ...ChargeKind[]]),
        amount: z
          .string()
          .trim()
          .regex(/^\d+(\.\d{1,2})?$/, t("rentals.validation.amount"))
          .refine((v) => Number(v) > 0, t("rentals.validation.amount")),
        reason: z
          .string()
          .trim()
          .min(3, t("rentals.validation.reason"))
          .max(500, t("rentals.validation.reason")),
      }),
    [t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !add.isPending) onOpenChange(false);
      }}
      title={t("rentals.charges.add_title")}
      description={t("rentals.charges.add_description", {
        currency: rental.reservation.currency,
      })}
      className="sm:max-w-md"
      footer={
        <DialogFooter
          formId={CHARGE_FORM_ID}
          label={t("rentals.actions.propose_charge")}
          pending={add.isPending}
          onCancel={() => onOpenChange(false)}
        />
      }
    >
      {open ? (
        <AppForm<{ kind: ChargeKind; amount: string; reason: string }>
          id={CHARGE_FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ kind: "damage", amount: "", reason: "" }}
          onSubmit={async (values) => {
            await add.mutateAsync({
              kind: values.kind,
              amount: values.amount.trim(),
              reason: values.reason.trim(),
            });
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="kind"
            label={t("rentals.charges.columns.kind")}
            options={CHARGE_KINDS.filter((k) => k !== "late_return").map(
              (k) => ({ value: k, label: t(CHARGE_KIND_LABEL_KEYS[k]) }),
            )}
          />
          <AppInput
            name="amount"
            inputMode="decimal"
            label={t("rentals.charges.columns.amount")}
            description={t("rentals.charges.amount_hint")}
          />
          <AppTextarea
            name="reason"
            rows={2}
            label={t("rentals.charges.columns.reason")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
