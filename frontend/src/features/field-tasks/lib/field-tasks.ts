import { addDays, startOfDay } from "date-fns";

import {
  DELIVERY_PROBLEM_KEYS,
  FIELD_TASK_LIVE_STATUSES,
  PROBLEM_KEYS,
} from "@/features/field-tasks/constants";
import type {
  FieldTask,
  FieldTaskAction,
  FieldTaskAssignee,
  FieldTaskStatus,
} from "@/features/field-tasks/types";
import { ApiError } from "@/lib/api";

export function isLive(status: FieldTaskStatus) {
  return FIELD_TASK_LIVE_STATUSES.includes(status);
}

export function can(task: FieldTask, action: FieldTaskAction) {
  return task.actions.includes(action);
}

/** Late: still to do and its planned start passed. */
export function isLate(task: FieldTask, now = new Date()) {
  return isLive(task.status) && new Date(task.due_at) < now;
}

/** Where staff go: the address, the service point or the branch. */
export function placeText(task: FieldTask): string {
  const loc = task.location;
  if (loc.kind === "address") {
    return [loc.address, loc.district, loc.city].filter(Boolean).join(", ");
  }
  if (loc.kind === "point" && loc.point) {
    return `${loc.point.name} · ${task.branch.code}`;
  }
  return `${task.branch.code} · ${task.branch.name}`;
}

/** A map search link of the task's place (coordinates when known). */
export function mapsHref(task: FieldTask): string | null {
  const loc = task.location;
  if (loc.latitude != null && loc.longitude != null) {
    return `https://www.google.com/maps/search/?api=1&query=${loc.latitude},${loc.longitude}`;
  }
  const text =
    loc.kind === "address"
      ? placeText(task)
      : loc.kind === "point" && loc.point
        ? loc.point.name
        : null;
  return text
    ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(text)}`
    : null;
}

export function personName(
  person: { name: string; surname: string; email: string } | null | undefined,
): string {
  if (!person) return "";
  return `${person.name} ${person.surname}`.trim() || person.email;
}

export function assigneeLabel(a: FieldTaskAssignee) {
  return personName(a);
}

/** Local [start, end) of today + `offset` days, as RFC 3339 instants. */
export function dayWindow(offset: number, base = new Date()) {
  const start = addDays(startOfDay(base), offset);
  return {
    start,
    end: addDays(start, 1),
    from: start.toISOString(),
    to: addDays(start, 1).toISOString(),
  };
}

/** Text of a failed field task action (known 409 codes translated). */
export function fieldTaskProblemText(
  t: (key: string) => string,
  error: unknown,
): string {
  if (!(error instanceof ApiError)) return String(error);
  const detail = error.details[0];
  const key = detail?.code ? PROBLEM_KEYS[detail.code] : undefined;
  if (key) return t(key);
  if (error.details.length > 0 && error.status === 400) {
    return error.details
      .map((d) => {
        const k = d.code ? PROBLEM_KEYS[d.code] : undefined;
        return k ? t(k) : d.field ? `${d.field}: ${d.message}` : d.message;
      })
      .join(" · ");
  }
  return error.message;
}

/** Delivery eligibility problems of a quote (`pickup_*` / `return_*`). */
export function deliveryProblems(
  t: (key: string, values?: Record<string, string | number>) => string,
  error: unknown,
): string[] {
  if (!(error instanceof ApiError)) return [];
  const out: string[] = [];
  for (const d of error.details) {
    const field = d.field ?? "";
    if (!/^(pickup|return)_(point_uuid|delivery\.)/.test(field)) continue;
    const key = d.code ? DELIVERY_PROBLEM_KEYS[d.code] : undefined;
    if (!key) continue;
    const side = field.startsWith("pickup")
      ? t("field_tasks.delivery.pickup")
      : t("field_tasks.delivery.return");
    out.push(`${side}: ${t(key)}`);
  }
  return out;
}
