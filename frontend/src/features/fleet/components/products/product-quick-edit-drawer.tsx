"use client";

import Link from "next/link";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSelect, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  PRODUCT_STATUSES,
  STATUS_LABEL_KEYS,
} from "@/features/fleet/constants";
import { useUpdateProduct } from "@/features/fleet/hooks/use-fleet";
import type { ProductStatus, RentalProduct } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "product-quick-edit";

/**
 * Title, publish status and active flag of a product: none of them creates
 * a new version.
 */
export function ProductQuickEditDrawer({
  product,
  onOpenChange,
}: {
  product: RentalProduct | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateProduct();
  const schema = useMemo(
    () =>
      z.object({
        title: z
          .string()
          .trim()
          .min(1, t("fleet.validation.title_required"))
          .max(255),
        status: z.enum(PRODUCT_STATUSES as [ProductStatus, ...ProductStatus[]]),
        is_active: z.boolean(),
      }),
    [t],
  );

  return (
    <EntityDrawer
      open={Boolean(product)}
      onOpenChange={onOpenChange}
      title={t("fleet.quick_edit.title")}
      description={
        product
          ? t("fleet.quick_edit.description", {
              code: product.code,
              name: product.title,
            })
          : undefined
      }
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {product ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link
                href={routes.tenant.fleet.products.edit(slug, product.uuid)}
              >
                {t("fleet.quick_edit.full_form")}
              </Link>
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={update.isPending}>
              {t("fleet.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {product ? (
        <AppForm<z.infer<typeof schema>>
          key={product.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            title: product.title,
            status: product.status,
            is_active: product.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: product.uuid,
              body: {
                title: values.title.trim(),
                status: values.status,
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="title" label={t("fleet.products.fields.title")} />
          <AppSelect
            name="status"
            label={t("fleet.products.fields.status")}
            options={PRODUCT_STATUSES.map((value) => ({
              value,
              label: t(STATUS_LABEL_KEYS[value]),
            }))}
          />
          <AppSwitch name="is_active" label={t("fleet.fields.is_active")} />
          <p className="text-muted-foreground text-xs">
            {t("fleet.quick_edit.no_version")}
          </p>
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
