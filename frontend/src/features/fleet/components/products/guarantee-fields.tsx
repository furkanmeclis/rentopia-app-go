"use client";

import { BadgeCheck, CarFront, Shuffle, X } from "lucide-react";
import { useCallback, useMemo } from "react";
import { Controller, get, useFormContext, useWatch } from "react-hook-form";

import { AppCombobox } from "@/components/forms";
import { FormFieldShell } from "@/components/forms/form-field";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { CatalogCascade } from "@/features/fleet/components/shared/catalog-picker";
import {
  GUARANTEE_EXAMPLE_KEYS,
  GUARANTEE_HINT_KEYS,
  GUARANTEE_LABEL_KEYS,
  GUARANTEE_TYPES,
  VARIANT_LEVEL_LABEL_KEYS,
} from "@/features/fleet/constants";
import {
  type ProductFormValues,
  UUID_RE,
} from "@/features/fleet/schemas/product-form";
import { fleetService } from "@/features/fleet/services/fleet.service";
import type { GuaranteeType } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useVehicle } from "@/features/vehicles/hooks/use-vehicles";
import { vehiclesService } from "@/features/vehicles/services/vehicles.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const ICONS: Record<GuaranteeType, typeof Shuffle> = {
  GROUP: Shuffle,
  EXACT_VARIANT: BadgeCheck,
  EXACT_VEHICLE: CarFront,
};

/** Guarantee level picker: one card per level, explained in plain words. */
function GuaranteeTypeCards() {
  const { t } = useLocale();
  const { control } = useFormContext<ProductFormValues>();
  return (
    <Controller
      control={control}
      name="guarantee.type"
      render={({ field }) => (
        <RadioGroup
          value={field.value}
          onValueChange={field.onChange}
          className="grid gap-3 md:grid-cols-3"
          aria-label={t("fleet.products.fields.guarantee_type")}
        >
          {GUARANTEE_TYPES.map((type) => {
            const Icon = ICONS[type];
            const selected = field.value === type;
            return (
              <Label
                key={type}
                htmlFor={`guarantee-${type}`}
                className={cn(
                  "hover:bg-accent/40 flex cursor-pointer flex-col items-start gap-2 rounded-xl border p-4 font-normal transition-colors",
                  selected &&
                    "border-primary bg-primary/5 ring-primary/30 ring-2",
                )}
              >
                <span className="flex w-full items-center gap-2">
                  <RadioGroupItem value={type} id={`guarantee-${type}`} />
                  <Icon className="size-4" aria-hidden />
                  <span className="font-semibold">
                    {t(GUARANTEE_LABEL_KEYS[type])}
                  </span>
                </span>
                <span className="text-muted-foreground text-sm leading-snug">
                  {t(GUARANTEE_HINT_KEYS[type])}
                </span>
                <Badge variant="outline" className="font-normal">
                  {t(GUARANTEE_EXAMPLE_KEYS[type])}
                </Badge>
              </Label>
            );
          })}
        </RadioGroup>
      )}
    />
  );
}

/** Current catalog pick with a clear button (or a hint when empty). */
function PickedRef({
  label,
  emptyText,
  onClear,
}: {
  label: string;
  emptyText: string;
  onClear: () => void;
}) {
  const { t } = useLocale();
  if (!label) {
    return <p className="text-muted-foreground text-sm">{emptyText}</p>;
  }
  return (
    <div className="flex items-center gap-2">
      <Badge variant="secondary" className="text-sm">
        {label}
      </Badge>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onClear}
        aria-label={t("form.clear")}
      >
        <X className="size-4" />
      </Button>
    </div>
  );
}

