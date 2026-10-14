"use client";

import { useCallback, useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  type ComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  useCreateTransfer,
  useTransferAction,
  useTransferAssignees,
  useUpdateTransfer,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import {
  isoToLocalInput,
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import type { Transfer, TransferAction } from "@/features/fleet-ops/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { vehiclesService } from "@/features/vehicles/services/vehicles.service";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "transfer-form";
const ACTION_FORM_ID = "transfer-action-form";
/** Select values for "no choice" (Radix select items cannot be empty). */
const AUTO = "auto";
const NONE = "none";

type Values = {
  vehicle_uuid: string;
  from_branch_uuid: string;
  to_branch_uuid: string;
  responsible_user_uuid: string;
  planned_departure: string;
  planned_arrival: string;
  cost: string;
  note: string;
};

const MONEY_RE = /^\d{1,12}([.,]\d{1,2})?$/;

/**
 * Plan / edit a transfer. Opened from the transfers list (vehicle picked
 * here) or from a vehicle page (vehicle fixed). A started transfer only
 * edits its cost and note.
 */
export function TransferDialog({
  open,
  transfer,
  vehicleUuid,
  onOpenChange,
}: {
  open: boolean;
  transfer?: Transfer | null;
  /** Fixed vehicle (vehicle page). */
  vehicleUuid?: string;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const create = useCreateTransfer();
  const update = useUpdateTransfer();
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const assignees = useTransferAssignees("", open);
  const started = Boolean(transfer && transfer.status !== "planned");

  const schema = useMemo(
    () =>
      z
        .object({
          vehicle_uuid: z.string().min(1, t("transfers.validation.vehicle")),
          from_branch_uuid: z.string(),
          to_branch_uuid: z.string().min(1, t("transfers.validation.to")),
          responsible_user_uuid: z.string(),
          planned_departure: z
            .string()
            .min(1, t("transfers.validation.departure")),
          planned_arrival: z.string().min(1, t("transfers.validation.arrival")),
          cost: z
            .string()
            .trim()
            .refine(
              (v) => v === "" || MONEY_RE.test(v),
              t("transfers.validation.cost"),
            ),
          note: z.string().max(2000),
        })
        .refine(
          (v) => new Date(v.planned_arrival) > new Date(v.planned_departure),
          {
            path: ["planned_arrival"],
            message: t("transfers.validation.arrival_after_departure"),
          },
        )
        .refine((v) => v.from_branch_uuid !== v.to_branch_uuid, {
          path: ["to_branch_uuid"],
          message: t("transfers.validation.same_branch"),
        }),
    [t],
  );

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

  const branchOptions = (branches.data?.items ?? []).map((b) => ({
    value: b.uuid,
    label: `${b.code} · ${b.name}`,
  }));
  const assigneeOptions = [
    { value: NONE, label: t("transfers.fields.no_responsible") },
    ...(assignees.data?.items ?? []).map((m) => ({
      value: m.uuid,
      label: `${m.name} ${m.surname}`.trim() || m.email,
    })),
  ];

  const defaults: Values = transfer
    ? {
        vehicle_uuid: transfer.vehicle.uuid,
        from_branch_uuid: transfer.from_branch.uuid,
        to_branch_uuid: transfer.to_branch.uuid,
        responsible_user_uuid: transfer.responsible?.uuid ?? NONE,
        planned_departure: isoToLocalInput(transfer.planned_departure),
        planned_arrival: isoToLocalInput(transfer.planned_arrival),
        cost: transfer.cost ?? "",
        note: transfer.note,
      }
    : {
        vehicle_uuid: vehicleUuid ?? "",
        from_branch_uuid: AUTO,
        to_branch_uuid: "",
        responsible_user_uuid: NONE,
        planned_departure: localInputFromNow(1),
        planned_arrival: localInputFromNow(4),
        cost: "",
        note: "",
      };

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        transfer
          ? t("transfers.dialog.edit_title")
          : t("transfers.dialog.title")
      }
      description={
        started
          ? t("transfers.dialog.started_description")
          : t("transfers.dialog.description")
      }
      className="sm:max-w-2xl"
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
            form={FORM_ID}
            disabled={create.isPending || update.isPending}
          >
            {t("transfers.actions.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<Values>
          key={transfer?.uuid ?? vehicleUuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={defaults}
          onSubmit={async (values) => {
            const cost = values.cost.trim().replace(",", ".");
            const note = values.note.trim();
            if (transfer && started) {
              await update.mutateAsync({
                uuid: transfer.uuid,
                body: { cost: cost || null, note },
              });
              onOpenChange(false);
              return;
            }
            const plan = {
              ...(values.from_branch_uuid !== AUTO
                ? { from_branch_uuid: values.from_branch_uuid }
                : {}),
              to_branch_uuid: values.to_branch_uuid,
              responsible_user_uuid:
                values.responsible_user_uuid === NONE
                  ? null
                  : values.responsible_user_uuid,
              planned_departure: localInputToISO(values.planned_departure)!,
              planned_arrival: localInputToISO(values.planned_arrival)!,
              cost: cost || null,
              note,
            };
            if (transfer) {
              await update.mutateAsync({ uuid: transfer.uuid, body: plan });
            } else {
              await create.mutateAsync({
                ...plan,
                vehicle_uuid: values.vehicle_uuid,
              });
            }
            onOpenChange(false);
          }}
        >
          {vehicleUuid || transfer ? null : (
            <AppCombobox
              name="vehicle_uuid"
              label={t("transfers.fields.vehicle")}
              className="sm:col-span-2"
              loadOptions={loadVehicles}
              placeholder={t("transfers.fields.vehicle_placeholder")}
              searchPlaceholder={t("transfers.fields.vehicle_search")}
              emptyText={t("transfers.fields.vehicle_empty")}
            />
          )}
          <AppSelect
            name="from_branch_uuid"
            label={t("transfers.fields.from_branch")}
            description={t("transfers.fields.from_branch_hint")}
            disabled={started}
            options={[
              { value: AUTO, label: t("transfers.fields.from_auto") },
              ...branchOptions,
            ]}
          />
          <AppSelect
            name="to_branch_uuid"
            label={t("transfers.fields.to_branch")}
            placeholder={t("transfers.fields.branch_placeholder")}
            disabled={started}
            options={branchOptions}
          />
          <AppDatePicker
            name="planned_departure"
            withTime
            label={t("transfers.fields.planned_departure")}
            disabled={started}
          />
          <AppDatePicker
            name="planned_arrival"
            withTime
            label={t("transfers.fields.planned_arrival")}
            disabled={started}
          />
          <AppSelect
            name="responsible_user_uuid"
            label={t("transfers.fields.responsible")}
            disabled={started}
            options={assigneeOptions}
          />
          <AppInput
            name="cost"
            inputMode="decimal"
            label={t("transfers.fields.cost")}
            placeholder="0.00"
          />
          <AppTextarea
            name="note"
            label={t("transfers.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

const ACTION_TITLE_KEYS: Record<TransferAction, string> = {
  start: "transfers.action_dialog.start_title",
  complete: "transfers.action_dialog.complete_title",
  cancel: "transfers.action_dialog.cancel_title",
};

const ACTION_DESCRIPTION_KEYS: Record<TransferAction, string> = {
  start: "transfers.action_dialog.start_description",
  complete: "transfers.action_dialog.complete_description",
  cancel: "transfers.action_dialog.cancel_description",
};

const ACTION_SUBMIT_KEYS: Record<TransferAction, string> = {
  start: "transfers.actions.start",
  complete: "transfers.actions.complete",
  cancel: "transfers.actions.cancel",
};

type ActionValues = { at: string; arrival_status: string; note: string };

/**
 * Lifecycle confirmation: start (departure time), complete (arrival time
 * and the vehicle status after arrival) or cancel (reason).
 */
export function TransferActionDialog({
  target,
  onOpenChange,
}: {
  target: { transfer: Transfer; action: TransferAction } | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const run = useTransferAction();
  const schema = useMemo(
    () =>
      z.object({
        at: z.string(),
        arrival_status: z.string(),
        note: z.string().max(2000),
      }),
    [],
  );
  const action = target?.action;

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={onOpenChange}
      title={action ? t(ACTION_TITLE_KEYS[action]) : ""}
      description={
        target && action
          ? t(ACTION_DESCRIPTION_KEYS[action], {
              plate: target.transfer.vehicle.plate,
              from: target.transfer.from_branch.name,
              to: target.transfer.to_branch.name,
            })
          : undefined
      }
      className="sm:max-w-md"
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
            form={ACTION_FORM_ID}
            variant={action === "cancel" ? "destructive" : "default"}
            disabled={run.isPending}
          >
            {action ? t(ACTION_SUBMIT_KEYS[action]) : ""}
          </Button>
        </>
      }
    >
      {target && action ? (
        <AppForm<ActionValues>
          key={`${target.transfer.uuid}-${action}`}
          id={ACTION_FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ at: "", arrival_status: "preparing", note: "" }}
          onSubmit={async (values) => {
            const at = localInputToISO(values.at);
            await run.mutateAsync({
              uuid: target.transfer.uuid,
              action,
              body: {
                ...(at && action !== "cancel" ? { at } : {}),
                ...(action === "complete"
                  ? {
                      arrival_status: values.arrival_status as
                        "available" | "preparing",
                    }
                  : {}),
                note: values.note.trim(),
              },
            });
            onOpenChange(false);
          }}
        >
          {action !== "cancel" ? (
            <AppDatePicker
              name="at"
              withTime
              label={
                action === "start"
                  ? t("transfers.fields.actual_departure")
                  : t("transfers.fields.actual_arrival")
              }
              placeholder={t("transfers.fields.now")}
            />
          ) : null}
          {action === "complete" ? (
            <AppSelect
              name="arrival_status"
              label={t("transfers.fields.arrival_status")}
              description={t("transfers.fields.arrival_status_hint")}
              options={[
                {
                  value: "available",
                  label: t("transfers.arrival_status.available"),
                },
                {
                  value: "preparing",
                  label: t("transfers.arrival_status.preparing"),
                },
              ]}
            />
          ) : null}
          <AppTextarea
            name="note"
            label={
              action === "cancel"
                ? t("transfers.fields.cancel_reason")
                : t("transfers.fields.note")
            }
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
