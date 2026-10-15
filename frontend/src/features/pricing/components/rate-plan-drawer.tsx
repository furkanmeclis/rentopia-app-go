"use client";

import { useRouter } from "next/navigation";
import { useMemo } from "react";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { FormSection } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  BandsEditor,
  bandRowsSchema,
  rowsToBands,
  type BandRow,
} from "@/features/pricing/components/bands-editor";
import {
  BRANCH_SCOPES,
  CHANNEL_SCOPES,
  DEFAULT_BANDS,
  PLAN_SCOPES,
  SCOPE_HINT_KEYS,
  SCOPE_LABEL_KEYS,
} from "@/features/pricing/constants";
import { useCreateRatePlan } from "@/features/pricing/hooks/use-pricing";
import {
  useBranchOptions,
  useChannelOptions,
  useProductOptions,
} from "@/features/pricing/hooks/use-pricing-options";
import type { RatePlanScope } from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "rate-plan-create-form";

type Values = {
  rental_product_uuid: string;
  scope: RatePlanScope;
  branch_uuid: string;
  channel: string;
  name: string;
  priority: string;
  bands: BandRow[];
};

/**
 * New rate plan (draft v1): product (alt grup), scope target, priority and
 * duration bands. Activation is a separate, permission-gated step.
 */
export function RatePlanCreateDrawer({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateRatePlan();
  const products = useProductOptions();
  const branches = useBranchOptions();
  const channels = useChannelOptions();

  const schema = useMemo(
    () =>
      z
        .object({
          rental_product_uuid: z
            .string()
            .min(1, t("pricing.validation.product")),
          scope: z.enum(["common", "branch", "channel", "branch_channel"]),
          branch_uuid: z.string(),
          channel: z.string(),
          name: z.string().max(255),
          priority: z
            .string()
            .trim()
            .refine(
              (v) => /^\d{1,4}$/.test(v) && Number(v) <= 1000,
              t("pricing.validation.priority"),
            ),
          bands: bandRowsSchema(t),
        })
        .refine(
          (v) => !BRANCH_SCOPES.includes(v.scope) || v.branch_uuid !== "",
          { path: ["branch_uuid"], message: t("pricing.validation.branch") },
        )
        .refine((v) => !CHANNEL_SCOPES.includes(v.scope) || v.channel !== "", {
          path: ["channel"],
          message: t("pricing.validation.channel"),
        }),
    [t],
  ) as z.ZodType<Values>;

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={t("pricing.plans.create_title")}
      description={t("pricing.plans.create_description")}
      size="lg"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("pricing.actions.save_draft")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<Values>
          id={FORM_ID}
          className="space-y-6 px-4"
          schema={schema}
          defaultValues={{
            rental_product_uuid: "",
            scope: "common",
            branch_uuid: "",
            channel: "",
            name: "",
            priority: "0",
            bands: DEFAULT_BANDS.map((b) => ({
              min_days: String(b.min_days),
              max_days: b.max_days == null ? "" : String(b.max_days),
              daily_price: "",
            })),
          }}
          onSubmit={async (values) => {
            const created = await create.mutateAsync({
              rental_product_uuid: values.rental_product_uuid,
              scope: values.scope,
              branch_uuid: BRANCH_SCOPES.includes(values.scope)
                ? values.branch_uuid
                : null,
              channel: CHANNEL_SCOPES.includes(values.scope)
                ? values.channel
                : null,
              name: values.name.trim(),
              priority: Number(values.priority),
              bands: rowsToBands(values.bands),
            });
            onOpenChange(false);
            router.push(routes.tenant.pricing.plans.detail(slug, created.uuid));
          }}
        >
          {(form) => {
            const scope = form.watch("scope");
            return (
              <>
                <FormSection title={t("pricing.plans.section_target")}>
                  <AppSelect
                    name="rental_product_uuid"
                    label={t("pricing.fields.rental_product")}
                    placeholder={t("pricing.fields.product_placeholder")}
                    options={products.options}
                  />
                  <AppSelect
                    name="scope"
                    label={t("pricing.fields.scope")}
                    description={t(SCOPE_HINT_KEYS[scope])}
                    options={PLAN_SCOPES.map((s) => ({
                      value: s,
                      label: t(SCOPE_LABEL_KEYS[s]),
                    }))}
                  />
                  {BRANCH_SCOPES.includes(scope) ? (
                    <AppSelect
                      name="branch_uuid"
                      label={t("pricing.fields.branch")}
                      placeholder={t("pricing.fields.branch_placeholder")}
                      options={branches.options}
                    />
                  ) : null}
                  {CHANNEL_SCOPES.includes(scope) ? (
                    <AppSelect
                      name="channel"
                      label={t("pricing.fields.channel")}
                      placeholder={t("pricing.fields.channel_placeholder")}
                      options={channels}
                    />
                  ) : null}
                  <AppInput
                    name="name"
                    label={t("pricing.fields.name")}
                    placeholder={t("pricing.fields.plan_name_placeholder")}
                  />
                  <AppInput
                    name="priority"
                    inputMode="numeric"
                    label={t("pricing.fields.priority")}
                    description={t("pricing.fields.priority_hint")}
                  />
                </FormSection>
                <FormSection
                  title={t("pricing.plans.section_bands")}
                  description={t("pricing.plans.section_bands_hint")}
                >
                  <BandsEditor />
                </FormSection>
              </>
            );
          }}
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
