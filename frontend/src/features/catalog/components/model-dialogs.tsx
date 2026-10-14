"use client";

import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  AppCombobox,
  AppForm,
  AppInput,
  FormActions,
  type ComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  useBulkModelRules,
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

/**
 * Loads models for combobox search: of one display brand (every TSB brand
 * under it, e.g. FIAT and TOFAS-FIAT) or of one TSB brand.
 */
export function useBrandModelOptions(
  scope: { displayBrand?: string; brand?: string } | undefined,
) {
  const { t } = useLocale();
  const [cache, setCache] = useState<ComboboxOption[]>([]);
  const displayBrand = scope?.displayBrand;
  const brand = scope?.brand;
  const loadOptions = useCallback(
    async (query: string) => {
      if (!displayBrand && !brand) return [];
      const page = await catalogService.listModels({
        ...(displayBrand ? { display_brand: displayBrand } : { brand }),
        q: query.trim() || undefined,
        limit: 50,
        offset: 0,
        sort: "name",
      });
      const options = page.items.map((m) => ({
        value: m.uuid,
        label: m.name,
        description: t("catalog.picker.model_description", {
          brand: m.brand.tsb_name ?? m.brand.display_name,
          source: t(`catalog.source.${m.source}`),
          count: m.types_count,
        }),
      }));
      setCache((prev) => {
        const byValue = new Map(prev.map((o) => [o.value, o]));
        options.forEach((o) => byValue.set(o.value, o));
        return [...byValue.values()];
      });
      return options;
    },
    [brand, displayBrand, t],
  );
  return { loadOptions, options: cache };
}

export function ModelCreateDialog({
  open,
  defaultBrand,
  defaultName,
  description,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  defaultBrand?: string;
  defaultName?: string;
  description?: string;
  onOpenChange: (open: boolean) => void;
  /** Called with the new model before the dialog closes. */
  onCreated?: (model: CatalogModel) => Promise<void> | void;
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
      description={description ?? t("catalog.models.create_description")}
    >
      {open ? (
        <AppForm
          schema={schema}
          defaultValues={{
            brand_uuid: defaultBrand ?? "",
            name: defaultName ?? "",
          }}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(values);
            await onCreated?.(created);
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
  const { loadOptions, options } = useBrandModelOptions(
    model ? { displayBrand: model.display_brand.uuid } : undefined,
  );
  const [into, setInto] = useState<ComboboxOption | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        into_uuid: z.string().uuid(t("catalog.validation.model_required")),
      }),
    [t],
  );
  const close = () => {
    setInto(null);
    onOpenChange(false);
  };
  return (
    <>
      <AppDialog
        open={Boolean(model) && !into}
        onOpenChange={(next) => {
          if (!next) close();
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
            onSubmit={(values) => {
              setInto(
                options.find((o) => o.value === values.into_uuid) ?? {
                  value: values.into_uuid,
                  label: values.into_uuid,
                },
              );
            }}
            className="space-y-4"
          >
            <AppCombobox
              name="into_uuid"
              label={t("catalog.models.merge_into")}
              description={t("catalog.models.merge_hint")}
              loadOptions={loadOptions}
              options={options.filter((o) => o.value !== model.uuid)}
              placeholder={t("catalog.picker.model_placeholder")}
              searchPlaceholder={t("catalog.picker.search")}
              emptyText={t("catalog.picker.empty")}
            />
            <FormActions>
              <Button type="button" variant="outline" onClick={close}>
                {t("form.cancel")}
              </Button>
              <Button type="submit">{t("catalog.actions.continue")}</Button>
            </FormActions>
          </AppForm>
        ) : null}
      </AppDialog>
      <ConfirmDialog
        open={Boolean(model && into)}
        title={t("catalog.models.merge_confirm_title")}
        description={
          model && into
            ? t("catalog.models.merge_confirm_description", {
                name: model.name,
                into: into.label,
                count: model.types_count,
              })
            : undefined
        }
        confirmLabel={t("catalog.actions.merge")}
        variant="destructive"
        isPending={merge.isPending}
        onCancel={() => setInto(null)}
        onConfirm={async () => {
          if (!model || !into) return;
          await merge.mutateAsync({ uuid: model.uuid, intoUuid: into.value });
          close();
        }}
      />
    </>
  );
}

/** Bulk "prefix rule = model name" for the selected models. */
export function ModelRulesConfirmDialog({
  models,
  onDone,
}: {
  models: CatalogModel[] | null;
  onDone: (applied: boolean) => void;
}) {
  const { t } = useLocale();
  const bulk = useBulkModelRules();
  return (
    <ConfirmDialog
      open={Boolean(models?.length)}
      title={t("catalog.models.rules_title")}
      description={
        models
          ? t("catalog.models.rules_description", {
              count: models.length,
              names: models
                .slice(0, 5)
                .map((m) => m.name)
                .join(", "),
            })
          : undefined
      }
      confirmLabel={t("catalog.models.rules_action")}
      isPending={bulk.isPending}
      onCancel={() => onDone(false)}
      onConfirm={async () => {
        if (!models?.length) return;
        await bulk.mutateAsync(models.map((m) => m.uuid));
        onDone(true);
      }}
    />
  );
}
