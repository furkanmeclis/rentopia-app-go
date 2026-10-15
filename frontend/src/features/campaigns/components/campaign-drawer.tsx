"use client";

import { useMemo, type ReactNode } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormSection,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  CAMPAIGN_SCOPES,
  CAMPAIGN_TYPES,
  DISCOUNT_KINDS,
  KIND_LABEL_KEYS,
  SCOPE_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/campaigns/constants";
import {
  useCreateCampaign,
  useUpdateCampaign,
} from "@/features/campaigns/hooks/use-campaigns";
import type {
  Campaign,
  CampaignCreateRequest,
  CampaignScope,
  CampaignType,
  DiscountKind,
} from "@/features/campaigns/types";
import {
  useChannelOptions,
  useProductOptions,
  useSegmentOptions,
} from "@/features/pricing/hooks/use-pricing-options";
import { isAmount, normalizeAmount } from "@/features/pricing/lib/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "campaign-form";
const CODE_RE = /^[A-Za-z0-9][A-Za-z0-9_-]{2,31}$/;
const LIMIT_RE = /^\d{1,8}$/;

type Values = {
  name: string;
  description: string;
  campaign_type: CampaignType;
  code: string;
  discount_kind: DiscountKind;
  discount_value: string;
  max_discount: string;
  min_subtotal: string;
  start_date: string;
  end_date: string;
  all_channels: boolean;
  channels: string[];
  product_scope: CampaignScope;
  segment_uuids: string[];
  rental_product_uuids: string[];
  usage_limit: string;
  per_customer_limit: string;
  min_completed_rentals: string;
  release_on_cancel: boolean;
  corporate_eligible: boolean;
  is_active: boolean;
};

function defaults(c: Campaign | null): Values {
  return {
    name: c?.name ?? "",
    description: c?.description ?? "",
    campaign_type: c?.campaign_type ?? "coupon",
    code: c?.code ?? "",
    discount_kind: c?.discount_kind ?? "percent",
    discount_value: c?.discount_value ?? "",
    max_discount: c?.max_discount ?? "",
    min_subtotal: c?.min_subtotal ?? "",
    start_date: c?.start_date ?? "",
    end_date: c?.end_date ?? "",
    all_channels: c ? c.channels == null : true,
    channels: c?.channels ?? [],
    product_scope: c?.product_scope ?? "all",
    segment_uuids: c?.segments.map((s) => s.uuid) ?? [],
    rental_product_uuids: c?.products.map((p) => p.uuid) ?? [],
    usage_limit: c?.usage_limit != null ? String(c.usage_limit) : "",
    per_customer_limit:
      c?.per_customer_limit != null ? String(c.per_customer_limit) : "",
    min_completed_rentals: String(c?.min_completed_rentals ?? 1),
    release_on_cancel: c?.release_on_cancel ?? true,
    corporate_eligible: c?.corporate_eligible ?? false,
    is_active: c?.is_active ?? true,
  };
}

function body(v: Values): CampaignCreateRequest {
  const money = (raw: string) => (raw.trim() ? normalizeAmount(raw) : null);
  return {
    name: v.name.trim(),
    description: v.description.trim(),
    campaign_type: v.campaign_type,
    code: v.campaign_type === "coupon" ? v.code.trim().toUpperCase() : null,
    discount_kind: v.discount_kind,
    discount_value: normalizeAmount(v.discount_value),
    max_discount: money(v.max_discount),
    min_subtotal: money(v.min_subtotal),
    start_date: v.start_date || null,
    end_date: v.end_date || null,
    channels: v.all_channels ? null : v.channels,
    product_scope: v.product_scope,
    segment_uuids: v.product_scope === "segments" ? v.segment_uuids : [],
    rental_product_uuids:
      v.product_scope === "products" ? v.rental_product_uuids : [],
    usage_limit: v.usage_limit.trim() ? Number(v.usage_limit) : null,
    per_customer_limit: v.per_customer_limit.trim()
      ? Number(v.per_customer_limit)
      : null,
    min_completed_rentals: Number(v.min_completed_rentals),
    release_on_cancel: v.release_on_cancel,
    corporate_eligible: v.corporate_eligible,
    is_active: v.is_active,
  };
}

function Watch<K extends keyof Values>({
  name,
  is,
  children,
}: {
  name: K;
  is: (value: Values[K]) => boolean;
  children: ReactNode;
}) {
  const value = useWatch({ name }) as Values[K];
  return is(value) ? <>{children}</> : null;
}

