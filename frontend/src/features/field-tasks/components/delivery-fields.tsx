"use client";

import { Clock, Info, Truck } from "lucide-react";
import { useEffect } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppInput, AppSelect, AppTextarea } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { useDeliveryPoints } from "@/features/field-tasks/hooks/use-field-tasks";
import type {
  DeliveryDetails,
  DeliveryPoint,
} from "@/features/field-tasks/types";
import { moneyText } from "@/features/pricing/lib/format";
import { useLocale } from "@/providers/locale-provider";

/** Select values (Radix items cannot be empty). */
export const DESK = "desk";
export const SAME_POINT = "same";

/** Form fields the delivery block adds to a search form. */
export const deliveryFieldsShape = {
  pickup_point_uuid: z.string().optional(),
  pickup_address: z.string().max(500).optional(),
  pickup_district: z.string().max(128).optional(),
  pickup_city: z.string().max(128).optional(),
  pickup_flight_number: z.string().max(16).optional(),
  meeting_note: z.string().max(1000).optional(),
  contact_name: z.string().max(255).optional(),
  contact_phone: z.string().max(32).optional(),
  return_point_uuid: z.string().optional(),
  return_address: z.string().max(500).optional(),
  return_district: z.string().max(128).optional(),
  return_city: z.string().max(128).optional(),
};

export type DeliveryFieldValues = {
  [K in keyof typeof deliveryFieldsShape]: string;
};

type DeliveryFieldInput = Partial<DeliveryFieldValues>;

export const deliveryFieldDefaults: DeliveryFieldValues = {
  pickup_point_uuid: DESK,
  pickup_address: "",
  pickup_district: "",
  pickup_city: "",
  pickup_flight_number: "",
  meeting_note: "",
  contact_name: "",
  contact_phone: "",
  return_point_uuid: SAME_POINT,
  return_address: "",
  return_district: "",
  return_city: "",
};

function compact(d: DeliveryDetails): DeliveryDetails | null {
  const out: DeliveryDetails = {};
  for (const [key, value] of Object.entries(d)) {
    if (typeof value === "string" && value.trim() !== "") {
      (out as Record<string, string>)[key] = value.trim();
    }
  }
  return Object.keys(out).length ? out : null;
}

/**
 * The request part of the delivery block: the points and the delivery
 * details of each side (the return reuses the pick-up's when it is the
 * same point).
 */
export function deliveryRequest(input: DeliveryFieldInput) {
  const v: DeliveryFieldValues = { ...deliveryFieldDefaults, ...input };
  const pickupPoint = v.pickup_point_uuid !== DESK ? v.pickup_point_uuid : null;
  const samePoint = v.return_point_uuid === SAME_POINT;
  const returnPoint = samePoint
    ? pickupPoint
    : v.return_point_uuid !== DESK
      ? v.return_point_uuid
      : null;
  const contact = {
    contact_name: v.contact_name,
    contact_phone: v.contact_phone,
  };
  const pickup = pickupPoint
    ? compact({
        address: v.pickup_address,
        district: v.pickup_district,
        city: v.pickup_city,
        flight_number: v.pickup_flight_number,
        meeting_note: v.meeting_note,
        ...contact,
      })
    : null;
  // The same point keeps the address and meeting details; the flight
  // number belongs to the arrival only.
  const ret = returnPoint
    ? samePoint
      ? pickup && compact({ ...pickup, flight_number: "" })
      : compact({
          address: v.return_address,
          district: v.return_district,
          city: v.return_city,
          ...contact,
        })
    : null;
  return {
    pickup_point_uuid: pickupPoint,
    return_point_uuid: returnPoint,
    pickup_delivery: pickup,
    return_delivery: ret,
  };
}

function PointHint({ point }: { point: DeliveryPoint | undefined }) {
  const { t, locale } = useLocale();
  if (!point) return null;
  return (
    <div className="bg-muted/40 text-muted-foreground space-y-1 rounded-md border p-2.5 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={point.fee_source === "none" ? "secondary" : "default"}>
          {point.fee_source === "none"
            ? t("field_tasks.delivery.no_fee")
            : t("field_tasks.delivery.fee", {
                amount: moneyText(point.fee, locale),
              })}
        </Badge>
        {point.min_notice_minutes > 0 ? (
          <span className="flex items-center gap-1">
            <Clock className="size-3" aria-hidden />
            {t("field_tasks.delivery.notice", {
              minutes: point.min_notice_minutes,
            })}
          </span>
        ) : null}
      </div>
      {point.service_districts.length ? (
        <p>
          {t("field_tasks.delivery.districts", {
            districts: point.service_districts.join(", "),
          })}
        </p>
      ) : null}
      {point.conditions ? (
        <p className="flex items-start gap-1">
          <Info className="mt-0.5 size-3 shrink-0" aria-hidden />
          {point.conditions}
        </p>
      ) : null}
    </div>
  );
}

