"use client";

import { useMemo } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCheckbox,
  AppDatePicker,
  AppForm,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  BLOCK_KINDS,
  BLOCK_KIND_LABEL_KEYS,
  STATUS_SYNC_KINDS,
} from "@/features/fleet-ops/constants";
import {
  useCreateBlock,
  useReleaseBlock,
  useUpdateBlock,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import {
  isoToLocalInput,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import type { BlockKind, VehicleBlock } from "@/features/fleet-ops/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "vehicle-block-form";
const RELEASE_FORM_ID = "vehicle-block-release-form";

/** What the block dialog edits: a new block, "Bakıma al" or an existing one. */
export type BlockDialogMode =
  | { type: "create" }
  | { type: "maintenance" }
  | { type: "edit"; block: VehicleBlock };

type Values = {
  kind: string;
  starts_at: string;
  ends_at: string;
  reason: string;
  sync_status: boolean;
};

/** "Araç durumunu da güncelle", only for kinds with a vehicle status. */
function SyncStatusField() {
  const { t } = useLocale();
  const kind = useWatch<Values, "kind">({ name: "kind" });
  const startsAt = useWatch<Values, "starts_at">({ name: "starts_at" });
  if (!STATUS_SYNC_KINDS.includes(kind as BlockKind)) return null;
  const startsLater = Boolean(startsAt) && new Date(startsAt) > new Date();
  if (startsLater) return null;
  return (
    <AppCheckbox
      name="sync_status"
      label={t("vehicle_blocks.fields.sync_status")}
      description={t("vehicle_blocks.fields.sync_status_hint")}
      className="sm:col-span-2"
    />
  );
}

/**
 * Create / edit a block. "Bakıma al" is the create form reduced to the
 * reason and the status checkbox (maintenance, from now, open-ended).
 */
export function BlockDialog({
  vehicleUuid,
  mode,
  onOpenChange,
}: {
  vehicleUuid: string;
  mode: BlockDialogMode | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const create = useCreateBlock(vehicleUuid);
  const update = useUpdateBlock(vehicleUuid);
  const schema = useMemo(
    () =>
      z
        .object({
          kind: z.string().min(1, t("vehicle_blocks.validation.kind")),
          starts_at: z.string(),
          ends_at: z.string(),
          reason: z.string().max(2000),
          sync_status: z.boolean(),
        })
        .refine(
          (v) =>
            !v.ends_at ||
            !v.starts_at ||
            new Date(v.ends_at) > new Date(v.starts_at),
          {
            path: ["ends_at"],
            message: t("vehicle_blocks.validation.ends_after_start"),
          },
        ),
    [t],
  );

  const block = mode?.type === "edit" ? mode.block : null;
  const quick = mode?.type === "maintenance";
  const defaults: Values = block
    ? {
        kind: block.kind,
        starts_at: isoToLocalInput(block.starts_at),
        ends_at: isoToLocalInput(block.ends_at),
        reason: block.reason,
        sync_status: false,
      }
    : {
        kind: "maintenance",
        starts_at: "",
        ends_at: "",
        reason: "",
        sync_status: true,
      };
  const title = quick
    ? t("vehicle_blocks.dialog.maintenance_title")
    : block
      ? t("vehicle_blocks.dialog.edit_title")
      : t("vehicle_blocks.dialog.create_title");
  const description = quick
    ? t("vehicle_blocks.dialog.maintenance_description")
    : t("vehicle_blocks.dialog.description");
  const pending = create.isPending || update.isPending;

  return (
    <AppDialog
      open={Boolean(mode)}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={pending}>
            {quick
              ? t("vehicle_blocks.actions.start_maintenance")
              : t("vehicle_blocks.actions.save")}
          </Button>
        </>
      }
    >
      {mode ? (
        <AppForm<Values>
          key={block?.uuid ?? mode.type}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={defaults}
          onSubmit={async (values) => {
            const startsAt = localInputToISO(values.starts_at);
            const endsAt = localInputToISO(values.ends_at) ?? null;
            if (block) {
              await update.mutateAsync({
                uuid: block.uuid,
                body: {
                  kind: values.kind as BlockKind,
                  ...(startsAt ? { starts_at: startsAt } : {}),
                  ends_at: endsAt,
                  reason: values.reason.trim(),
                },
              });
            } else {
              const syncable = STATUS_SYNC_KINDS.includes(
                values.kind as BlockKind,
              );
              await create.mutateAsync({
                kind: values.kind as BlockKind,
                ...(startsAt ? { starts_at: startsAt } : {}),
                ...(endsAt ? { ends_at: endsAt } : {}),
                reason: values.reason.trim(),
                sync_status:
                  syncable &&
                  values.sync_status &&
                  (!startsAt || new Date(startsAt) <= new Date()),
              });
            }
            onOpenChange(false);
          }}
        >
          {quick ? null : (
            <>
              <AppSelect
                name="kind"
                label={t("vehicle_blocks.fields.kind")}
                className="sm:col-span-2"
                options={BLOCK_KINDS.map((value) => ({
                  value,
                  label: t(BLOCK_KIND_LABEL_KEYS[value]),
                }))}
              />
              <AppDatePicker
                name="starts_at"
                withTime
                label={t("vehicle_blocks.fields.starts_at")}
                placeholder={t("vehicle_blocks.fields.starts_now")}
              />
              <AppDatePicker
                name="ends_at"
                withTime
                label={t("vehicle_blocks.fields.ends_at")}
                placeholder={t("vehicle_blocks.open_ended")}
              />
            </>
          )}
          <AppTextarea
            name="reason"
            label={t("vehicle_blocks.fields.reason")}
            className="sm:col-span-2"
          />
          {block ? null : <SyncStatusField />}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** "Engeli kapat": the block ends now, optionally the vehicle is available again. */
export function ReleaseBlockDialog({
  vehicleUuid,
  block,
  onOpenChange,
}: {
  vehicleUuid: string;
  block: VehicleBlock | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const release = useReleaseBlock(vehicleUuid);
  const schema = useMemo(() => z.object({ sync_status: z.boolean() }), []);
  const syncable = block ? STATUS_SYNC_KINDS.includes(block.kind) : false;

  return (
    <AppDialog
      open={Boolean(block)}
      onOpenChange={onOpenChange}
      title={t("vehicle_blocks.dialog.release_title")}
      description={t("vehicle_blocks.dialog.release_description")}
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
            form={RELEASE_FORM_ID}
            disabled={release.isPending}
          >
            {t("vehicle_blocks.actions.release")}
          </Button>
        </>
      }
    >
      {block ? (
        <AppForm<{ sync_status: boolean }>
          key={block.uuid}
          id={RELEASE_FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ sync_status: syncable }}
          onSubmit={async (values) => {
            await release.mutateAsync({
              uuid: block.uuid,
              body: { sync_status: syncable && values.sync_status },
            });
            onOpenChange(false);
          }}
        >
          {syncable ? (
            <AppCheckbox
              name="sync_status"
              label={t("vehicle_blocks.fields.release_sync_status")}
            />
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
