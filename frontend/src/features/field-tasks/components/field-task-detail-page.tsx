"use client";

import {
  AlertTriangle,
  CheckCircle2,
  ClipboardCheck,
  ExternalLink,
  MapPin,
  Navigation,
  Pencil,
  Phone,
  Play,
  UserCheck,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Timeline } from "@/components/common/timeline";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  FieldTaskPlace,
  FieldTaskStatusChip,
  FieldTaskTypeBadge,
} from "@/features/field-tasks/components/field-task-cells";
import {
  AssignFieldTaskDialog,
  EditFieldTaskDialog,
} from "@/features/field-tasks/components/field-task-dialogs";
import {
  EVENT_LABEL_KEYS,
  FIELD_TASK_STATUS_LABEL_KEYS,
  LOCATION_KIND_LABEL_KEYS,
  MY_FIELD_TASKS_RESOURCE,
  taskTypeLabel,
} from "@/features/field-tasks/constants";
import {
  useFieldTask,
  useFieldTaskEvents,
  useFieldTaskStep,
} from "@/features/field-tasks/hooks/use-field-tasks";
import {
  can,
  fieldTaskProblemText,
  isLate,
  mapsHref,
  personName,
} from "@/features/field-tasks/lib/field-tasks";
import type {
  FieldTask,
  FieldTaskEvent,
  FieldTaskStatus,
} from "@/features/field-tasks/types";
import type { FieldTaskStep } from "@/features/field-tasks/services/field-tasks.service";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-2 py-1.5 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

function eventTitle(
  t: (key: string, values?: Record<string, string | number>) => string,
  e: FieldTaskEvent,
) {
  if (e.action === "status_changed" && e.to_status) {
    return t("field_tasks.events.status_to", {
      status: t(FIELD_TASK_STATUS_LABEL_KEYS[e.to_status as FieldTaskStatus]),
    });
  }
  const key = EVENT_LABEL_KEYS[e.action];
  return key ? t(key) : e.action;
}

/** Inspection form route of a handover / return task. */
function inspectionHref(slug: string, task: FieldTask) {
  if (!task.rental) return null;
  if (task.inspection === "handover") {
    return routes.tenant.field.handover(slug, task.rental.uuid);
  }
  if (task.inspection === "return") {
    return routes.tenant.field.return(slug, task.rental.uuid);
  }
  return null;
}

type Pending = { step: FieldTaskStep } | null;

/**
 * One field task for the phone: when and where, whom to meet, the vehicle
 * and the booking; then the next step: start the trip, continue in the
 * handover / return form of the rental, complete, or report an issue.
 * Planners also assign, re-plan and cancel here.
 */
