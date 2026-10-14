"use client";

import { Boxes, ListChecks, ShieldCheck } from "lucide-react";
import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  CatalogCascade,
  CatalogChips,
  type CatalogPick,
} from "@/features/fleet/components/shared/catalog-picker";
import {
  BODY_TYPES,
  BODY_TYPE_LABEL_KEYS,
  FUELS,
  FUEL_LABEL_KEYS,
  TRANSMISSIONS,
  TRANSMISSION_LABEL_KEYS,
} from "@/features/fleet/constants";
import { useSegmentOptions } from "@/features/fleet/hooks/use-fleet";
import {
  ANY,
  createGroupFormSchema,
  type GroupFormValues,
} from "@/features/fleet/schemas/group-form";
import { useLocale } from "@/providers/locale-provider";

type GroupFormProps = {
  mode: "create" | "edit";
  defaultValues: GroupFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: GroupFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Catalog model / type criteria editor (brand → model → type cascade). */
function CatalogCriteriaEditor() {
  const { t } = useLocale();
  const { control, setValue } = useFormContext<GroupFormValues>();
  const models = useWatch({ control, name: "catalog_models" });
  const types = useWatch({ control, name: "catalog_types" });

  const add = (
    name: "catalog_models" | "catalog_types",
    list: CatalogPick[],
    pick: CatalogPick,
  ) => {
    if (list.some((item) => item.uuid === pick.uuid)) return;
    setValue(name, [...list, pick], { shouldDirty: true });
  };
  const remove = (
    name: "catalog_models" | "catalog_types",
    list: CatalogPick[],
    uuid: string,
  ) =>
    setValue(
      name,
      list.filter((item) => item.uuid !== uuid),
      { shouldDirty: true },
    );

  return (
    <div className="space-y-5">
      <Alert>
        <AlertDescription>
          {t("fleet.groups.form.criteria_help")}
        </AlertDescription>
      </Alert>
      <CatalogCascade
        onPickModel={(pick) => add("catalog_models", models, pick)}
        onPickType={(pick) => add("catalog_types", types, pick)}
        modelActionLabel={t("fleet.groups.form.add_model")}
        typeActionLabel={t("fleet.groups.form.add_type")}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label>{t("fleet.groups.fields.catalog_models")}</Label>
          <CatalogChips
            items={models}
            emptyText={t("fleet.groups.form.no_models")}
            onRemove={(uuid) => remove("catalog_models", models, uuid)}
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("fleet.groups.fields.catalog_types")}</Label>
          <CatalogChips
            items={types}
            emptyText={t("fleet.groups.form.no_types")}
            onRemove={(uuid) => remove("catalog_types", types, uuid)}
          />
        </div>
      </div>
    </div>
  );
}

/** Create / edit form: details, protected attributes, catalog criteria. */
export function GroupForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: GroupFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createGroupFormSchema(t), [t]);
  const segments = useSegmentOptions();
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "details",
          label: t("fleet.groups.form.section_details"),
          description: t("fleet.groups.form.section_details_desc"),
          icon: Boxes,
        },
        {
          key: "attributes",
          label: t("fleet.groups.form.section_attributes"),
          description: t("fleet.groups.form.section_attributes_desc"),
          icon: ShieldCheck,
        },
        {
          key: "criteria",
          label: t("fleet.groups.form.section_criteria"),
          description: t("fleet.groups.form.section_criteria_desc"),
          icon: ListChecks,
        },
      ]),
    [t],
  );

  const anyOption = {
    value: ANY,
    label: t("fleet.groups.form.not_guaranteed"),
  };
  const segmentOptions = (segments.data?.items ?? []).map((segment) => ({
    value: segment.uuid,
    label: `${segment.code} · ${segment.name}`,
  }));

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("details")}
            title={t("fleet.groups.form.section_details")}
            description={t("fleet.groups.form.section_details_desc")}
            columns={2}
          >
            <AppInput
              name="code"
              label={t("fleet.fields.code")}
              description={t("fleet.groups.fields.code_hint")}
              autoCapitalize="characters"
            />
            <AppInput
              name="name"
              label={t("fleet.groups.fields.name")}
              placeholder={t("fleet.groups.fields.name_placeholder")}
            />
            <AppSelect
              name="segment_uuid"
              label={t("fleet.fields.segment")}
              placeholder={t("fleet.fields.segment_placeholder")}
              options={segmentOptions}
            />
            <AppSwitch
              name="is_active"
              label={t("fleet.fields.is_active")}
              description={t("fleet.groups.fields.is_active_hint")}
            />
            <AppTextarea
              name="description"
              label={t("fleet.fields.description")}
              className="sm:col-span-2"
            />
          </FormSection>

          <FormSection
            id={sections.id("attributes")}
            title={t("fleet.groups.form.section_attributes")}
            description={t("fleet.groups.form.section_attributes_desc")}
            columns={2}
          >
            <AppSelect
              name="transmission"
              label={t("fleet.groups.fields.transmission")}
              options={[
                anyOption,
                ...TRANSMISSIONS.map((value) => ({
                  value,
                  label: t(TRANSMISSION_LABEL_KEYS[value]),
                })),
              ]}
            />
            <AppSelect
              name="fuel"
              label={t("fleet.groups.fields.fuel")}
              options={[
                anyOption,
                ...FUELS.map((value) => ({
                  value,
                  label: t(FUEL_LABEL_KEYS[value]),
                })),
              ]}
            />
            <AppSelect
              name="body_type"
              label={t("fleet.groups.fields.body_type")}
              options={[
                anyOption,
                ...BODY_TYPES.map((value) => ({
                  value,
                  label: t(BODY_TYPE_LABEL_KEYS[value]),
                })),
              ]}
            />
            <AppSwitch
              name="air_conditioning"
              label={t("fleet.groups.fields.air_conditioning")}
              description={t("fleet.groups.fields.air_conditioning_hint")}
            />
            <AppInput
              name="min_seats"
              type="number"
              min={1}
              max={60}
              label={t("fleet.groups.fields.min_seats")}
              description={t("fleet.groups.fields.optional_hint")}
            />
            <AppInput
              name="min_luggage"
              type="number"
              min={0}
              max={30}
              label={t("fleet.groups.fields.min_luggage")}
              description={t("fleet.groups.fields.min_luggage_hint")}
            />
            <AppInput
              name="acriss_code"
              label={t("fleet.groups.fields.acriss_code")}
              description={t("fleet.groups.fields.acriss_code_hint")}
              autoCapitalize="characters"
              maxLength={4}
              placeholder="ECMR"
            />
          </FormSection>

          <FormSection
            id={sections.id("criteria")}
            title={t("fleet.groups.form.section_criteria")}
            description={t("fleet.groups.form.section_criteria_desc")}
          >
            <CatalogCriteriaEditor />
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
                ? t("fleet.groups.actions.create")
                : t("fleet.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
