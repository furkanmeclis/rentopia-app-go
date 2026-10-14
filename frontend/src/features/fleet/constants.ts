import type {
  BodyType,
  Fuel,
  FuelPolicy,
  GuaranteeBadge,
  GuaranteeType,
  ProductStatus,
  Transmission,
} from "@/features/fleet/types";

/** Engine resource slugs shared by io / bulk / activity. */
export const SEGMENTS_RESOURCE = "tenant.segments" as const;
export const RENTAL_GROUPS_RESOURCE = "tenant.rental_groups" as const;
export const RENTAL_PRODUCTS_RESOURCE = "tenant.rental_products" as const;

export const TRANSMISSIONS: Transmission[] = ["manual", "automatic"];
export const FUELS: Fuel[] = ["petrol", "diesel", "hybrid", "electric", "lpg"];
export const BODY_TYPES: BodyType[] = [
  "hatchback",
  "sedan",
  "suv",
  "crossover",
  "station_wagon",
  "mpv",
  "van",
  "pickup",
  "coupe",
  "cabrio",
];
export const GUARANTEE_TYPES: GuaranteeType[] = [
  "GROUP",
  "EXACT_VARIANT",
  "EXACT_VEHICLE",
];
export const PRODUCT_STATUSES: ProductStatus[] = [
  "draft",
  "published",
  "archived",
];
export const FUEL_POLICIES: FuelPolicy[] = [
  "same_to_same",
  "full_to_full",
  "prepaid",
  "full_to_empty",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const TRANSMISSION_LABEL_KEYS: Record<Transmission, string> = {
  manual: "fleet.transmission.manual",
  automatic: "fleet.transmission.automatic",
};

export const FUEL_LABEL_KEYS: Record<Fuel, string> = {
  petrol: "fleet.fuel.petrol",
  diesel: "fleet.fuel.diesel",
  hybrid: "fleet.fuel.hybrid",
  electric: "fleet.fuel.electric",
  lpg: "fleet.fuel.lpg",
};

export const BODY_TYPE_LABEL_KEYS: Record<BodyType, string> = {
  hatchback: "fleet.body_type.hatchback",
  sedan: "fleet.body_type.sedan",
  suv: "fleet.body_type.suv",
  crossover: "fleet.body_type.crossover",
  station_wagon: "fleet.body_type.station_wagon",
  mpv: "fleet.body_type.mpv",
  van: "fleet.body_type.van",
  pickup: "fleet.body_type.pickup",
  coupe: "fleet.body_type.coupe",
  cabrio: "fleet.body_type.cabrio",
};

export const GUARANTEE_LABEL_KEYS: Record<GuaranteeType, string> = {
  GROUP: "fleet.guarantee.group",
  EXACT_VARIANT: "fleet.guarantee.exact_variant",
  EXACT_VEHICLE: "fleet.guarantee.exact_vehicle",
};

export const GUARANTEE_HINT_KEYS: Record<GuaranteeType, string> = {
  GROUP: "fleet.guarantee.group_hint",
  EXACT_VARIANT: "fleet.guarantee.exact_variant_hint",
  EXACT_VEHICLE: "fleet.guarantee.exact_vehicle_hint",
};

export const GUARANTEE_EXAMPLE_KEYS: Record<GuaranteeType, string> = {
  GROUP: "fleet.guarantee.group_example",
  EXACT_VARIANT: "fleet.guarantee.exact_variant_example",
  EXACT_VEHICLE: "fleet.guarantee.exact_vehicle_example",
};

export const BADGE_LABEL_KEYS: Record<GuaranteeBadge, string> = {
  or_similar: "fleet.badge.or_similar",
  model_guaranteed: "fleet.badge.model_guaranteed",
  vehicle_guaranteed: "fleet.badge.vehicle_guaranteed",
};

export const STATUS_LABEL_KEYS: Record<ProductStatus, string> = {
  draft: "fleet.status.draft",
  published: "fleet.status.published",
  archived: "fleet.status.archived",
};

export const STATUS_TONES: Record<
  ProductStatus,
  "default" | "success" | "warning"
> = {
  draft: "warning",
  published: "success",
  archived: "default",
};

export const FUEL_POLICY_LABEL_KEYS: Record<FuelPolicy, string> = {
  same_to_same: "fleet.fuel_policy.same_to_same",
  full_to_full: "fleet.fuel_policy.full_to_full",
  prepaid: "fleet.fuel_policy.prepaid",
  full_to_empty: "fleet.fuel_policy.full_to_empty",
};

export const VARIANT_LEVEL_LABEL_KEYS = {
  model: "fleet.guarantee.level_model",
  type: "fleet.guarantee.level_type",
} as const;

export const CODE_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$/;
export const MONEY_PATTERN = /^[0-9]{1,12}([.,][0-9]{1,2})?$/;

/**
 * ACRISS (SIPP) letters per position; mirrors the API validation. The code
 * is a classification hint only, never a guarantee.
 */
export const ACRISS_LETTERS = [
  "MNEHCDIJSRFGPULWOX",
  "BCDWVLSTFJXPQZEMRHYNGK",
  "MNCABD",
  "RNDQHIECLSABMFVZUX",
] as const;

export function isValidAcriss(code: string): boolean {
  if (code.length !== 4) return false;
  return [...code].every((letter, index) =>
    ACRISS_LETTERS[index]!.includes(letter),
  );
}

/** Max images per product and per-file size (mirrors the API). */
export const MAX_PRODUCT_IMAGES = 12;
export const MAX_PRODUCT_IMAGE_BYTES = 5 * 1024 * 1024;
