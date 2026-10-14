"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { LogoControl } from "@/components/branding/logo-control";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  AppCombobox,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  useCatalogDisplayBrand,
  useCatalogDisplayBrandOptions,
  useDisplayBrandLogo,
  useMergeDisplayBrand,
  usePatchDisplayBrand,
  useUnmergeDisplayBrand,
} from "@/features/catalog/hooks/use-catalog";
import type { CatalogDisplayBrand } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type DialogProps = {
  brand: CatalogDisplayBrand | null;
  onOpenChange: (open: boolean) => void;
};

/** Rename and (de)activate a display brand. */
export function DisplayBrandEditDialog({ brand, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const patch = usePatchDisplayBrand();
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("catalog.validation.display_name_required")),
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
      title={t("catalog.display_brands.edit_title")}
      description={
        brand
          ? t("catalog.display_brands.edit_description", {
              names: brand.tsb_names.join(", "),
            })
          : undefined
      }
    >
      {brand ? (
        <AppForm
          key={brand.uuid}
          schema={schema}
          defaultValues={{ name: brand.name, is_active: brand.is_active }}
          onSubmit={async (values) => {
            await patch.mutateAsync({ uuid: brand.uuid, body: values });
            onOpenChange(false);
          }}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppInput name="name" label={t("catalog.columns.display_name")} />
              <AppSwitch
                name="is_active"
                label={t("catalog.columns.is_active")}
                description={t("catalog.display_brands.active_hint")}
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

/** Merge a display brand into another one, with a confirmation step. */
export function DisplayBrandMergeDialog({ brand, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const merge = useMergeDisplayBrand();
  const options = useCatalogDisplayBrandOptions();
  const [target, setTarget] = useState<CatalogDisplayBrand | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        into_uuid: z.string().uuid(t("catalog.validation.brand_required")),
      }),
    [t],
  );
  const choices = useMemo(
    () =>
      (options.data ?? [])
        .filter((d) => d.uuid !== brand?.uuid)
        .map((d) => ({
          value: d.uuid,
          label: d.name,
          description: d.tsb_names.join(", "),
        })),
    [brand?.uuid, options.data],
  );
  const close = () => {
    setTarget(null);
    onOpenChange(false);
  };

  return (
    <>
      <AppDialog
        open={Boolean(brand) && !target}
        onOpenChange={(next) => {
          if (!next) close();
        }}
        title={t("catalog.display_brands.merge_title")}
        description={
          brand
            ? t("catalog.display_brands.merge_description", {
                name: brand.name,
                names: brand.tsb_names.join(", "),
              })
            : undefined
        }
      >
        {brand ? (
          <AppForm
            key={brand.uuid}
            schema={schema}
            defaultValues={{ into_uuid: "" }}
            onSubmit={(values) => {
              setTarget(
                options.data?.find((d) => d.uuid === values.into_uuid) ?? null,
              );
            }}
            className="space-y-4"
          >
            <AppCombobox
              name="into_uuid"
              label={t("catalog.display_brands.merge_into")}
              options={choices}
              placeholder={t("catalog.picker.brand_placeholder")}
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
        open={Boolean(brand && target)}
        title={t("catalog.display_brands.merge_confirm_title")}
        description={
          brand && target
            ? t("catalog.display_brands.merge_confirm_description", {
                from: brand.name,
                into: target.name,
                names: brand.tsb_names.join(", "),
              })
            : undefined
        }
        confirmLabel={t("catalog.actions.merge")}
        variant="destructive"
        isPending={merge.isPending}
        onCancel={() => setTarget(null)}
        onConfirm={async () => {
          if (!brand || !target) return;
          await merge.mutateAsync({ uuid: brand.uuid, intoUuid: target.uuid });
          close();
        }}
      />
    </>
  );
}

/** Detach one TSB brand into its own display brand. */
export function DisplayBrandUnmergeDialog({
  brand,
  onOpenChange,
}: DialogProps) {
  const { t } = useLocale();
  const unmerge = useUnmergeDisplayBrand();
  const detail = useCatalogDisplayBrand(brand?.uuid);
  const [pending, setPending] = useState<{
    brand_uuid: string;
    name: string;
  } | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        brand_uuid: z.string().uuid(t("catalog.validation.brand_required")),
        name: z.string().trim(),
      }),
    [t],
  );
  const members = detail.data?.brands ?? [];
  const memberLabel = (uuid: string) =>
    members.find((m) => m.uuid === uuid)?.tsb_name ?? "";
  const close = () => {
    setPending(null);
    onOpenChange(false);
  };

  return (
    <>
      <AppDialog
        open={Boolean(brand) && !pending}
        onOpenChange={(next) => {
          if (!next) close();
        }}
        title={t("catalog.display_brands.unmerge_title")}
        description={
          brand
            ? t("catalog.display_brands.unmerge_description", {
                name: brand.name,
              })
            : undefined
        }
      >
        {brand && detail.data ? (
          <AppForm
            key={brand.uuid}
            schema={schema}
            defaultValues={{ brand_uuid: "", name: "" }}
            onSubmit={(values) => setPending(values)}
            className="space-y-4"
          >
            <AppSelect
              name="brand_uuid"
              label={t("catalog.display_brands.unmerge_brand")}
              placeholder={t("catalog.picker.brand_placeholder")}
              options={members.map((m) => ({
                value: m.uuid,
                label: `${m.tsb_name} (${m.tsb_code}) · ${t("catalog.display_brands.types_n", { count: m.types_count })}`,
              }))}
            />
            <AppInput
              name="name"
              label={t("catalog.display_brands.unmerge_name")}
              description={t("catalog.display_brands.unmerge_name_hint")}
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
        open={Boolean(brand && pending)}
        title={t("catalog.display_brands.unmerge_confirm_title")}
        description={
          brand && pending
            ? t("catalog.display_brands.unmerge_confirm_description", {
                tsb: memberLabel(pending.brand_uuid),
                from: brand.name,
                name:
                  pending.name ||
                  t("catalog.display_brands.unmerge_default_name"),
              })
            : undefined
        }
        confirmLabel={t("catalog.actions.unmerge")}
        isPending={unmerge.isPending}
        onCancel={() => setPending(null)}
        onConfirm={async () => {
          if (!brand || !pending) return;
          await unmerge.mutateAsync({
            uuid: brand.uuid,
            body: {
              brand_uuid: pending.brand_uuid,
              ...(pending.name ? { name: pending.name } : {}),
            },
          });
          close();
        }}
      />
    </>
  );
}

/** Upload / remove a display brand logo (stored and served by the API). */
export function DisplayBrandLogoDialog({ brand, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const { upload, clear } = useDisplayBrandLogo();
  // The list row is a snapshot; the detail query follows uploads.
  const detail = useCatalogDisplayBrand(brand?.uuid);
  const logoUrl = detail.data?.logo_url ?? brand?.logo_url ?? null;
  return (
    <AppDialog
      open={Boolean(brand)}
      onOpenChange={onOpenChange}
      title={t("catalog.display_brands.logo_title")}
      description={
        brand
          ? t("catalog.display_brands.logo_description", { name: brand.name })
          : undefined
      }
      footer={
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.close")}
        </Button>
      }
    >
      {brand ? (
        <LogoControl
          logoUrl={logoUrl}
          onUpload={async (file) => {
            await upload.mutateAsync({ uuid: brand.uuid, file });
          }}
          onClear={async () => {
            await clear.mutateAsync(brand.uuid);
          }}
        />
      ) : null}
    </AppDialog>
  );
}
