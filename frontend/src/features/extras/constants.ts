import type {
  AllocationKind,
  AllocationStatus,
  ChargeType,
  ExtraStatus,
  ExtraType,
} from "@/features/extras/types";

/** Engine / live resource slugs. */
export const EXTRAS_RESOURCE = "tenant.extras" as const;
export const ALLOCATIONS_RESOURCE = "tenant.equipment_allocations" as const;

export const EXTRA_TYPES: ExtraType[] = [
  "additional_driver",
  "child_seat",
  "km_package",
  "delivery",
  "other",
];

export const CHARGE_TYPES: ChargeType[] = ["per_day", "per_unit", "per_rental"];

export const EXTRA_STATUSES: ExtraStatus[] = ["draft", "published", "archived"];

export const ALLOCATION_KINDS: AllocationKind[] = [
  "hold",
  "reservation",
  "rental",
];

export const ALLOCATION_STATUSES: AllocationStatus[] = [
  "active",
  "released",
  "expired",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const TYPE_LABEL_KEYS: Record<ExtraType, string> = {
  additional_driver: "extras.type.additional_driver",
  child_seat: "extras.type.child_seat",
  km_package: "extras.type.km_package",
  delivery: "extras.type.delivery",
  other: "extras.type.other",
};

export const CHARGE_LABEL_KEYS: Record<ChargeType, string> = {
  per_day: "extras.charge.per_day",
  per_unit: "extras.charge.per_unit",
  per_rental: "extras.charge.per_rental",
};

export const CHARGE_HINT_KEYS: Record<ChargeType, string> = {
  per_day: "extras.charge.per_day_hint",
  per_unit: "extras.charge.per_unit_hint",
  per_rental: "extras.charge.per_rental_hint",
};

export const STATUS_LABEL_KEYS: Record<ExtraStatus, string> = {
  draft: "extras.status.draft",
  published: "extras.status.published",
  archived: "extras.status.archived",
};

export const STATUS_TONES: Record<
  ExtraStatus,
  "default" | "success" | "warning" | "danger"
> = {
  draft: "warning",
  published: "success",
  archived: "default",
};

export const KIND_LABEL_KEYS: Record<AllocationKind, string> = {
  hold: "extras.allocations.kind.hold",
  reservation: "extras.allocations.kind.reservation",
  rental: "extras.allocations.kind.rental",
};

export const ALLOCATION_STATUS_LABEL_KEYS: Record<AllocationStatus, string> = {
  active: "extras.allocations.status.active",
  released: "extras.allocations.status.released",
  expired: "extras.allocations.status.expired",
};

export const ALLOCATION_STATUS_TONES: Record<
  AllocationStatus,
  "default" | "success" | "warning" | "danger"
> = {
  active: "success",
  released: "default",
  expired: "warning",
};

export const PRICE_SOURCE_LABEL_KEYS: Record<string, string> = {
  product: "extras.prices.source.product",
  default: "extras.prices.source.default",
  none: "extras.prices.source.none",
};
