"use client";

import { Building2, CarFront, Gauge, ShieldCheck } from "lucide-react";
import { useCallback, useMemo, useRef } from "react";
import { get, useFormContext, useWatch } from "react-hook-form";

import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  FormActions,
  FormFieldShell,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useSupplierOptions } from "@/features/suppliers/hooks/use-suppliers";
import {
  BODY_TYPES,
  BODY_TYPE_LABEL_KEYS,
  FUELS,
  FUEL_LABEL_KEYS,
  TRANSMISSIONS,
  TRANSMISSION_LABEL_KEYS,
} from "@/features/fleet/constants";
import { fleetService } from "@/features/fleet/services/fleet.service";
import type { TenantCatalogType } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  OWNERSHIPS,
  OWNERSHIP_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  VEHICLE_STATUSES,
} from "@/features/vehicles/constants";
import {
  NONE,
  createVehicleFormSchema,
  type VehicleFormValues,
} from "@/features/vehicles/schemas/vehicle-form";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type VehicleFormProps = {
  mode: "create" | "edit";
  defaultValues: VehicleFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: VehicleFormValues) => Promise<void>;
  onCancel: () => void;
};

const FALLBACK_YEARS = (() => {
  const last = new Date().getFullYear() + 1;
  return Array.from({ length: last - 1989 }, (_, i) => last - i);
})();

/**
 * Brand → model → type cascade over the read-only tenant catalog (same
 * lookups as the rental group criteria editor), bound to the form. Picking a
 * type restricts the model year to its valid years and fills the
 * transmission / fuel the catalog knows when they are still empty.
 */
function CatalogFields() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const {
    control,
    setValue,
    getValues,
    formState: { errors },
  } = useFormContext<VehicleFormValues>();
  const [brandUuid, brandLabel, modelUuid, modelLabel, typeUuid, typeLabel] =
    useWatch({
      control,
      name: [
        "brand_uuid",
        "brand_label",
        "model_uuid",
        "model_label",
        "catalog_type_uuid",
        "type_label",
      ],
    });
  const typesSeen = useRef(new Map<string, TenantCatalogType>());
  const labels = useRef(new Map<string, string>());
  const remember = (options: ComboboxOption[]) => {
    for (const option of options)
      labels.current.set(option.value, option.label);
    return options;
  };
  const dirty = { shouldDirty: true, shouldValidate: false };

  const loadBrands = useCallback(
    async (q: string) => {
      const page = await fleetService.catalogBrands(slug, q);
      return remember(
        page.items.map((b) => ({ value: b.uuid, label: b.display_name })),
      );
    },
    [slug],
  );
  const loadModels = useCallback(
    async (q: string) => {
      if (!brandUuid) return [];
      const page = await fleetService.catalogModels(slug, brandUuid, q);
      return remember(
        page.items.map((m) => ({ value: m.uuid, label: m.name })),
      );
    },
    [brandUuid, slug],
  );
  const loadTypes = useCallback(
    async (q: string) => {
      if (!modelUuid) return [];
      const page = await fleetService.catalogTypes(slug, modelUuid, q);
      for (const type of page.items) typesSeen.current.set(type.uuid, type);
      return remember(
        page.items.map((type) => ({
          value: type.uuid,
          label: type.name,
          description: type.years.length
            ? `${Math.min(...type.years)}–${Math.max(...type.years)} · ${type.tsb_type_code}`
            : type.tsb_type_code,
        })),
      );
    },
    [modelUuid, slug],
  );

  const pickType = (value: string) => {
    if (!value) {
      setValue("catalog_type_uuid", "", dirty);
      setValue("type_label", "", dirty);
      setValue("type_years", [], dirty);
      return;
    }
    const type = typesSeen.current.get(value);
    setValue("catalog_type_uuid", value, dirty);
    setValue("type_label", labels.current.get(value) ?? value, dirty);
    setValue("type_years", type?.years ?? [], dirty);
    if (type?.suggested_transmission && getValues("transmission") === NONE) {
      setValue("transmission", type.suggested_transmission, dirty);
    }
    if (type?.suggested_fuel && getValues("fuel") === NONE) {
      setValue("fuel", type.suggested_fuel, dirty);
    }
    const year = getValues("model_year");
    if (year && type?.years.length && !type.years.includes(Number(year))) {
      setValue("model_year", "", dirty);
    }
  };

  const typeError = get(errors, "catalog_type_uuid")?.message as
    string | undefined;

  return (
    <div className="grid gap-4 sm:col-span-2 sm:grid-cols-3">
      <FormFieldShell name="brand_uuid" label={t("vehicles.fields.brand")}>
        <AsyncCombobox
          id="brand_uuid"
          value={brandUuid}
          initialOptions={
            brandUuid ? [{ value: brandUuid, label: brandLabel }] : undefined
          }
          loadOptions={loadBrands}
          placeholder={t("fleet.catalog.brand_placeholder")}
          onValueChange={(value) => {
            setValue("brand_uuid", value, dirty);
            setValue("brand_label", labels.current.get(value) ?? "", dirty);
            setValue("model_uuid", "", dirty);
            setValue("model_label", "", dirty);
            pickType("");
          }}
        />
      </FormFieldShell>
      <FormFieldShell name="model_uuid" label={t("vehicles.fields.model")}>
        <AsyncCombobox
          key={brandUuid || "no-brand"}
          id="model_uuid"
          value={modelUuid}
          initialOptions={
            modelUuid ? [{ value: modelUuid, label: modelLabel }] : undefined
          }
          loadOptions={loadModels}
          placeholder={t("fleet.catalog.model_placeholder")}
          disabled={!brandUuid}
          onValueChange={(value) => {
            setValue("model_uuid", value, dirty);
            setValue("model_label", labels.current.get(value) ?? "", dirty);
            pickType("");
          }}
        />
      </FormFieldShell>
      <FormFieldShell
        name="catalog_type_uuid"
        label={t("vehicles.fields.type")}
        error={typeError}
      >
        <AsyncCombobox
          key={modelUuid || "no-model"}
          id="catalog_type_uuid"
          value={typeUuid}
          initialOptions={
            typeUuid ? [{ value: typeUuid, label: typeLabel }] : undefined
          }
          loadOptions={loadTypes}
          placeholder={t("fleet.catalog.type_placeholder")}
          disabled={!modelUuid}
          aria-invalid={Boolean(typeError)}
          onValueChange={pickType}
        />
      </FormFieldShell>
    </div>
  );
}