/** Conditional reference fields of the selected guarantee type. */
export function GuaranteeFields() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const {
    control,
    setValue,
    formState: { errors },
  } = useFormContext<ProductFormValues>();
  const guarantee = useWatch({ control, name: "guarantee" });

  const loadGroups = useCallback(
    async (q: string) => {
      const page = await fleetService.listGroups(slug, {
        limit: 50,
        offset: 0,
        sort: "code",
        q: q || undefined,
      });
      return page.items.map((group) => ({
        value: group.uuid,
        label: `${group.code} · ${group.name}`,
        description: group.segment.name,
      }));
    },
    [slug],
  );

  // EXACT_VEHICLE: async search over the organization's vehicles (plate),
  // like the other pickers; the saved vehicle is loaded for its label.
  const loadVehicles = useCallback(
    async (q: string) => {
      const page = await vehiclesService.list(slug, {
        limit: 20,
        offset: 0,
        sort: "plate",
        q: q || undefined,
      });
      return page.items.map((v) => ({
        value: v.uuid,
        label: v.plate,
        description: [v.label, v.model_year].filter(Boolean).join(" · "),
      }));
    },
    [slug],
  );
  const vehicleId =
    guarantee.type === "EXACT_VEHICLE" && UUID_RE.test(guarantee.vehicle_id)
      ? guarantee.vehicle_id
      : null;
  const pickedVehicle = useVehicle(vehicleId);
  const pickedVehicleOptions = useMemo(
    () =>
      pickedVehicle.data
        ? [
            {
              value: pickedVehicle.data.uuid,
              label: pickedVehicle.data.plate,
              description: [
                pickedVehicle.data.label,
                pickedVehicle.data.model_year,
              ]
                .filter(Boolean)
                .join(" · "),
            },
          ]
        : undefined,
    [pickedVehicle.data],
  );

  const modelError = get(errors, "guarantee.catalog_model_uuid")?.message as
    string | undefined;
  const typeError = get(errors, "guarantee.catalog_type_uuid")?.message as
    string | undefined;

  return (
    <div className="space-y-5">
      <GuaranteeTypeCards />

      {guarantee.type === "GROUP" ? (
        <div className="space-y-3">
          <AppCombobox
            name="guarantee.rental_group_uuid"
            label={t("fleet.products.fields.rental_group")}
            description={t("fleet.products.fields.rental_group_hint")}
            loadOptions={loadGroups}
            initialOptions={
              guarantee.rental_group_uuid
                ? [
                    {
                      value: guarantee.rental_group_uuid,
                      label: guarantee.rental_group_label,
                    },
                  ]
                : undefined
            }
            placeholder={t("fleet.products.fields.rental_group_placeholder")}
          />
        </div>
      ) : null}

      {guarantee.type === "EXACT_VARIANT" ? (
        <div className="space-y-4">
          <Controller
            control={control}
            name="guarantee.variant_level"
            render={({ field }) => (
              <div className="grid gap-2">
                <Label>{t("fleet.products.fields.variant_level")}</Label>
                <RadioGroup
                  value={field.value}
                  onValueChange={field.onChange}
                  className="flex flex-wrap gap-4"
                >
                  {(["model", "type"] as const).map((level) => (
                    <div key={level} className="flex items-center gap-2">
                      <RadioGroupItem value={level} id={`level-${level}`} />
                      <Label htmlFor={`level-${level}`} className="font-normal">
                        {t(VARIANT_LEVEL_LABEL_KEYS[level])}
                      </Label>
                    </div>
                  ))}
                </RadioGroup>
                <p className="text-muted-foreground text-xs">
                  {guarantee.variant_level === "model"
                    ? t("fleet.guarantee.level_model_hint")
                    : t("fleet.guarantee.level_type_hint")}
                </p>
              </div>
            )}
          />
          <FormFieldShell
            name={
              guarantee.variant_level === "model"
                ? "guarantee.catalog_model_uuid"
                : "guarantee.catalog_type_uuid"
            }
            label={
              guarantee.variant_level === "model"
                ? t("fleet.products.fields.guaranteed_model")
                : t("fleet.products.fields.guaranteed_type")
            }
            error={guarantee.variant_level === "model" ? modelError : typeError}
          >
            <div className="space-y-3">
              <PickedRef
                label={
                  guarantee.variant_level === "model"
                    ? guarantee.catalog_model_label
                    : guarantee.catalog_type_label
                }
                emptyText={t("fleet.products.fields.pick_from_catalog")}
                onClear={() => {
                  if (guarantee.variant_level === "model") {
                    setValue("guarantee.catalog_model_uuid", "", {
                      shouldDirty: true,
                    });
                    setValue("guarantee.catalog_model_label", "");
                  } else {
                    setValue("guarantee.catalog_type_uuid", "", {
                      shouldDirty: true,
                    });
                    setValue("guarantee.catalog_type_label", "");
                  }
                }}
              />
              <CatalogCascade
                allowModel={guarantee.variant_level === "model"}
                allowType={guarantee.variant_level === "type"}
                modelActionLabel={t("fleet.products.fields.use_model")}
                typeActionLabel={t("fleet.products.fields.use_type")}
                onPickModel={(pick) => {
                  setValue("guarantee.catalog_model_uuid", pick.uuid, {
                    shouldDirty: true,
                    shouldValidate: true,
                  });
                  setValue("guarantee.catalog_model_label", pick.label);
                }}
                onPickType={(pick) => {
                  setValue("guarantee.catalog_type_uuid", pick.uuid, {
                    shouldDirty: true,
                    shouldValidate: true,
                  });
                  setValue("guarantee.catalog_type_label", pick.label);
                }}
              />
            </div>
          </FormFieldShell>
        </div>
      ) : null}

      {guarantee.type === "EXACT_VEHICLE" ? (
        <div className="space-y-3">
          <AppCombobox
            name="guarantee.vehicle_id"
            label={t("fleet.products.fields.vehicle_id")}
            description={t("fleet.products.fields.vehicle_id_hint")}
            loadOptions={loadVehicles}
            initialOptions={pickedVehicleOptions}
            placeholder={t("fleet.products.fields.vehicle_placeholder")}
            searchPlaceholder={t("fleet.products.fields.vehicle_search")}
            emptyText={t("fleet.products.fields.vehicle_empty")}
            clearable
          />
          {pickedVehicle.isError && guarantee.vehicle_id ? (
            <Alert>
              <AlertDescription>
                {t("fleet.products.fields.vehicle_missing")}
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** Representative catalog model / type ("X" in "X veya benzeri"). */
export function RepresentativeField() {
  const { t } = useLocale();
  const { control, setValue } = useFormContext<ProductFormValues>();
  const modelLabel = useWatch({ control, name: "representative_model_label" });
  const typeLabel = useWatch({ control, name: "representative_type_label" });

  return (
    <div className="space-y-3 sm:col-span-2">
      <Label>{t("fleet.products.fields.representative")}</Label>
      <p className="text-muted-foreground text-xs">
        {t("fleet.products.fields.representative_hint")}
      </p>
      <div className="flex flex-wrap gap-4">
        <PickedRef
          label={modelLabel}
          emptyText={t("fleet.products.fields.no_representative_model")}
          onClear={() => {
            setValue("representative_model_uuid", "", { shouldDirty: true });
            setValue("representative_model_label", "");
          }}
        />
        {typeLabel ? (
          <PickedRef
            label={typeLabel}
            emptyText=""
            onClear={() => {
              setValue("representative_type_uuid", "", { shouldDirty: true });
              setValue("representative_type_label", "");
            }}
          />
        ) : null}
      </div>
      <CatalogCascade
        modelActionLabel={t("fleet.products.fields.use_model")}
        typeActionLabel={t("fleet.products.fields.use_type")}
        onPickModel={(pick) => {
          setValue("representative_model_uuid", pick.uuid, {
            shouldDirty: true,
          });
          setValue("representative_model_label", pick.label);
        }}
        onPickType={(pick) => {
          setValue("representative_type_uuid", pick.uuid, {
            shouldDirty: true,
          });
          setValue("representative_type_label", pick.label);
        }}
      />
    </div>
  );
}
