"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import type { ComboboxOption } from "@/components/ui/async-combobox";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import type { BulkActionDef } from "@/features/bulk-engine";
import {
  FIELD_TASK_TYPE_LABEL_KEYS,
  MANUAL_TASK_TYPES,
} from "@/features/field-tasks/constants";
import {
  useAssignFieldTask,
  useCreateFieldTask,
  useFieldTaskAssignees,
  usePatchFieldTask,
} from "@/features/field-tasks/hooks/use-field-tasks";
import {
  assigneeLabel,
  fieldTaskProblemText,
} from "@/features/field-tasks/lib/field-tasks";
import type {
  FieldTask,
  FieldTaskAssignee,
  FieldTaskType,
} from "@/features/field-tasks/types";
import {
  isoToLocalInput,
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { vehiclesService } from "@/features/vehicles/services/vehicles.service";
import { useLocale } from "@/providers/locale-provider";

/** Select value of "nobody" (Radix items cannot be empty). */
const NONE = "none";

/** Assignees who cover a branch (every branch, or that one). */
export function assigneesFor(
  people: FieldTaskAssignee[],
  branchUuids: string[],
) {
  return people.filter(
    (p) =>
      p.branch_scope === "all" ||
      branchUuids.some((b) => p.branch_uuids.includes(b)),
  );
}

const ASSIGN_FORM_ID = "field-task-assign-form";

/** Assign one task to a member who works its branch (or unassign). */
export function AssignFieldTaskDialog({
  task,
  onClose,
}: {
  task: FieldTask | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const people = useFieldTaskAssignees(Boolean(task));
  const assign = useAssignFieldTask();
  const [error, setError] = useState<string | null>(null);
  const options = useMemo(() => {
    if (!task) return [];
    const branches = [task.branch.uuid, task.to_branch?.uuid ?? ""].filter(
      Boolean,
    );
    return [
      { value: NONE, label: t("field_tasks.assign.nobody") },
      ...assigneesFor(people.data ?? [], branches).map((p) => ({
        value: p.uuid,
        label: assigneeLabel(p),
      })),
    ];
  }, [people.data, t, task]);
  return (
    <AppDialog
      open={Boolean(task)}
      onOpenChange={(open) => {
        if (!open) {
          setError(null);
          onClose();
        }
      }}
      title={t("field_tasks.assign.title")}
      description={t("field_tasks.assign.description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={ASSIGN_FORM_ID}
            disabled={assign.isPending}
          >
            {t("field_tasks.actions.assign")}
          </Button>
        </>
      }
    >
      {task ? (
        <AppForm<{ user_uuid: string }>
          key={task.uuid}
          id={ASSIGN_FORM_ID}
          className="space-y-4"
          schema={z.object({ user_uuid: z.string() })}
          defaultValues={{ user_uuid: task.assignee?.uuid ?? NONE }}
          onSubmit={async (values) => {
            setError(null);
            try {
              await assign.mutateAsync({
                uuid: task.uuid,
                userUuid: values.user_uuid === NONE ? null : values.user_uuid,
              });
              onClose();
            } catch (e) {
              setError(fieldTaskProblemText(t, e));
            }
          }}
        >
          <AppSelect
            name="user_uuid"
            label={t("field_tasks.fields.assignee")}
            description={t("field_tasks.assign.hint")}
            options={options}
          />
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

const BULK_FORM_ID = "field-task-bulk-params";

type BulkRequest = {
  action: BulkActionDef;
  resolve: (params: Record<string, string> | null) => void;
};

/** Params of the bulk assign action (a member, or nobody). */
export function useFieldTaskBulkParams(): {
  resolveParams: (
    action: BulkActionDef,
  ) => Promise<Record<string, string> | null | undefined>;
  dialog: ReactNode;
} {
  const { t } = useLocale();
  const [request, setRequest] = useState<BulkRequest | null>(null);
  const people = useFieldTaskAssignees(Boolean(request));
  const resolveParams = useCallback(
    (action: BulkActionDef) =>
      new Promise<Record<string, string> | null>((resolve) =>
        setRequest({ action, resolve }),
      ),
    [],
  );
  const finish = (params: Record<string, string> | null) => {
    request?.resolve(params);
    setRequest(null);
  };
  const options = [
    { value: NONE, label: t("field_tasks.assign.nobody") },
    ...(people.data ?? []).map((p) => ({
      value: p.uuid,
      label: assigneeLabel(p),
    })),
  ];
  const dialog = (
    <AppDialog
      open={Boolean(request)}
      onOpenChange={(open) => {
        if (!open) finish(null);
      }}
      title={request ? t(request.action.label_key) : ""}
      description={t("field_tasks.bulk.assign_hint")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => finish(null)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={BULK_FORM_ID}>
            {t("field_tasks.bulk.continue")}
          </Button>
        </>
      }
    >
      {request ? (
        <AppForm<{ user_uuid: string }>
          key={request.action.id}
          id={BULK_FORM_ID}
          className="space-y-4"
          schema={z.object({ user_uuid: z.string() })}
          defaultValues={{ user_uuid: NONE }}
          onSubmit={async (values) =>
            finish({
              user_uuid: values.user_uuid === NONE ? "" : values.user_uuid,
            })
          }
        >
          <AppSelect
            name="user_uuid"
            label={t("field_tasks.fields.assignee")}
            options={options}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
  return { resolveParams, dialog };
}

const CREATE_FORM_ID = "field-task-create-form";

type CreateValues = {
  task_type: string;
  branch_uuid: string;
  to_branch_uuid: string;
  vehicle_uuid: string;
  due_at: string;
  planned_minutes: string;
  assignee_uuid: string;
  address: string;
  contact_name: string;
  contact_phone: string;
  note: string;
};

/** Open a task by hand: transfer, preparation, cleaning, maintenance, other. */
export function CreateFieldTaskDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const create = useCreateFieldTask();
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );
  const people = useFieldTaskAssignees(open);
  const [error, setError] = useState<string | null>(null);
  const branchOptions = (branches.data?.items ?? [])
    .filter((b) => b.is_active)
    .map((b) => ({ value: b.uuid, label: `${b.code} · ${b.name}` }));
  const loadVehicles = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await vehiclesService.list(slug, {
        limit: 20,
        offset: 0,
        sort: "plate",
        q: q || undefined,
        is_active: "true",
      });
      return page.items.map((v) => ({
        value: v.uuid,
        label: v.plate,
        description: `${v.label} · ${v.current_branch.name}`,
      }));
    },
    [slug],
  );
  const schema = useMemo(
    () =>
      z
        .object({
          task_type: z.string().min(1),
          branch_uuid: z.string().min(1, t("field_tasks.validation.branch")),
          to_branch_uuid: z.string(),
          vehicle_uuid: z.string(),
          due_at: z.string().min(1, t("field_tasks.validation.due_at")),
          planned_minutes: z
            .string()
            .regex(/^\d*$/, t("field_tasks.validation.minutes")),
          assignee_uuid: z.string(),
          address: z.string().max(500),
          contact_name: z.string().max(255),
          contact_phone: z.string().max(32),
          note: z.string().max(1000),
        })
        .superRefine((v, ctx) => {
          if (v.task_type !== "transfer") return;
          if (!v.vehicle_uuid) {
            ctx.addIssue({
              code: "custom",
              path: ["vehicle_uuid"],
              message: t("field_tasks.validation.vehicle"),
            });
          }
          if (!v.to_branch_uuid || v.to_branch_uuid === NONE) {
            ctx.addIssue({
              code: "custom",
              path: ["to_branch_uuid"],
              message: t("field_tasks.validation.to_branch"),
            });
          }
        }),
    [t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setError(null);
        onOpenChange(next);
      }}
      title={t("field_tasks.create.title")}
      description={t("field_tasks.create.description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={CREATE_FORM_ID}
            disabled={create.isPending}
          >
            {t("field_tasks.actions.create")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<CreateValues>
          id={CREATE_FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            task_type: "cleaning",
            branch_uuid: "",
            to_branch_uuid: NONE,
            vehicle_uuid: "",
            due_at: localInputFromNow(2),
            planned_minutes: "",
            assignee_uuid: NONE,
            address: "",
            contact_name: "",
            contact_phone: "",
            note: "",
          }}
          onSubmit={async (v) => {
            setError(null);
            try {
              await create.mutateAsync({
                task_type: v.task_type as Exclude<
                  FieldTaskType,
                  "handover" | "return" | "deposit"
                >,
                branch_uuid: v.branch_uuid,
                to_branch_uuid:
                  v.task_type === "transfer" && v.to_branch_uuid !== NONE
                    ? v.to_branch_uuid
                    : null,
                vehicle_uuid: v.vehicle_uuid || null,
                due_at: localInputToISO(v.due_at) ?? v.due_at,
                planned_minutes: v.planned_minutes
                  ? Number(v.planned_minutes)
                  : null,
                assignee_uuid:
                  v.assignee_uuid === NONE ? null : v.assignee_uuid,
                address: v.address.trim() || null,
                contact_name: v.contact_name.trim() || null,
                contact_phone: v.contact_phone.trim() || null,
                note: v.note.trim(),
              });
              onOpenChange(false);
            } catch (e) {
              setError(fieldTaskProblemText(t, e));
            }
          }}
        >
          {(form) => {
            const type = form.watch("task_type");
            const branch = form.watch("branch_uuid");
            const assigneeOptions = [
              { value: NONE, label: t("field_tasks.assign.nobody") },
              ...assigneesFor(people.data ?? [], branch ? [branch] : []).map(
                (p) => ({ value: p.uuid, label: assigneeLabel(p) }),
              ),
            ];
            return (
              <>
                <AppSelect
                  name="task_type"
                  label={t("field_tasks.fields.task_type")}
                  options={MANUAL_TASK_TYPES.map((v) => ({
                    value: v,
                    label: t(FIELD_TASK_TYPE_LABEL_KEYS[v]),
                  }))}
                />
                <AppSelect
                  name="branch_uuid"
                  label={
                    type === "transfer"
                      ? t("field_tasks.fields.from_branch")
                      : t("field_tasks.fields.branch")
                  }
                  placeholder={t("field_tasks.fields.branch_placeholder")}
                  options={branchOptions}
                />
                {type === "transfer" ? (
                  <AppSelect
                    name="to_branch_uuid"
                    label={t("field_tasks.fields.to_branch")}
                    options={[
                      { value: NONE, label: t("field_tasks.fields.choose") },
                      ...branchOptions.filter((o) => o.value !== branch),
                    ]}
                  />
                ) : null}
                <AppCombobox
                  name="vehicle_uuid"
                  label={t("field_tasks.fields.vehicle")}
                  loadOptions={loadVehicles}
                  placeholder={t("field_tasks.fields.vehicle_placeholder")}
                  searchPlaceholder={t("field_tasks.fields.vehicle_search")}
                  emptyText={t("field_tasks.fields.vehicle_empty")}
                  clearable
                />
                <AppDatePicker
                  name="due_at"
                  withTime
                  label={t("field_tasks.fields.due_at")}
                />
                <AppInput
                  name="planned_minutes"
                  inputMode="numeric"
                  label={t("field_tasks.fields.planned_minutes")}
                  description={t("field_tasks.fields.planned_minutes_hint")}
                />
                <AppSelect
                  name="assignee_uuid"
                  label={t("field_tasks.fields.assignee")}
                  options={assigneeOptions}
                />
                <AppInput
                  name="address"
                  label={t("field_tasks.fields.address")}
                  description={t("field_tasks.fields.address_hint")}
                />
                <AppInput
                  name="contact_name"
                  label={t("field_tasks.fields.contact_name")}
                />
                <AppInput
                  name="contact_phone"
                  inputMode="tel"
                  label={t("field_tasks.fields.contact_phone")}
                />
                <AppTextarea
                  name="note"
                  label={t("field_tasks.fields.note")}
                  rows={2}
                  className="sm:col-span-2"
                />
                {error ? (
                  <Alert variant="destructive" className="sm:col-span-2">
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                ) : null}
              </>
            );
          }}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

const EDIT_FORM_ID = "field-task-edit-form";

type EditValues = {
  due_at: string;
  planned_minutes: string;
  address: string;
  contact_name: string;
  contact_phone: string;
  flight_number: string;
  meeting_note: string;
  note: string;
};

/** Re-plan a live task: time, duration, place, contact, meeting details. */
export function EditFieldTaskDialog({
  task,
  onClose,
}: {
  task: FieldTask | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const patch = usePatchFieldTask();
  const [error, setError] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        due_at: z.string().min(1, t("field_tasks.validation.due_at")),
        planned_minutes: z
          .string()
          .regex(/^\d+$/, t("field_tasks.validation.minutes")),
        address: z.string().max(500),
        contact_name: z.string().max(255),
        contact_phone: z.string().max(32),
        flight_number: z.string().max(16),
        meeting_note: z.string().max(1000),
        note: z.string().max(1000),
      }),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(task)}
      onOpenChange={(open) => {
        if (!open) {
          setError(null);
          onClose();
        }
      }}
      title={t("field_tasks.edit.title")}
      description={t("field_tasks.edit.description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={EDIT_FORM_ID} disabled={patch.isPending}>
            {t("field_tasks.actions.save")}
          </Button>
        </>
      }
    >
      {task ? (
        <AppForm<EditValues>
          key={task.uuid}
          id={EDIT_FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            due_at: isoToLocalInput(task.due_at),
            planned_minutes: String(task.planned_minutes),
            address: task.location.address,
            contact_name: task.contact.name,
            contact_phone: task.contact.phone,
            flight_number: task.flight_number,
            meeting_note: task.meeting_note,
            note: task.note,
          }}
          onSubmit={async (v) => {
            setError(null);
            try {
              await patch.mutateAsync({
                uuid: task.uuid,
                body: {
                  due_at: localInputToISO(v.due_at) ?? v.due_at,
                  planned_minutes: Number(v.planned_minutes),
                  address: v.address.trim(),
                  // The customer's contact shows when the task has none;
                  // only a change is stored on the task.
                  contact_name:
                    v.contact_name.trim() !== task.contact.name
                      ? v.contact_name.trim()
                      : undefined,
                  contact_phone:
                    v.contact_phone.trim() !== task.contact.phone
                      ? v.contact_phone.trim()
                      : undefined,
                  flight_number: v.flight_number.trim(),
                  meeting_note: v.meeting_note.trim(),
                  note: v.note.trim(),
                },
              });
              onClose();
            } catch (e) {
              setError(fieldTaskProblemText(t, e));
            }
          }}
        >
          <AppDatePicker
            name="due_at"
            withTime
            label={t("field_tasks.fields.due_at")}
          />
          <AppInput
            name="planned_minutes"
            inputMode="numeric"
            label={t("field_tasks.fields.planned_minutes")}
          />
          <AppInput
            name="address"
            label={t("field_tasks.fields.address")}
            description={t("field_tasks.fields.address_hint")}
            className="sm:col-span-2"
          />
          <AppInput
            name="contact_name"
            label={t("field_tasks.fields.contact_name")}
          />
          <AppInput
            name="contact_phone"
            inputMode="tel"
            label={t("field_tasks.fields.contact_phone")}
          />
          <AppInput
            name="flight_number"
            label={t("field_tasks.fields.flight_number")}
          />
          <AppTextarea
            name="meeting_note"
            label={t("field_tasks.fields.meeting_note")}
            rows={2}
          />
          <AppTextarea
            name="note"
            label={t("field_tasks.fields.note")}
            rows={2}
            className="sm:col-span-2"
          />
          {error ? (
            <Alert variant="destructive" className="sm:col-span-2">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
