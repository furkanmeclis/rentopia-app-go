"use client";

import { addDays, format, parseISO } from "date-fns";
import { ChevronLeft, ChevronRight, UserX } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { COLUMN_WIDTH } from "@/features/fleet-calendar/constants";
import {
  calendarWindow,
  packLanes,
  timelineColumns,
  trackWidth,
  xOf,
} from "@/features/fleet-calendar/lib/timeline";
import { taskTypeLabel } from "@/features/field-tasks/constants";
import {
  useAssignFieldTask,
  useFieldTaskAssignees,
  useFieldTasks,
} from "@/features/field-tasks/hooks/use-field-tasks";
import {
  fieldTaskProblemText,
  personName,
  placeText,
} from "@/features/field-tasks/lib/field-tasks";
import type { FieldTask, FieldTaskStatus } from "@/features/field-tasks/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const PERSON_COLUMN = 200;
const HEADER_HEIGHT = 40;
const LANE_HEIGHT = 30;
const BAR_HEIGHT = 24;
const ROW_PADDING = 8;
const MIN_ROW_HEIGHT = 48;
const MIN_BAR_WIDTH = 10;
const UNASSIGNED = "unassigned";
const DRAG_TYPE = "application/x-field-task";

const STATUS_BAR_CLASSES: Record<FieldTaskStatus, string> = {
  planned:
    "border-amber-400 bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-100",
  assigned:
    "border-sky-400 bg-sky-100 text-sky-900 dark:bg-sky-950 dark:text-sky-100",
  en_route:
    "border-indigo-500 bg-indigo-500 text-white dark:bg-indigo-600 dark:text-white",
  issue_reported:
    "border-red-500 bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-100",
  completed:
    "border-emerald-400 bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-100",
  cancelled: "border-border bg-muted text-muted-foreground line-through",
};

type Row = { id: string; label: string; sub: string; tasks: FieldTask[] };

/**
 * One day of field work per staff member (the fleet calendar's timeline
 * grid: hour columns, a "now" line, lanes for overlapping tasks). The
 * unassigned row comes first; a planner drags a task onto a person (or
 * back to unassigned) to assign it.
 */
