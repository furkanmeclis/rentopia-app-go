import type {
  DamageSeverity,
  DamageType,
  InspectionKind,
  InspectionReviewStatus,
  InspectionSignatureStatus,
  InspectionStatus,
  VehicleStatusAfter,
} from "@/features/inspections/types";

/** Live list resources. */
export const INSPECTIONS_RESOURCE = "tenant.inspections" as const;
export const INSPECTION_TEMPLATES_RESOURCE =
  "tenant.inspection_templates" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const INSPECTION_KINDS: InspectionKind[] = ["handover", "return"];
export const INSPECTION_STATUSES: InspectionStatus[] = [
  "draft",
  "finalized",
  "superseded",
];
export const REVIEW_STATUSES: InspectionReviewStatus[] = [
  "none",
  "pending",
  "resolved",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const KIND_LABEL_KEYS: Record<InspectionKind, string> = {
  handover: "inspections.kinds.handover",
  return: "inspections.kinds.return",
};

export const STATUS_LABEL_KEYS: Record<InspectionStatus, string> = {
  draft: "inspections.statuses.draft",
  finalized: "inspections.statuses.finalized",
  superseded: "inspections.statuses.superseded",
};

export const STATUS_TONES: Record<InspectionStatus, Tone> = {
  draft: "warning",
  finalized: "success",
  superseded: "default",
};

export const REVIEW_LABEL_KEYS: Record<InspectionReviewStatus, string> = {
  none: "inspections.review.none",
  pending: "inspections.review.pending",
  resolved: "inspections.review.resolved",
};

export const REVIEW_TONES: Record<InspectionReviewStatus, Tone> = {
  none: "default",
  pending: "danger",
  resolved: "success",
};

export const SIGNATURE_LABEL_KEYS: Record<InspectionSignatureStatus, string> = {
  pending: "inspections.signature_status.pending",
  signed: "inspections.signature_status.signed",
  refused: "inspections.signature_status.refused",
  waived: "inspections.signature_status.waived",
};

export const DAMAGE_TYPES: DamageType[] = [
  "scratch",
  "dent",
  "crack",
  "broken",
  "missing",
  "stain",
  "other",
];

export const DAMAGE_TYPE_LABEL_KEYS: Record<DamageType, string> = {
  scratch: "inspections.damage_types.scratch",
  dent: "inspections.damage_types.dent",
  crack: "inspections.damage_types.crack",
  broken: "inspections.damage_types.broken",
  missing: "inspections.damage_types.missing",
  stain: "inspections.damage_types.stain",
  other: "inspections.damage_types.other",
};

export const SEVERITIES: DamageSeverity[] = ["minor", "moderate", "major"];

export const SEVERITY_LABEL_KEYS: Record<DamageSeverity, string> = {
  minor: "inspections.severities.minor",
  moderate: "inspections.severities.moderate",
  major: "inspections.severities.major",
};

export const VEHICLE_STATUSES_AFTER: VehicleStatusAfter[] = [
  "cleaning",
  "maintenance",
  "preparing",
  "available",
];

export const VEHICLE_STATUS_AFTER_LABEL_KEYS: Record<
  VehicleStatusAfter,
  string
> = {
  cleaning: "inspections.vehicle_after.cleaning",
  maintenance: "inspections.vehicle_after.maintenance",
  preparing: "inspections.vehicle_after.preparing",
  available: "inspections.vehicle_after.available",
};

/** Zones of the vehicle diagram (the API refuses anything else). */
export const ZONES = [
  "front_bumper",
  "hood",
  "windshield",
  "roof",
  "rear_window",
  "trunk",
  "rear_bumper",
  "left_front_fender",
  "left_front_door",
  "left_rear_door",
  "left_rear_fender",
  "left_mirror",
  "right_front_fender",
  "right_front_door",
  "right_rear_door",
  "right_rear_fender",
  "right_mirror",
  "left_front_wheel",
  "left_rear_wheel",
  "right_front_wheel",
  "right_rear_wheel",
  "interior",
] as const;

export type Zone = (typeof ZONES)[number];

export const ZONE_LABEL_KEYS: Record<Zone, string> = {
  front_bumper: "inspections.zones.front_bumper",
  hood: "inspections.zones.hood",
  windshield: "inspections.zones.windshield",
  roof: "inspections.zones.roof",
  rear_window: "inspections.zones.rear_window",
  trunk: "inspections.zones.trunk",
  rear_bumper: "inspections.zones.rear_bumper",
  left_front_fender: "inspections.zones.left_front_fender",
  left_front_door: "inspections.zones.left_front_door",
  left_rear_door: "inspections.zones.left_rear_door",
  left_rear_fender: "inspections.zones.left_rear_fender",
  left_mirror: "inspections.zones.left_mirror",
  right_front_fender: "inspections.zones.right_front_fender",
  right_front_door: "inspections.zones.right_front_door",
  right_rear_door: "inspections.zones.right_rear_door",
  right_rear_fender: "inspections.zones.right_rear_fender",
  right_mirror: "inspections.zones.right_mirror",
  left_front_wheel: "inspections.zones.left_front_wheel",
  left_rear_wheel: "inspections.zones.left_rear_wheel",
  right_front_wheel: "inspections.zones.right_front_wheel",
  right_rear_wheel: "inspections.zones.right_rear_wheel",
  interior: "inspections.zones.interior",
};

export function zoneLabel(t: (key: string) => string, zone: string) {
  const key = ZONE_LABEL_KEYS[zone as Zone];
  return key ? t(key) : zone;
}

/** Wizard steps in order (handover: 8, return: 8 with the charges review). */
export const HANDOVER_STEPS = [
  "verify",
  "amounts",
  "readings",
  "damages",
  "photos",
  "review",
  "signature",
  "complete",
] as const;

export const RETURN_STEPS = [
  "verify",
  "readings",
  "damages",
  "photos",
  "charges",
  "review",
  "signature",
  "complete",
] as const;

export type WizardStep =
  (typeof HANDOVER_STEPS)[number] | (typeof RETURN_STEPS)[number];

export const STEP_LABEL_KEYS: Record<WizardStep, string> = {
  verify: "inspections.steps.verify",
  amounts: "inspections.steps.amounts",
  readings: "inspections.steps.readings",
  damages: "inspections.steps.damages",
  photos: "inspections.steps.photos",
  charges: "inspections.steps.charges",
  review: "inspections.steps.review",
  signature: "inspections.steps.signature",
  complete: "inspections.steps.complete",
};

/** Accepted image types (the API sniffs the bytes as well). */
export const PHOTO_TYPES = [
  "image/jpeg",
  "image/png",
  "image/webp",
  "image/heic",
  "image/heif",
] as const;

export const MAX_PHOTO_BYTES = 15 * 1024 * 1024;
