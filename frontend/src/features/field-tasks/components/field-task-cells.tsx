"use client";

import { MapPin, Plane, Truck } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  FIELD_TASK_STATUS_LABEL_KEYS,
  FIELD_TASK_STATUS_TONES,
  taskTypeLabel,
} from "@/features/field-tasks/constants";
import { placeText } from "@/features/field-tasks/lib/field-tasks";
import type { FieldTask, FieldTaskStatus } from "@/features/field-tasks/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export function FieldTaskStatusChip({ status }: { status: FieldTaskStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(FIELD_TASK_STATUS_LABEL_KEYS[status])}
      tone={FIELD_TASK_STATUS_TONES[status]}
    />
  );
}

/** Type badge; deliveries and collections are highlighted. */
export function FieldTaskTypeBadge({
  task,
}: {
  task: Pick<FieldTask, "task_type" | "delivery">;
}) {
  const { t } = useLocale();
  return (
    <Badge
      variant={task.delivery ? "default" : "secondary"}
      className="gap-1 whitespace-nowrap"
    >
      {task.delivery ? <Truck className="size-3" aria-hidden /> : null}
      {taskTypeLabel(t, task)}
    </Badge>
  );
}

/** Place line: icon by kind, the address / point / branch text. */
export function FieldTaskPlace({
  task,
  className,
}: {
  task: FieldTask;
  className?: string;
}) {
  const airport = task.location.point?.type === "airport";
  const Icon = airport ? Plane : MapPin;
  return (
    <span className={cn("flex min-w-0 items-center gap-1.5", className)}>
      <Icon className="text-muted-foreground size-3.5 shrink-0" aria-hidden />
      <span className="truncate">{placeText(task)}</span>
    </span>
  );
}
