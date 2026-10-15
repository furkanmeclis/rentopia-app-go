import { z } from "zod";

import {
  CODE_PATTERN,
  DAY_KEYS,
  MONEY_PATTERN,
  SERVICE_POINT_TYPES,
  TIME_PATTERN,
  defaultWeeklyHours,
} from "@/features/branches/constants";
import type {
  Branch,
  BranchCreateRequest,
  DayHours,
  ServicePoint,
  ServicePointCreateRequest,
  WeeklyHours,
} from "@/features/branches/types";

type Translate = (key: string) => string;

const dayHoursSchema = z.object({
  closed: z.boolean(),
  open: z.string(),
  close: z.string(),
});

const weeklyHoursSchema = z.object({
  mon: dayHoursSchema,
  tue: dayHoursSchema,
  wed: dayHoursSchema,
  thu: dayHoursSchema,
  fri: dayHoursSchema,
  sat: dayHoursSchema,
  sun: dayHoursSchema,
});

function checkDay(
  t: Translate,
  ctx: z.RefinementCtx,
  path: (string | number)[],
  day: DayHours,
) {
  if (day.closed) return;
  const okOpen = TIME_PATTERN.test(day.open);
  const okClose = TIME_PATTERN.test(day.close);
  if (!okOpen) {
    ctx.addIssue({
      code: "custom",
      path: [...path, "open"],
      message: t("branches.validation.time"),
    });
  }
  if (!okClose) {
    ctx.addIssue({
      code: "custom",
      path: [...path, "close"],
      message: t("branches.validation.time"),
    });
  }
  if (okOpen && okClose && day.open >= day.close) {
    ctx.addIssue({
      code: "custom",
      path: [...path, "close"],
      message: t("branches.validation.close_after_open"),
    });
  }
}

function checkWeek(
  t: Translate,
  ctx: z.RefinementCtx,
  path: (string | number)[],
  hours: WeeklyHours,
) {
  for (const key of DAY_KEYS) {
    checkDay(t, ctx, [...path, key], hours[key]);
  }
}

function optionalNumber(
  t: Translate,
  min: number,
  max: number,
  messageKey: string,
) {
  return z
    .string()
    .trim()
    .refine((v) => {
      if (v === "") return true;
      const n = Number(v.replace(",", "."));
      return Number.isFinite(n) && n >= min && n <= max;
    }, t(messageKey));
}

export function createBranchFormSchema(t: Translate) {
  return z
    .object({
      code: z
        .string()
        .trim()
        .min(1, t("branches.validation.code_required"))
        .regex(CODE_PATTERN, t("branches.validation.code_format")),
      name: z
        .string()
        .trim()
        .min(1, t("branches.validation.name_required"))
        .max(255),
      address: z.string().max(1000),
      city: z.string().max(128),
      district: z.string().max(128),
      phone: z.string().max(32),
      email: z
        .string()
        .trim()
        .refine(
          (v) => v === "" || z.email().safeParse(v).success,
          t("branches.validation.email"),
        ),
      latitude: optionalNumber(t, -90, 90, "branches.validation.latitude"),
      longitude: optionalNumber(t, -180, 180, "branches.validation.longitude"),
      prep_minutes: z
        .string()
        .trim()
        .refine((v) => {
          const n = Number(v);
          return v !== "" && Number.isInteger(n) && n >= 0 && n <= 10080;
        }, t("branches.validation.prep_minutes")),
      is_active: z.boolean(),
      working_hours: weeklyHoursSchema,
      holiday_exceptions: z.array(
        z.object({
          date: z.string(),
          closed: z.boolean(),
          open: z.string(),
          close: z.string(),
          note: z.string().max(255),
        }),
      ),
    })
    .superRefine((values, ctx) => {
      if ((values.latitude === "") !== (values.longitude === "")) {
        ctx.addIssue({
          code: "custom",
          path: [values.latitude === "" ? "latitude" : "longitude"],
          message: t("branches.validation.coordinates_pair"),
        });
      }
      checkWeek(t, ctx, ["working_hours"], values.working_hours);
      const seen = new Set<string>();
      values.holiday_exceptions.forEach((holiday, index) => {
        const path = ["holiday_exceptions", index];
        if (!/^\d{4}-\d{2}-\d{2}$/.test(holiday.date)) {
          ctx.addIssue({
            code: "custom",
            path: [...path, "date"],
            message: t("branches.validation.date"),
          });
        } else if (seen.has(holiday.date)) {
          ctx.addIssue({
            code: "custom",
            path: [...path, "date"],
            message: t("branches.validation.date_duplicate"),
          });
        }
        seen.add(holiday.date);
        checkDay(t, ctx, path, holiday);
      });
    });
}

