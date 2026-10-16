import { z } from "zod";

import type { BodyType, Fuel, Transmission } from "@/features/fleet/types";
import type {
  Vehicle,
  VehicleCreateRequest,
  VehicleOwnership,
  VehicleStatus,
} from "@/features/vehicles/types";

type Translate = (key: string) => string;

/** Select value for "unknown" (Radix Select has no empty value). */
export const NONE = "none";

const PLATE = /^[A-Za-z0-9][A-Za-z0-9 -]{0,18}[A-Za-z0-9]$/;
const VIN = /^[A-HJ-NPR-Z0-9]{17}$/;
const POOL = /^[A-Z0-9][A-Z0-9_-]{0,31}$/;

function optionalInt(t: Translate, min: number, max: number, key: string) {
  return z
    .string()
    .trim()
    .refine((v) => {
      if (v === "") return true;
      const n = Number(v);
      return Number.isInteger(n) && n >= min && n <= max;
    }, t(key));
}

export function createVehicleFormSchema(t: Translate) {
  return z
    .object({
      plate: z
        .string()
        .trim()
        .min(1, t("vehicles.validation.plate_required"))
        .regex(PLATE, t("vehicles.validation.plate_format")),
      vin: z
        .string()
        .trim()
        .refine(
          (v) => v === "" || VIN.test(v.toUpperCase().replace(/\s/g, "")),
          t("vehicles.validation.vin_format"),
        ),
      manual_entry: z.boolean(),
      brand_uuid: z.string(),
      brand_label: z.string(),
      model_uuid: z.string(),
      model_label: z.string(),
      catalog_type_uuid: z.string(),
      type_label: z.string(),
      /** Valid model years of the picked catalog type (empty = any). */
      type_years: z.array(z.number()),
      manual_brand: z.string().max(128),
      manual_model: z.string().max(128),
      manual_type: z.string().max(255),
      model_year: z
        .string()
        .min(1, t("vehicles.validation.model_year_required")),
      transmission: z.string(),
      fuel: z.string(),
      body_type: z.string(),
      seats: optionalInt(t, 1, 60, "vehicles.validation.seats"),
      luggage: optionalInt(t, 0, 30, "vehicles.validation.luggage"),
      air_conditioning: z.boolean(),
      attributes_verified: z.boolean(),
      color: z.string().max(64),
      home_branch_uuid: z
        .string()
        .min(1, t("vehicles.validation.home_branch_required")),
      current_branch_uuid: z.string(),
      fleet_pool: z
        .string()
        .trim()
        .refine(
          (v) => v === "" || POOL.test(v.toUpperCase()),
          t("vehicles.validation.pool_format"),
        ),
      ownership: z.string(),
      supplier_name: z.string().max(255),
      supplier_uuid: z.string(),
      had_supplier: z.boolean(),
      in_service_date: z.string(),
      out_of_service_date: z.string(),
      odometer_km: optionalInt(t, 0, 5_000_000, "vehicles.validation.km"),
      fuel_level: optionalInt(t, 0, 100, "vehicles.validation.fuel_level"),
      status: z.string(),
      is_active: z.boolean(),
    })
    .superRefine((v, ctx) => {
      if (v.manual_entry) {
        if (!v.manual_brand.trim())
          ctx.addIssue({
            code: "custom",
            path: ["manual_brand"],
            message: t("vehicles.validation.manual_brand_required"),
          });
        if (!v.manual_model.trim())
          ctx.addIssue({
            code: "custom",
            path: ["manual_model"],
            message: t("vehicles.validation.manual_model_required"),
          });
      } else if (!v.catalog_type_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["catalog_type_uuid"],
          message: t("vehicles.validation.catalog_type_required"),
        });
      }
      if (
        !v.manual_entry &&
        v.model_year &&
        v.type_years.length > 0 &&
        !v.type_years.includes(Number(v.model_year))
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["model_year"],
          message: t("vehicles.validation.model_year_invalid"),
        });
      }
      if (
        v.ownership === "supplied" &&
        !v.supplier_name.trim() &&
        v.supplier_uuid === NONE
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["supplier_name"],
          message: t("vehicles.validation.supplier_required"),
        });
      }
      if (
        v.in_service_date &&
        v.out_of_service_date &&
        v.out_of_service_date < v.in_service_date
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["out_of_service_date"],
          message: t("vehicles.validation.service_dates"),
        });
      }
    });
}

export type VehicleFormValues = z.infer<
  ReturnType<typeof createVehicleFormSchema>
>;