/** Model year select: the catalog type's valid years, or a plain range. */
function ModelYearField() {
  const { t } = useLocale();
  const { control } = useFormContext<VehicleFormValues>();
  const [manual, years] = useWatch({
    control,
    name: ["manual_entry", "type_years"],
  });
  const options = (
    !manual && years.length ? [...years].sort((a, b) => b - a) : FALLBACK_YEARS
  ).map((year) => ({ value: String(year), label: String(year) }));
  return (
    <AppSelect
      name="model_year"
      label={t("vehicles.fields.model_year")}
      placeholder={t("vehicles.fields.model_year_placeholder")}
      description={
        !manual && years.length
          ? t("vehicles.form.model_year_catalog_hint")
          : undefined
      }
      options={options}
    />
  );
}

function SupplierField() {
  const { t } = useLocale();
  const { can } = usePermission();
  const { control } = useFormContext<VehicleFormValues>();
  const [ownership, supplier] = useWatch({
    control,
    name: ["ownership", "supplier_uuid"],
  });
  const canPick = can(permissions.tenant.suppliers.read);
  const suppliers = useSupplierOptions(ownership === "supplied" && canPick);
  if (ownership !== "supplied") return null;
  return (
    <>
      {canPick ? (
        <AppCombobox
          name="supplier_uuid"
          label={t("vehicles.fields.supplier")}
          description={t("vehicles.fields.supplier_hint")}
          options={[
            { value: NONE, label: t("vehicles.fields.supplier_none") },
            ...suppliers,
          ]}
        />
      ) : null}
      {supplier === NONE ? (
        <AppInput
          name="supplier_name"
          label={t("vehicles.fields.supplier_name")}
          maxLength={255}
        />
      ) : null}
    </>
  );
}

