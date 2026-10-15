"use client";

import { ChevronRight, Clock, Phone, Plane } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  FieldTaskPlace,
  FieldTaskStatusChip,
  FieldTaskTypeBadge,
} from "@/features/field-tasks/components/field-task-cells";
import {
  FIELD_TASK_LIVE_STATUSES,
  MY_FIELD_TASKS_RESOURCE,
} from "@/features/field-tasks/constants";
import { useMyFieldTasks } from "@/features/field-tasks/hooks/use-field-tasks";
import { dayWindow, isLate } from "@/features/field-tasks/lib/field-tasks";
import type { FieldTask } from "@/features/field-tasks/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const LIVE = FIELD_TASK_LIVE_STATUSES.join(",");

/** One task as a large touch card (the whole card opens the task). */
function TaskCard({ task }: { task: FieldTask }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const late = isLate(task);
  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-0">
        <Link
          href={routes.tenant.field.task(slug, task.uuid)}
          className="focus-visible:ring-ring flex items-stretch gap-3 rounded-xl p-4 outline-none focus-visible:ring-2"
        >
          <div
            className={cn(
              "flex w-14 shrink-0 flex-col items-center justify-center rounded-lg border text-center",
              late
                ? "border-destructive/40 bg-destructive/10 text-destructive"
                : "bg-muted/50",
            )}
          >
            <Clock className="mb-0.5 size-3.5" aria-hidden />
            <span className="text-base leading-none font-semibold tabular-nums">
              {datetime(task.due_at, "HH:mm", locale)}
            </span>
            <span className="text-muted-foreground mt-1 text-[10px]">
              {datetime(task.due_at, "d MMM", locale)}
            </span>
          </div>
          <div className="min-w-0 flex-1 space-y-1.5">
            <div className="flex flex-wrap items-center gap-1.5">
              <FieldTaskTypeBadge task={task} />
              <FieldTaskStatusChip status={task.status} />
              {late ? (
                <Badge variant="danger">{t("field_tasks.my.late")}</Badge>
              ) : null}
            </div>
            <FieldTaskPlace task={task} className="text-sm font-medium" />
            <div className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
              {task.vehicle ? (
                <span className="text-foreground font-mono font-semibold">
                  {task.vehicle.plate}
                </span>
              ) : null}
              {task.reservation ? (
                <span>{task.reservation.reference}</span>
              ) : null}
              {task.flight_number ? (
                <span className="flex items-center gap-1">
                  <Plane className="size-3" aria-hidden />
                  {task.flight_number}
                </span>
              ) : null}
              {task.contact.phone ? (
                <span className="flex items-center gap-1">
                  <Phone className="size-3" aria-hidden />
                  {task.contact.name || task.contact.phone}
                </span>
              ) : null}
            </div>
          </div>
          <ChevronRight
            className="text-muted-foreground size-5 shrink-0 self-center"
            aria-hidden
          />
        </Link>
      </CardContent>
    </Card>
  );
}

function TaskList({
  tasks,
  loading,
  error,
  onRetry,
  emptyTitle,
  emptyDescription,
}: {
  tasks: FieldTask[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  emptyTitle: string;
  emptyDescription: string;
}) {
  const { t } = useLocale();
  if (loading) {
    return (
      <div className="space-y-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-24 w-full rounded-xl" />
        ))}
      </div>
    );
  }
  if (error) {
    return (
      <ErrorState title={t("field_tasks.load_failed")} onRetry={onRetry} />
    );
  }
  if (tasks.length === 0) {
    return <EmptyState title={emptyTitle} description={emptyDescription} />;
  }
  return (
    <ul className="space-y-3">
      {tasks.map((task) => (
        <li key={task.uuid}>
          <TaskCard task={task} />
        </li>
      ))}
    </ul>
  );
}

/**
 * "Görevlerim": the signed-in member's field tasks for today (late ones
 * included) and tomorrow, as touch cards for the phone. New assignments
 * arrive live (the member's channel and the task list).
 */
export function MyFieldTasksPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [tab, setTab] = useState("today");
  const windows = useMemo(
    () => ({ today: dayWindow(0), tomorrow: dayWindow(1) }),
    [],
  );
  const today = useMyFieldTasks({
    limit: 100,
    offset: 0,
    sort: "due_at",
    status: LIVE,
    due_to: windows.today.to,
  });
  const tomorrow = useMyFieldTasks({
    limit: 100,
    offset: 0,
    sort: "due_at",
    status: LIVE,
    due_from: windows.tomorrow.from,
    due_to: windows.tomorrow.to,
  });
  useTenantLiveRefetch([MY_FIELD_TASKS_RESOURCE], () =>
    Promise.all([today.refetch(), tomorrow.refetch()]),
  );
  const todayItems = today.data?.items ?? [];
  const tomorrowItems = tomorrow.data?.items ?? [];

  return (
    <EntityPage
      title={t("field_tasks.my.title")}
      description={t("field_tasks.my.description")}
      permission={permissions.tenant.fieldTasks.execute}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("field_tasks.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("field_tasks.my.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList className="grid w-full grid-cols-2 sm:w-80">
          <TabsTrigger value="today">
            {t("field_tasks.my.today")}
            <Badge variant="secondary" className="ms-1.5">
              {today.data?.total ?? 0}
            </Badge>
          </TabsTrigger>
          <TabsTrigger value="tomorrow">
            {t("field_tasks.my.tomorrow")}
            <Badge variant="secondary" className="ms-1.5">
              {tomorrow.data?.total ?? 0}
            </Badge>
          </TabsTrigger>
        </TabsList>
        <TabsContent value="today">
          <TaskList
            tasks={todayItems}
            loading={today.isLoading}
            error={today.isError}
            onRetry={() => void today.refetch()}
            emptyTitle={t("field_tasks.my.empty_today")}
            emptyDescription={t("field_tasks.my.empty_description")}
          />
        </TabsContent>
        <TabsContent value="tomorrow">
          <TaskList
            tasks={tomorrowItems}
            loading={tomorrow.isLoading}
            error={tomorrow.isError}
            onRetry={() => void tomorrow.refetch()}
            emptyTitle={t("field_tasks.my.empty_tomorrow")}
            emptyDescription={t("field_tasks.my.empty_description")}
          />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
