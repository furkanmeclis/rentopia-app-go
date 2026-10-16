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
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  ENTITY_TYPE_LABEL_KEYS,
  ENTITY_TYPES,
} from "@/features/channels/constants";
import { useSaveChannelMapping } from "@/features/channels/hooks/use-channels";
import type {
  ChannelAccount,
  ChannelEntityType,
  ChannelMapping,
  ChannelMappingInput,
} from "@/features/channels/types";
import { useExtrasList } from "@/features/extras/hooks/use-extras";
import { useProductsList } from "@/features/fleet/hooks/use-fleet";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "channel-mapping-form";
/** Select value of "every mapped location" (items cannot be empty). */
const ALL = "all";

type Values = {
  account_uuid: string;
  entity_type: string;
  branch_uuid: string;
  product_uuid: string;
  extra_uuid: string;
  external_code: string;
  external_name: string;
  priority: string;
  stop_sale: boolean;
  max_quota: string;
  is_active: boolean;
  note: string;
};

/** Fields of the selected entity type. */
function EntityFields({
  editing,
  branchOptions,
  productOptions,
  extraOptions,
}: {
  editing: boolean;
  branchOptions: { value: string; label: string }[];
  productOptions: { value: string; label: string }[];
  extraOptions: { value: string; label: string }[];
}) {
  const { t } = useLocale();
  const type = useWatch<Values, "entity_type">({ name: "entity_type" });
  return (
    <>
      {type === "branch" ? (
        <AppSelect
          name="branch_uuid"
          label={t("channels.mappings.fields.branch")}
          options={branchOptions}
        />
      ) : null}
      {type === "product" ? (
        <>
          <AppSelect
            name="product_uuid"
            label={t("channels.mappings.fields.product")}
            options={productOptions}
            disabled={editing}
          />
          <AppSelect
            name="branch_uuid"
            label={t("channels.mappings.fields.location_scope")}
            description={t("channels.mappings.fields.location_scope_hint")}
            options={[
              {
                value: ALL,
                label: t("channels.mappings.fields.all_locations"),
              },
              ...branchOptions,
            ]}
          />
        </>
      ) : null}
      {type === "extra" ? (
        <AppSelect
          name="extra_uuid"
          label={t("channels.mappings.fields.extra")}
          options={extraOptions}
          disabled={editing}
        />
      ) : null}
    </>
  );
}

/** Product-only fields (priority, stop-sale, quota). */
function ProductFields() {
  const { t } = useLocale();
  const type = useWatch<Values, "entity_type">({ name: "entity_type" });
  if (type !== "product") return null;
  return (
    <>
      <AppInput
        name="priority"
        type="number"
        inputMode="numeric"
        label={t("channels.mappings.fields.priority")}
        description={t("channels.mappings.fields.priority_hint")}
      />
      <AppInput
        name="max_quota"
        type="number"
        inputMode="numeric"
        min={0}
        label={t("channels.mappings.fields.max_quota")}
        description={t("channels.mappings.fields.max_quota_hint")}
      />
      <AppSwitch
        name="stop_sale"
        label={t("channels.mappings.fields.stop_sale")}
        description={t("channels.mappings.fields.stop_sale_hint")}
      />
    </>
  );
}

