"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSelect, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  STATUS_LABEL_KEYS,
  VEHICLE_STATUSES,
} from "@/features/vehicles/constants";
import { useChangeVehicleStatus } from "@/features/vehicles/hooks/use-vehicles";
import type { Vehicle, VehicleStatus } from "@/features/vehicles/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "vehicle-status-form";

function intField(t: (key: string) => string, max: number, key: string) {
  return z
    .string()
    .trim()
    .refine((v) => {
      if (v === "") return true;
      const n = Number(v);
      return Number.isInteger(n) && n >= 0 && n <= max;
    }, t(key));
}

/**
 * Status change with the km / fuel reading and a note. Every change is
 * recorded in the status history (who, when, from, to).
 */
export function VehicleStatusDialog({
  vehicle,
  onOpenChange,
}: {
  vehicle: Vehicle | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const change = useChangeVehicleStatus();
  const schema = useMemo(
    () =>
      z.object({
        status: z.string().min(1),
        odometer_km: intField(t, 5_000_000, "vehicles.validation.km"),
        fuel_level: intField(t, 100, "vehicles.validation.fuel_level"),
        note: z.string().max(2000),
      }),
    [t],
  );
  type Values = z.infer<typeof schema>;

  return (
    <AppDialog
      open={Boolean(vehicle)}
      onOpenChange={onOpenChange}
      title={t("vehicles.status_dialog.title")}
      description={
        vehicle
          ? t("vehicles.status_dialog.description", { plate: vehicle.plate })
          : undefined
      }
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
          <Button type="submit" form={FORM_ID} disabled={change.isPending}>
            {t("vehicles.status_dialog.submit")}
          </Button>
        </>
      }
    >
      {vehicle ? (
        <AppForm<Values>
          key={vehicle.uuid}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            status: vehicle.status,
            odometer_km: String(vehicle.odometer_km),
            fuel_level:
              vehicle.fuel_level != null ? String(vehicle.fuel_level) : "",
            note: "",
          }}
          onSubmit={async (values) => {
            await change.mutateAsync({
              uuid: vehicle.uuid,
              body: {
                status: values.status as VehicleStatus,
                odometer_km:
                  values.odometer_km === ""
                    ? undefined
                    : Number(values.odometer_km),
                fuel_level:
                  values.fuel_level === "" ? null : Number(values.fuel_level),
                note: values.note.trim(),
              },
            });
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="status"
            label={t("vehicles.fields.status")}
            className="sm:col-span-2"
            options={VEHICLE_STATUSES.map((value) => ({
              value,
              label: t(STATUS_LABEL_KEYS[value]),
            }))}
          />
          <AppInput
            name="odometer_km"
            type="number"
            min={0}
            label={t("vehicles.fields.odometer_km")}
          />
          <AppInput
            name="fuel_level"
            type="number"
            min={0}
            max={100}
            label={t("vehicles.fields.fuel_level")}
          />
          <AppTextarea
            name="note"
            label={t("vehicles.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