export function StaffDayTimeline({
  day,
  onDayChange,
  branches,
  onOpen,
}: {
  day: string;
  onDayChange: (day: string) => void;
  branches: string[];
  onOpen: (task: FieldTask) => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const planner = can(permissions.tenant.fieldTasks.assign);
  const assign = useAssignFieldTask();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [dragOver, setDragOver] = useState<string | null>(null);
  const window = useMemo(() => calendarWindow("day", parseISO(day)), [day]);
  const columns = useMemo(() => timelineColumns("day", window), [window]);
  const width = trackWidth("day", columns.length);
  const query = useFieldTasks({
    limit: 200,
    offset: 0,
    sort: "due_at",
    status: "planned,assigned,en_route,issue_reported,completed",
    branch: branches.length ? branches.join(",") : undefined,
    due_from: window.from.toISOString(),
    due_to: window.to.toISOString(),
  });
  const people = useFieldTaskAssignees();
  const now = new Date();
  const nowX =
    now >= window.from && now < window.to ? xOf(now, window, width) : null;

  // Once per day (the grid mounts after the first load): scroll to now,
  // else to the first task, else to 07:00.
  const scrolledFor = useRef<string | null>(null);
  const firstDue = query.data?.items[0]?.due_at;
  useEffect(() => {
    const el = scrollRef.current;
    if (!el || !query.isSuccess || scrolledFor.current === day) return;
    scrolledFor.current = day;
    const target =
      nowX ??
      (firstDue
        ? xOf(parseISO(firstDue), window, width)
        : xOf(addHoursTo(window.from, 7), window, width));
    el.scrollLeft = Math.max(0, target - (el.clientWidth - PERSON_COLUMN) / 3);
  }, [day, firstDue, nowX, query.isSuccess, width, window]);

  const rows = useMemo<Row[]>(() => {
    const tasks = query.data?.items ?? [];
    const byPerson = new Map<string, FieldTask[]>();
    for (const task of tasks) {
      const key = task.assignee?.uuid ?? UNASSIGNED;
      byPerson.set(key, [...(byPerson.get(key) ?? []), task]);
    }
    const out: Row[] = [
      {
        id: UNASSIGNED,
        label: t("field_tasks.assign.unassigned"),
        sub: t("field_tasks.timeline.unassigned_hint"),
        tasks: byPerson.get(UNASSIGNED) ?? [],
      },
    ];
    const seen = new Set<string>();
    for (const p of people.data ?? []) {
      if (
        branches.length &&
        p.branch_scope !== "all" &&
        !branches.some((b) => p.branch_uuids.includes(b))
      ) {
        continue;
      }
      seen.add(p.uuid);
      out.push({
        id: p.uuid,
        label: personName(p),
        sub: p.email,
        tasks: byPerson.get(p.uuid) ?? [],
      });
    }
    for (const task of tasks) {
      const a = task.assignee;
      if (a && !seen.has(a.uuid)) {
        seen.add(a.uuid);
        out.push({
          id: a.uuid,
          label: personName(a),
          sub: a.email,
          tasks: byPerson.get(a.uuid) ?? [],
        });
      }
    }
    return out;
  }, [branches, people.data, query.data?.items, t]);

  const drop = (task: FieldTask, target: string) => {
    if ((task.assignee?.uuid ?? UNASSIGNED) === target) return;
    assign.mutate(
      { uuid: task.uuid, userUuid: target === UNASSIGNED ? null : target },
      { onError: (e) => appToast.error(fieldTaskProblemText(t, e)) },
    );
  };

  const shift = (days: number) =>
    onDayChange(format(addDays(parseISO(day), days), "yyyy-MM-dd"));

  const allTasks = query.data?.items ?? [];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="icon"
          variant="outline"
          aria-label={t("field_tasks.timeline.previous_day")}
          onClick={() => shift(-1)}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <DatePicker
          value={day}
          onChange={(v) => {
            if (v) onDayChange(v);
          }}
          className="w-48"
        />
        <Button
          type="button"
          size="icon"
          variant="outline"
          aria-label={t("field_tasks.timeline.next_day")}
          onClick={() => shift(1)}
        >
          <ChevronRight className="size-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => onDayChange(format(new Date(), "yyyy-MM-dd"))}
        >
          {t("field_tasks.timeline.today")}
        </Button>
        <span className="text-muted-foreground ms-auto text-sm">
          {t("field_tasks.timeline.count", { count: allTasks.length })}
        </span>
      </div>
      {query.isLoading || people.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : rows.length === 1 && allTasks.length === 0 ? (
        <EmptyState
          title={t("field_tasks.timeline.empty_title")}
          description={t("field_tasks.timeline.empty_description")}
        />
      ) : (
        <div
          ref={scrollRef}
          className="bg-background relative overflow-auto rounded-md border"
          style={{ maxHeight: "70vh" }}
        >
          <div
            role="grid"
            aria-label={t("field_tasks.timeline.grid_label")}
            className="relative"
            style={{ width: PERSON_COLUMN + width, minWidth: "100%" }}
          >
            <div
              role="row"
              className="bg-background sticky top-0 z-30 flex border-b"
              style={{ height: HEADER_HEIGHT }}
            >
              <div
                role="columnheader"
                className="bg-background text-muted-foreground sticky left-0 z-40 flex shrink-0 items-center border-r px-3 text-xs font-medium"
                style={{ width: PERSON_COLUMN }}
              >
                {t("field_tasks.timeline.person_column")}
              </div>
              <div className="relative shrink-0" style={{ width }}>
                {columns.map((col) => (
                  <div
                    key={col.key}
                    role="columnheader"
                    className="text-muted-foreground absolute inset-y-0 flex items-center border-s px-1.5 text-[11px] font-medium"
                    style={{
                      left: xOf(col.start, window, width),
                      width: COLUMN_WIDTH.day,
                    }}
                  >
                    {datetime(col.start, "HH:mm", locale)}
                  </div>
                ))}
              </div>
            </div>
            <div className="relative">
              <div
                aria-hidden
                className="pointer-events-none absolute inset-y-0"
                style={{ left: PERSON_COLUMN, width }}
              >
                {columns.map((col) => (
                  <div
                    key={col.key}
                    className="border-border/60 absolute inset-y-0 border-s"
                    style={{
                      left: xOf(col.start, window, width),
                      width: COLUMN_WIDTH.day,
                    }}
                  />
                ))}
                {nowX != null ? (
                  <div
                    className="bg-primary absolute inset-y-0 z-20 w-0.5"
                    style={{ left: nowX - 1 }}
                  />
                ) : null}
              </div>
              {rows.map((row) => {
                const { placed, lanes } = packLanes(row.tasks, (task) => ({
                  start: new Date(task.due_at).getTime(),
                  end: new Date(task.planned_end_at).getTime(),
                }));
                const height = Math.max(
                  MIN_ROW_HEIGHT,
                  lanes * LANE_HEIGHT + ROW_PADDING * 2,
                );
                const unassignedRow = row.id === UNASSIGNED;
                return (
                  <div
                    key={row.id}
                    role="row"
                    className={cn(
                      "relative flex border-b last:border-b-0",
                      unassignedRow &&
                        row.tasks.length > 0 &&
                        "bg-amber-50/60 dark:bg-amber-950/20",
                    )}
                    style={{ height }}
                  >
                    <div
                      role="rowheader"
                      className="bg-background sticky left-0 z-20 flex shrink-0 flex-col justify-center border-r px-3"
                      style={{ width: PERSON_COLUMN }}
                    >
                      <span className="flex items-center gap-1.5 truncate text-sm font-medium">
                        {unassignedRow ? (
                          <UserX className="size-3.5 text-amber-600" />
                        ) : null}
                        {row.label}
                      </span>
                      <span className="text-muted-foreground truncate text-xs">
                        {unassignedRow
                          ? t("field_tasks.timeline.task_count", {
                              count: row.tasks.length,
                            })
                          : row.sub}
                      </span>
                    </div>
                    <div
                      role="gridcell"
                      className={cn(
                        "relative shrink-0",
                        dragOver === row.id && "bg-primary/10",
                      )}
                      style={{ width }}
                      onDragOver={
                        planner
                          ? (event) => {
                              if (
                                event.dataTransfer.types.includes(DRAG_TYPE)
                              ) {
                                event.preventDefault();
                                setDragOver(row.id);
                              }
                            }
                          : undefined
                      }
                      onDragLeave={
                        planner ? () => setDragOver(null) : undefined
                      }
                      onDrop={
                        planner
                          ? (event) => {
                              setDragOver(null);
                              const id = event.dataTransfer.getData(DRAG_TYPE);
                              const task = allTasks.find((x) => x.uuid === id);
                              if (task) drop(task, row.id);
                            }
                          : undefined
                      }
                    >
                      {placed.map(({ item: task, lane }) => {
                        const left = xOf(task.due_at, window, width);
                        const barWidth = Math.max(
                          MIN_BAR_WIDTH,
                          xOf(task.planned_end_at, window, width) - left,
                        );
                        const label = `${taskTypeLabel(t, task)} · ${datetime(task.due_at, "HH:mm", locale)} · ${placeText(task)}`;
                        const draggable =
                          planner && task.actions.includes("assign");
                        return (
                          <button
                            key={task.uuid}
                            type="button"
                            title={label}
                            aria-label={label}
                            draggable={draggable}
                            onDragStart={
                              draggable
                                ? (event) =>
                                    event.dataTransfer.setData(
                                      DRAG_TYPE,
                                      task.uuid,
                                    )
                                : undefined
                            }
                            onClick={() => onOpen(task)}
                            className={cn(
                              "focus-visible:ring-ring absolute z-10 flex items-center gap-1 overflow-hidden rounded border px-1.5 text-start text-[11px] leading-none font-medium whitespace-nowrap shadow-xs outline-none hover:brightness-95 focus-visible:ring-2",
                              STATUS_BAR_CLASSES[task.status],
                              draggable ? "cursor-grab" : "cursor-pointer",
                            )}
                            style={{
                              left,
                              width: barWidth,
                              top:
                                ROW_PADDING +
                                lane * LANE_HEIGHT +
                                (LANE_HEIGHT - BAR_HEIGHT) / 2,
                              height: BAR_HEIGHT,
                            }}
                          >
                            <span className="truncate">
                              {task.vehicle?.plate ?? taskTypeLabel(t, task)}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function addHoursTo(date: Date, hours: number) {
  return new Date(date.getTime() + hours * 3_600_000);
}
