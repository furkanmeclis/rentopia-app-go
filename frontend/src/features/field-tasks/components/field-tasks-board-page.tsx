"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { format, parseISO } from "date-fns";
import {
  AlertTriangle,
  ExternalLink,
  Pencil,
  UserCheck,
  XCircle,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { useUrlListDefaults } from "@/components/entity/use-url-list-defaults";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import {
  FieldTaskPlace,
  FieldTaskStatusChip,
  FieldTaskTypeBadge,
} from "@/features/field-tasks/components/field-task-cells";
import {
  AssignFieldTaskDialog,
  CreateFieldTaskDialog,
  EditFieldTaskDialog,
  useFieldTaskBulkParams,
} from "@/features/field-tasks/components/field-task-dialogs";
import { StaffDayTimeline } from "@/features/field-tasks/components/staff-day-timeline";
import {
  FIELD_TASKS_RESOURCE,
  FIELD_TASK_LIVE_STATUSES,
  FIELD_TASK_STATUSES,
  FIELD_TASK_STATUS_LABEL_KEYS,
  FIELD_TASK_TYPES,
  FIELD_TASK_TYPE_LABEL_KEYS,
  LOCATION_KINDS,
  LOCATION_KIND_LABEL_KEYS,
} from "@/features/field-tasks/constants";
import {
  useFieldTaskFacets,
  useFieldTaskStep,
  useFieldTasks,
  useFieldTasksMeta,
} from "@/features/field-tasks/hooks/use-field-tasks";
import {
  can,
  dayWindow,
  fieldTaskProblemText,
  isLate,
  personName,
} from "@/features/field-tasks/lib/field-tasks";
import type {
  FieldTask,
  FieldTaskFacets,
  ListFieldTasksParams,
} from "@/features/field-tasks/types";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type FacetValue = FieldTaskFacets["status"][number];

function countOf(facets: FacetValue[] | undefined, value: string) {
  return facets?.find((f) => f.value === value)?.count ?? 0;
}

function dayBounds(day: string) {
  if (!day) return { from: undefined, to: undefined };
  const start = parseISO(day);
  const offset = Math.round(
    (start.getTime() - dayWindow(0).start.getTime()) / 86_400_000,
  );
  const w = dayWindow(offset);
  return { from: w.from, to: w.to };
}

/**
 * "Görev panosu": every field task of the branches in scope (DataTable with
 * date, branch, status, assignee and type filters, bulk assign) and a day
 * timeline per staff member. Unassigned work is called out above the list.
 */
export function FieldTasksBoardPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can: allowed } = usePermission();
  const planner = allowed(permissions.tenant.fieldTasks.assign);
  const [tab, setTab] = useState("list");
  const [day, setDay] = useState("");
  const [timelineDay, setTimelineDay] = useState(() =>
    format(new Date(), "yyyy-MM-dd"),
  );
  const [creating, setCreating] = useState(false);
  const [assignTarget, setAssignTarget] = useState<FieldTask | null>(null);
  const [editTarget, setEditTarget] = useState<FieldTask | null>(null);
  const [cancelTarget, setCancelTarget] = useState<FieldTask | null>(null);
  const step = useFieldTaskStep();
  const bulkParams = useFieldTaskBulkParams();
  const urlDefaults = useUrlListDefaults(
    ["status", "task_type", "branch", "assignee", "location_kind"],
    ["due_at", "status", "task_type", "created_at"],
  );
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "due_at",
    initialColumnFilters: urlDefaults.initialColumnFilters ?? [
      { id: "status", value: [...FIELD_TASK_LIVE_STATUSES] },
    ],
    initialPageSize: 20,
    realtime: tenantLiveList(organization, FIELD_TASKS_RESOURCE),
  });
  const facetsQuery = useFieldTaskFacets();
  const facets = facetsQuery.data;
  const metaQuery = useFieldTasksMeta();
  const { from: dueFrom, to: dueTo } = dayBounds(day);

  const params = useMemo<ListFieldTasksParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      task_type: joinFilter(column("task_type")),
      branch: joinFilter(column("branch")),
      assignee: joinFilter(column("assignee")),
      location_kind: joinFilter(column("location_kind")),
      due_from: dueFrom,
      due_to: dueTo,
    };
  }, [dueFrom, dueTo, listState.columnFilters, listState.params]);
  const query = useFieldTasks(params);
  const total = query.data?.total ?? 0;

  // Unassigned work still to do (whatever the list filters say).
  const unassignedQuery = useFieldTasks({
    limit: 1,
    offset: 0,
    status: "planned,issue_reported",
    assignee: "none",
    due_from: dueFrom,
    due_to: dueTo,
  });
  const unassigned = unassignedQuery.data?.total ?? 0;

  const engineQuery = useMemo(() => {
    const out: Record<string, string> = {};
    for (const [key, value] of Object.entries(params)) {
      if (["limit", "offset"].includes(key) || value === undefined) continue;
      out[key] = String(value);
    }
    return out;
  }, [params]);
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery: engineQuery,
    total,
  });
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        FIELD_TASKS_RESOURCE,
        metaQuery.data?.bulk_actions ?? [],
      ),
    [metaQuery.data?.bulk_actions],
  );

  const showUnassigned = () => {
    listState.onColumnFiltersChange([
      { id: "status", value: ["planned", "issue_reported"] },
      { id: "assignee", value: ["none"] },
    ]);
    setTab("list");
  };

  const columns = useMemo(
    () =>
      [
        createColumn<FieldTask>({
          accessorKey: "task_type",
          labelKey: "field_tasks.columns.task_type",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: FIELD_TASK_TYPES.map((v) => ({
            value: v,
            labelKey: FIELD_TASK_TYPE_LABEL_KEYS[v],
            label: `${t(FIELD_TASK_TYPE_LABEL_KEYS[v])} (${countOf(facets?.task_type, v)})`,
          })),
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <FieldTaskTypeBadge task={row.original} />
              {row.original.flight_number ? (
                <span className="text-muted-foreground font-mono text-xs">
                  {row.original.flight_number}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<FieldTask>({
          accessorKey: "status",
          labelKey: "field_tasks.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: FIELD_TASK_STATUSES.map((v) => ({
            value: v,
            labelKey: FIELD_TASK_STATUS_LABEL_KEYS[v],
            label: `${t(FIELD_TASK_STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => (
            <FieldTaskStatusChip status={row.original.status} />
          ),
        }),
        createColumn<FieldTask>({
          accessorKey: "due_at",
          labelKey: "field_tasks.columns.due_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span
              className={cn(
                "tabular-nums",
                isLate(row.original) && "text-destructive font-medium",
              )}
            >
              {datetime(row.original.due_at, "d MMM HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<FieldTask>({
          id: "place",
          accessorFn: (row) => row.location.kind,
          labelKey: "field_tasks.columns.place",
          enableSorting: false,
          cell: ({ row }) => (
            <FieldTaskPlace task={row.original} className="max-w-64" />
          ),
        }),
        createColumn<FieldTask>({
          id: "location_kind",
          accessorFn: (row) => row.location.kind,
          labelKey: "field_tasks.columns.location_kind",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: LOCATION_KINDS.map((v) => ({
            value: v,
            labelKey: LOCATION_KIND_LABEL_KEYS[v],
            label: `${t(LOCATION_KIND_LABEL_KEYS[v])} (${countOf(facets?.location_kind, v)})`,
          })),
          cell: ({ row }) =>
            t(LOCATION_KIND_LABEL_KEYS[row.original.location.kind]),
        }),
        createColumn<FieldTask>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle?.plate ?? "",
          labelKey: "field_tasks.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.vehicle ? (
              <span className="font-mono text-xs font-semibold">
                {row.original.vehicle.plate}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<FieldTask>({
          id: "reference",
          accessorFn: (row) =>
            row.rental?.reference ?? row.reservation?.reference ?? "",
          labelKey: "field_tasks.columns.reference",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rental?.reference ??
            row.original.reservation?.reference ??
            "—",
        }),
        createColumn<FieldTask>({
          id: "branch",
          accessorFn: (row) => row.branch.code,
          labelKey: "field_tasks.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.branch ?? []).map((f) => ({
            value: f.value,
            label: `${f.label || f.value} (${f.count})`,
          })),
          cell: ({ row }) =>
            row.original.to_branch
              ? `${row.original.branch.code} → ${row.original.to_branch.code}`
              : row.original.branch.code,
        }),
        createColumn<FieldTask>({
          id: "assignee",
          accessorFn: (row) => row.assignee?.uuid ?? "none",
          labelKey: "field_tasks.columns.assignee",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.assignee ?? []).map((f) => ({
            value: f.value,
            label: `${f.value === "none" ? t("field_tasks.assign.unassigned") : f.label} (${f.count})`,
          })),
          cell: ({ row }) =>
            row.original.assignee ? (
              personName(row.original.assignee)
            ) : (
              <Badge variant="warning">
                {t("field_tasks.assign.unassigned")}
              </Badge>
            ),
        }),
        createColumn<FieldTask>({
          id: "contact",
          accessorFn: (row) => row.contact.name,
          labelKey: "field_tasks.columns.contact",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            [row.original.contact.name, row.original.contact.phone]
              .filter(Boolean)
              .join(" · ") || "—",
        }),
        createColumn<FieldTask>({
          id: "actions",
          labelKey: "field_tasks.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const task = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("field_tasks.actions.open"),
                    icon: ExternalLink,
                    onSelect: () =>
                      router.push(routes.tenant.field.task(slug, task.uuid)),
                  },
                  ...(can(task, "assign")
                    ? [
                        {
                          id: "assign",
                          label: t("field_tasks.actions.assign"),
                          icon: UserCheck,
                          onSelect: () => setAssignTarget(task),
                        },
                      ]
                    : []),
                  ...(can(task, "edit")
                    ? [
                        {
                          id: "edit",
                          label: t("field_tasks.actions.edit"),
                          icon: Pencil,
                          onSelect: () => setEditTarget(task),
                        },
                      ]
                    : []),
                  ...(can(task, "cancel")
                    ? [
                        {
                          id: "cancel",
                          label: t("field_tasks.actions.cancel"),
                          icon: XCircle,
                          variant: "destructive" as const,
                          separatorBefore: true,
                          onSelect: () => setCancelTarget(task),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<FieldTask, unknown>[],
    [facets, locale, router, slug, t],
  );

  const refresh = () => {
    void query.refetch();
    void facetsQuery.refetch();
    void unassignedQuery.refetch();
  };

  return (
    <EntityPage
      title={t("field_tasks.board.title")}
      description={t("field_tasks.board.description")}
      permission={permissions.tenant.fieldTasks.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("field_tasks.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("field_tasks.board.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreating(true)}
          label={t("field_tasks.actions.create")}
          permission={permissions.tenant.fieldTasks.assign}
        />
      }
    >
      {unassigned > 0 ? (
        <Alert className="border-amber-500/50 bg-amber-50 text-amber-900 dark:bg-amber-950/40 dark:text-amber-100">
          <AlertTriangle className="size-4" />
          <AlertTitle>
            {t("field_tasks.board.unassigned_title", { count: unassigned })}
          </AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>{t("field_tasks.board.unassigned_hint")}</span>
            <Button size="sm" variant="outline" onClick={showUnassigned}>
              {t("field_tasks.board.show_unassigned")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList>
          <TabsTrigger value="list">
            {t("field_tasks.board.tab_list")}
          </TabsTrigger>
          <TabsTrigger value="timeline">
            {t("field_tasks.board.tab_timeline")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="list" className="space-y-4">
          <SelectionBanner
            selectedCount={bulkSelection.selectedCount}
            total={total}
            showSelectAll={bulkSelection.showSelectAllBanner}
            allMatchingSelected={bulkSelection.scope.mode === "all"}
            onSelectAllMatching={bulkSelection.selectAllMatching}
            onClearSelection={bulkSelection.clearSelection}
          />
          <EntityTable
            columns={columns}
            data={query.data?.items ?? []}
            getRowId={(row) => row.uuid}
            onRowClick={(row) =>
              router.push(routes.tenant.field.task(slug, row.uuid))
            }
            isLoading={query.isLoading}
            isError={query.isError}
            errorDescription={t("field_tasks.load_failed")}
            onRetry={() => void query.refetch()}
            realtime={listState.realtimeFor(() =>
              Promise.all([
                query.refetch(),
                facetsQuery.refetch(),
                unassignedQuery.refetch(),
              ]),
            )}
            emptyTitle={t("field_tasks.board.empty_title")}
            emptyDescription={t("field_tasks.board.empty_description")}
            pageCount={Math.max(
              1,
              Math.ceil(total / (listState.pagination.pageSize || 20)),
            )}
            state={{
              ...listState.tableState,
              rowSelection: bulkSelection.rowSelection,
              onRowSelectionChange: bulkSelection.onRowSelectionChange,
            }}
            features={{
              persistKey: "tenant-field-task-board-v1",
              rowSelection: planner,
            }}
            toolbarExtra={
              <>
                <div className="flex items-center gap-1">
                  <DatePicker
                    value={day}
                    onChange={(v) => setDay(v ?? "")}
                    placeholder={t("field_tasks.board.any_day")}
                    className="w-44"
                  />
                  {day ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setDay("")}
                    >
                      {t("field_tasks.board.clear_day")}
                    </Button>
                  ) : null}
                </div>
                <BulkActionMenu
                  resource={FIELD_TASKS_RESOURCE}
                  actions={bulkActions}
                  scope={bulkSelection.scope}
                  selectedCount={bulkSelection.selectedCount}
                  resolveParams={bulkParams.resolveParams}
                  onComplete={() => {
                    bulkSelection.clearSelection();
                    refresh();
                  }}
                />
                <EntityToolbar
                  onRefresh={refresh}
                  refreshDisabled={query.isFetching}
                />
              </>
            }
          />
        </TabsContent>
        <TabsContent value="timeline">
          <StaffDayTimeline
            day={timelineDay}
            onDayChange={setTimelineDay}
            branches={filterValues(listState.columnFilters, "branch")}
            onOpen={(task) =>
              router.push(routes.tenant.field.task(slug, task.uuid))
            }
          />
        </TabsContent>
      </Tabs>
      {bulkParams.dialog}
      <CreateFieldTaskDialog open={creating} onOpenChange={setCreating} />
      <AssignFieldTaskDialog
        task={assignTarget}
        onClose={() => setAssignTarget(null)}
      />
      <EditFieldTaskDialog
        task={editTarget}
        onClose={() => setEditTarget(null)}
      />
      <ReasonDialog
        open={Boolean(cancelTarget)}
        title={t("field_tasks.cancel.title")}
        description={t("field_tasks.cancel.description")}
        label={t("field_tasks.fields.reason")}
        confirmLabel={t("field_tasks.actions.cancel")}
        destructive
        pending={step.isPending}
        onCancel={() => setCancelTarget(null)}
        onConfirm={(reason) => {
          if (!cancelTarget) return;
          step.mutate(
            { uuid: cancelTarget.uuid, step: "cancel", note: reason },
            {
              onSuccess: () => setCancelTarget(null),
              onError: (e) => appToast.error(fieldTaskProblemText(t, e)),
            },
          );
        }}
      />
    </EntityPage>
  );
}