/** Create / edit drawer of a campaign or coupon. */
export function CampaignDrawer({
  open,
  campaign,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  campaign: Campaign | null;
  onOpenChange: (open: boolean) => void;
  onCreated?: (campaign: Campaign) => void;
}) {
  const { t } = useLocale();
  const create = useCreateCampaign();
  const update = useUpdateCampaign();
  const channels = useChannelOptions();
  const products = useProductOptions();
  const segments = useSegmentOptions(products.segments);
  const schema = useMemo(
    () =>
      z
        .object({
          name: z
            .string()
            .trim()
            .min(1, t("campaigns.validation.name"))
            .max(255),
          description: z.string().max(2000),
          campaign_type: z.enum(["coupon", "repeat_customer"]),
          code: z.string(),
          discount_kind: z.enum(["percent", "amount"]),
          discount_value: z
            .string()
            .refine(
              (v) => isAmount(v) && Number(normalizeAmount(v)) > 0,
              t("campaigns.validation.value"),
            ),
          max_discount: z
            .string()
            .refine(
              (v) => v.trim() === "" || isAmount(v),
              t("campaigns.validation.amount"),
            ),
          min_subtotal: z
            .string()
            .refine(
              (v) => v.trim() === "" || isAmount(v),
              t("campaigns.validation.amount"),
            ),
          start_date: z.string(),
          end_date: z.string(),
          all_channels: z.boolean(),
          channels: z.array(z.string()),
          product_scope: z.enum(["all", "segments", "products"]),
          segment_uuids: z.array(z.string()),
          rental_product_uuids: z.array(z.string()),
          usage_limit: z
            .string()
            .refine(
              (v) =>
                v.trim() === "" || (LIMIT_RE.test(v.trim()) && Number(v) >= 1),
              t("campaigns.validation.limit"),
            ),
          per_customer_limit: z
            .string()
            .refine(
              (v) =>
                v.trim() === "" ||
                (LIMIT_RE.test(v.trim()) &&
                  Number(v) >= 1 &&
                  Number(v) <= 1000),
              t("campaigns.validation.limit"),
            ),
          min_completed_rentals: z
            .string()
            .refine(
              (v) =>
                LIMIT_RE.test(v.trim()) && Number(v) >= 1 && Number(v) <= 1000,
              t("campaigns.validation.min_rentals"),
            ),
          release_on_cancel: z.boolean(),
          corporate_eligible: z.boolean(),
          is_active: z.boolean(),
        })
        .refine(
          (v) => v.campaign_type !== "coupon" || CODE_RE.test(v.code.trim()),
          {
            path: ["code"],
            message: t("campaigns.validation.code"),
          },
        )
        .refine(
          (v) =>
            v.discount_kind !== "percent" ||
            Number(normalizeAmount(v.discount_value)) <= 100,
          {
            path: ["discount_value"],
            message: t("campaigns.validation.percent"),
          },
        )
        .refine(
          (v) => !v.start_date || !v.end_date || v.end_date >= v.start_date,
          {
            path: ["end_date"],
            message: t("campaigns.validation.end_before_start"),
          },
        )
        .refine((v) => v.all_channels || v.channels.length > 0, {
          path: ["channels"],
          message: t("campaigns.validation.channels"),
        })
        .refine(
          (v) => v.product_scope !== "segments" || v.segment_uuids.length > 0,
          {
            path: ["segment_uuids"],
            message: t("campaigns.validation.segments"),
          },
        )
        .refine(
          (v) =>
            v.product_scope !== "products" || v.rental_product_uuids.length > 0,
          {
            path: ["rental_product_uuids"],
            message: t("campaigns.validation.products"),
          },
        ),
    [t],
  ) as unknown as z.ZodType<Values>;

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        campaign
          ? t("campaigns.drawer.edit_title")
          : t("campaigns.drawer.create_title")
      }
      description={t("campaigns.drawer.description")}
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
          <Button
            type="submit"
            form={FORM_ID}
            disabled={create.isPending || update.isPending}
          >
            {t("campaigns.actions.save")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<Values>
          key={campaign?.uuid ?? "new"}
          id={FORM_ID}
          className="space-y-6 px-4"
          schema={schema}
          defaultValues={defaults(campaign)}
          onSubmit={async (values) => {
            if (campaign) {
              await update.mutateAsync({
                uuid: campaign.uuid,
                body: body(values),
              });
            } else {
              const created = await create.mutateAsync(body(values));
              onCreated?.(created);
            }
            onOpenChange(false);
          }}
        >
          <FormSection title={t("campaigns.drawer.section_identity")}>
            <AppInput name="name" label={t("campaigns.fields.name")} />
            <div className="grid gap-4 sm:grid-cols-2">
              <AppSelect
                name="campaign_type"
                label={t("campaigns.fields.campaign_type")}
                options={CAMPAIGN_TYPES.map((v) => ({
                  value: v,
                  label: t(TYPE_LABEL_KEYS[v]),
                }))}
              />
              <Watch name="campaign_type" is={(v) => v === "coupon"}>
                <AppInput
                  name="code"
                  label={t("campaigns.fields.code")}
                  description={t("campaigns.fields.code_hint")}
                  placeholder="YAZ20"
                />
              </Watch>
              <Watch name="campaign_type" is={(v) => v === "repeat_customer"}>
                <AppInput
                  name="min_completed_rentals"
                  inputMode="numeric"
                  label={t("campaigns.fields.min_completed_rentals")}
                  description={t("campaigns.fields.min_completed_rentals_hint")}
                />
              </Watch>
            </div>
            <AppTextarea
              name="description"
              label={t("campaigns.fields.description")}
            />
          </FormSection>
          <FormSection title={t("campaigns.drawer.section_discount")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <AppSelect
                name="discount_kind"
                label={t("campaigns.fields.discount_kind")}
                options={DISCOUNT_KINDS.map((v) => ({
                  value: v,
                  label: t(KIND_LABEL_KEYS[v]),
                }))}
              />
              <AppInput
                name="discount_value"
                inputMode="decimal"
                label={t("campaigns.fields.discount_value")}
                description={t("campaigns.fields.discount_value_hint")}
              />
              <AppInput
                name="max_discount"
                inputMode="decimal"
                label={t("campaigns.fields.max_discount")}
                placeholder="—"
              />
              <AppInput
                name="min_subtotal"
                inputMode="decimal"
                label={t("campaigns.fields.min_subtotal")}
                placeholder="—"
              />
            </div>
          </FormSection>
          <FormSection title={t("campaigns.drawer.section_validity")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <AppDatePicker
                name="start_date"
                label={t("campaigns.fields.start_date")}
              />
              <AppDatePicker
                name="end_date"
                label={t("campaigns.fields.end_date")}
                description={t("campaigns.fields.end_date_hint")}
              />
            </div>
            <AppSwitch
              name="all_channels"
              label={t("campaigns.fields.all_channels")}
            />
            <Watch name="all_channels" is={(v) => !v}>
              <AppMultiCombobox
                name="channels"
                label={t("campaigns.fields.channels")}
                options={channels}
                placeholder={t("campaigns.fields.channels_placeholder")}
              />
            </Watch>
            <AppSelect
              name="product_scope"
              label={t("campaigns.fields.product_scope")}
              options={CAMPAIGN_SCOPES.map((v) => ({
                value: v,
                label: t(SCOPE_LABEL_KEYS[v]),
              }))}
            />
            <Watch name="product_scope" is={(v) => v === "segments"}>
              <AppMultiCombobox
                name="segment_uuids"
                label={t("campaigns.fields.segments")}
                options={segments}
                placeholder={t("campaigns.fields.segments_placeholder")}
              />
            </Watch>
            <Watch name="product_scope" is={(v) => v === "products"}>
              <AppMultiCombobox
                name="rental_product_uuids"
                label={t("campaigns.fields.products")}
                options={products.options}
                placeholder={t("campaigns.fields.products_placeholder")}
              />
            </Watch>
          </FormSection>
          <FormSection title={t("campaigns.drawer.section_limits")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <AppInput
                name="usage_limit"
                inputMode="numeric"
                label={t("campaigns.fields.usage_limit")}
                description={t("campaigns.fields.usage_limit_hint")}
                placeholder="∞"
              />
              <AppInput
                name="per_customer_limit"
                inputMode="numeric"
                label={t("campaigns.fields.per_customer_limit")}
                placeholder="∞"
              />
            </div>
            <AppSwitch
              name="release_on_cancel"
              label={t("campaigns.fields.release_on_cancel")}
              description={t("campaigns.fields.release_on_cancel_hint")}
            />
            <AppSwitch
              name="corporate_eligible"
              label={t("campaigns.fields.corporate_eligible")}
              description={t("campaigns.fields.corporate_eligible_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("campaigns.fields.is_active")}
            />
          </FormSection>
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
