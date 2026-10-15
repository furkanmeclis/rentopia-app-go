"use client";

import { useMemo } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  RULE_TYPE_HINT_KEYS,
  RULE_TYPE_LABEL_KEYS,
  RULE_TYPES,
} from "@/features/availability/constants";
import { useSaveCapacityRule } from "@/features/availability/hooks/use-availability";
import type {
  CapacityRule,
  CapacityRuleType,
} from "@/features/availability/types";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useProductsList } from "@/features/fleet/hooks/use-fleet";
import {
  SALES_CHANNEL_LABEL_KEYS,
  SALES_CHANNELS,
} from "@/features/tenant-settings/constants";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "capacity-rule-form";
/** Select value of "every product / branch / channel" (items cannot be empty). */
const ALL = "all";

type Values = {
  rule_type: string;
  channel: string;
  product_uuid: string;
  branch_uuid: string;
  quantity: string;
  note: string;
  is_active: boolean;
};

/** The hint of the selected rule type (what the quantity means). */
function RuleTypeHint() {
  const { t } = useLocale();
  const ruleType = useWatch<Values, "rule_type">({ name: "rule_type" });
  if (!ruleType) return null;
  return (
    <p className="text-muted-foreground text-sm">
      {t(RULE_TYPE_HINT_KEYS[ruleType as CapacityRuleType])}
    </p>
  );
}

/** Create / edit drawer of a channel capacity rule. */
export function CapacityRuleDrawer({
  open,
  rule,
  onOpenChange,
}: {
  open: boolean;
  rule: CapacityRule | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const save = useSaveCapacityRule();
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const products = useProductsList(
    { limit: 100, offset: 0, sort: "code" },
    open && can(permissions.tenant.fleet.read),
  );
  const schema = useMemo(
    () =>
      z
        .object({
          rule_type: z.string().min(1),
          channel: z.string(),
          product_uuid: z.string(),
          branch_uuid: z.string(),
          quantity: z
            .string()
            .trim()
            .refine((v) => {
              const n = Number(v);
              return Number.isInteger(n) && n >= 0 && n <= 10000;
            }, t("availability.rules.validation.quantity")),
          note: z.string().max(2000),
          is_active: z.boolean(),
        })
        .refine((v) => v.rule_type === "safety_margin" || v.channel !== ALL, {
          path: ["channel"],
          message: t("availability.rules.validation.channel"),
        }),
    [t],
  );

  const typeOptions = RULE_TYPES.map((type) => ({
    value: type,
    label: t(RULE_TYPE_LABEL_KEYS[type]),
  }));
  const channelOptions = [
    { value: ALL, label: t("availability.rules.fields.all_channels") },
    ...SALES_CHANNELS.map((code) => ({
      value: code,
      label: t(SALES_CHANNEL_LABEL_KEYS[code]),
    })),
  ];
  const productOptions = [
    { value: ALL, label: t("availability.rules.fields.all_products") },
    ...(products.data?.items ?? []).map((p) => ({
      value: p.uuid,
      label: `${p.code} · ${p.title}`,
    })),
  ];
  if (
    rule?.product &&
    !productOptions.some((o) => o.value === rule.product?.uuid)
  ) {
    productOptions.push({
      value: rule.product.uuid,
      label: `${rule.product.code} · ${rule.product.title}`,
    });
  }
  const branchOptions = [
    { value: ALL, label: t("availability.rules.fields.all_branches") },
    ...(branches.data?.items ?? []).map((b) => ({
      value: b.uuid,
      label: `${b.code} · ${b.name}`,
    })),
  ];

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        rule
          ? t("availability.rules.drawer.edit_title")
          : t("availability.rules.drawer.create_title")
      }
      description={t("availability.rules.drawer.description")}
      size="md"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {t("availability.rules.actions.save")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<Values>
          key={rule?.uuid ?? "new"}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={
            rule
              ? {
                  rule_type: rule.rule_type,
                  channel: rule.channel ?? ALL,
                  product_uuid: rule.product?.uuid ?? ALL,
                  branch_uuid: rule.branch?.uuid ?? ALL,
                  quantity: String(rule.quantity),
                  note: rule.note,
                  is_active: rule.is_active,
                }
              : {
                  rule_type: "safety_margin",
                  channel: ALL,
                  product_uuid: ALL,
                  branch_uuid: ALL,
                  quantity: "1",
                  note: "",
                  is_active: true,
                }
          }
          onSubmit={async (values) => {
            await save.mutateAsync({
              uuid: rule?.uuid ?? null,
              body: {
                rule_type: values.rule_type as CapacityRuleType,
                channel: values.channel === ALL ? null : values.channel,
                product_uuid:
                  values.product_uuid === ALL ? null : values.product_uuid,
                branch_uuid:
                  values.branch_uuid === ALL ? null : values.branch_uuid,
                quantity: Number(values.quantity),
                note: values.note.trim(),
                is_active: values.is_active,
              },
            });
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="rule_type"
            label={t("availability.rules.fields.rule_type")}
            options={typeOptions}
          />
          <RuleTypeHint />
          <AppSelect
            name="channel"
            label={t("availability.rules.fields.channel")}
            options={channelOptions}
          />
          <AppSelect
            name="product_uuid"
            label={t("availability.rules.fields.product")}
            description={t("availability.rules.fields.product_hint")}
            options={productOptions}
          />
          <AppSelect
            name="branch_uuid"
            label={t("availability.rules.fields.branch")}
            description={t("availability.rules.fields.branch_hint")}
            options={branchOptions}
          />
          <AppInput
            name="quantity"
            type="number"
            inputMode="numeric"
            min={0}
            max={10000}
            label={t("availability.rules.fields.quantity")}
          />
          <AppTextarea
            name="note"
            label={t("availability.rules.fields.note")}
          />
          <AppSwitch
            name="is_active"
            label={t("availability.rules.fields.is_active")}
          />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
