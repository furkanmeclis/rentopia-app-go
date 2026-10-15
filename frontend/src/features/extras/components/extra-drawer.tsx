"use client";

import { useMemo, type ReactNode } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import {
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
  CHARGE_HINT_KEYS,
  CHARGE_LABEL_KEYS,
  CHARGE_TYPES,
  EXTRA_STATUSES,
  EXTRA_TYPES,
  STATUS_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/extras/constants";
import {
  useCreateExtra,
  useUpdateExtra,
} from "@/features/extras/hooks/use-extras";
import type {
  ChargeType,
  Extra,
  ExtraStatus,
  ExtraType,
} from "@/features/extras/types";
import { useChannelOptions } from "@/features/pricing/hooks/use-pricing-options";
import { isAmount, normalizeAmount } from "@/features/pricing/lib/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "extra-form";
const INT_RE = /^\d{1,3}$/;

type Values = {
  code: string;
  name: string;
  description: string;
  extra_type: ExtraType;
  charge_type: ChargeType;
  min_quantity: string;
  max_quantity: string;
  max_total: string;
  all_channels: boolean;
  channels: string[];
  status: ExtraStatus;
  is_physical: boolean;
  sort_order: string;
  default_price: string;
};

function defaults(extra: Extra | null): Values {
  return {
    code: extra?.code ?? "",
    name: extra?.name ?? "",
    description: extra?.description ?? "",
    extra_type: extra?.extra_type ?? "child_seat",
    charge_type: extra?.charge_type ?? "per_day",
    min_quantity: String(extra?.min_quantity ?? 1),
    max_quantity: String(extra?.max_quantity ?? 1),
    max_total: extra?.max_total ?? "",
    all_channels: extra ? extra.channels == null : true,
    channels: extra?.channels ?? [],
    status: extra?.status ?? "draft",
    is_physical: extra?.is_physical ?? false,
    sort_order: String(extra?.sort_order ?? 0),
    default_price: "",
  };
}

function ChargeHint() {
  const { t } = useLocale();
  const charge = useWatch({ name: "charge_type" }) as ChargeType;
  return (
    <p className="text-muted-foreground text-sm">
      {t(CHARGE_HINT_KEYS[charge])}
    </p>
  );
}

function Dependent({
  when,
  children,
}: {
  when: (values: { all_channels: boolean; charge_type: ChargeType }) => boolean;
  children: ReactNode;
}) {
  const all = useWatch({ name: "all_channels" }) as boolean;
  const charge = useWatch({ name: "charge_type" }) as ChargeType;
  return when({ all_channels: all, charge_type: charge }) ? (
    <>{children}</>
  ) : null;
}

/** Create / edit drawer of an extra (prices and stock live on the detail page). */
export function ExtraDrawer({
  open,
  extra,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  extra: Extra | null;
  onOpenChange: (open: boolean) => void;
  onCreated?: (extra: Extra) => void;
}) {
  const { t } = useLocale();
  const create = useCreateExtra();
  const update = useUpdateExtra();
  const channels = useChannelOptions();
  const schema = useMemo(
    () =>
      z
        .object({
          code: z
            .string()
            .trim()
            .min(1, t("extras.validation.code"))
            .max(32, t("extras.validation.code")),
          name: z.string().trim().min(1, t("extras.validation.name")).max(255),
          description: z.string().max(2000),
          extra_type: z.enum([
            "additional_driver",
            "child_seat",
            "km_package",
            "delivery",
            "other",
          ]),
          charge_type: z.enum(["per_day", "per_unit", "per_rental"]),
          min_quantity: z
            .string()
            .trim()
            .refine(
              (v) => INT_RE.test(v) && Number(v) >= 1 && Number(v) <= 100,
              t("extras.validation.quantity"),
            ),
          max_quantity: z
            .string()
            .trim()
            .refine(
              (v) => INT_RE.test(v) && Number(v) >= 1 && Number(v) <= 100,
              t("extras.validation.quantity"),
            ),
          max_total: z
            .string()
            .refine(
              (v) => v.trim() === "" || isAmount(v),
              t("extras.validation.price"),
            ),
          all_channels: z.boolean(),
          channels: z.array(z.string()),
          status: z.enum(["draft", "published", "archived"]),
          is_physical: z.boolean(),
          sort_order: z
            .string()
            .trim()
            .refine(
              (v) => /^\d{1,5}$/.test(v) && Number(v) <= 10000,
              t("extras.validation.sort_order"),
            ),
          default_price: z
            .string()
            .refine(
              (v) => v.trim() === "" || isAmount(v),
              t("extras.validation.price"),
            ),
        })
        .refine(
          (v) =>
            v.charge_type === "per_rental" ||
            Number(v.max_quantity) >= Number(v.min_quantity),
          {
            path: ["max_quantity"],
            message: t("extras.validation.max_below_min"),
          },
        )
        .refine((v) => v.all_channels || v.channels.length > 0, {
          path: ["channels"],
          message: t("extras.validation.channels"),
        })
        .refine(
          (v) => !(v.is_physical && v.extra_type === "additional_driver"),
          {
            path: ["is_physical"],
            message: t("extras.validation.driver_not_physical"),
          },
        ),
    [t],
  ) as unknown as z.ZodType<Values>;

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        extra ? t("extras.drawer.edit_title") : t("extras.drawer.create_title")
      }
      description={t("extras.drawer.description")}
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
            {t("extras.actions.save")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<Values>
          key={extra?.uuid ?? "new"}
          id={FORM_ID}
          className="space-y-6 px-4"
          schema={schema}
          defaultValues={defaults(extra)}
          onSubmit={async (values) => {
            const perRental = values.charge_type === "per_rental";
            const body = {
              code: values.code.trim(),
              name: values.name.trim(),
              description: values.description.trim(),
              extra_type: values.extra_type,
              charge_type: values.charge_type,
              min_quantity: perRental ? 1 : Number(values.min_quantity),
              max_quantity: perRental ? 1 : Number(values.max_quantity),
              max_total: values.max_total.trim()
                ? normalizeAmount(values.max_total)
                : null,
              channels: values.all_channels ? null : values.channels,
              status: values.status,
              is_physical: values.is_physical,
              sort_order: Number(values.sort_order),
            };
            if (extra) {
              await update.mutateAsync({ uuid: extra.uuid, body });
            } else {
              const created = await create.mutateAsync({
                ...body,
                default_price: values.default_price.trim()
                  ? normalizeAmount(values.default_price)
                  : null,
              });
              onCreated?.(created);
            }
            onOpenChange(false);
          }}
        >
          <FormSection title={t("extras.drawer.section_identity")}>
            <div className="grid gap-4 sm:grid-cols-[10rem_1fr]">
              <AppInput
                name="code"
                label={t("extras.fields.code")}
                placeholder="SEAT"
              />
              <AppInput
                name="name"
                label={t("extras.fields.name")}
                placeholder={t("extras.fields.name_placeholder")}
              />
            </div>
            <AppTextarea
              name="description"
              label={t("extras.fields.description")}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <AppSelect
                name="extra_type"
                label={t("extras.fields.extra_type")}
                options={EXTRA_TYPES.map((v) => ({
                  value: v,
                  label: t(TYPE_LABEL_KEYS[v]),
                }))}
              />
              <AppSelect
                name="status"
                label={t("extras.fields.status")}
                description={t("extras.fields.status_hint")}
                options={EXTRA_STATUSES.map((v) => ({
                  value: v,
                  label: t(STATUS_LABEL_KEYS[v]),
                }))}
              />
            </div>
          </FormSection>
          <FormSection title={t("extras.drawer.section_charge")}>
            <AppSelect
              name="charge_type"
              label={t("extras.fields.charge_type")}
              options={CHARGE_TYPES.map((v) => ({
                value: v,
                label: t(CHARGE_LABEL_KEYS[v]),
              }))}
            />
            <ChargeHint />
            <Dependent when={(v) => v.charge_type !== "per_rental"}>
              <div className="grid gap-4 sm:grid-cols-2">
                <AppInput
                  name="min_quantity"
                  inputMode="numeric"
                  label={t("extras.fields.min_quantity")}
                />
                <AppInput
                  name="max_quantity"
                  inputMode="numeric"
                  label={t("extras.fields.max_quantity")}
                />
              </div>
            </Dependent>
            <div className="grid gap-4 sm:grid-cols-2">
              <AppInput
                name="max_total"
                inputMode="decimal"
                label={t("extras.fields.max_total")}
                description={t("extras.fields.max_total_hint")}
                placeholder="—"
              />
              {extra ? null : (
                <AppInput
                  name="default_price"
                  inputMode="decimal"
                  label={t("extras.fields.default_price")}
                  description={t("extras.fields.default_price_hint")}
                  placeholder="0,00"
                />
              )}
            </div>
          </FormSection>
          <FormSection title={t("extras.drawer.section_sales")}>
            <AppSwitch
              name="all_channels"
              label={t("extras.fields.all_channels")}
            />
            <Dependent when={(v) => !v.all_channels}>
              <AppMultiCombobox
                name="channels"
                label={t("extras.fields.channels")}
                options={channels}
                placeholder={t("extras.fields.channels_placeholder")}
              />
            </Dependent>
            <AppSwitch
              name="is_physical"
              label={t("extras.fields.is_physical")}
              description={t("extras.fields.is_physical_hint")}
            />
            <AppInput
              name="sort_order"
              inputMode="numeric"
              label={t("extras.fields.sort_order")}
            />
          </FormSection>
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
