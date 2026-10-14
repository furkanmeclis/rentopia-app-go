"use client";

import Link from "next/link";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useUpdateSegment } from "@/features/fleet/hooks/use-fleet";
import type { Segment } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "segment-quick-edit";

/** Drawer for the fields people change most, without leaving the list. */
export function SegmentQuickEditDrawer({
  segment,
  onOpenChange,
}: {
  segment: Segment | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const update = useUpdateSegment();
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("fleet.validation.name_required"))
          .max(255),
        sort_order: z
          .string()
          .trim()
          .refine((v) => {
            const n = Number(v);
            return v !== "" && Number.isInteger(n) && n >= 0 && n <= 100000;
          }, t("fleet.validation.sort_order")),
        is_active: z.boolean(),
      }),
    [t],
  );

  return (
    <EntityDrawer
      open={Boolean(segment)}
      onOpenChange={onOpenChange}
      title={t("fleet.quick_edit.title")}
      description={
        segment
          ? t("fleet.quick_edit.description", {
              code: segment.code,
              name: segment.name,
            })
          : undefined
      }
      size="md"
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          {segment ? (
            <Button asChild variant="link" size="sm" className="px-0">
              <Link
                href={routes.tenant.fleet.segments.edit(slug, segment.uuid)}
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
      {segment ? (
        <AppForm<z.infer<typeof schema>>
          key={segment.uuid}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            name: segment.name,
            sort_order: String(segment.sort_order),
            is_active: segment.is_active,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              uuid: segment.uuid,
              body: {
                name: values.name.trim(),
                sort_order: Number(values.sort_order),
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput name="name" label={t("fleet.segments.fields.name")} />
          <AppInput
            name="sort_order"
            type="number"
            min={0}
            label={t("fleet.segments.fields.sort_order")}
          />
          <AppSwitch name="is_active" label={t("fleet.fields.is_active")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
