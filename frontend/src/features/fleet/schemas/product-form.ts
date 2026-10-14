import { z } from "zod";

import {
  CODE_PATTERN,
  FUEL_POLICIES,
  GUARANTEE_TYPES,
  MONEY_PATTERN,
  PRODUCT_STATUSES,
} from "@/features/fleet/constants";
import type {
  FuelPolicy,
  GuaranteeBadge,
  GuaranteeType,
  ProductStatus,
  RentalProduct,
  RentalProductCreateRequest,
} from "@/features/fleet/types";

type Translate = (key: string) => string;

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function intInRange(t: Translate, min: number, max: number, key: string) {
  return z
    .string()
    .trim()
    .refine((v) => {
      const n = Number(v);
      return v !== "" && Number.isInteger(n) && n >= min && n <= max;
    }, t(key));
}

function money(t: Translate, key: string) {
  return z
    .string()
    .trim()
    .refine((v) => v === "" || MONEY_PATTERN.test(v), t(key));
}

/**
 * Field names follow the API body (`guarantee.catalog_model_uuid`,
 * `conditions.deposit_amount`) so server validation details land on the
 * matching inputs. `*_label` fields only feed the UI.
 */
export function createProductFormSchema(t: Translate) {
  return z
    .object({
      code: z
        .string()
        .trim()
        .min(1, t("fleet.validation.code_required"))
        .regex(CODE_PATTERN, t("fleet.validation.code_format")),
      title: z
        .string()
        .trim()
        .min(1, t("fleet.validation.title_required"))
        .max(255),
      description: z.string().max(10000),
      segment_uuid: z.string().min(1, t("fleet.validation.segment_required")),
      representative_model_uuid: z.string(),
      representative_model_label: z.string(),
      representative_type_uuid: z.string(),
      representative_type_label: z.string(),
      status: z.enum(PRODUCT_STATUSES as [ProductStatus, ...ProductStatus[]]),
      is_active: z.boolean(),
      guarantee: z.object({
        type: z.enum(GUARANTEE_TYPES as [GuaranteeType, ...GuaranteeType[]]),
        vehicle_id: z.string().trim(),
        variant_level: z.enum(["model", "type"]),
        catalog_model_uuid: z.string(),
        catalog_model_label: z.string(),
        catalog_type_uuid: z.string(),
        catalog_type_label: z.string(),
        rental_group_uuid: z.string(),
        rental_group_label: z.string(),
      }),
      conditions: z.object({
        deposit_amount: money(t, "fleet.validation.money"),
        min_driver_age: intInRange(t, 18, 99, "fleet.validation.driver_age"),
        min_license_years: intInRange(
          t,
          0,
          60,
          "fleet.validation.license_years",
        ),
        unlimited_km: z.boolean(),
        included_km_per_day: z.string().trim(),
        extra_km_fee: money(t, "fleet.validation.money"),
        fuel_policy: z.enum(FUEL_POLICIES as [FuelPolicy, ...FuelPolicy[]]),
        notes: z.string().max(4000),
      }),
      change_note: z.string().max(1000),
    })
    .superRefine((values, ctx) => {
      const g = values.guarantee;
      if (g.type === "GROUP" && !g.rental_group_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["guarantee", "rental_group_uuid"],
          message: t("fleet.validation.group_required"),
        });
      }
      if (g.type === "EXACT_VARIANT") {
        if (g.variant_level === "model" && !g.catalog_model_uuid) {
          ctx.addIssue({
            code: "custom",
            path: ["guarantee", "catalog_model_uuid"],
            message: t("fleet.validation.model_required"),
          });
        }
        if (g.variant_level === "type" && !g.catalog_type_uuid) {
          ctx.addIssue({
            code: "custom",
            path: ["guarantee", "catalog_type_uuid"],
            message: t("fleet.validation.type_required"),
          });
        }
      }
      if (
        g.type === "EXACT_VEHICLE" &&
        g.vehicle_id !== "" &&
        !UUID_RE.test(g.vehicle_id)
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["guarantee", "vehicle_id"],
          message: t("fleet.validation.vehicle_id"),
        });
      }
      const km = values.conditions.included_km_per_day;
      if (!values.conditions.unlimited_km) {
        const n = Number(km);
        if (km === "" || !Number.isInteger(n) || n < 0 || n > 100000) {
          ctx.addIssue({
            code: "custom",
            path: ["conditions", "included_km_per_day"],
            message: t("fleet.validation.km_per_day"),
          });
        }
      }
    });
}

export type ProductFormValues = z.infer<
  ReturnType<typeof createProductFormSchema>
>;