/** Create / edit form: identity, catalog, verified attributes, location, service. */
export function VehicleForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: VehicleFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createVehicleFormSchema(t), [t]);
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("vehicles.form.section_identity"),
          description: t("vehicles.form.section_identity_desc"),
          icon: CarFront,
        },
        {
          key: "attributes",
          label: t("vehicles.form.section_attributes"),
          description: t("vehicles.form.section_attributes_desc"),
          icon: ShieldCheck,
        },
        {
          key: "location",
          label: t("vehicles.form.section_location"),
          description: t("vehicles.form.section_location_desc"),
          icon: Building2,
        },
        {
          key: "service",
          label: t("vehicles.form.section_service"),
          description: t("vehicles.form.section_service_desc"),
          icon: Gauge,
        },
      ]),
    [t],
  );

  const branchOptions = (branches.data?.items ?? []).map((branch) => ({
    value: branch.uuid,
    label: `${branch.code} · ${branch.name}`,
  }));
  const unknown = { value: NONE, label: t("vehicles.form.unknown") };

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => {
        const manual = form.watch("manual_entry");
        return (
          <FormLayout navItems={sections.navItems}>
            <FormSection
              id={sections.id("identity")}
              title={t("vehicles.form.section_identity")}
              description={t("vehicles.form.section_identity_desc")}
              columns={2}
            >
              <AppInput
                name="plate"
                label={t("vehicles.fields.plate")}
                placeholder="34 ABC 123"
                autoCapitalize="characters"
                maxLength={20}
              />
              <AppInput
                name="vin"
                label={t("vehicles.fields.vin")}
                description={t("vehicles.fields.vin_hint")}
                autoCapitalize="characters"
                maxLength={17}
              />
              <AppSwitch
                name="manual_entry"
                label={t("vehicles.fields.manual_entry")}
                description={t("vehicles.fields.manual_entry_hint")}
                className="sm:col-span-2"
              />
              {manual ? (
                <>
                  <AppInput
                    name="manual_brand"
                    label={t("vehicles.fields.brand")}
                    maxLength={128}
                  />
                  <AppInput
                    name="manual_model"
                    label={t("vehicles.fields.model")}
                    maxLength={128}
                  />
                  <AppInput
                    name="manual_type"
                    label={t("vehicles.fields.type")}
                    maxLength={255}
                  />
                </>
              ) : (
                <CatalogFields />
              )}
              <ModelYearField />
              <AppInput
                name="color"
                label={t("vehicles.fields.color")}
                maxLength={64}
              />
            </FormSection>

            <FormSection
              id={sections.id("attributes")}
              title={t("vehicles.form.section_attributes")}
              description={t("vehicles.form.section_attributes_desc")}
              columns={2}
            >
              {!manual ? (
                <Alert className="sm:col-span-2">
                  <AlertDescription>
                    {t("vehicles.form.attributes_catalog_hint")}
                  </AlertDescription>
                </Alert>
              ) : null}
              <AppSelect
                name="transmission"
                label={t("vehicles.fields.transmission")}
                options={[
                  unknown,
                  ...TRANSMISSIONS.map((value) => ({
                    value,
                    label: t(TRANSMISSION_LABEL_KEYS[value]),
                  })),
                ]}
              />
              <AppSelect
                name="fuel"
                label={t("vehicles.fields.fuel")}
                options={[
                  unknown,
                  ...FUELS.map((value) => ({
                    value,
                    label: t(FUEL_LABEL_KEYS[value]),
                  })),
                ]}
              />
              <AppSelect
                name="body_type"
                label={t("vehicles.fields.body_type")}
                options={[
                  unknown,
                  ...BODY_TYPES.map((value) => ({
                    value,
                    label: t(BODY_TYPE_LABEL_KEYS[value]),
                  })),
                ]}
              />
              <AppSwitch
                name="air_conditioning"
                label={t("vehicles.fields.air_conditioning")}
              />
              <AppInput
                name="seats"
                type="number"
                min={1}
                max={60}
                label={t("vehicles.fields.seats")}
              />
              <AppInput
                name="luggage"
                type="number"
                min={0}
                max={30}
                label={t("vehicles.fields.luggage")}
                description={t("vehicles.fields.luggage_hint")}
              />
              <AppSwitch
                name="attributes_verified"
                label={t("vehicles.fields.attributes_verified")}
                description={t("vehicles.fields.attributes_verified_hint")}
                className="sm:col-span-2"
              />
            </FormSection>

            <FormSection
              id={sections.id("location")}
              title={t("vehicles.form.section_location")}
              description={t("vehicles.form.section_location_desc")}
              columns={2}
            >
              <AppSelect
                name="home_branch_uuid"
                label={t("vehicles.fields.home_branch")}
                placeholder={t("vehicles.fields.branch_placeholder")}
                options={branchOptions}
              />
              <AppSelect
                name="current_branch_uuid"
                label={t("vehicles.fields.current_branch")}
                placeholder={t("vehicles.fields.current_branch_placeholder")}
                description={t("vehicles.fields.current_branch_hint")}
                options={branchOptions}
              />
              <AppInput
                name="fleet_pool"
                label={t("vehicles.fields.fleet_pool")}
                description={t("vehicles.fields.fleet_pool_hint")}
                autoCapitalize="characters"
                maxLength={32}
              />
              <AppSelect
                name="ownership"
                label={t("vehicles.fields.ownership")}
                options={OWNERSHIPS.map((value) => ({
                  value,
                  label: t(OWNERSHIP_LABEL_KEYS[value]),
                }))}
              />
              <SupplierField />
            </FormSection>

            <FormSection
              id={sections.id("service")}
              title={t("vehicles.form.section_service")}
              description={t("vehicles.form.section_service_desc")}
              columns={2}
            >
              <AppDatePicker
                name="in_service_date"
                label={t("vehicles.fields.in_service_date")}
                placeholder={t("vehicles.fields.date_placeholder")}
              />
              <AppDatePicker
                name="out_of_service_date"
                label={t("vehicles.fields.out_of_service_date")}
                placeholder={t("vehicles.fields.date_placeholder")}
                description={t("vehicles.fields.service_dates_hint")}
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
              {mode === "create" ? (
                <AppSelect
                  name="status"
                  label={t("vehicles.fields.initial_status")}
                  options={VEHICLE_STATUSES.map((value) => ({
                    value,
                    label: t(STATUS_LABEL_KEYS[value]),
                  }))}
                />
              ) : null}
              <AppSwitch
                name="is_active"
                label={t("vehicles.fields.is_active")}
                description={t("vehicles.fields.is_active_hint")}
              />
            </FormSection>

            <FormActions>
              <Button type="button" variant="outline" onClick={onCancel}>
                {t("form.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={isSubmitting || form.formState.isSubmitting}
              >
                {mode === "create"
                  ? t("vehicles.actions.create")
                  : t("vehicles.actions.save")}
              </Button>
            </FormActions>
          </FormLayout>
        );
      }}
    </AppForm>
  );
}
