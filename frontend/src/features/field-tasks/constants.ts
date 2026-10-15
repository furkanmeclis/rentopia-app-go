import type {
  FieldTask,
  FieldTaskLocationKind,
  FieldTaskStatus,
  FieldTaskType,
} from "@/features/field-tasks/types";

/** Live / engine resources: the board and the executors' own list. */
export const FIELD_TASKS_RESOURCE = "tenant.field_tasks" as const;
export const MY_FIELD_TASKS_RESOURCE = "tenant.my_field_tasks" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const FIELD_TASK_STATUSES: FieldTaskStatus[] = [
  "planned",
  "assigned",
  "en_route",
  "issue_reported",
  "completed",
  "cancelled",
];

export const FIELD_TASK_LIVE_STATUSES: FieldTaskStatus[] = [
  "planned",
  "assigned",
  "en_route",
  "issue_reported",
];

export const FIELD_TASK_STATUS_LABEL_KEYS: Record<FieldTaskStatus, string> = {
  planned: "field_tasks.statuses.planned",
  assigned: "field_tasks.statuses.assigned",
  en_route: "field_tasks.statuses.en_route",
  issue_reported: "field_tasks.statuses.issue_reported",
  completed: "field_tasks.statuses.completed",
  cancelled: "field_tasks.statuses.cancelled",
};

export const FIELD_TASK_STATUS_TONES: Record<FieldTaskStatus, Tone> = {
  planned: "warning",
  assigned: "default",
  en_route: "default",
  issue_reported: "danger",
  completed: "success",
  cancelled: "default",
};

export const FIELD_TASK_TYPES: FieldTaskType[] = [
  "handover",
  "return",
  "transfer",
  "preparation",
  "cleaning",
  "maintenance",
  "deposit",
  "other",
];

/** Types staff open by hand (handover / return come from reservations). */
export const MANUAL_TASK_TYPES: FieldTaskType[] = [
  "transfer",
  "preparation",
  "cleaning",
  "maintenance",
  "other",
];

export const FIELD_TASK_TYPE_LABEL_KEYS: Record<FieldTaskType, string> = {
  handover: "field_tasks.types.handover",
  return: "field_tasks.types.return",
  transfer: "field_tasks.types.transfer",
  preparation: "field_tasks.types.preparation",
  cleaning: "field_tasks.types.cleaning",
  maintenance: "field_tasks.types.maintenance",
  deposit: "field_tasks.types.deposit",
  other: "field_tasks.types.other",
};

/** A handover / return at a delivery point or an address. */
export const DELIVERY_TYPE_LABEL_KEYS: Record<"handover" | "return", string> = {
  handover: "field_tasks.types.delivery",
  return: "field_tasks.types.collection",
};

export function taskTypeLabel(
  t: (key: string) => string,
  task: Pick<FieldTask, "task_type" | "delivery">,
) {
  if (
    task.delivery &&
    (task.task_type === "handover" || task.task_type === "return")
  ) {
    return t(DELIVERY_TYPE_LABEL_KEYS[task.task_type]);
  }
  return t(FIELD_TASK_TYPE_LABEL_KEYS[task.task_type]);
}

export const LOCATION_KINDS: FieldTaskLocationKind[] = [
  "branch",
  "point",
  "address",
];

export const LOCATION_KIND_LABEL_KEYS: Record<FieldTaskLocationKind, string> = {
  branch: "field_tasks.location_kinds.branch",
  point: "field_tasks.location_kinds.point",
  address: "field_tasks.location_kinds.address",
};

/** Timeline actions. */
export const EVENT_LABEL_KEYS: Record<string, string> = {
  created: "field_tasks.events.created",
  status_changed: "field_tasks.events.status_changed",
  assignee_changed: "field_tasks.events.assignee_changed",
  rescheduled: "field_tasks.events.rescheduled",
  relocated: "field_tasks.events.relocated",
};

/** 409 detail codes of the field task API. */
export const PROBLEM_KEYS: Record<string, string> = {
  invalid_transition: "field_tasks.problems.invalid_transition",
  task_closed: "field_tasks.problems.task_closed",
  record_handover: "field_tasks.problems.record_handover",
  record_return: "field_tasks.problems.record_return",
  concurrent: "field_tasks.problems.concurrent",
  transfer_missing: "field_tasks.problems.transfer_missing",
  vehicle_not_at_origin: "field_tasks.problems.vehicle_not_at_origin",
  vehicle_busy: "field_tasks.problems.vehicle_busy",
  transfer_overlap: "field_tasks.problems.transfer_overlap",
  block_overlap: "field_tasks.problems.block_overlap",
  not_found: "field_tasks.problems.member_not_found",
  not_field_staff: "field_tasks.problems.not_field_staff",
  branch_not_allowed: "field_tasks.problems.branch_not_allowed",
};

/** Delivery eligibility codes a quote answers (`<side>_point_uuid` …). */
export const DELIVERY_PROBLEM_KEYS: Record<string, string> = {
  not_delivery_point: "field_tasks.delivery.problems.not_delivery_point",
  inactive: "field_tasks.delivery.problems.inactive",
  not_online: "field_tasks.delivery.problems.not_online",
  too_soon: "field_tasks.delivery.problems.too_soon",
  outside_hours: "field_tasks.delivery.problems.outside_hours",
  closed: "field_tasks.delivery.problems.closed",
  address_required: "field_tasks.delivery.problems.address_required",
  outside_area: "field_tasks.delivery.problems.outside_area",
  point_required: "field_tasks.delivery.problems.point_required",
};
