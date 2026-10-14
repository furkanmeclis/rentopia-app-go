"use client";

import { useMemo } from "react";
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
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useProductsList } from "@/features/fleet/hooks/use-fleet";
import {
  useCreateOneWayRule,
  useUpdateOneWayRule,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import type { OneWayRule } from "@/features/fleet-ops/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "one-way-rule-form";
/** Select value of the generic rule (Radix select items cannot be empty). */
const GENERIC = "generic";
const MONEY_RE = /^\d{1,12}([.,]\d{1,2})?$/;

type Values = {
  from_branch_uuid: string;
  to_branch_uuid: string;
  rental_product_uuid: string;
  allowed: boolean;
  fee: string;
  note: string;
  is_active: boolean;
};

/** Create / edit drawer of a one-way rule. */
export function OneWayRuleDrawer({
  open,
  rule,
  onOpenChange,
}: {
  open: boolean;
  rule: OneWayRule | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const create = useCreateOneWayRule();
  const update = useUpdateOneWayRule();
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const products = useProductsList(
    { limit: 100, offset: 0, sort: "code" },
    open && can(permissions.tenant.fleet.read),
  );
  const schema = useMemo(
    () =>
      z
        .object({
          from_branch_uuid: z
            .string()
            .min(1, t("one_way_rules.validation.from")),
          to_branch_uuid: z.string().min(1, t("one_way_rules.validation.to")),
          rental_product_uuid: z.string(),
          allowed: z.boolean(),
          fee: z
            .string()
            .trim()
            .refine(
              (v) => v === "" || MONEY_RE.test(v),
              t("one_way_rules.validation.fee"),
            ),
          note: z.string().max(2000),
          is_active: z.boolean(),
        })
        .refine((v) => v.from_branch_uuid !== v.to_branch_uuid, {
          path: ["to_branch_uuid"],
          message: t("one_way_rules.validation.same_branch"),
        }),
    [t],
  );

  const branchOptions = (branches.data?.items ?? []).map((b) => ({
    value: b.uuid,
    label: `${b.code} · ${b.name}`,
  }));
  const productOptions = [
    { value: GENERIC, label: t("one_way_rules.fields.all_products") },
    ...(products.data?.items ?? []).map((p) => ({
      value: p.uuid,
      label: `${p.code} · ${p.title}`,
    })),
  ];
  if (
    rule?.rental_product &&
    !productOptions.some((o) => o.value === rule.rental_product?.uuid)
  ) {
    productOptions.push({
      value: rule.rental_product.uuid,
      label: `${rule.rental_product.code} · ${rule.rental_product.title}`,
    });
  }

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        rule
          ? t("one_way_rules.drawer.edit_title")
          : t("one_way_rules.drawer.create_title")
      }
      description={t("one_way_rules.drawer.description")}
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
          <Button
            type="submit"
            form={FORM_ID}
            disabled={create.isPending || update.isPending}
          >
            {t("one_way_rules.actions.save")}
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
                  from_branch_uuid: rule.from_branch.uuid,
                  to_branch_uuid: rule.to_branch.uuid,
                  rental_product_uuid: rule.rental_product?.uuid ?? GENERIC,
                  allowed: rule.allowed,
                  fee: rule.fee,
                  note: rule.note,
                  is_active: rule.is_active,
                }
              : {
                  from_branch_uuid: "",
                  to_branch_uuid: "",
                  rental_product_uuid: GENERIC,
                  allowed: true,
                  fee: "",
                  note: "",
                  is_active: true,
                }
          }
          onSubmit={async (values) => {
            const body = {
              from_branch_uuid: values.from_branch_uuid,
              to_branch_uuid: values.to_branch_uuid,
              rental_product_uuid:
                values.rental_product_uuid === GENERIC
                  ? null
                  : values.rental_product_uuid,
              allowed: values.allowed,
              fee: values.fee.trim().replace(",", ".") || "0",
              note: values.note.trim(),
              is_active: values.is_active,
            };
            if (rule) {
              await update.mutateAsync({ uuid: rule.uuid, body });
            } else {
              await create.mutateAsync(body);
            }
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="from_branch_uuid"
            label={t("one_way_rules.fields.from_branch")}
            placeholder={t("one_way_rules.fields.branch_placeholder")}
            options={branchOptions}
          />
          <AppSelect
            name="to_branch_uuid"
            label={t("one_way_rules.fields.to_branch")}
            placeholder={t("one_way_rules.fields.branch_placeholder")}
            options={branchOptions}
          />
          <AppSelect
            name="rental_product_uuid"
            label={t("one_way_rules.fields.rental_product")}
            description={t("one_way_rules.fields.rental_product_hint")}
            options={productOptions}
          />
          <AppSwitch
            name="allowed"
            label={t("one_way_rules.fields.allowed")}
            description={t("one_way_rules.fields.allowed_hint")}
          />
          <AppInput
            name="fee"
            inputMode="decimal"
            label={t("one_way_rules.fields.fee")}
            placeholder="0.00"
          />
          <AppTextarea name="note" label={t("one_way_rules.fields.note")} />
          <AppSwitch
            name="is_active"
            label={t("one_way_rules.fields.is_active")}
          />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
