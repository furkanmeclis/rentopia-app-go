import type {
  ChargeKind,
  ManualChargeKind,
  ChargeStatus,
  OperationTaskStatus,
  OperationTaskType,
  RentalStatus,
} from "@/features/rentals/types";

/** Engine / live resource slugs. */
export const RENTALS_RESOURCE = "tenant.rentals" as const;
export const OPERATION_TASKS_RESOURCE = "tenant.operation_tasks" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const RENTAL_STATUSES: RentalStatus[] = [
  "preparation",
  "active",
  "return_check",
  "returned",
  "cancelled",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const RENTAL_STATUS_LABEL_KEYS: Record<RentalStatus, string> = {
  preparation: "rentals.statuses.preparation",
  active: "rentals.statuses.active",
  return_check: "rentals.statuses.return_check",
  returned: "rentals.statuses.returned",
  cancelled: "rentals.statuses.cancelled",
};

export const RENTAL_STATUS_TONES: Record<RentalStatus, Tone> = {
  preparation: "warning",
  active: "success",
  return_check: "warning",
  returned: "default",
  cancelled: "danger",
};

/** Kinds a person may propose (toll charges come from toll passes only). */
export const CHARGE_KINDS: ManualChargeKind[] = [
  "late_return",
  "early_return",
  "damage",
  "fuel",
  "mileage",
  "equipment",
  "other",
];

export const CHARGE_KIND_LABEL_KEYS: Record<ChargeKind, string> = {
  late_return: "rentals.charges.kinds.late_return",
  early_return: "rentals.charges.kinds.early_return",
  damage: "rentals.charges.kinds.damage",
  fuel: "rentals.charges.kinds.fuel",
  mileage: "rentals.charges.kinds.mileage",
  equipment: "rentals.charges.kinds.equipment",
  toll: "rentals.charges.kinds.toll",
  toll_fee: "rentals.charges.kinds.toll_fee",
  other: "rentals.charges.kinds.other",
};

export const CHARGE_STATUS_LABEL_KEYS: Record<ChargeStatus, string> = {
  proposed: "rentals.charges.statuses.proposed",
  approved: "rentals.charges.statuses.approved",
  waived: "rentals.charges.statuses.waived",
};

export const CHARGE_STATUS_TONES: Record<ChargeStatus, Tone> = {
  proposed: "warning",
  approved: "success",
  waived: "default",
};

export const TASK_TYPES: OperationTaskType[] = [
  "preparation",
  "handover",
  "return",
  "cleaning",
  "maintenance",
  "deposit",
];

export const TASK_TYPE_LABEL_KEYS: Record<OperationTaskType, string> = {
  preparation: "rentals.tasks.types.preparation",
  handover: "rentals.tasks.types.handover",
  return: "rentals.tasks.types.return",
  cleaning: "rentals.tasks.types.cleaning",
  maintenance: "rentals.tasks.types.maintenance",
  deposit: "rentals.tasks.types.deposit",
};

export const TASK_STATUSES: OperationTaskStatus[] = [
  "open",
  "in_progress",
  "done",
  "cancelled",
];

export const TASK_STATUS_LABEL_KEYS: Record<OperationTaskStatus, string> = {
  open: "rentals.tasks.statuses.open",
  in_progress: "rentals.tasks.statuses.in_progress",
  done: "rentals.tasks.statuses.done",
  cancelled: "rentals.tasks.statuses.cancelled",
};

export const TASK_STATUS_TONES: Record<OperationTaskStatus, Tone> = {
  open: "warning",
  in_progress: "default",
  done: "success",
  cancelled: "default",
};

/** Timeline actions (unknown actions fall back to the raw code). */
export const EVENT_LABEL_KEYS: Record<string, string> = {
  created: "rentals.events.actions.created",
  handed_over: "rentals.events.actions.handed_over",
  returned: "rentals.events.actions.returned",
  return_checked: "rentals.events.actions.return_checked",
  cancelled: "rentals.events.actions.cancelled",
  extended: "rentals.events.actions.extended",
  vehicle_swapped: "rentals.events.actions.vehicle_swapped",
  plate_changed: "rentals.events.actions.plate_changed",
  overdue: "rentals.events.actions.overdue",
  overdue_extended: "rentals.events.actions.overdue_extended",
  late_return_tasks: "rentals.events.actions.late_return_tasks",
  charge_proposed: "rentals.events.actions.charge_proposed",
  charge_approved: "rentals.events.actions.charge_approved",
  charge_waived: "rentals.events.actions.charge_waived",
  contract_waived: "rentals.events.actions.contract_waived",
  early_return: "rentals.events.actions.early_return",
  reservation_changed: "rentals.events.actions.reservation_changed",
  drivers_changed: "rentals.events.actions.drivers_changed",
  document_prepared: "rentals.events.actions.document_prepared",
  document_approved: "rentals.events.actions.document_approved",
  reapproval_required: "rentals.events.actions.reapproval_required",
  charge_overridden: "rentals.events.actions.charge_overridden",
  inspection_finalized: "rentals.events.actions.inspection_finalized",
  inspection_revised: "rentals.events.actions.inspection_revised",
  inspection_reviewed: "rentals.events.actions.inspection_reviewed",
  toll_charged: "rentals.events.actions.toll_charged",
  toll_reversed: "rentals.events.actions.toll_reversed",
};

export function eventLabel(t: (key: string) => string, action: string) {
  const key = EVENT_LABEL_KEYS[action];
  return key ? t(key) : action;
}

/** System reason codes of timeline rows (free-text reasons pass through). */
export const EVENT_REASON_KEYS: Record<string, string> = {
  rental_created: "rentals.events.reasons.rental_created",
  handover: "rentals.events.reasons.handover",
  return: "rentals.events.reasons.return",
  plate_changed: "rentals.events.reasons.plate_changed",
  vehicle_swapped: "rentals.events.reasons.vehicle_swapped",
  changed: "rentals.events.reasons.changed",
  drivers_changed: "rentals.events.reasons.drivers_changed",
};

export function eventReason(t: (key: string) => string, reason: string) {
  if (!reason) return "—";
  const key = EVENT_REASON_KEYS[reason];
  return key ? t(key) : reason;
}