export type BranchFormValues = z.infer<
  ReturnType<typeof createBranchFormSchema>
>;

export function branchFormDefaults(branch?: Branch): BranchFormValues {
  return {
    code: branch?.code ?? "",
    name: branch?.name ?? "",
    address: branch?.address ?? "",
    city: branch?.city ?? "",
    district: branch?.district ?? "",
    phone: branch?.phone ?? "",
    email: branch?.email ?? "",
    latitude: branch?.latitude != null ? String(branch.latitude) : "",
    longitude: branch?.longitude != null ? String(branch.longitude) : "",
    prep_minutes: String(branch?.prep_minutes ?? 60),
    is_active: branch?.is_active ?? true,
    working_hours: branch?.working_hours ?? defaultWeeklyHours(),
    holiday_exceptions: (branch?.holiday_exceptions ?? []).map((h) => ({
      ...h,
    })),
  };
}

function numberOrNull(value: string): number | null {
  const v = value.trim();
  if (!v) return null;
  return Number(v.replace(",", "."));
}

function normalizeDay(day: DayHours): DayHours {
  return day.closed ? { closed: true, open: "", close: "" } : day;
}

function normalizeWeek(hours: WeeklyHours): WeeklyHours {
  const out = { ...hours };
  for (const key of DAY_KEYS) out[key] = normalizeDay(hours[key]);
  return out;
}

/** Form values → API body (create and full update share the shape). */
export function branchFormToRequest(
  values: BranchFormValues,
): BranchCreateRequest {
  return {
    code: values.code.trim().toUpperCase(),
    name: values.name.trim(),
    address: values.address.trim(),
    city: values.city.trim(),
    district: values.district.trim(),
    phone: values.phone.trim(),
    email: values.email.trim(),
    latitude: numberOrNull(values.latitude),
    longitude: numberOrNull(values.longitude),
    prep_minutes: Number(values.prep_minutes),
    is_active: values.is_active,
    working_hours: normalizeWeek(values.working_hours),
    holiday_exceptions: values.holiday_exceptions.map((h) => ({
      ...normalizeDay(h),
      date: h.date,
      note: h.note.trim(),
    })),
  };
}

// ------------------------------------------------------------ quick edit

export function createQuickEditSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("branches.validation.name_required"))
      .max(255),
    phone: z.string().max(32),
    email: z
      .string()
      .trim()
      .refine(
        (v) => v === "" || z.email().safeParse(v).success,
        t("branches.validation.email"),
      ),
    prep_minutes: z
      .string()
      .trim()
      .refine((v) => {
        const n = Number(v);
        return v !== "" && Number.isInteger(n) && n >= 0 && n <= 10080;
      }, t("branches.validation.prep_minutes")),
    is_active: z.boolean(),
  });
}

export type QuickEditValues = z.infer<ReturnType<typeof createQuickEditSchema>>;

// ---------------------------------------------------------- service point

/** Select value of "no delivery extra" (Radix items cannot be empty). */
export const NO_EXTRA = "none";