/** Create / edit drawer of a channel mapping. */
export function ChannelMappingDrawer({
  open,
  mapping,
  accounts,
  onOpenChange,
}: {
  open: boolean;
  mapping: ChannelMapping | null;
  accounts: ChannelAccount[];
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const save = useSaveChannelMapping();
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );
  const products = useProductsList(
    { limit: 100, offset: 0, sort: "code" },
    open && can(permissions.tenant.fleet.read),
  );
  const extras = useExtrasList(
    { limit: 100, offset: 0, sort: "code" },
    open && can(permissions.tenant.extras.read),
  );
  const schema = useMemo(
    () =>
      z
        .object({
          account_uuid: z.string().min(1, t("channels.validation.account")),
          entity_type: z.string().min(1),
          branch_uuid: z.string(),
          product_uuid: z.string(),
          extra_uuid: z.string(),
          external_code: z
            .string()
            .trim()
            .min(1, t("channels.validation.code"))
            .max(64, t("channels.validation.code")),
          external_name: z.string().max(255),
          priority: z.string().refine((v) => Number.isInteger(Number(v)), {
            message: t("channels.validation.integer"),
          }),
          stop_sale: z.boolean(),
          max_quota: z
            .string()
            .refine(
              (v) =>
                v.trim() === "" ||
                (Number.isInteger(Number(v)) && Number(v) >= 0),
              t("channels.validation.quota"),
            ),
          is_active: z.boolean(),
          note: z.string().max(1000),
        })
        .refine((v) => v.entity_type !== "branch" || v.branch_uuid !== ALL, {
          path: ["branch_uuid"],
          message: t("channels.validation.branch"),
        })
        .refine((v) => v.entity_type !== "product" || v.product_uuid !== "", {
          path: ["product_uuid"],
          message: t("channels.validation.product"),
        })
        .refine((v) => v.entity_type !== "extra" || v.extra_uuid !== "", {
          path: ["extra_uuid"],
          message: t("channels.validation.extra"),
        }),
    [t],
  );
  const branchOptions = (branches.data?.items ?? []).map((b) => ({
    value: b.uuid,
    label: `${b.code} · ${b.name}`,
  }));
  const productOptions = (products.data?.items ?? []).map((p) => ({
    value: p.uuid,
    label: `${p.code} · ${p.title}`,
  }));
  const extraOptions = (extras.data?.items ?? []).map((e) => ({
    value: e.uuid,
    label: `${e.code} · ${e.name}`,
  }));
  // The edited record stays selectable even beyond the first page.
  const keep = (
    list: { value: string; label: string }[],
    ref: { uuid: string; code?: string; name?: string } | null | undefined,
  ) => {
    if (ref && !list.some((o) => o.value === ref.uuid)) {
      list.push({
        value: ref.uuid,
        label: `${ref.code ?? ""} · ${ref.name ?? ""}`,
      });
    }
  };
  keep(branchOptions, mapping?.branch);
  keep(productOptions, mapping?.product);
  keep(extraOptions, mapping?.extra);
  const accountOptions = accounts.map((a) => ({
    value: a.uuid,
    label: a.display_name,
  }));

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        mapping
          ? t("channels.mappings.drawer.edit_title")
          : t("channels.mappings.drawer.create_title")
      }
      description={t("channels.mappings.drawer.description")}
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
            {t("channels.actions.save")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<Values>
          key={mapping?.uuid ?? "new"}
          id={FORM_ID}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={
            mapping
              ? {
                  account_uuid: mapping.account.uuid,
                  entity_type: mapping.entity_type,
                  branch_uuid: mapping.branch?.uuid ?? ALL,
                  product_uuid: mapping.product?.uuid ?? "",
                  extra_uuid: mapping.extra?.uuid ?? "",
                  external_code: mapping.external_code,
                  external_name: mapping.external_name,
                  priority: String(mapping.priority),
                  stop_sale: mapping.stop_sale,
                  max_quota:
                    mapping.max_quota === null ? "" : String(mapping.max_quota),
                  is_active: mapping.is_active,
                  note: mapping.note,
                }
              : {
                  account_uuid: accounts[0]?.uuid ?? "",
                  entity_type: "branch",
                  branch_uuid: ALL,
                  product_uuid: "",
                  extra_uuid: "",
                  external_code: "",
                  external_name: "",
                  priority: "0",
                  stop_sale: false,
                  max_quota: "",
                  is_active: true,
                  note: "",
                }
          }
          onSubmit={async (v) => {
            const type = v.entity_type as ChannelEntityType;
            const body: ChannelMappingInput = {
              external_code: v.external_code.trim(),
              external_name: v.external_name.trim(),
              is_active: v.is_active,
              note: v.note.trim(),
            };
            if (type === "product") {
              body.priority = Number(v.priority);
              body.stop_sale = v.stop_sale;
              body.max_quota =
                v.max_quota.trim() === "" ? null : Number(v.max_quota);
              body.branch_uuid = v.branch_uuid === ALL ? null : v.branch_uuid;
            }
            if (type === "branch") body.branch_uuid = v.branch_uuid;
            if (!mapping) {
              body.account_uuid = v.account_uuid;
              body.entity_type = type;
              if (type === "product") body.product_uuid = v.product_uuid;
              if (type === "extra") body.extra_uuid = v.extra_uuid;
            }
            await save.mutateAsync({ uuid: mapping?.uuid ?? null, body });
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="account_uuid"
            label={t("channels.mappings.fields.account")}
            options={accountOptions}
            disabled={Boolean(mapping)}
          />
          <AppSelect
            name="entity_type"
            label={t("channels.mappings.fields.entity_type")}
            options={ENTITY_TYPES.map((v) => ({
              value: v,
              label: t(ENTITY_TYPE_LABEL_KEYS[v]),
            }))}
            disabled={Boolean(mapping)}
          />
          <EntityFields
            editing={Boolean(mapping)}
            branchOptions={branchOptions}
            productOptions={productOptions}
            extraOptions={extraOptions}
          />
          <AppInput
            name="external_code"
            label={t("channels.mappings.fields.external_code")}
            description={t("channels.mappings.fields.external_code_hint")}
          />
          <AppInput
            name="external_name"
            label={t("channels.mappings.fields.external_name")}
          />
          <ProductFields />
          <AppSwitch
            name="is_active"
            label={t("channels.mappings.fields.is_active")}
          />
          <AppTextarea name="note" label={t("channels.mappings.fields.note")} />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