export function FieldTaskDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useFieldTask(uuid);
  const events = useFieldTaskEvents(uuid);
  const step = useFieldTaskStep();
  const [pending, setPending] = useState<Pending>(null);
  const [error, setError] = useState<string | null>(null);
  const [assigning, setAssigning] = useState(false);
  const [editing, setEditing] = useState(false);
  useTenantLiveRefetch([MY_FIELD_TASKS_RESOURCE], () =>
    Promise.all([query.refetch(), events.refetch()]),
  );
  const task = query.data;

  const run = (s: FieldTaskStep, note?: string) => {
    if (!task) return;
    setError(null);
    step.mutate(
      { uuid: task.uuid, step: s, note },
      {
        onSuccess: () => {
          setPending(null);
          void events.refetch();
        },
        onError: (e) => {
          setPending(null);
          setError(fieldTaskProblemText(t, e));
        },
      },
    );
  };

  const title = task ? taskTypeLabel(t, task) : t("field_tasks.detail.title");
  const inspection = task ? inspectionHref(slug, task) : null;
  const maps = task ? mapsHref(task) : null;

  return (
    <EntityPage
      title={title}
      description={
        task
          ? `${datetime(task.due_at, "d MMMM yyyy HH:mm", locale)} · ${task.branch.code}`
          : undefined
      }
      permission={[
        permissions.tenant.fieldTasks.read,
        permissions.tenant.fieldTasks.assign,
        permissions.tenant.fieldTasks.execute,
      ]}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("field_tasks.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("field_tasks.my.title"),
          href: routes.tenant.field.root(slug),
        },
        { label: title },
      ]}
    >
      {query.isLoading ? (
        <Skeleton className="h-72 w-full rounded-xl" />
      ) : query.isError || !task ? (
        <ErrorState
          title={t("field_tasks.detail.not_found")}
          onRetry={() => void query.refetch()}
        />
      ) : (
        <div className="grid gap-4 pb-24 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] lg:pb-0">
          <div className="space-y-4">
            <EntitySectionCard
              title={t("field_tasks.detail.summary")}
              actions={
                <div className="flex flex-wrap items-center gap-1.5">
                  <FieldTaskTypeBadge task={task} />
                  <FieldTaskStatusChip status={task.status} />
                </div>
              }
            >
              {task.status === "issue_reported" && task.issue_note ? (
                <Alert variant="destructive" className="mb-3">
                  <AlertTriangle className="size-4" />
                  <AlertTitle>{t("field_tasks.detail.issue")}</AlertTitle>
                  <AlertDescription>{task.issue_note}</AlertDescription>
                </Alert>
              ) : null}
              <dl className="divide-y">
                <Row label={t("field_tasks.fields.due_at")}>
                  <span
                    className={cn(
                      "tabular-nums",
                      isLate(task) && "text-destructive font-medium",
                    )}
                  >
                    {datetime(task.due_at, "d MMM HH:mm", locale)} –{" "}
                    {datetime(task.planned_end_at, "HH:mm", locale)}
                  </span>
                </Row>
                <Row label={t("field_tasks.fields.place")}>
                  <div className="space-y-1">
                    <FieldTaskPlace task={task} />
                    <span className="text-muted-foreground text-xs">
                      {t(LOCATION_KIND_LABEL_KEYS[task.location.kind])}
                      {task.to_branch
                        ? ` · ${task.branch.code} → ${task.to_branch.code}`
                        : ""}
                    </span>
                  </div>
                </Row>
                {task.contact.name || task.contact.phone ? (
                  <Row label={t("field_tasks.fields.contact")}>
                    <div className="flex flex-wrap items-center gap-2">
                      <span>{task.contact.name}</span>
                      {task.contact.phone ? (
                        <a
                          href={`tel:${task.contact.phone.replace(/\s/g, "")}`}
                          className="text-primary inline-flex items-center gap-1 font-medium underline-offset-2 hover:underline"
                        >
                          <Phone className="size-3.5" aria-hidden />
                          {task.contact.phone}
                        </a>
                      ) : null}
                    </div>
                  </Row>
                ) : null}
                {task.flight_number ? (
                  <Row label={t("field_tasks.fields.flight_number")}>
                    <span className="font-mono font-semibold">
                      {task.flight_number}
                    </span>
                  </Row>
                ) : null}
                {task.meeting_note ? (
                  <Row label={t("field_tasks.fields.meeting_note")}>
                    {task.meeting_note}
                  </Row>
                ) : null}
                <Row label={t("field_tasks.fields.vehicle")}>
                  {task.vehicle ? (
                    <span className="font-mono font-semibold">
                      {task.vehicle.plate}
                    </span>
                  ) : (
                    "—"
                  )}
                </Row>
                {task.reservation ? (
                  <Row label={t("field_tasks.fields.reservation")}>
                    <Link
                      className="text-primary underline-offset-2 hover:underline"
                      href={routes.tenant.reservations.detail(
                        slug,
                        task.reservation.uuid,
                      )}
                    >
                      {task.reservation.reference}
                    </Link>
                  </Row>
                ) : null}
                {task.rental ? (
                  <Row label={t("field_tasks.fields.rental")}>
                    <Link
                      className="text-primary underline-offset-2 hover:underline"
                      href={routes.tenant.rentals.detail(
                        slug,
                        task.rental.uuid,
                      )}
                    >
                      {task.rental.reference}
                    </Link>
                  </Row>
                ) : null}
                <Row label={t("field_tasks.fields.assignee")}>
                  {task.assignee
                    ? personName(task.assignee)
                    : t("field_tasks.assign.unassigned")}
                </Row>
                {task.note ? (
                  <Row label={t("field_tasks.fields.note")}>{task.note}</Row>
                ) : null}
              </dl>
              {maps ? (
                <Button
                  asChild
                  variant="outline"
                  className="mt-3 w-full sm:w-auto"
                >
                  <a href={maps} target="_blank" rel="noreferrer">
                    <Navigation className="size-4" />
                    {t("field_tasks.detail.directions")}
                  </a>
                </Button>
              ) : null}
            </EntitySectionCard>
            {error ? (
              <Alert variant="destructive">
                <AlertTriangle className="size-4" />
                <AlertTitle>{t("field_tasks.detail.action_failed")}</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
          </div>
          <div className="space-y-4">
            <EntitySectionCard title={t("field_tasks.detail.next_step")}>
              <ActionButtons
                task={task}
                inspection={inspection}
                busy={step.isPending}
                onStart={() => run("start")}
                onComplete={() => setPending({ step: "complete" })}
                onIssue={() => setPending({ step: "report-issue" })}
                onCancel={() => setPending({ step: "cancel" })}
                onAssign={() => setAssigning(true)}
                onEdit={() => setEditing(true)}
              />
            </EntitySectionCard>
            <EntitySectionCard title={t("field_tasks.detail.timeline")}>
              {events.isLoading ? (
                <Skeleton className="h-24 w-full" />
              ) : (
                <Timeline
                  items={(events.data ?? [])
                    .slice()
                    .reverse()
                    .map((e) => ({
                      id: e.uuid,
                      title: eventTitle(t, e),
                      description: e.note || undefined,
                      meta: `${datetime(e.created_at, "d MMM HH:mm", locale)}${
                        e.actor ? ` · ${personName(e.actor)}` : ""
                      }`,
                    }))}
                />
              )}
            </EntitySectionCard>
          </div>
          {/* Phone: the main step stays under the thumb. */}
          <MobileActionBar
            task={task}
            inspection={inspection}
            busy={step.isPending}
            onStart={() => run("start")}
            onComplete={() => setPending({ step: "complete" })}
          />
        </div>
      )}
      <ReasonDialog
        open={pending?.step === "report-issue"}
        title={t("field_tasks.issue.title")}
        description={t("field_tasks.issue.description")}
        label={t("field_tasks.fields.issue_note")}
        confirmLabel={t("field_tasks.actions.report_issue")}
        destructive
        pending={step.isPending}
        onCancel={() => setPending(null)}
        onConfirm={(note) => run("report-issue", note)}
      />
      <ReasonDialog
        open={pending?.step === "complete"}
        title={t("field_tasks.complete.title")}
        description={t("field_tasks.complete.description")}
        label={t("field_tasks.fields.completion_note")}
        confirmLabel={t("field_tasks.actions.complete")}
        required={false}
        pending={step.isPending}
        onCancel={() => setPending(null)}
        onConfirm={(note) => run("complete", note)}
      />
      <ReasonDialog
        open={pending?.step === "cancel"}
        title={t("field_tasks.cancel.title")}
        description={t("field_tasks.cancel.description")}
        label={t("field_tasks.fields.reason")}
        confirmLabel={t("field_tasks.actions.cancel")}
        destructive
        pending={step.isPending}
        onCancel={() => setPending(null)}
        onConfirm={(note) => run("cancel", note)}
      />
      <AssignFieldTaskDialog
        task={assigning && task ? task : null}
        onClose={() => setAssigning(false)}
      />
      <EditFieldTaskDialog
        task={editing && task ? task : null}
        onClose={() => setEditing(false)}
      />
    </EntityPage>
  );
}

