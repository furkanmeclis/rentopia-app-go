"use client";

import { format } from "date-fns";
import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  MAINTENANCE_TYPES,
  MAINTENANCE_TYPE_LABEL_KEYS,
} from "@/features/vehicle-records/constants";
import {
  useCompleteVehicleMaintenance,
  useSaveVehicleMaintenance,
} from "@/features/vehicle-records/hooks/use-vehicle-records";
import type {
  MaintenanceType,
  VehicleMaintenance,
} from "@/features/vehicle-records/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "vehicle-maintenance-form";
const COMPLETE_FORM_ID = "vehicle-maintenance-complete-form";
const MONEY_RE = /^\d{1,12}([.,]\d{1,2})?$/;

function today() {
  return format(new Date(), "yyyy-MM-dd");
}

function useFieldRules() {
  const { t } = useLocale();
  return useMemo(
    () => ({
      km: z
        .string()
        .trim()
        .refine((v) => {
          if (v === "") return true;
          const n = Number(v);
          return Number.isInteger(n) && n >= 0 && n <= 5_000_000;
        }, t("vehicle_records.validation.km")),
      cost: z
        .string()
        .trim()
        .refine(
          (v) => v === "" || MONEY_RE.test(v),
          t("vehicle_records.validation.cost"),
        ),
      notFuture: (v: string) => !v || v <= today(),
    }),
    [t],
  );
}

const kmOrNull = (v: string) => (v.trim() === "" ? null : Number(v));
const costOrNull = (v: string) =>
  v.trim() === "" ? null : v.trim().replace(",", ".");

export type MaintenanceDialogMode =
  | { kind: "planned" | "done"; item: null }
  | { kind: "planned" | "done"; item: VehicleMaintenance };

/**
 * Plan a maintenance (date and / or km target), record a done one, or edit
 * an item (its kind stays; a plan is completed with the complete dialog).
 */
