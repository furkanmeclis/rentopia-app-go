"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { CATALOG_CATEGORIES } from "@/features/catalog/constants";
import { usePatchBrand } from "@/features/catalog/hooks/use-catalog";
import type { CatalogBrand } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type BrandEditDialogProps = {
  brand: CatalogBrand | null;
  onOpenChange: (open: boolean) => void;
};

export function BrandEditDialog({ brand, onOpenChange }: BrandEditDialogProps) {
  const { t } = useLocale();
  const patch = usePatchBrand();
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("catalog.validation.display_name_required")),
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
            display_name: brand.display_name,
            category: brand.category,
            is_active: brand.is_active,
          }}
          onSubmit={async (values) => {
            await patch.mutateAsync({ uuid: brand.uuid, body: values });
            onOpenChange(false);
          }}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppInput
                name="display_name"
                label={t("catalog.columns.display_name")}
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