function ActionButtons({
  task,
  inspection,
  busy,
  onStart,
  onComplete,
  onIssue,
  onCancel,
  onAssign,
  onEdit,
}: {
  task: FieldTask;
  inspection: string | null;
  busy: boolean;
  onStart: () => void;
  onComplete: () => void;
  onIssue: () => void;
  onCancel: () => void;
  onAssign: () => void;
  onEdit: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const none = task.actions.length === 0;
  return (
    <div className="grid gap-2">
      {none ? (
        <p className="text-muted-foreground text-sm">
          {task.status === "completed" || task.status === "cancelled"
            ? t("field_tasks.detail.closed")
            : t("field_tasks.detail.no_actions")}
        </p>
      ) : null}
      {can(task, "start") ? (
        <Button onClick={onStart} disabled={busy}>
          <Play className="size-4" />
          {task.status === "issue_reported"
            ? t("field_tasks.actions.resume")
            : t("field_tasks.actions.start")}
        </Button>
      ) : null}
      {can(task, "inspection") && inspection ? (
        <Button asChild variant={can(task, "start") ? "outline" : "default"}>
          <Link href={inspection}>
            <ClipboardCheck className="size-4" />
            {task.inspection === "return"
              ? t("field_tasks.actions.open_return_form")
              : t("field_tasks.actions.open_handover_form")}
          </Link>
        </Button>
      ) : null}
      {(task.task_type === "handover" || task.task_type === "return") &&
      task.actions.length > 0 &&
      !task.rental ? (
        <p className="text-muted-foreground text-xs">
          {t("field_tasks.detail.waiting_rental")}
        </p>
      ) : null}
      {can(task, "complete") ? (
        <Button variant="outline" onClick={onComplete} disabled={busy}>
          <CheckCircle2 className="size-4" />
          {t("field_tasks.actions.complete")}
        </Button>
      ) : null}
      {can(task, "report_issue") ? (
        <Button variant="outline" onClick={onIssue} disabled={busy}>
          <AlertTriangle className="size-4" />
          {t("field_tasks.actions.report_issue")}
        </Button>
      ) : null}
      {can(task, "assign") ? (
        <Button variant="outline" onClick={onAssign}>
          <UserCheck className="size-4" />
          {t("field_tasks.actions.assign")}
        </Button>
      ) : null}
      {can(task, "edit") ? (
        <Button variant="outline" onClick={onEdit}>
          <Pencil className="size-4" />
          {t("field_tasks.actions.edit")}
        </Button>
      ) : null}
      {can(task, "cancel") ? (
        <Button variant="ghost" className="text-destructive" onClick={onCancel}>
          <XCircle className="size-4" />
          {t("field_tasks.actions.cancel")}
        </Button>
      ) : null}
      {task.transfer ? (
        <p className="text-muted-foreground flex items-center gap-1 text-xs">
          <MapPin className="size-3" aria-hidden />
          {t("field_tasks.detail.transfer_hint")}
        </p>
      ) : null}
      {task.rental ? (
        <Button asChild variant="link" className="justify-start px-0">
          <Link href={routes.tenant.rentals.detail(slug, task.rental.uuid)}>
            <ExternalLink className="size-3.5" />
            {t("field_tasks.actions.open_rental")}
          </Link>
        </Button>
      ) : null}
    </div>
  );
}

/** Sticky bottom bar on small screens with the one main step. */
function MobileActionBar({
  task,
  inspection,
  busy,
  onStart,
  onComplete,
}: {
  task: FieldTask;
  inspection: string | null;
  busy: boolean;
  onStart: () => void;
  onComplete: () => void;
}) {
  const { t } = useLocale();
  let main: React.ReactNode = null;
  if (can(task, "start")) {
    main = (
      <Button className="h-11 w-full" onClick={onStart} disabled={busy}>
        <Play className="size-4" />
        {task.status === "issue_reported"
          ? t("field_tasks.actions.resume")
          : t("field_tasks.actions.start")}
      </Button>
    );
  } else if (can(task, "inspection") && inspection) {
    main = (
      <Button asChild className="h-11 w-full">
        <Link href={inspection}>
          <ClipboardCheck className="size-4" />
          {task.inspection === "return"
            ? t("field_tasks.actions.open_return_form")
            : t("field_tasks.actions.open_handover_form")}
        </Link>
      </Button>
    );
  } else if (can(task, "complete")) {
    main = (
      <Button className="h-11 w-full" onClick={onComplete} disabled={busy}>
        <CheckCircle2 className="size-4" />
        {t("field_tasks.actions.complete")}
      </Button>
    );
  }
  if (!main) return null;
  return (
    <div className="bg-background/95 supports-[backdrop-filter]:bg-background/80 fixed inset-x-0 bottom-0 z-40 border-t p-3 backdrop-blur lg:hidden">
      {main}
    </div>
  );
}
