"use client";

import Link from "next/link";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { isValidAcriss } from "@/features/fleet/constants";
import { useUpdateGroup } from "@/features/fleet/hooks/use-fleet";
import type { RentalGroup } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "group-quick-edit";

/** Drawer for the name, ACRISS code and active flag of a rental group. */
export function GroupQuickEditDrawer({
  group,
  onOpenChange,
}: {
  group: RentalGroup | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateGroup();
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("fleet.validation.name_required"))
          .max(255),
        acriss_code: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || isValidAcriss(v.toUpperCase()),
            t("fleet.validation.acriss"),
          ),
        is_active: z.boolean(),
      }),
    [t],
  );

  return (
    <EntityDrawer
      open={Boolean(group)}
      onOpenChange={onOpenChange}
      title={t("fleet.quick_edit.title")}
      description={
        group
          ? t("fleet.quick_edit.description", {
              code: group.code,
              name: group.name,
            })
          : undefined
      }
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {group ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link href={routes.tenant.fleet.groups.edit(slug, group.uuid)}>
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
      {group ? (
        <AppForm<z.infer<typeof schema>>
          key={group.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: group.name,
            acriss_code: group.acriss_code ?? "",
            is_active: group.is_active,
          }}
          onSubmit={async (values) => {
            const acriss = values.acriss_code.trim().toUpperCase();
            await update.mutateAsync({
              uuid: group.uuid,
              body: {
                name: values.name.trim(),
                acriss_code: acriss || null,
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("fleet.groups.fields.name")} />
          <AppInput
            name="acriss_code"
            label={t("fleet.groups.fields.acriss_code")}
            description={t("fleet.groups.fields.acriss_code_hint")}
            maxLength={4}
          />
          <AppSwitch name="is_active" label={t("fleet.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
