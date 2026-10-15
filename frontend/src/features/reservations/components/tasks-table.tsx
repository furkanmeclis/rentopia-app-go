"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, ExternalLink, RefreshCw, UserCheck } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { AppForm } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { MemberPickerField } from "@/features/reservations/components/member-picker";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import {
  ReferenceText,
  TaskStatusChip,
  actorName,
} from "@/features/reservations/components/reservation-cells";
import {
  TASKS_RESOURCE,
  TASK_STATUSES,
  TASK_STATUS_LABEL_KEYS,
  TASK_TYPES,
  TASK_TYPE_LABEL_KEYS,
} from "@/features/reservations/constants";
import {
  useAssignTask,
  useReplanTask,
  useResolveTask,
  useTasksList,
} from "@/features/reservations/hooks/use-reservations";
import type {
  ListTasksParams,
  ResolutionTask,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const ASSIGN_FORM_ID = "resolution-task-assign";

/**
 * Resolution tasks ("Çözüm görevleri"): broken vehicle plans (a block or a
 * release the engine could not absorb, a refused extension, a failed
 * re-plan). Resolve with a note, assign, or retry the engine re-plan.
 * `reservationUuid` limits the table to one reservation (detail tab).
 */
export function TasksTable({
  reservationUuid,
  persistKey,
  fromUrl = false,
}: {
  reservationUuid?: string;
  persistKey: string;
  /** Start from the URL's `status` / `task_type` filters (list page). */
  fromUrl?: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const urlDefaults = useUrlListDefaults(["status", "task_type"]);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialColumnFilters: fromUrl
      ? urlDefaults.initialColumnFilters
      : undefined,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, TASKS_RESOURCE),
  });
  const [resolveTarget, setResolveTarget] = useState<ResolutionTask | null>(
    null,
  );
  const [assignTarget, setAssignTarget] = useState<ResolutionTask | null>(null);
  const resolve = useResolveTask();
  const assign = useAssignTask();
  const replan = useReplanTask();
  const write = permissions.tenant.resolutionTasks.write;

  const params = useMemo<ListTasksParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      task_type: joinFilter(column("task_type")),
      reservation: reservationUuid,
    };
  }, [listState.columnFilters, listState.params, reservationUuid]);
  const query = useTasksList(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ResolutionTask>({
          accessorKey: "task_type",
          labelKey: "reservations.tasks.columns.task_type",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: TASK_TYPES.map((v) => ({
            value: v,
            labelKey: TASK_TYPE_LABEL_KEYS[v],
            label: t(TASK_TYPE_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(TASK_TYPE_LABEL_KEYS[row.original.task_type])}
            </Badge>
          ),
        }),
        createColumn<ResolutionTask>({
          accessorKey: "status",
          labelKey: "reservations.tasks.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: TASK_STATUSES.map((v) => ({
            value: v,
            labelKey: TASK_STATUS_LABEL_KEYS[v],
            label: t(TASK_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => <TaskStatusChip status={row.original.status} />,
        }),
        ...(reservationUuid
          ? []
          : [
              createColumn<ResolutionTask>({
                id: "reservation",
                accessorFn: (row) => row.reservation.reference,
                labelKey: "reservations.tasks.columns.reservation",
                enableSorting: false,
                gridSecondary: true,
                cell: ({ row }) => (
                  <ReferenceText
                    reference={row.original.reservation.reference}
                  />
                ),
              }),
            ]),
        createColumn<ResolutionTask>({
          id: "starts_at",
          accessorFn: (row) => row.reservation.starts_at,
          labelKey: "reservations.tasks.columns.starts_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.reservation.starts_at, "d MMM HH:mm", locale),
        }),
        createColumn<ResolutionTask>({
          accessorKey: "reason",
          labelKey: "reservations.tasks.columns.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 text-sm">{row.original.reason}</span>
          ),
        }),
        createColumn<ResolutionTask>({
          id: "assignee",
          accessorFn: (row) => row.assignee?.email ?? "",
          labelKey: "reservations.tasks.columns.assignee",
          enableSorting: false,
          cell: ({ row }) => actorName(row.original.assignee),
        }),
        createColumn<ResolutionTask>({
          accessorKey: "resolution_note",
          labelKey: "reservations.tasks.columns.resolution_note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.resolution_note || "—",
        }),
        createColumn<ResolutionTask>({
          accessorKey: "created_at",
          labelKey: "reservations.tasks.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<ResolutionTask>({
          id: "actions",
          labelKey: "reservations.tasks.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const task = row.original;
            const open = task.status === "open";
            return (
              <EntityRowActions
                actions={[
                  ...(reservationUuid
                    ? []
                    : [
                        {
                          id: "open",
                          label: t(
                            "reservations.tasks.actions.open_reservation",
                          ),
                          icon: ExternalLink,
                          onSelect: () =>
                            router.push(
                              routes.tenant.reservations.detail(
                                slug,
                                task.reservation.uuid,
                              ),
                            ),
                        },
                      ]),
                  ...(open
                    ? [
                        {
                          id: "replan",
                          label: t("reservations.tasks.actions.replan"),
                          icon: RefreshCw,
                          permission: write,
                          onSelect: () => replan.mutate(task.uuid),
                        },
                        {
                          id: "assign",
                          label: t("reservations.tasks.actions.assign"),
                          icon: UserCheck,
                          permission: write,
                          onSelect: () => setAssignTarget(task),
                        },
                        {
                          id: "resolve",
                          label: t("reservations.tasks.actions.resolve"),
                          icon: CheckCircle2,
                          permission: write,
                          onSelect: () => setResolveTarget(task),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ResolutionTask, unknown>[],
    [locale, replan, reservationUuid, router, slug, t, write],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("reservations.tasks.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("reservations.tasks.empty_title")}
        emptyDescription={t("reservations.tasks.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <ReasonDialog
        open={Boolean(resolveTarget)}
        title={t("reservations.tasks.resolve_title")}
        description={resolveTarget?.reason}
        label={t("reservations.tasks.resolution_note")}
        confirmLabel={t("reservations.tasks.actions.resolve")}
        required={false}
        pending={resolve.isPending}
        onCancel={() => setResolveTarget(null)}
        onConfirm={(note) => {
          if (!resolveTarget) return;
          resolve.mutate(
            { uuid: resolveTarget.uuid, note },
            { onSuccess: () => setResolveTarget(null) },
          );
        }}
      />
      <AppDialog
        open={Boolean(assignTarget)}
        onOpenChange={(open) => {
          if (!open) setAssignTarget(null);
        }}
        title={t("reservations.tasks.assign_title")}
        className="sm:max-w-md"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setAssignTarget(null)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={ASSIGN_FORM_ID}
              disabled={assign.isPending}
            >
              {t("reservations.actions.save")}
            </Button>
          </>
        }
      >
        {assignTarget ? (
          <AppForm<{ user_uuid: string }>
            id={ASSIGN_FORM_ID}
            schema={z.object({ user_uuid: z.string() })}
            defaultValues={{ user_uuid: assignTarget.assignee?.uuid ?? "" }}
            onSubmit={async (values) => {
              await assign.mutateAsync({
                uuid: assignTarget.uuid,
                userUuid: values.user_uuid || null,
              });
              setAssignTarget(null);
            }}
          >
            <MemberPickerField
              name="user_uuid"
              label={t("reservations.fields.assigned_user")}
              description={t("reservations.assign.clear_hint")}
            />
          </AppForm>
        ) : null}
      </AppDialog>
    </>
  );
}
