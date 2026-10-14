"use client";

import {
  Eye,
  FileText,
  ImageIcon,
  ShieldCheck,
  SlidersHorizontal,
} from "lucide-react";
import { useMemo, type ReactNode } from "react";
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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ProductCardPreview } from "@/features/fleet/components/products/product-card-preview";
import {
  GuaranteeFields,
  RepresentativeField,
} from "@/features/fleet/components/products/guarantee-fields";
import {
  FUEL_POLICIES,
  FUEL_POLICY_LABEL_KEYS,
  PRODUCT_STATUSES,
  STATUS_LABEL_KEYS,
} from "@/features/fleet/constants";
import { useSegmentOptions } from "@/features/fleet/hooks/use-fleet";
import {
  createProductFormSchema,
  previewDisplayText,
  type ProductFormValues,
} from "@/features/fleet/schemas/product-form";
import { useLocale } from "@/providers/locale-provider";

type ProductFormProps = {
  mode: "create" | "edit";
  defaultValues: ProductFormValues;
  /** Current version (edit): announces version N+1 on guarantee changes. */
  currentVersion?: number;
  /** Cover image of the product (edit) for the preview. */
  coverUrl?: string | null;
  /** Image manager (edit only; images need a saved product). */
  images?: ReactNode;
  isSubmitting?: boolean;
  onSubmit: (values: ProductFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Conditions inputs (deposit, driver, km, fuel policy, notes). */
function ConditionsFields() {
  const { t } = useLocale();
  const { control } = useFormContext<ProductFormValues>();
  const unlimited = useWatch({ control, name: "conditions.unlimited_km" });
  return (
    <>
      <AppInput
        name="conditions.deposit_amount"
        inputMode="decimal"
        label={t("fleet.products.fields.deposit_amount")}
        description={t("fleet.products.fields.money_hint")}
      />
      <AppSelect
        name="conditions.fuel_policy"
        label={t("fleet.products.fields.fuel_policy")}
        options={FUEL_POLICIES.map((value) => ({
          value,
          label: t(FUEL_POLICY_LABEL_KEYS[value]),
        }))}
      />
      <AppInput
        name="conditions.min_driver_age"
        type="number"
        min={18}
        max={99}
        label={t("fleet.products.fields.min_driver_age")}
      />
      <AppInput
        name="conditions.min_license_years"
        type="number"
        min={0}
        max={60}
        label={t("fleet.products.fields.min_license_years")}
      />
      <AppSwitch
        name="conditions.unlimited_km"
        label={t("fleet.products.fields.unlimited_km")}
        description={t("fleet.products.fields.unlimited_km_hint")}
      />
      {unlimited ? (
        <span />
      ) : (
        <AppInput
          name="conditions.included_km_per_day"
          type="number"
          min={0}
          label={t("fleet.products.fields.included_km_per_day")}
        />
      )}
      <AppInput
        name="conditions.extra_km_fee"
        inputMode="decimal"
        label={t("fleet.products.fields.extra_km_fee")}
        description={t("fleet.products.fields.money_hint")}
      />
      <AppTextarea
        name="conditions.notes"
        label={t("fleet.products.fields.condition_notes")}
        className="sm:col-span-2"
      />
    </>
  );
}

/** "This edit creates version N+1" notice + change note (edit only). */
function VersionNotice({ currentVersion }: { currentVersion: number }) {
  const { t } = useLocale();
  const {
    control,
    formState: { dirtyFields },
  } = useFormContext<ProductFormValues>();
  // Subscribe so the notice follows edits.
  useWatch({ control, name: ["guarantee", "conditions"] });
  const versioned = Boolean(dirtyFields.guarantee || dirtyFields.conditions);
  if (!versioned) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("fleet.products.form.version_unchanged", {
          version: currentVersion,
        })}
      </p>
    );
  }
  return (
    <div className="space-y-3">
      <Alert>
        <AlertTitle>
          {t("fleet.products.form.version_new_title", {
            version: currentVersion + 1,
          })}
        </AlertTitle>
        <AlertDescription>
          {t("fleet.products.form.version_new_description", {
            version: currentVersion,
          })}
        </AlertDescription>
      </Alert>
      <AppInput
        name="change_note"
        label={t("fleet.products.fields.change_note")}
        placeholder={t("fleet.products.fields.change_note_placeholder")}
      />
    </div>
  );
}

