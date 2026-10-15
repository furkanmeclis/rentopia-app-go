import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type FieldTask = Schemas["FieldTask"];
export type FieldTaskPage = Schemas["EnvelopeFieldTaskPage"]["data"];
export type FieldTaskEvent = Schemas["FieldTaskEvent"];
export type FieldTaskFacets = Schemas["FieldTaskFacets"];
export type FieldTaskAssignee = Schemas["FieldTaskAssignee"];
export type FieldTaskCreateRequest = Schemas["FieldTaskCreateRequest"];
export type FieldTaskPatchRequest = Schemas["FieldTaskPatchRequest"];
export type FieldTaskStatus = FieldTask["status"];
export type FieldTaskType = FieldTask["task_type"];
export type FieldTaskAction = FieldTask["actions"][number];
export type FieldTaskLocationKind = FieldTask["location"]["kind"];
export type DeliveryPoint = Schemas["DeliveryPoint"];
export type DeliveryDetails = Schemas["DeliveryDetails"];

/** List query of the board and "my tasks" (multi-values comma-joined). */
export type ListFieldTasksParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  task_type?: string;
  location_kind?: string;
  branch?: string;
  assignee?: string;
  unassigned?: string;
  reservation?: string;
  due_from?: string;
  due_to?: string;
};
