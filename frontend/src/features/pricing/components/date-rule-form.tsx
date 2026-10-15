"use client";

import { useMemo, type ReactNode } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppSwitch,
  FormSection,
} from "@/components/forms";
import {
  KIND_LABEL_KEYS,
  PRODUCT_SCOPES,
  PRODUCT_SCOPE_LABEL_KEYS,
  RULE_KINDS,
} from "@/features/pricing/constants";
import {
  useBranchOptions,
  useChannelOptions,
  useProductOptions,
  useSegmentOptions,
} from "@/features/pricing/hooks/use-pricing-options";
import { normalizeAmount, SIGNED_RE } from "@/features/pricing/lib/format";
import type {
  DateRule,
  DateRuleCreateRequest,
  DateRuleKind,
  ProductScope,
} from "@/features/pricing/types";
import { useLocale } from "@/providers/locale-provider";

export type DateRuleValues = {
  name: string;
  start_date: string;
  end_date: string;
  product_scope: ProductScope;
  segment_uuids: string[];
  rental_product_uuids: string[];
  all_branches: boolean;
  branch_uuids: string[];
  all_channels: boolean;
  channels: string[];
  kind: DateRuleKind;
  value: string;
  priority: string;
  change_note: string;
};

/** Form defaults from a rule's effective version (draft, else active). */
export function dateRuleDefaults(rule?: DateRule | null): DateRuleValues {
  const v = rule?.draft ?? rule?.active;
  return {
    name: rule?.name ?? "",
    start_date: v?.start_date ?? "",
    end_date: v?.end_date ?? "",
    product_scope: v?.product_scope ?? "all",
    segment_uuids: v?.segments.map((s) => s.uuid) ?? [],
    rental_product_uuids: v?.rental_products.map((p) => p.uuid) ?? [],
    all_branches: v?.all_branches ?? true,
    branch_uuids: v?.branches.map((b) => b.uuid) ?? [],
    all_channels: v ? v.channels == null : true,
    channels: v?.channels ?? [],
    kind: v?.kind ?? "percent",
    value: v?.value ?? "",
    priority: String(v?.priority ?? 0),
    change_note: rule?.draft?.change_note ?? "",
  };
}

/** Form values → API body (create; PATCH sends the same fields). */
export function dateRuleBody(values: DateRuleValues): DateRuleCreateRequest {
  return {
    name: values.name.trim(),
    start_date: values.start_date,
    end_date: values.end_date,
    product_scope: values.product_scope,
    segment_uuids:
      values.product_scope === "segments" ? values.segment_uuids : [],
    rental_product_uuids:
      values.product_scope === "products" ? values.rental_product_uuids : [],
    all_branches: values.all_branches,
    branch_uuids: values.all_branches ? [] : values.branch_uuids,
    all_channels: values.all_channels,
    channels: values.all_channels ? [] : values.channels,
    kind: values.kind,
    value: normalizeAmount(values.value),
    priority: Number(values.priority),
    change_note: values.change_note.trim(),
  };
}

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export function dateRuleSchema(t: Translate, withName = true) {
  return z
    .object({
      name: withName
        ? z.string().trim().min(1, t("pricing.validation.name")).max(64)
        : z.string(),
      start_date: z.string().min(1, t("pricing.validation.start_date")),
      end_date: z.string().min(1, t("pricing.validation.end_date")),
      product_scope: z.enum(["all", "segments", "products"]),
      segment_uuids: z.array(z.string()),
      rental_product_uuids: z.array(z.string()),
      all_branches: z.boolean(),
      branch_uuids: z.array(z.string()),
      all_channels: z.boolean(),
      channels: z.array(z.string()),
      kind: z.enum(["percent", "amount"]),
      value: z
        .string()
        .trim()
        .refine(
          (v) =>
            SIGNED_RE.test(normalizeAmount(v)) &&
            Number(normalizeAmount(v)) !== 0,
          t("pricing.validation.value"),
        ),
      priority: z
        .string()
        .trim()
        .refine(
          (v) => /^\d{1,4}$/.test(v) && Number(v) <= 1000,
          t("pricing.validation.priority"),
        ),
      change_note: z.string().max(2000),
    })
    .refine((v) => !v.start_date || !v.end_date || v.end_date > v.start_date, {
      path: ["end_date"],
      message: t("pricing.validation.end_after_start"),
    })
    .refine(
      (v) => v.product_scope !== "segments" || v.segment_uuids.length > 0,
      {
        path: ["segment_uuids"],
        message: t("pricing.validation.segments"),
      },
    )
    .refine(
      (v) =>
        v.product_scope !== "products" || v.rental_product_uuids.length > 0,
      {
        path: ["rental_product_uuids"],
        message: t("pricing.validation.products"),
      },
    )
    .refine((v) => v.all_branches || v.branch_uuids.length > 0, {
      path: ["branch_uuids"],
      message: t("pricing.validation.branches"),
    })
    .refine((v) => v.all_channels || v.channels.length > 0, {
      path: ["channels"],
      message: t("pricing.validation.channels"),
    })
    .refine(
      (v) =>
        v.kind !== "percent" ||
        (Number(normalizeAmount(v.value)) >= -100 &&
          Number(normalizeAmount(v.value)) <= 1000),
      { path: ["value"], message: t("pricing.validation.percent_range") },
    );
}

