"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppForm,
  AppSelect,
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { CATALOG_CATEGORIES } from "@/features/catalog/constants";
import {
  useCatalogDisplayBrandOptions,
  usePatchBrand,
} from "@/features/catalog/hooks/use-catalog";
import type { CatalogBrand } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type BrandEditDialogProps = {
  brand: CatalogBrand | null;
  onOpenChange: (open: boolean) => void;
};

export function BrandEditDialog({ brand, onOpenChange }: BrandEditDialogProps) {
  const { t } = useLocale();
  const patch = usePatchBrand();
  const displayBrands = useCatalogDisplayBrandOptions();
  const displayOptions = useMemo(
    () =>
      (displayBrands.data ?? []).map((d) => ({
        value: d.uuid,
        label: d.name,
        description: d.tsb_names.join(", "),
      })),
    [displayBrands.data],
  );
  const schema = useMemo(
    () =>
      z.object({
        display_brand_uuid: z
          .string()
          .uuid(t("catalog.validation.display_brand_required")),
        category: z.enum(["passenger", "light_commercial", "other"]),
        is_active: z.boolean(),
      }),
    [t],
  );

  return (
    <AppDialog
      open={Boolean(brand)}
      onOpenChange={(next) => {
        if (!patch.isPending) onOpenChange(next);
      }}
      title={t("catalog.brands.edit_title")}
      description={
        brand
          ? t("catalog.brands.edit_description", {
              name: brand.tsb_name,
              code: brand.tsb_code,
            })
          : undefined
      }
    >
      {brand ? (
        <AppForm
          key={brand.uuid}
          schema={schema}
          defaultValues={{
            display_brand_uuid: brand.display_brand.uuid,
            category: brand.category,
            is_active: brand.is_active,
          }}
          onSubmit={async ({ display_brand_uuid, ...rest }) => {
            await patch.mutateAsync({
              uuid: brand.uuid,
              body: {
                ...rest,
                ...(display_brand_uuid !== brand.display_brand.uuid
                  ? { display_brand_uuid }
                  : {}),
              },
            });
            onOpenChange(false);
          }}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppCombobox
                name="display_brand_uuid"
                label={t("catalog.columns.display_brand")}
                description={t("catalog.brands.display_brand_hint")}
                options={displayOptions}
                placeholder={t("catalog.picker.brand_placeholder")}
                searchPlaceholder={t("catalog.picker.search")}
                emptyText={t("catalog.picker.empty")}
              />
              <AppSelect
                name="category"
                label={t("catalog.columns.category")}
                options={CATALOG_CATEGORIES.map((value) => ({
                  value,
                  label: t(`catalog.category.${value}`),
                }))}
              />
              <AppSwitch
                name="is_active"
                label={t("catalog.columns.is_active")}
                description={t("catalog.brands.active_hint")}
              />
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  disabled={patch.isPending || form.formState.isSubmitting}
                  onClick={() => onOpenChange(false)}
                >
                  {t("form.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={patch.isPending || form.formState.isSubmitting}
                >
                  {t("form.save")}
                </Button>
              </FormActions>
            </>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