export function vehicleFormDefaults(vehicle?: Vehicle): VehicleFormValues {
  const catalog = vehicle?.catalog ?? null;
  return {
    plate: vehicle?.plate ?? "",
    vin: vehicle?.vin ?? "",
    manual_entry: vehicle?.manual_entry ?? false,
    brand_uuid: catalog?.brand_uuid ?? "",
    brand_label: catalog?.brand_name ?? "",
    model_uuid: catalog?.model_uuid ?? "",
    model_label: catalog?.model_name ?? "",
    catalog_type_uuid: catalog?.type_uuid ?? "",
    type_label: catalog?.type_name ?? "",
    type_years: catalog?.years ?? [],
    manual_brand: vehicle?.manual_entry ? vehicle.brand : "",
    manual_model: vehicle?.manual_entry ? vehicle.model : "",
    manual_type: vehicle?.manual_entry ? vehicle.type_name : "",
    model_year: vehicle ? String(vehicle.model_year) : "",
    transmission: vehicle?.transmission ?? NONE,
    fuel: vehicle?.fuel ?? NONE,
    body_type: vehicle?.body_type ?? NONE,
    seats: vehicle?.seats != null ? String(vehicle.seats) : "",
    luggage: vehicle?.luggage != null ? String(vehicle.luggage) : "",
    air_conditioning: vehicle?.air_conditioning ?? true,
    attributes_verified: vehicle?.attributes_verified ?? true,
    color: vehicle?.color ?? "",
    home_branch_uuid: vehicle?.home_branch.uuid ?? "",
    current_branch_uuid: vehicle?.current_branch.uuid ?? "",
    fleet_pool: vehicle?.fleet_pool ?? "MAIN",
    ownership: vehicle?.ownership ?? "owned",
    supplier_name: vehicle?.supplier_name ?? "",
    supplier_uuid: vehicle?.supplier?.uuid ?? NONE,
    had_supplier: Boolean(vehicle?.supplier),
    in_service_date: vehicle?.in_service_date ?? "",
    out_of_service_date: vehicle?.out_of_service_date ?? "",
    odometer_km: vehicle ? String(vehicle.odometer_km) : "",
    fuel_level: vehicle?.fuel_level != null ? String(vehicle.fuel_level) : "",
    status: vehicle?.status ?? "available",
    is_active: vehicle?.is_active ?? true,
  };
}

function orNull<T extends string>(v: string): T | null {
  return v && v !== NONE ? (v as T) : null;
}

function intOrNull(v: string): number | null {
  return v.trim() === "" ? null : Number(v);
}

/**
 * Create / patch body. `status` is only sent on create: later changes go
 * through the status dialog (recorded with km, fuel and a note).
 */
export function vehicleFormToRequest(
  values: VehicleFormValues,
  mode: "create" | "edit",
): VehicleCreateRequest {
  const body: VehicleCreateRequest = {
    plate: values.plate.trim().toUpperCase(),
    vin: values.vin.trim().toUpperCase().replace(/\s/g, ""),
    manual_entry: values.manual_entry,
    model_year: Number(values.model_year),
    transmission: orNull<Transmission>(values.transmission),
    fuel: orNull<Fuel>(values.fuel),
    body_type: orNull<BodyType>(values.body_type),
    seats: intOrNull(values.seats),
    luggage: intOrNull(values.luggage),
    air_conditioning: values.air_conditioning,
    attributes_verified: values.attributes_verified,
    color: values.color.trim(),
    home_branch_uuid: values.home_branch_uuid,
    fleet_pool: values.fleet_pool.trim().toUpperCase(),
    ownership: values.ownership as VehicleOwnership,
    supplier_name:
      values.ownership === "supplied" ? values.supplier_name.trim() : "",
    in_service_date: values.in_service_date || null,
    out_of_service_date: values.out_of_service_date || null,
    fuel_level: intOrNull(values.fuel_level),
    is_active: values.is_active,
  };
  if (values.current_branch_uuid) {
    body.current_branch_uuid = values.current_branch_uuid;
  }
  if (values.odometer_km.trim() !== "") {
    body.odometer_km = Number(values.odometer_km);
  }
  if (values.manual_entry) {
    body.manual_brand = values.manual_brand.trim();
    body.manual_model = values.manual_model.trim();
    body.manual_type = values.manual_type.trim();
  } else {
    body.catalog_type_uuid = values.catalog_type_uuid;
  }
  if (values.ownership === "supplied") {
    // A chosen card links the supplier (its title becomes the name);
    // "no card" clears an existing link, otherwise the free-text name
    // links the card with that title server-side.
    if (values.supplier_uuid !== NONE) {
      body.supplier_uuid = values.supplier_uuid;
    } else if (values.had_supplier) {
      body.supplier_uuid = null;
    }
  }
  if (mode === "create") {
    body.status = values.status as VehicleStatus;
  }
  return body;
}
