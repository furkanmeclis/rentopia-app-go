"use client";

import { Layers } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  createSegmentFormSchema,
  type SegmentFormValues,
} from "@/features/fleet/schemas/segment-form";
import { useLocale } from "@/providers/locale-provider";

type SegmentFormProps = {
  mode: "create" | "edit";
  defaultValues: SegmentFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: SegmentFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Create / edit form of a segment (commercial class). */
export function SegmentForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: SegmentFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createSegmentFormSchema(t), [t]);
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "details",
          label: t("fleet.segments.form.section_details"),
          description: t("fleet.segments.form.section_details_desc"),
          icon: Layers,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("details")}
            title={t("fleet.segments.form.section_details")}
            description={t("fleet.segments.form.section_details_desc")}
            columns={2}
          >
            <AppInput
              name="code"
              label={t("fleet.fields.code")}
              description={t("fleet.fields.code_hint")}
              autoCapitalize="characters"
            />
            <AppInput
              name="name"
              label={t("fleet.segments.fields.name")}
              placeholder={t("fleet.segments.fields.name_placeholder")}
            />
            <AppTextarea
              name="description"
              label={t("fleet.fields.description")}
              className="sm:col-span-2"
            />
            <AppInput
              name="sort_order"
              type="number"
              min={0}
              label={t("fleet.segments.fields.sort_order")}
              description={t("fleet.segments.fields.sort_order_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("fleet.fields.is_active")}
              description={t("fleet.segments.fields.is_active_hint")}
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
                ? t("fleet.segments.actions.create")
                : t("fleet.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