export function createServicePointSchema(t: Translate) {
  return z
    .object({
      type: z.enum(SERVICE_POINT_TYPES as [string, ...string[]]),
      name: z
        .string()
        .trim()
        .min(1, t("branches.validation.name_required"))
        .max(255),
      description: z.string().max(2000),
      extra_fee: z
        .string()
        .trim()
        .regex(MONEY_PATTERN, t("branches.validation.extra_fee")),
      online_selectable: z.boolean(),
      is_active: z.boolean(),
      own_hours: z.boolean(),
      working_hours: weeklyHoursSchema,
      delivery_enabled: z.boolean(),
      delivery_extra_uuid: z.string(),
      service_districts: z.string().max(5000),
      latitude: z.string().trim(),
      longitude: z.string().trim(),
      service_radius_km: z.string().trim(),
      min_notice_minutes: z
        .string()
        .trim()
        .regex(/^\d{0,5}$/, t("branches.validation.min_notice")),
      delivery_conditions: z.string().max(2000),
    })
    .superRefine((values, ctx) => {
      if (values.own_hours) {
        checkWeek(t, ctx, ["working_hours"], values.working_hours);
      }
      const num = (v: string) =>
        v === "" ? null : Number(v.replace(",", "."));
      const lat = num(values.latitude);
      const lng = num(values.longitude);
      if (lat !== null && (Number.isNaN(lat) || lat < -90 || lat > 90)) {
        ctx.addIssue({
          code: "custom",
          path: ["latitude"],
          message: t("branches.validation.latitude"),
        });
      }
      if (lng !== null && (Number.isNaN(lng) || lng < -180 || lng > 180)) {
        ctx.addIssue({
          code: "custom",
          path: ["longitude"],
          message: t("branches.validation.longitude"),
        });
      }
      if ((lat === null) !== (lng === null)) {
        ctx.addIssue({
          code: "custom",
          path: ["longitude"],
          message: t("branches.validation.coordinates_pair"),
        });
      }
      const radius = num(values.service_radius_km);
      if (radius !== null && (Number.isNaN(radius) || radius <= 0)) {
        ctx.addIssue({
          code: "custom",
          path: ["service_radius_km"],
          message: t("branches.validation.radius"),
        });
      }
    });
}

export type ServicePointFormValues = z.infer<
  ReturnType<typeof createServicePointSchema>
>;

export function servicePointFormDefaults(
  point?: ServicePoint,
  branchHours?: WeeklyHours,
): ServicePointFormValues {
  return {
    type: point?.type ?? "office",
    name: point?.name ?? "",
    description: point?.description ?? "",
    extra_fee: point?.extra_fee ?? "0.00",
    online_selectable: point?.online_selectable ?? true,
    is_active: point?.is_active ?? true,
    own_hours: Boolean(point?.working_hours),
    working_hours: point?.working_hours ?? branchHours ?? defaultWeeklyHours(),
    delivery_enabled: point?.delivery_enabled ?? false,
    delivery_extra_uuid: point?.delivery_extra?.uuid ?? NO_EXTRA,
    service_districts: (point?.service_districts ?? []).join(", "),
    latitude: point?.latitude != null ? String(point.latitude) : "",
    longitude: point?.longitude != null ? String(point.longitude) : "",
    service_radius_km:
      point?.service_radius_km != null ? String(point.service_radius_km) : "",
    min_notice_minutes: String(point?.min_notice_minutes ?? 0),
    delivery_conditions: point?.delivery_conditions ?? "",
  };
}

export function servicePointFormToRequest(
  values: ServicePointFormValues,
): ServicePointCreateRequest {
  return {
    type: values.type as ServicePointCreateRequest["type"],
    name: values.name.trim(),
    description: values.description.trim(),
    extra_fee: values.extra_fee.trim().replace(",", "."),
    online_selectable: values.online_selectable,
    is_active: values.is_active,
    working_hours: values.own_hours
      ? normalizeWeek(values.working_hours)
      : null,
    delivery_enabled: values.delivery_enabled,
    delivery_extra_uuid:
      values.delivery_extra_uuid === NO_EXTRA
        ? null
        : values.delivery_extra_uuid,
    service_districts: values.service_districts
      .split(/[,\n]/)
      .map((d) => d.trim())
      .filter(Boolean),
    latitude: values.latitude
      ? Number(values.latitude.replace(",", "."))
      : null,
    longitude: values.longitude
      ? Number(values.longitude.replace(",", "."))
      : null,
    service_radius_km: values.service_radius_km
      ? Number(values.service_radius_km.replace(",", "."))
      : null,
    min_notice_minutes: Number(values.min_notice_minutes || 0),
    delivery_conditions: values.delivery_conditions.trim(),
  };
}
