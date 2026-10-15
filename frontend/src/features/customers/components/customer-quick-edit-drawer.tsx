"use client";

import Link from "next/link";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useUpdateCustomer } from "@/features/customers/hooks/use-customers";
import { splitTags } from "@/features/customers/lib/format";
import type { Customer } from "@/features/customers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-quick-edit";

type Values = { tags: string; note: string; tax_office: string };

/** Tags, note and tax office without leaving the list (no sensitive values). */
export function CustomerQuickEditDrawer({
  customer,
  onOpenChange,
}: {
  customer: Customer | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateCustomer();
  const schema = useMemo(
    () =>
      z.object({
        tags: z
          .string()
          .max(1000)
          .refine((v) => splitTags(v).every((tag) => tag.length <= 40), {
            message: t("customers.validation.tag_length"),
          }),
        note: z.string().max(5000),
        tax_office: z.string().max(128),
      }),
    [t],
  );

  return (
    <EntityDrawer
      open={Boolean(customer)}
      onOpenChange={onOpenChange}
      title={t("customers.quick_edit.title")}
      description={
        customer
          ? t("customers.quick_edit.description", {
              name: customer.display_name,
            })
          : undefined
      }
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {customer ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.customers.edit(slug, customer.uuid)}>
                {t("customers.quick_edit.full_form")}
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
              {t("customers.actions.save")}
            </Button>
          </div>
        </div>
      }
    >
      {customer ? (
        <AppForm<Values>
          key={customer.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            tags: customer.tags.join(", "),
            note: customer.note,
            tax_office: customer.tax_office,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: customer.uuid,
              body: {
                tags: splitTags(values.tags),
                note: values.note.trim(),
                tax_office: values.tax_office.trim(),
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput
            name="tags"
            label={t("customers.fields.tags")}
            description={t("customers.form.tags_hint")}
          />
          {customer.customer_type === "corporate" ? (
            <AppInput
              name="tax_office"
              label={t("customers.fields.tax_office")}
            />
          ) : null}
          <AppTextarea name="note" label={t("customers.fields.note")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
