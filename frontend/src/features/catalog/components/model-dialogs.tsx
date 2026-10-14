"use client";

import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppForm,
  AppInput,
  FormActions,
  type ComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  useCatalogBrandOptions,
  useCreateModel,
  useMergeModel,
  usePatchModel,
} from "@/features/catalog/hooks/use-catalog";
import {
  catalogService,
  type CatalogModel,
} from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

/** Loads the models of one brand for combobox search. */
export function useBrandModelOptions(brandUuid: string | undefined) {
  const [cache, setCache] = useState<ComboboxOption[]>([]);
  const loadOptions = useCallback(
    async (query: string) => {
      if (!brandUuid) return [];
      const page = await catalogService.listModels({
        brand: brandUuid,
        q: query.trim() || undefined,
        limit: 50,
        offset: 0,
        sort: "name",
      });
      const options = page.items.map((m) => ({
        value: m.uuid,
        label: m.name,
        description: `${m.source} · ${m.types_count}`,
      }));
      setCache((prev) => {
        const byValue = new Map(prev.map((o) => [o.value, o]));
        options.forEach((o) => byValue.set(o.value, o));
        return [...byValue.values()];
      });
      return options;
    },
    [brandUuid],
  );
  return { loadOptions, options: cache };
}

export function ModelCreateDialog({
  open,
  defaultBrand,
  onOpenChange,
}: {
  open: boolean;
  defaultBrand?: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const create = useCreateModel();
  const brands = useCatalogBrandOptions();
  const schema = useMemo(
    () =>
      z.object({
        brand_uuid: z.string().uuid(t("catalog.validation.brand_required")),
        name: z.string().trim().min(1, t("catalog.validation.name_required")),
      }),
    [t],
  );
  const brandOptions = useMemo(
    () =>
      (brands.data ?? []).map((b) => ({
        value: b.uuid,
        label: b.display_name,
        description: b.tsb_name,
      })),
    [brands.data],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!create.isPending) onOpenChange(next);
      }}
      title={t("catalog.models.create_title")}
      description={t("catalog.models.create_description")}
    >
      {open ? (
        <AppForm
          schema={schema}
          defaultValues={{ brand_uuid: defaultBrand ?? "", name: "" }}
          onSubmit={async (values) => {
            await create.mutateAsync(values);
            onOpenChange(false);
          }}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppCombobox
                name="brand_uuid"
                label={t("catalog.columns.brand")}
                options={brandOptions}
                placeholder={t("catalog.picker.brand_placeholder")}
                searchPlaceholder={t("catalog.picker.search")}
                emptyText={t("catalog.picker.empty")}
              />
              <AppInput name="name" label={t("catalog.columns.model")} />
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  disabled={create.isPending || form.formState.isSubmitting}
                  onClick={() => onOpenChange(false)}
                >
                  {t("form.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={create.isPending || form.formState.isSubmitting}
                >
                  {t("catalog.actions.create_model")}
                </Button>
              </FormActions>
            </>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

export function ModelRenameDialog({
  model,
  onOpenChange,
}: {
  model: CatalogModel | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const patch = usePatchModel();
  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("catalog.validation.name_required")),
      }),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(model)}
      onOpenChange={(next) => {
        if (!patch.isPending) onOpenChange(next);
      }}
      title={t("catalog.models.rename_title")}
      description={
        model
          ? t("catalog.models.rename_description", {
              brand: model.brand.display_name,
            })
          : undefined
      }
    >
      {model ? (
        <AppForm
          key={model.uuid}
          schema={schema}
          defaultValues={{ name: model.name }}
          onSubmit={async (values) => {
            await patch.mutateAsync({ uuid: model.uuid, body: values });
            onOpenChange(false);
          }}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppInput name="name" label={t("catalog.columns.model")} />
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

export function ModelMergeDialog({
  model,
  onOpenChange,
}: {
  model: CatalogModel | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const merge = useMergeModel();
  const { loadOptions, options } = useBrandModelOptions(model?.brand.uuid);
  const schema = useMemo(
    () =>
      z.object({
        into_uuid: z.string().uuid(t("catalog.validation.model_required")),
      }),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(model)}
      onOpenChange={(next) => {
        if (!merge.isPending) onOpenChange(next);
      }}
      title={t("catalog.models.merge_title")}
      description={
        model
          ? t("catalog.models.merge_description", {
              name: model.name,
              count: model.types_count,
            })
          : undefined
      }
    >
      {model ? (
        <AppForm
          key={model.uuid}
          schema={schema}
          defaultValues={{ into_uuid: "" }}
          onSubmit={async (values) => {
            await merge.mutateAsync({
              uuid: model.uuid,
              intoUuid: values.into_uuid,
            });
            onOpenChange(false);
          }}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppCombobox
                name="into_uuid"
                label={t("catalog.models.merge_into")}
                loadOptions={loadOptions}
                options={options.filter((o) => o.value !== model.uuid)}
                placeholder={t("catalog.picker.model_placeholder")}
                searchPlaceholder={t("catalog.picker.search")}
                emptyText={t("catalog.picker.empty")}
              />
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  disabled={merge.isPending || form.formState.isSubmitting}
                  onClick={() => onOpenChange(false)}
                >
                  {t("form.cancel")}
                </Button>
                <Button
                  type="submit"
                  variant="destructive"
                  disabled={merge.isPending || form.formState.isSubmitting}
                >
                  {t("catalog.actions.merge")}
                </Button>
              </FormActions>
            </>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
