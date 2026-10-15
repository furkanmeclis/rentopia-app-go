"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, CheckCircle2, ExternalLink, Play, UserCheck } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
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
import { OperationTaskStatusChip } from "@/features/rentals/components/rental-cells";
import {
  OPERATION_TASKS_RESOURCE,
  TASK_STATUSES,
  TASK_STATUS_LABEL_KEYS,
  TASK_TYPES,
  TASK_TYPE_LABEL_KEYS,
} from "@/features/rentals/constants";
import {
  useAssignOperationTask,
  useOperationTaskFacets,
  useOperationTasks,
  useTaskAction,
} from "@/features/rentals/hooks/use-rentals";
import type {
  ListOperationTasksParams,
  OperationTask,
  RentalFacetValue,
} from "@/features/rentals/types";
import { MemberPickerField } from "@/features/reservations/components/member-picker";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import {
  ReferenceText,
  actorName,
} from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const ASSIGN_FORM_ID = "operation-task-assign";

function countOf(facets: RentalFacetValue[] | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

type Pending = { task: OperationTask; action: "complete" | "cancel" } | null;

/**
 * Operation tasks ("Operasyon görevleri"): preparation, handover, return
 * and cleaning work of the branches. Handover / return tasks close with
 * the rental action; finishing a cleaning task makes the vehicle
 * available. `rentalUuid` limits the table to one rental (detail page).
 */
export function OperationTasksTable({
  rentalUuid,
  persistKey,
}: {
  rentalUuid?: string;
  persistKey: string;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "due_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, OPERATION_TASKS_RESOURCE),
  });
  const [pending, setPending] = useState<Pending>(null);
  const [assignTarget, setAssignTarget] = useState<OperationTask | null>(null);
  const action = useTaskAction();
  const assign = useAssignOperationTask();
  const facetsQuery = useOperationTaskFacets();
  const facets = facetsQuery.data;
  const write = permissions.tenant.rentals.write;

  const params = useMemo<ListOperationTasksParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      task_type: joinFilter(column("task_type")),
      branch: joinFilter(column("branch")),
      rental: rentalUuid,
    };
  }, [listState.columnFilters, listState.params, rentalUuid]);
  const query = useOperationTasks(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<OperationTask>({
          accessorKey: "task_type",
          labelKey: "rentals.tasks.columns.task_type",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: TASK_TYPES.map((v) => ({
            value: v,
            labelKey: TASK_TYPE_LABEL_KEYS[v],
            label: `${t(TASK_TYPE_LABEL_KEYS[v])} (${countOf(facets?.task_type, v)})`,
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(TASK_TYPE_LABEL_KEYS[row.original.task_type])}
            </Badge>
          ),
        }),
        createColumn<OperationTask>({
          accessorKey: "status",
          labelKey: "rentals.tasks.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: TASK_STATUSES.map((v) => ({
            value: v,
            labelKey: TASK_STATUS_LABEL_KEYS[v],
            label: `${t(TASK_STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => (
            <OperationTaskStatusChip status={row.original.status} />
          ),
        }),
        ...(rentalUuid
          ? []
          : [
              createColumn<OperationTask>({
                id: "rental",
                accessorFn: (row) => row.rental?.reference ?? "",
                labelKey: "rentals.tasks.columns.rental",
                enableSorting: false,
                gridSecondary: true,
                cell: ({ row }) =>
                  row.original.rental ? (
                    <ReferenceText reference={row.original.rental.reference} />
                  ) : (
                    "—"
                  ),
              }),
            ]),
        createColumn<OperationTask>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle?.plate ?? "",
          labelKey: "rentals.tasks.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.vehicle ? (
              <span className="font-mono text-xs">
                {row.original.vehicle.plate}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<OperationTask>({
          id: "branch",
          accessorFn: (row) => row.branch.code ?? row.branch.name,
          labelKey: "rentals.tasks.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.branch ?? []).map((f) => ({
            value: f.value,
            label: `${f.label || f.value} (${f.count})`,
          })),
          cell: ({ row }) =>
            row.original.branch.code ?? row.original.branch.name,
        }),
        createColumn<OperationTask>({
          accessorKey: "due_at",
          labelKey: "rentals.tasks.columns.due_at",
          enableSorting: true,
          cell: ({ row }) => {
            const late =
              (row.original.status === "open" ||
                row.original.status === "in_progress") &&
              new Date(row.original.due_at) < new Date();
            return (
              <span className={late ? "text-destructive font-medium" : ""}>
                {datetime(row.original.due_at, "d MMM yyyy HH:mm", locale)}
              </span>
            );
          },
        }),
        createColumn<OperationTask>({
          id: "assignee",
          accessorFn: (row) => row.assignee?.email ?? "",
          labelKey: "rentals.tasks.columns.assignee",
          enableSorting: false,
          cell: ({ row }) => actorName(row.original.assignee),
        }),
        createColumn<OperationTask>({
          accessorKey: "note",
          labelKey: "rentals.tasks.columns.note",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 text-sm">
              {row.original.note || "—"}
            </span>
          ),
        }),
        createColumn<OperationTask>({
          id: "completed_at",
          accessorFn: (row) => row.completed_at ?? "",
          labelKey: "rentals.tasks.columns.completed_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.completed_at
              ? `${datetime(row.original.completed_at, "d MMM HH:mm", locale)} · ${actorName(row.original.completed_by)}`
              : "—",
        }),
        createColumn<OperationTask>({
          accessorKey: "created_at",
          labelKey: "rentals.tasks.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<OperationTask>({
          id: "actions",
          labelKey: "rentals.tasks.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const task = row.original;
            const live =
              task.status === "open" || task.status === "in_progress";
            // Handover / return tasks close with the rental action.
            const manual =
              task.task_type === "preparation" || task.task_type === "cleaning";
            return (
              <EntityRowActions
                actions={[
                  ...(!rentalUuid && task.rental
                    ? [
                        {
                          id: "open",
                          label: t("rentals.tasks.actions.open_rental"),
                          icon: ExternalLink,
                          onSelect: () =>
                            router.push(
                              routes.tenant.rentals.detail(
                                slug,
                                task.rental!.uuid,
                              ),
                            ),
                        },
                      ]
                    : []),
                  ...(task.status === "open"
                    ? [
                        {
                          id: "start",
                          label: t("rentals.tasks.actions.start"),
                          icon: Play,
                          permission: write,
                          onSelect: () =>
                            action.mutate({ uuid: task.uuid, action: "start" }),
                        },
                      ]
                    : []),
                  ...(live
                    ? [
                        {
                          id: "assign",
                          label: t("rentals.tasks.actions.assign"),
                          icon: UserCheck,
                          permission: write,
                          onSelect: () => setAssignTarget(task),
                        },
                        ...(manual
                          ? [
                              {
                                id: "complete",
                                label: t("rentals.tasks.actions.complete"),
                                icon: CheckCircle2,
                                permission: write,
                                onSelect: () =>
                                  setPending({ task, action: "complete" }),
                              },
                            ]
                          : []),
                        {
                          id: "cancel",
                          label: t("rentals.tasks.actions.cancel"),
                          icon: Ban,
                          permission: write,
                          variant: "destructive" as const,
                          onSelect: () =>
                            setPending({ task, action: "cancel" }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<OperationTask, unknown>[],
    [action, facets, locale, rentalUuid, router, slug, t, write],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("rentals.tasks.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([query.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={t("rentals.tasks.empty_title")}
        emptyDescription={t("rentals.tasks.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void query.refetch();
              void facetsQuery.refetch();
            }}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <ReasonDialog
        open={Boolean(pending)}
        title={
          pending?.action === "cancel"
            ? t("rentals.tasks.cancel_title")
            : t("rentals.tasks.complete_title")
        }
        description={
          pending?.action === "complete" &&
          pending.task.task_type === "cleaning"
            ? t("rentals.tasks.complete_cleaning_hint")
            : undefined
        }
        label={t("rentals.tasks.note")}
        confirmLabel={
          pending?.action === "cancel"
            ? t("rentals.tasks.actions.cancel")
            : t("rentals.tasks.actions.complete")
        }
        destructive={pending?.action === "cancel"}
        required={pending?.action === "cancel"}
        pending={action.isPending}
        onCancel={() => setPending(null)}
        onConfirm={(note) => {
          if (!pending) return;
          action.mutate(
            { uuid: pending.task.uuid, action: pending.action, note },
            { onSuccess: () => setPending(null) },
          );
        }}
      />
      <AppDialog
        open={Boolean(assignTarget)}
        onOpenChange={(open) => {
          if (!open) setAssignTarget(null);
        }}
        title={t("rentals.tasks.assign_title")}
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
              {t("rentals.actions.save")}
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
              label={t("rentals.tasks.columns.assignee")}
              description={t("rentals.tasks.assign_hint")}
            />
          </AppForm>
        ) : null}
      </AppDialog>
    </>
  );
}