/**
 * Date rule fields (create drawer, draft editor, preview draft). The range is
 * `[start, end)`: the end day is excluded ("bitiş günü hariç").
 */
export function DateRuleFields({ showName = true }: { showName?: boolean }) {
  const { t } = useLocale();
  const products = useProductOptions();
  const segments = useSegmentOptions(products.segments);
  const branches = useBranchOptions();
  const channels = useChannelOptions();
  return (
    <>
      <FormSection title={t("pricing.rules.section_when")}>
        {showName ? (
          <AppInput
            name="name"
            label={t("pricing.fields.rule_name")}
            placeholder={t("pricing.fields.rule_name_placeholder")}
          />
        ) : null}
        <div className="grid gap-4 sm:grid-cols-2">
          <AppDatePicker
            name="start_date"
            label={t("pricing.fields.start_date")}
          />
          <AppDatePicker
            name="end_date"
            label={t("pricing.fields.end_date")}
            description={t("pricing.fields.end_date_hint")}
          />
        </div>
      </FormSection>
      <FormSection title={t("pricing.rules.section_what")}>
        <div className="grid gap-4 sm:grid-cols-3">
          <AppSelect
            name="kind"
            label={t("pricing.fields.kind")}
            options={RULE_KINDS.map((k) => ({
              value: k,
              label: t(KIND_LABEL_KEYS[k]),
            }))}
          />
          <AppInput
            name="value"
            inputMode="decimal"
            label={t("pricing.fields.value")}
            description={t("pricing.fields.value_hint")}
          />
          <AppInput
            name="priority"
            inputMode="numeric"
            label={t("pricing.fields.priority")}
            description={t("pricing.fields.rule_priority_hint")}
          />
        </div>
      </FormSection>
      <ScopeFields
        products={products.options}
        segments={segments}
        branches={branches.options}
        channels={channels}
      />
    </>
  );
}

function ScopeFields({
  products,
  segments,
  branches,
  channels,
}: {
  products: { value: string; label: string }[];
  segments: { value: string; label: string }[];
  branches: { value: string; label: string }[];
  channels: { value: string; label: string }[];
}) {
  const { t } = useLocale();
  return (
    <FormSection title={t("pricing.rules.section_where")}>
      <AppSelect
        name="product_scope"
        label={t("pricing.fields.product_scope")}
        options={PRODUCT_SCOPES.map((s) => ({
          value: s,
          label: t(PRODUCT_SCOPE_LABEL_KEYS[s]),
        }))}
      />
      <ScopeDependent kind="segments">
        <AppMultiCombobox
          name="segment_uuids"
          label={t("pricing.fields.segments")}
          options={segments}
          placeholder={t("pricing.fields.segments_placeholder")}
        />
      </ScopeDependent>
      <ScopeDependent kind="products">
        <AppMultiCombobox
          name="rental_product_uuids"
          label={t("pricing.fields.products")}
          options={products}
          placeholder={t("pricing.fields.products_placeholder")}
        />
      </ScopeDependent>
      <AppSwitch name="all_branches" label={t("pricing.fields.all_branches")} />
      <SwitchDependent name="all_branches">
        <AppMultiCombobox
          name="branch_uuids"
          label={t("pricing.fields.branches")}
          options={branches}
          placeholder={t("pricing.fields.branch_placeholder")}
        />
      </SwitchDependent>
      <AppSwitch name="all_channels" label={t("pricing.fields.all_channels")} />
      <SwitchDependent name="all_channels">
        <AppMultiCombobox
          name="channels"
          label={t("pricing.fields.channels")}
          options={channels}
          placeholder={t("pricing.fields.channel_placeholder")}
        />
      </SwitchDependent>
    </FormSection>
  );
}

function ScopeDependent({
  kind,
  children,
}: {
  kind: ProductScope;
  children: ReactNode;
}) {
  const scope = useWatch({ name: "product_scope" }) as ProductScope;
  return scope === kind ? <>{children}</> : null;
}

function SwitchDependent({
  name,
  children,
}: {
  name: "all_branches" | "all_channels";
  children: ReactNode;
}) {
  const all = useWatch({ name }) as boolean;
  return all ? null : <>{children}</>;
}

/** Create / draft form of a date rule. */
export function DateRuleForm({
  id,
  defaultValues,
  onSubmit,
  withName = true,
}: {
  id: string;
  defaultValues: DateRuleValues;
  onSubmit: (values: DateRuleValues) => Promise<void>;
  withName?: boolean;
}) {
  const { t } = useLocale();
  const schema = useMemo(
    () => dateRuleSchema(t, withName) as unknown as z.ZodType<DateRuleValues>,
    [t, withName],
  );
  return (
    <AppForm<DateRuleValues>
      id={id}
      schema={schema}
      className="space-y-6"
      defaultValues={defaultValues}
      onSubmit={onSubmit}
    >
      <DateRuleFields showName={withName} />
      {withName ? null : (
        <AppInput
          name="change_note"
          label={t("pricing.fields.change_note")}
          placeholder={t("pricing.activation.note_placeholder")}
        />
      )}
    </AppForm>
  );
}
