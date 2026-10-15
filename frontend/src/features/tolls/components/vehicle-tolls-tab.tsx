"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CalendarX2, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { StatusChip } from "@/components/common/status-chip";
import { permissions } from "@/config/permissions";
import {
  isoToLocalInput,
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import { tollProblemText } from "@/features/tolls/components/toll-decision-dialogs";
import { TollPassesTable } from "@/features/tolls/components/toll-passes-table";
import {
  TAG_NETWORKS,
  TAG_NETWORK_LABEL_KEYS,
} from "@/features/tolls/constants";
import {
  useTollTagMutation,
  useVehicleTollTags,
} from "@/features/tolls/hooks/use-tolls";
import type { TollTag } from "@/features/tolls/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TAG_FORM = "vehicle-toll-tag-form";
const END_FORM = "vehicle-toll-tag-end-form";

type TagValues = {
  provider: string;
  network: string;
  tag_number: string;
  valid_from: string;
  note: string;
};

function AddTagDialog({
  vehicleUuid,
  open,
  onClose,
}: {
  vehicleUuid: string;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const mutation = useTollTagMutation(vehicleUuid);
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        provider: z
          .string()
          .trim()
          .regex(/^[a-z0-9][a-z0-9_]{1,31}$/, t("tolls.validation.provider")),
        network: z.string(),
        tag_number: z
          .string()
          .trim()
          .min(4, t("tolls.validation.tag_number"))
          .max(40, t("tolls.validation.tag_number")),
        valid_from: z.string().min(1, t("tolls.validation.required")),
        note: z.string().max(500),
      }),
    [t],
  );
  const close = () => {
    setProblem(null);
    onClose();
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !mutation.isPending) close();
      }}
      title={t("tolls.tags.add_title")}
      description={t("tolls.tags.add_description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={TAG_FORM} disabled={mutation.isPending}>
            {t("tolls.tags.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<TagValues>
            id={TAG_FORM}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              provider: "hgs",
              network: "hgs",
              tag_number: "",
              valid_from: localInputFromNow(0),
              note: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await mutation.mutateAsync({
                  action: "create",
                  body: {
                    provider: values.provider.trim(),
                    network: values.network as "hgs" | "ogs",
                    tag_number: values.tag_number.trim(),
                    valid_from: localInputToISO(values.valid_from) ?? "",
                    note: values.note.trim() || undefined,
                  },
                });
                close();
              } catch (error) {
                setProblem(tollProblemText(t, error));
              }
            }}
          >
            <AppInput
              name="tag_number"
              label={t("tolls.tags.fields.tag_number")}
            />
            <AppSelect
              name="network"
              label={t("tolls.tags.fields.network")}
              options={TAG_NETWORKS.map((n) => ({
                value: n,
                label: t(TAG_NETWORK_LABEL_KEYS[n]),
              }))}
            />
            <AppInput
              name="provider"
              label={t("tolls.tags.fields.provider")}
              description={t("tolls.tags.fields.provider_hint")}
            />
            <AppDatePicker
              name="valid_from"
              withTime
              label={t("tolls.tags.fields.valid_from")}
            />
            <AppTextarea
              name="note"
              rows={2}
              label={t("tolls.tags.fields.note")}
            />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}

function EndTagDialog({
  vehicleUuid,
  tag,
  onClose,
}: {
  vehicleUuid: string;
  tag: TollTag | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const mutation = useTollTagMutation(vehicleUuid);
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        valid_to: z.string().min(1, t("tolls.validation.required")),
      }),
    [t],
  );
  const close = () => {
    setProblem(null);
    onClose();
  };
  return (
    <AppDialog
      open={Boolean(tag)}
      onOpenChange={(next) => {
        if (!next && !mutation.isPending) close();
      }}
      title={t("tolls.tags.end_title")}
      description={
        tag ? t("tolls.tags.end_description", { tag: tag.tag_number }) : ""
      }
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={END_FORM} disabled={mutation.isPending}>
            {t("tolls.tags.end")}
          </Button>
        </>
      }
    >
      {tag ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<{ valid_to: string }>
            id={END_FORM}
            schema={schema}
            defaultValues={{
              valid_to: tag.valid_to
                ? isoToLocalInput(tag.valid_to)
                : localInputFromNow(0),
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await mutation.mutateAsync({
                  action: "update",
                  tag: tag.uuid,
                  body: { valid_to: localInputToISO(values.valid_to) ?? null },
                });
                close();
              } catch (error) {
                setProblem(tollProblemText(t, error));
              }
            }}
          >
            <AppDatePicker
              name="valid_to"
              withTime
              label={t("tolls.tags.fields.valid_to")}
            />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}