export function productFormDefaults(
  product?: RentalProduct,
): ProductFormValues {
  const version = product?.version;
  const g = version?.guarantee;
  const c = version?.conditions;
  return {
    code: product?.code ?? "",
    title: product?.title ?? "",
    description: product?.description ?? "",
    segment_uuid: product?.segment.uuid ?? "",
    representative_model_uuid: product?.representative_model?.uuid ?? "",
    representative_model_label: product?.representative_model?.label ?? "",
    representative_type_uuid: product?.representative_type?.uuid ?? "",
    representative_type_label: product?.representative_type?.label ?? "",
    status: product?.status ?? "draft",
    is_active: product?.is_active ?? true,
    guarantee: {
      type: g?.type ?? "GROUP",
      vehicle_id: g?.vehicle_id ?? "",
      variant_level: g?.variant_level ?? "model",
      catalog_model_uuid: g?.catalog_model?.uuid ?? "",
      catalog_model_label: g?.catalog_model?.label ?? "",
      catalog_type_uuid: g?.catalog_type?.uuid ?? "",
      catalog_type_label: g?.catalog_type?.label ?? "",
      rental_group_uuid: g?.rental_group?.uuid ?? "",
      rental_group_label: g?.rental_group
        ? `${g.rental_group.code} · ${g.rental_group.name}`
        : "",
    },
    conditions: {
      deposit_amount: c?.deposit_amount ?? "0.00",
      min_driver_age: String(c?.min_driver_age ?? 21),
      min_license_years: String(c?.min_license_years ?? 1),
      unlimited_km: c ? c.included_km_per_day == null : true,
      included_km_per_day:
        c?.included_km_per_day != null ? String(c.included_km_per_day) : "",
      extra_km_fee: c?.extra_km_fee ?? "0.00",
      fuel_policy: c?.fuel_policy ?? "same_to_same",
      notes: c?.notes ?? "",
    },
    change_note: "",
  };
}

function decimal(v: string): string {
  const out = v.trim().replace(",", ".");
  return out === "" ? "0" : out;
}

/** Guarantee + conditions part of the body (creates a version on change). */
export function productVersionBody(values: ProductFormValues) {
  const g = values.guarantee;
  const c = values.conditions;
  return {
    guarantee: {
      type: g.type,
      vehicle_id:
        g.type === "EXACT_VEHICLE" && g.vehicle_id ? g.vehicle_id : null,
      variant_level: g.type === "EXACT_VARIANT" ? g.variant_level : null,
      catalog_model_uuid:
        g.type === "EXACT_VARIANT" && g.variant_level === "model"
          ? g.catalog_model_uuid || null
          : null,
      catalog_type_uuid:
        g.type === "EXACT_VARIANT" && g.variant_level === "type"
          ? g.catalog_type_uuid || null
          : null,
      rental_group_uuid:
        g.type === "GROUP" ? g.rental_group_uuid || null : null,
    },
    conditions: {
      deposit_amount: decimal(c.deposit_amount),
      min_driver_age: Number(c.min_driver_age),
      min_license_years: Number(c.min_license_years),
      included_km_per_day: c.unlimited_km
        ? null
        : Number(c.included_km_per_day),
      extra_km_fee: decimal(c.extra_km_fee),
      fuel_policy: c.fuel_policy,
      notes: c.notes.trim(),
    },
  };
}

export function productFormToRequest(
  values: ProductFormValues,
): RentalProductCreateRequest {
  return {
    code: values.code.trim().toUpperCase(),
    title: values.title.trim(),
    description: values.description.trim(),
    segment_uuid: values.segment_uuid,
    representative_model_uuid: values.representative_model_uuid || null,
    representative_type_uuid: values.representative_type_uuid || null,
    status: values.status,
    is_active: values.is_active,
    ...productVersionBody(values),
    change_note: values.change_note.trim() || undefined,
  };
}

/** Same rules as the API `DisplayText` helper (live form preview). */
export function previewDisplayText(
  values: ProductFormValues,
  t: (key: string, params?: Record<string, string | number>) => string,
): {
  text: string;
  badge: GuaranteeBadge;
} {
  const g = values.guarantee;
  const representative =
    values.representative_type_label || values.representative_model_label;
  const title = values.title.trim();
  const first = (...items: string[]) =>
    items.find((item) => item.trim() !== "")?.trim() ?? "";
  switch (g.type) {
    case "EXACT_VARIANT": {
      const label =
        g.variant_level === "type"
          ? g.catalog_type_label
          : g.catalog_model_label;
      return {
        text: t("fleet.display.model_guaranteed", {
          label: first(label, representative, title),
        }),
        badge: "model_guaranteed",
      };
    }
    case "EXACT_VEHICLE":
      return {
        text: t("fleet.display.vehicle_guaranteed", {
          label: first(representative, title),
        }),
        badge: "vehicle_guaranteed",
      };
    default: {
      const groupName = g.rental_group_label.split(" · ").slice(1).join(" · ");
      return {
        text: t("fleet.display.or_similar", {
          label: first(representative, groupName, title),
        }),
        badge: "or_similar",
      };
    }
  }
}
