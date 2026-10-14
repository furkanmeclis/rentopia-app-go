import { z } from "zod";

import { CODE_PATTERN, isValidAcriss } from "@/features/fleet/constants";
import type {
  BodyType,
  Fuel,
  RentalGroup,
  RentalGroupCreateRequest,
  Transmission,
} from "@/features/fleet/types";

type Translate = (key: string) => string;

/** Select value for "not guaranteed" (Radix Select has no empty value). */
export const ANY = "any";

const pickSchema = z.object({ uuid: z.string(), label: z.string() });

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

export function createGroupFormSchema(t: Translate) {
  return z.object({
    code: z
      .string()
      .trim()
      .min(1, t("fleet.validation.code_required"))
      .regex(CODE_PATTERN, t("fleet.validation.code_format")),
    name: z
      .string()
      .trim()
      .min(1, t("fleet.validation.name_required"))
      .max(255),
    description: z.string().max(2000),
    segment_uuid: z.string().min(1, t("fleet.validation.segment_required")),
    /** `ANY` = not guaranteed. */
    transmission: z.string(),
    fuel: z.string(),
    body_type: z.string(),
    min_seats: optionalInt(t, 1, 60, "fleet.validation.min_seats"),
    min_luggage: optionalInt(t, 0, 30, "fleet.validation.min_luggage"),
    air_conditioning: z.boolean(),
    acriss_code: z
      .string()
      .trim()
      .refine(
        (v) => v === "" || isValidAcriss(v.toUpperCase()),
        t("fleet.validation.acriss"),
      ),
    catalog_models: z.array(pickSchema),
    catalog_types: z.array(pickSchema),
    is_active: z.boolean(),
  });
}

export type GroupFormValues = z.infer<ReturnType<typeof createGroupFormSchema>>;

export function groupFormDefaults(group?: RentalGroup): GroupFormValues {
  return {
    code: group?.code ?? "",
    name: group?.name ?? "",
    description: group?.description ?? "",
    segment_uuid: group?.segment.uuid ?? "",
    transmission: group?.transmission ?? ANY,
    fuel: group?.fuel ?? ANY,
    body_type: group?.body_type ?? ANY,
    min_seats: group?.min_seats != null ? String(group.min_seats) : "",
    min_luggage: group?.min_luggage != null ? String(group.min_luggage) : "",
    air_conditioning: group?.air_conditioning ?? true,
    acriss_code: group?.acriss_code ?? "",
    catalog_models: (group?.catalog_models ?? []).map((m) => ({
      uuid: m.uuid,
      label: m.label,
    })),
    catalog_types: (group?.catalog_types ?? []).map((ty) => ({
      uuid: ty.uuid,
      label: ty.label,
    })),
    is_active: group?.is_active ?? true,
  };
}

function orNull<T extends string>(v: string): T | null {
  return v && v !== ANY ? (v as T) : null;
}

function intOrNull(v: string): number | null {
  return v.trim() === "" ? null : Number(v);
}

export function groupFormToRequest(
  values: GroupFormValues,
): RentalGroupCreateRequest {
  return {
    code: values.code.trim().toUpperCase(),
    name: values.name.trim(),
    description: values.description.trim(),
    segment_uuid: values.segment_uuid,
    transmission: orNull<Transmission>(values.transmission),
    fuel: orNull<Fuel>(values.fuel),
    body_type: orNull<BodyType>(values.body_type),
    min_seats: intOrNull(values.min_seats),
    min_luggage: intOrNull(values.min_luggage),
    air_conditioning: values.air_conditioning,
    acriss_code: values.acriss_code.trim()
      ? values.acriss_code.trim().toUpperCase()
      : null,
    catalog_model_uuids: values.catalog_models.map((m) => m.uuid),
    catalog_type_uuids: values.catalog_types.map((ty) => ty.uuid),
    is_active: values.is_active,
  };
}
