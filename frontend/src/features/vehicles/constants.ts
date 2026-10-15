import type {
  VehicleOwnership,
  VehicleStatus,
} from "@/features/vehicles/types";

/** Engine resource slug shared by io / bulk / activity / live lists. */
export const VEHICLES_RESOURCE = "tenant.vehicles" as const;

export const VEHICLE_STATUSES: VehicleStatus[] = [
  "available",
  "preparing",
  "rented",
  "return_check",
  "cleaning",
  "maintenance",
  "damaged",
  "in_transfer",
  "out_of_service",
];

export const OWNERSHIPS: VehicleOwnership[] = ["owned", "supplied"];

/** Static i18n key maps (the i18n checker verifies every key). */
export const STATUS_LABEL_KEYS: Record<VehicleStatus, string> = {
  available: "vehicles.status.available",
  preparing: "vehicles.status.preparing",
  rented: "vehicles.status.rented",
  return_check: "vehicles.status.return_check",
  cleaning: "vehicles.status.cleaning",
  maintenance: "vehicles.status.maintenance",
  damaged: "vehicles.status.damaged",
  in_transfer: "vehicles.status.in_transfer",
  out_of_service: "vehicles.status.out_of_service",
};

export const STATUS_TONES: Record<
  VehicleStatus,
  "default" | "success" | "warning" | "danger"
> = {
  available: "success",
  preparing: "warning",
  rented: "default",
  return_check: "warning",
  cleaning: "warning",
  maintenance: "danger",
  damaged: "danger",
  in_transfer: "default",
  out_of_service: "default",
};

export const OWNERSHIP_LABEL_KEYS: Record<VehicleOwnership, string> = {
  owned: "vehicles.ownership.owned",
  supplied: "vehicles.ownership.supplied",
};

/** Eligibility reason codes (fleet/eligibility) → labels. */
export const REASON_LABEL_KEYS: Record<string, string> = {
  transmission_mismatch: "vehicles.reasons.transmission_mismatch",
  fuel_mismatch: "vehicles.reasons.fuel_mismatch",
  body_type_mismatch: "vehicles.reasons.body_type_mismatch",
  seats_below_minimum: "vehicles.reasons.seats_below_minimum",
  luggage_below_minimum: "vehicles.reasons.luggage_below_minimum",
  air_conditioning_required: "vehicles.reasons.air_conditioning_required",
  catalog_not_allowed: "vehicles.reasons.catalog_not_allowed",
  not_group_member: "vehicles.reasons.not_group_member",
};

/** Why a commitment blocks a rental group membership removal (409). */
export const BLOCK_REASON_LABEL_KEYS: Record<string, string> = {
  plate_fixed: "vehicles.groups.block_reasons.plate_fixed",
  started: "vehicles.groups.block_reasons.started",
  no_vehicle: "vehicles.groups.block_reasons.no_vehicle",
};