export function MaintenanceDialog({
  vehicleUuid,
  mode,
  onOpenChange,
}: {
  vehicleUuid: string;
  mode: MaintenanceDialogMode | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveVehicleMaintenance(vehicleUuid);
  const rules = useFieldRules();
  const kind = mode?.kind ?? "planned";
  const item = mode?.item ?? null;

  const schema = useMemo(
    () =>
      z
        .object({
          maintenance_type: z
            .string()
            .min(1, t("vehicle_records.validation.required")),
          planned_date: z.string(),
          planned_km: rules.km,
          done_date: z.string(),
          done_km: rules.km,
          service_provider: z.string().max(255),
          cost: rules.cost,
          note: z.string().max(2000),
        })
        .superRefine((v, ctx) => {
          if (kind === "planned" && !v.planned_date && !v.planned_km) {
            ctx.addIssue({
              code: "custom",
              path: ["planned_date"],
              message: t("vehicle_records.validation.plan_target"),
            });
          }
          if (kind === "done" && !v.done_date) {
            ctx.addIssue({
              code: "custom",
              path: ["done_date"],
              message: t("vehicle_records.validation.required"),
            });
          }
          if (kind === "done" && !rules.notFuture(v.done_date)) {
            ctx.addIssue({
              code: "custom",
              path: ["done_date"],
              message: t("vehicle_records.validation.not_future"),
            });
          }
        }),
    [kind, rules, t],
  );
  type Values = z.infer<typeof schema>;

  const title = item
    ? t("vehicle_records.maintenance.edit_title")
    : kind === "planned"
      ? t("vehicle_records.maintenance.plan_title")
      : t("vehicle_records.maintenance.record_title");

  return (
    <AppDialog
      open={Boolean(mode)}
      onOpenChange={(open) => {
        if (!save.isPending) onOpenChange(open);
      }}
      title={title}
      description={
        kind === "planned"
          ? t("vehicle_records.maintenance.plan_description")
          : t("vehicle_records.maintenance.record_description")
      }
      className="sm:max-w-2xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {save.isPending
              ? t("vehicle_records.saving")
              : t("vehicle_records.save")}
          </Button>
        </>
      }
    >
      {mode ? (
        <AppForm<Values>
          key={`${kind}-${item?.uuid ?? "new"}`}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            maintenance_type: item?.maintenance_type ?? "periodic",
            planned_date: item?.planned_date ?? "",
            planned_km: item?.planned_km != null ? String(item.planned_km) : "",
            done_date: item?.done_date ?? (kind === "done" ? today() : ""),
            done_km: item?.done_km != null ? String(item.done_km) : "",
            service_provider: item?.service_provider ?? "",
            cost: item?.cost ?? "",
            note: item?.note ?? "",
          }}
          onSubmit={async (values) => {
            const common = {
              maintenance_type: values.maintenance_type as MaintenanceType,
              service_provider: values.service_provider.trim(),
              cost: costOrNull(values.cost),
              note: values.note.trim(),
              planned_date: values.planned_date || null,
              planned_km: kmOrNull(values.planned_km),
            };
            const body =
              kind === "done"
                ? {
                    ...common,
                    done_date: values.done_date || null,
                    done_km: kmOrNull(values.done_km),
                  }
                : common;
            await save.mutateAsync(
              item
                ? { itemUuid: item.uuid, body }
                : { body: { ...body, kind } },
            );
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="maintenance_type"
            label={t("vehicle_records.fields.maintenance_type")}
            className="sm:col-span-2"
            options={MAINTENANCE_TYPES.map((value) => ({
              value,
              label: t(MAINTENANCE_TYPE_LABEL_KEYS[value]),
            }))}
          />
          <AppDatePicker
            name="planned_date"
            label={t("vehicle_records.fields.planned_date")}
          />
          <AppInput
            name="planned_km"
            type="number"
            min={0}
            label={t("vehicle_records.fields.planned_km")}
          />
          {kind === "done" ? (
            <>
              <AppDatePicker
                name="done_date"
                label={t("vehicle_records.fields.done_date")}
              />
              <AppInput
                name="done_km"
                type="number"
                min={0}
                label={t("vehicle_records.fields.done_km")}
              />
            </>
          ) : null}
          <AppInput
            name="service_provider"
            label={t("vehicle_records.fields.service_provider")}
          />
          <AppInput
            name="cost"
            inputMode="decimal"
            label={t("vehicle_records.fields.cost")}
            description={t("vehicle_records.fields.cost_hint")}
          />
          <AppTextarea
            name="note"
            label={t("vehicle_records.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** Complete a plan: done date (today by default), km, provider, cost. */
export function CompleteMaintenanceDialog({
  vehicleUuid,
  item,
  onOpenChange,
}: {
  vehicleUuid: string;
  item: VehicleMaintenance | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const complete = useCompleteVehicleMaintenance(vehicleUuid);
  const rules = useFieldRules();
  const schema = useMemo(
    () =>
      z.object({
        done_date: z
          .string()
          .min(1, t("vehicle_records.validation.required"))
          .refine(rules.notFuture, t("vehicle_records.validation.not_future")),
        done_km: rules.km,
        service_provider: z.string().max(255),
        cost: rules.cost,
        note: z.string().max(2000),
      }),
    [rules, t],
  );
  type Values = z.infer<typeof schema>;

  return (
    <AppDialog
      open={Boolean(item)}
      onOpenChange={(open) => {
        if (!complete.isPending) onOpenChange(open);
      }}
      title={t("vehicle_records.maintenance.complete_title")}
      description={t("vehicle_records.maintenance.complete_description")}
      className="sm:max-w-lg"
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
            form={COMPLETE_FORM_ID}
            disabled={complete.isPending}
          >
            {t("vehicle_records.maintenance.complete")}
          </Button>
        </>
      }
    >
      {item ? (
        <AppForm<Values>
          key={item.uuid}
          id={COMPLETE_FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            done_date: today(),
            done_km: item.planned_km != null ? String(item.planned_km) : "",
            service_provider: item.service_provider,
            cost: item.cost ?? "",
            note: item.note,
          }}
          onSubmit={async (values) => {
            await complete.mutateAsync({
              itemUuid: item.uuid,
              body: {
                done_date: values.done_date,
                done_km: kmOrNull(values.done_km),
                service_provider: values.service_provider.trim(),
                cost: costOrNull(values.cost),
                note: values.note.trim(),
              },
            });
            onOpenChange(false);
          }}
        >
          <AppDatePicker
            name="done_date"
            label={t("vehicle_records.fields.done_date")}
          />
          <AppInput
            name="done_km"
            type="number"
            min={0}
            label={t("vehicle_records.fields.done_km")}
          />
          <AppInput
            name="service_provider"
            label={t("vehicle_records.fields.service_provider")}
          />
          <AppInput
            name="cost"
            inputMode="decimal"
            label={t("vehicle_records.fields.cost")}
          />
          <AppTextarea
            name="note"
            label={t("vehicle_records.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
