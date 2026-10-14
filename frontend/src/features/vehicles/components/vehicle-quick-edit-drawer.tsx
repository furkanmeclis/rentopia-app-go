"use client";

import Link from "next/link";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSelect, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useUpdateVehicle } from "@/features/vehicles/hooks/use-vehicles";
import type { Vehicle } from "@/features/vehicles/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "vehicle-quick-edit";

/** Drawer for the plate, colour, current branch, pool and active flag. */
export function VehicleQuickEditDrawer({
  vehicle,
  onOpenChange,
}: {
  vehicle: Vehicle | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateVehicle();
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const schema = useMemo(
    () =>
      z.object({
        plate: z
          .string()
          .trim()
          .min(1, t("vehicles.validation.plate_required"))
          .regex(
            /^[A-Za-z0-9][A-Za-z0-9 -]{0,18}[A-Za-z0-9]$/,
            t("vehicles.validation.plate_format"),
          ),
        color: z.string().max(64),
        current_branch_uuid: z.string().min(1),
        fleet_pool: z
          .string()
          .trim()
          .regex(
            /^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$/,
            t("vehicles.validation.pool_format"),
          ),
        is_active: z.boolean(),
      }),
    [t],
  );
  type Values = z.infer<typeof schema>;
  const branchOptions = (branches.data?.items ?? []).map((b) => ({
    value: b.uuid,
    label: `${b.code} · ${b.name}`,
  }));
  if (
    vehicle &&
    !branchOptions.some((o) => o.value === vehicle.current_branch.uuid)
  ) {
    branchOptions.push({
      value: vehicle.current_branch.uuid,
      label: `${vehicle.current_branch.code} · ${vehicle.current_branch.name}`,
    });
  }

  return (
    <EntityDrawer
      open={Boolean(vehicle)}
      onOpenChange={onOpenChange}
      title={t("vehicles.quick_edit.title")}
      description={
        vehicle
          ? t("vehicles.quick_edit.description", {
              plate: vehicle.plate,
              label: vehicle.label,
            })
          : undefined
      }
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {vehicle ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link
                href={routes.tenant.fleet.vehicles.edit(slug, vehicle.uuid)}
              >
                {t("vehicles.quick_edit.full_form")}
              </Link>
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={update.isPending}>
              {t("vehicles.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {vehicle ? (
        <AppForm<Values>
          key={vehicle.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            plate: vehicle.plate,
            color: vehicle.color,
            current_branch_uuid: vehicle.current_branch.uuid,
            fleet_pool: vehicle.fleet_pool,
            is_active: vehicle.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: vehicle.uuid,
              body: {
                plate: values.plate.trim().toUpperCase(),
                color: values.color.trim(),
                current_branch_uuid: values.current_branch_uuid,
                fleet_pool: values.fleet_pool.trim().toUpperCase(),
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput
            name="plate"
            label={t("vehicles.fields.plate")}
            autoCapitalize="characters"
            maxLength={20}
          />
          <AppInput
            name="color"
            label={t("vehicles.fields.color")}
            maxLength={64}
          />
          <AppSelect
            name="current_branch_uuid"
            label={t("vehicles.fields.current_branch")}
            options={branchOptions}
          />
          <AppInput
            name="fleet_pool"
            label={t("vehicles.fields.fleet_pool")}
            autoCapitalize="characters"
            maxLength={32}
          />
          <AppSwitch name="is_active" label={t("vehicles.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