/** Live card preview from the form values. */
function LivePreview({ coverUrl }: { coverUrl?: string | null }) {
  const { t } = useLocale();
  const { control } = useFormContext<ProductFormValues>();
  const values = useWatch({ control }) as ProductFormValues;
  const segments = useSegmentOptions();
  const preview = previewDisplayText(values, t);
  const segment = segments.data?.items.find(
    (item) => item.uuid === values.segment_uuid,
  );
  return (
    <div className="flex justify-center">
      <ProductCardPreview
        title={values.title}
        displayText={preview.text}
        badge={preview.badge}
        imageUrl={coverUrl}
        caption={segment?.name}
      />
    </div>
  );
}

/**
 * Product form: general info, guarantee level (radio cards with plain
 * explanations), rental conditions, images (edit) and a live card preview.
 */
export function ProductForm({
  mode,
  defaultValues,
  currentVersion,
  coverUrl,
  images,
  isSubmitting,
  onSubmit,
  onCancel,
}: ProductFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createProductFormSchema(t), [t]);
  const segments = useSegmentOptions();
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "general",
          label: t("fleet.products.form.section_general"),
          description: t("fleet.products.form.section_general_desc"),
          icon: FileText,
        },
        {
          key: "guarantee",
          label: t("fleet.products.form.section_guarantee"),
          description: t("fleet.products.form.section_guarantee_desc"),
          icon: ShieldCheck,
        },
        {
          key: "conditions",
          label: t("fleet.products.form.section_conditions"),
          description: t("fleet.products.form.section_conditions_desc"),
          icon: SlidersHorizontal,
        },
        {
          key: "images",
          label: t("fleet.products.form.section_images"),
          description: t("fleet.products.form.section_images_desc"),
          icon: ImageIcon,
        },
        {
          key: "preview",
          label: t("fleet.products.form.section_preview"),
          description: t("fleet.products.form.section_preview_desc"),
          icon: Eye,
        },
      ]),
    [t],
  );
  const segmentOptions = (segments.data?.items ?? []).map((segment) => ({
    value: segment.uuid,
    label: `${segment.code} · ${segment.name}`,
  }));

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("general")}
            title={t("fleet.products.form.section_general")}
            description={t("fleet.products.form.section_general_desc")}
            columns={2}
          >
            <AppInput
              name="code"
              label={t("fleet.fields.code")}
              description={t("fleet.products.fields.code_hint")}
              autoCapitalize="characters"
            />
            <AppInput
              name="title"
              label={t("fleet.products.fields.title")}
              placeholder={t("fleet.products.fields.title_placeholder")}
            />
            <AppSelect
              name="segment_uuid"
              label={t("fleet.fields.segment")}
              placeholder={t("fleet.fields.segment_placeholder")}
              options={segmentOptions}
            />
            <AppSelect
              name="status"
              label={t("fleet.products.fields.status")}
              options={PRODUCT_STATUSES.map((value) => ({
                value,
                label: t(STATUS_LABEL_KEYS[value]),
              }))}
            />
            <AppTextarea
              name="description"
              label={t("fleet.fields.description")}
              className="sm:col-span-2"
            />
            <RepresentativeField />
            <AppSwitch
              name="is_active"
              label={t("fleet.fields.is_active")}
              description={t("fleet.products.fields.is_active_hint")}
            />
          </FormSection>

          <FormSection
            id={sections.id("guarantee")}
            title={t("fleet.products.form.section_guarantee")}
            description={t("fleet.products.form.section_guarantee_desc")}
          >
            <GuaranteeFields />
          </FormSection>

          <FormSection
            id={sections.id("conditions")}
            title={t("fleet.products.form.section_conditions")}
            description={t("fleet.products.form.section_conditions_desc")}
            columns={2}
          >
            <ConditionsFields />
          </FormSection>

          <FormSection
            id={sections.id("images")}
            title={t("fleet.products.form.section_images")}
            description={t("fleet.products.form.section_images_desc")}
          >
            {images ?? (
              <p className="text-muted-foreground text-sm">
                {t("fleet.images.after_create")}
              </p>
            )}
          </FormSection>

          <FormSection
            id={sections.id("preview")}
            title={t("fleet.products.form.section_preview")}
            description={t("fleet.products.form.section_preview_desc")}
          >
            <LivePreview coverUrl={coverUrl} />
          </FormSection>

          {mode === "edit" && currentVersion ? (
            <VersionNotice currentVersion={currentVersion} />
          ) : null}

          <FormActions>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || form.formState.isSubmitting}
            >
              {mode === "create"
                ? t("fleet.products.actions.create")
                : t("fleet.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