/**
 * Vehicle detail "HGS / OGS": the tags the vehicle carried over time
 * (add, end, delete a mistaken one) and its passes.
 */
export function VehicleTollsTab({ vehicleUuid }: { vehicleUuid: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.vehicles.write);
  const tags = useVehicleTollTags(vehicleUuid);
  const mutation = useTollTagMutation(vehicleUuid);
  const [adding, setAdding] = useState(false);
  const [ending, setEnding] = useState<TollTag | null>(null);
  const [deleting, setDeleting] = useState<TollTag | null>(null);
  const items = tags.data?.items ?? [];

  const columns = useMemo(
    () =>
      [
        createColumn<TollTag>({
          accessorKey: "tag_number",
          labelKey: "tolls.tags.columns.tag_number",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-sm font-medium">
              {row.original.tag_number}
            </span>
          ),
        }),
        createColumn<TollTag>({
          accessorKey: "network",
          labelKey: "tolls.tags.columns.network",
          enableSorting: false,
          cell: ({ row }) =>
            `${t(TAG_NETWORK_LABEL_KEYS[row.original.network])} · ${row.original.provider}`,
        }),
        createColumn<TollTag>({
          id: "period",
          accessorFn: (row) => row.valid_from,
          labelKey: "tolls.tags.columns.period",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {datetime(row.original.valid_from, "d MMM yyyy HH:mm", locale)} →{" "}
              {row.original.valid_to
                ? datetime(row.original.valid_to, "d MMM yyyy HH:mm", locale)
                : t("tolls.tags.open_ended")}
            </span>
          ),
        }),
        createColumn<TollTag>({
          accessorKey: "active",
          labelKey: "tolls.tags.columns.active",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("tolls.tags.active")
                  : t("tolls.tags.inactive")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<TollTag>({
          accessorKey: "note",
          labelKey: "tolls.tags.columns.note",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<TollTag>({
          id: "actions",
          labelKey: "tolls.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canWrite ? (
              <EntityRowActions
                actions={[
                  {
                    id: "end",
                    label: t("tolls.tags.end"),
                    icon: CalendarX2,
                    onSelect: () => setEnding(row.original),
                  },
                  {
                    id: "delete",
                    label: t("tolls.tags.delete"),
                    icon: Trash2,
                    variant: "destructive",
                    onSelect: () => setDeleting(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<TollTag, unknown>[],
    [canWrite, locale, t],
  );

  return (
    <div className="space-y-6">
      <EntitySectionCard
        title={t("tolls.tags.title")}
        badge={items.length}
        actions={
          canWrite ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setAdding(true)}
            >
              <Plus className="size-4" />
              {t("tolls.tags.add")}
            </Button>
          ) : null
        }
      >
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(row) => row.uuid}
          isLoading={tags.isLoading}
          isError={tags.isError}
          onRetry={() => void tags.refetch()}
          emptyTitle={t("tolls.tags.empty")}
          emptyDescription={t("tolls.tags.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-vehicle-toll-tags-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
      </EntitySectionCard>
      <EntitySectionCard title={t("tolls.vehicle.passes_title")}>
        <TollPassesTable
          mode="vehicle"
          vehicleUuid={vehicleUuid}
          persistKey="tenant-vehicle-toll-passes-v1"
        />
      </EntitySectionCard>
      <AddTagDialog
        vehicleUuid={vehicleUuid}
        open={adding}
        onClose={() => setAdding(false)}
      />
      <EndTagDialog
        vehicleUuid={vehicleUuid}
        tag={ending}
        onClose={() => setEnding(null)}
      />
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={deleting?.tag_number ?? ""}
        softDelete={false}
        isPending={mutation.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() => {
          if (!deleting) return;
          mutation.mutate(
            { action: "delete", tag: deleting.uuid },
            { onSuccess: () => setDeleting(null) },
          );
        }}
      />
    </div>
  );
}