/**
 * Delivery / collection block of a reservation search: the pick-up and
 * return place (the branch desk or one of the branch's delivery points),
 * the address an address zone needs, the flight number of an airport
 * meeting, the contact and a meeting note. Used inside an `AppForm` whose
 * schema includes `deliveryFieldsShape`.
 */
export function DeliveryFields() {
  const { t } = useLocale();
  const form = useFormContext();
  const pickupBranch = useWatch({
    control: form.control,
    name: "pickup_branch_uuid",
  }) as string;
  const returnBranchRaw = useWatch({
    control: form.control,
    name: "return_branch_uuid",
  }) as string;
  const pickupPoint = useWatch({
    control: form.control,
    name: "pickup_point_uuid",
  }) as string;
  const returnPoint = useWatch({
    control: form.control,
    name: "return_point_uuid",
  }) as string;
  const returnBranch =
    !returnBranchRaw || returnBranchRaw === "same"
      ? pickupBranch
      : returnBranchRaw;
  const pickupPoints = useDeliveryPoints(pickupBranch || undefined);
  const returnPoints = useDeliveryPoints(returnBranch || undefined);
  const pickups = pickupBranch ? (pickupPoints.data ?? []) : [];
  const returns = returnBranch ? (returnPoints.data ?? []) : [];
  const selectedPickup = pickups.find((p) => p.uuid === pickupPoint);
  const selectedReturn =
    returnPoint === SAME_POINT
      ? selectedPickup
      : returns.find((p) => p.uuid === returnPoint);
  const pointOption = (p: DeliveryPoint) => ({
    value: p.uuid,
    label: p.name,
  });
  // "Same as the pick-up" only makes sense at the same branch; a return
  // at another branch falls back to its desk.
  const sameAvailable = returnBranch === pickupBranch;
  useEffect(() => {
    if (!sameAvailable && returnPoint === SAME_POINT) {
      form.setValue("return_point_uuid", DESK);
    }
  }, [form, returnPoint, sameAvailable]);

  return (
    <div className="space-y-4 rounded-lg border border-dashed p-4 sm:col-span-2 xl:col-span-6">
      <div className="flex items-center gap-2 text-sm font-semibold">
        <Truck className="size-4" aria-hidden />
        {t("field_tasks.delivery.title")}
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <AppSelect
            name="pickup_point_uuid"
            label={t("field_tasks.delivery.pickup_place")}
            disabled={!pickupBranch}
            options={[
              { value: DESK, label: t("field_tasks.delivery.desk") },
              ...pickups.map(pointOption),
            ]}
          />
          <PointHint point={selectedPickup} />
        </div>
        <div className="space-y-2">
          <AppSelect
            name="return_point_uuid"
            label={t("field_tasks.delivery.return_place")}
            disabled={!returnBranch}
            options={[
              ...(sameAvailable
                ? [{ value: SAME_POINT, label: t("field_tasks.delivery.same") }]
                : []),
              { value: DESK, label: t("field_tasks.delivery.desk") },
              ...returns.map(pointOption),
            ]}
          />
          {returnPoint !== SAME_POINT ? (
            <PointHint point={selectedReturn} />
          ) : null}
        </div>
        {selectedPickup?.requires_address ? (
          <>
            <AppInput
              name="pickup_address"
              label={t("field_tasks.delivery.pickup_address")}
              className="sm:col-span-2"
            />
            <AppInput
              name="pickup_district"
              label={t("field_tasks.fields.district")}
            />
            <AppInput name="pickup_city" label={t("field_tasks.fields.city")} />
          </>
        ) : null}
        {selectedReturn?.requires_address && returnPoint !== SAME_POINT ? (
          <>
            <AppInput
              name="return_address"
              label={t("field_tasks.delivery.return_address")}
              className="sm:col-span-2"
            />
            <AppInput
              name="return_district"
              label={t("field_tasks.fields.district")}
            />
            <AppInput name="return_city" label={t("field_tasks.fields.city")} />
          </>
        ) : null}
        {selectedPickup?.type === "airport" ? (
          <AppInput
            name="pickup_flight_number"
            label={t("field_tasks.fields.flight_number")}
            description={t("field_tasks.delivery.flight_hint")}
          />
        ) : null}
        {selectedPickup || (selectedReturn && returnPoint !== SAME_POINT) ? (
          <>
            <AppInput
              name="contact_name"
              label={t("field_tasks.fields.contact_name")}
              description={t("field_tasks.delivery.contact_hint")}
            />
            <AppInput
              name="contact_phone"
              inputMode="tel"
              label={t("field_tasks.fields.contact_phone")}
            />
            <AppTextarea
              name="meeting_note"
              label={t("field_tasks.fields.meeting_note")}
              rows={2}
              className="sm:col-span-2"
            />
          </>
        ) : null}
      </div>
    </div>
  );
}
