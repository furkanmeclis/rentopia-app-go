"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { PlayCircle, QrCode } from "lucide-react";
import { useRouter } from "next/navigation";
import QRCode from "qrcode";
import { useEffect, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { OperationTaskStatusChip } from "@/features/rentals/components/rental-cells";
import {
  OPERATION_TASKS_RESOURCE,
  TASK_TYPE_LABEL_KEYS,
} from "@/features/rentals/constants";
import { useOperationTasks } from "@/features/rentals/hooks/use-rentals";
import type {
  ListOperationTasksParams,
  OperationTask,
} from "@/features/rentals/types";
import { ReferenceText } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FIELD_TYPES = ["handover", "return"] as const;

function wizardHref(slug: string, task: OperationTask) {
  if (!task.rental) return null;
  return task.task_type === "return"
    ? routes.tenant.field.return(slug, task.rental.uuid)
    : routes.tenant.field.handover(slug, task.rental.uuid);
}

/** QR of the task reference: scanning it opens the same wizard. */
function TaskQrDialog({
  task,
  onClose,
}: {
  task: OperationTask | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [qr, setQr] = useState<{ task: string; image: string } | null>(null);
  const image = task && qr?.task === task.uuid ? qr.image : null;
  useEffect(() => {
    if (!task) return;
    const href = wizardHref(slug, task);
    if (!href) return;
    const url = `${window.location.origin}${href}`;
    let alive = true;
    void QRCode.toDataURL(url, { margin: 1, width: 240 })
      .then((next) => {
        if (alive) setQr({ task: task.uuid, image: next });
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [slug, task]);
  return (
    <AppDialog
      open={Boolean(task)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("inspections.field.qr_title")}
      description={
        task?.rental
          ? `${task.rental.reference} · ${task.vehicle?.plate ?? ""}`
          : undefined
      }
      className="sm:max-w-sm"
    >
      <div className="flex justify-center">
        {image ? (
          // eslint-disable-next-line @next/next/no-img-element -- generated data URL
          <img
            src={image}
            alt={t("inspections.field.qr_title")}
            className="size-60 rounded-md border bg-white p-2"
          />
        ) : null}
      </div>
    </AppDialog>
  );
}

/**
 * "Saha görevleri": open handover and return tasks of the branches in the
 * member's scope; each opens the mobile wizard of its rental.
 */
export function FieldTasksPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const write = can(permissions.tenant.rentals.write);
  const [qr, setQr] = useState<OperationTask | null>(null);
  const listState = useServerListState({
    initialSort: "due_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, OPERATION_TASKS_RESOURCE),
  });
  const params = useMemo<ListOperationTasksParams>(() => {
    const types = filterValues(listState.columnFilters, "task_type");
    return {
      ...listState.params,
      task_type: joinFilter(types.length ? types : [...FIELD_TYPES]),
      status: "open,in_progress",
    };
  }, [listState.columnFilters, listState.params]);
  const query = useOperationTasks(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<OperationTask>({
          accessorKey: "task_type",
          labelKey: "inspections.field.columns.type",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: FIELD_TYPES.map((v) => ({
            value: v,
            labelKey: TASK_TYPE_LABEL_KEYS[v],
            label: t(TASK_TYPE_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <Badge
                variant={
                  row.original.task_type === "handover"
                    ? "default"
                    : "secondary"
                }
              >
                {t(TASK_TYPE_LABEL_KEYS[row.original.task_type])}
              </Badge>
              <span className="font-mono text-sm font-semibold">
                {row.original.vehicle?.plate ?? "—"}
              </span>
            </span>
          ),
        }),
        createColumn<OperationTask>({
          id: "rental",
          accessorFn: (row) => row.rental?.reference ?? "",
          labelKey: "inspections.field.columns.rental",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.rental ? (
              <ReferenceText reference={row.original.rental.reference} />
            ) : (
              "—"
            ),
        }),
        createColumn<OperationTask>({
          accessorKey: "due_at",
          labelKey: "inspections.field.columns.due_at",
          enableSorting: true,
          cell: ({ row }) => {
            const late = new Date(row.original.due_at) < new Date();
            return (
              <span className={late ? "text-destructive font-medium" : ""}>
                {datetime(row.original.due_at, "d MMM HH:mm", locale)}
              </span>
            );
          },
        }),
        createColumn<OperationTask>({
          id: "branch",
          accessorFn: (row) => row.branch.code ?? row.branch.name,
          labelKey: "inspections.field.columns.branch",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.branch.code ?? row.original.branch.name,
        }),
        createColumn<OperationTask>({
          accessorKey: "status",
          labelKey: "inspections.field.columns.status",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <OperationTaskStatusChip status={row.original.status} />
          ),
        }),
        createColumn<OperationTask>({
          id: "actions",
          labelKey: "inspections.field.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const href = wizardHref(slug, row.original);
            return (
              <EntityRowActions
                actions={[
                  ...(href && write
                    ? [
                        {
                          id: "start",
                          label: t("inspections.field.start"),
                          icon: PlayCircle,
                          onSelect: () => router.push(href),
                        },
                      ]
                    : []),
                  ...(href
                    ? [
                        {
                          id: "qr",
                          label: t("inspections.field.qr"),
                          icon: QrCode,
                          onSelect: () => setQr(row.original),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<OperationTask, unknown>[],
    [locale, router, slug, t, write],
  );

  return (
    <EntityPage
      title={t("inspections.field.title")}
      description={t("inspections.field.description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("inspections.field.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("inspections.field.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        onRowClick={(row) => {
          const href = wizardHref(slug, row);
          if (href && write) router.push(href);
        }}
        emptyTitle={t("inspections.field.empty_title")}
        emptyDescription={t("inspections.field.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey: "tenant-field-tasks-v1", rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <TaskQrDialog task={qr} onClose={() => setQr(null)} />
    </EntityPage>
  );
}
