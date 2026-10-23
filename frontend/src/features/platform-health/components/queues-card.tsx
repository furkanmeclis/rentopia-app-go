"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle } from "lucide-react";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard } from "@/components/entity";
import { createColumn, DataTable } from "@/components/tables";
import { healthLabel } from "@/features/platform-health/lib/labels";
import type {
  HealthJobStatusCount,
  HealthQueue,
} from "@/features/platform-health/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Job statuses that need a person (shown with a warning tone). */
const ATTENTION_STATUSES = new Set([
  "failed",
  "needs_verification",
  "blocked",
  "waiting_dependency",
]);

function CountCell({ value, tone }: { value: number; tone?: "danger" }) {
  const { locale } = useLocale();
  return (
    <span
      className={cn(
        "tabular-nums",
        value > 0 && tone === "danger" && "text-destructive font-semibold",
      )}
    >
      {value.toLocaleString(locale)}
    </span>
  );
}

/**
 * Background queue snapshot (Asynq: pending / active / scheduled / retry /
 * archived = dead per queue) and integration jobs per status (TEC-744).
 */
export function QueuesCard({
  queues,
  queuesError,
  jobs,
}: {
  queues: HealthQueue[];
  queuesError?: string;
  jobs: HealthJobStatusCount[];
}) {
  const { t, locale } = useLocale();

  const columns = useMemo<ColumnDef<HealthQueue>[]>(
    () => [
      createColumn<HealthQueue>({
        accessorKey: "queue",
        labelKey: "platform_health.queues.columns.queue",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono text-sm">{row.original.queue}</span>
        ),
      }),
      createColumn<HealthQueue>({
        accessorKey: "pending",
        labelKey: "platform_health.queues.columns.pending",
        enableSorting: true,
        cell: ({ row }) => <CountCell value={row.original.pending} />,
      }),
      createColumn<HealthQueue>({
        accessorKey: "active",
        labelKey: "platform_health.queues.columns.active",
        enableSorting: true,
        cell: ({ row }) => <CountCell value={row.original.active} />,
      }),
      createColumn<HealthQueue>({
        accessorKey: "scheduled",
        labelKey: "platform_health.queues.columns.scheduled",
        enableSorting: true,
        cell: ({ row }) => <CountCell value={row.original.scheduled} />,
      }),
      createColumn<HealthQueue>({
        accessorKey: "retry",
        labelKey: "platform_health.queues.columns.retry",
        enableSorting: true,
        cell: ({ row }) => <CountCell value={row.original.retry} />,
      }),
      createColumn<HealthQueue>({
        accessorKey: "archived",
        labelKey: "platform_health.queues.columns.archived",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <CountCell value={row.original.archived} tone="danger" />
        ),
      }),
      createColumn<HealthQueue>({
        accessorKey: "paused",
        labelKey: "platform_health.queues.columns.state",
        cell: ({ row }) =>
          row.original.paused ? (
            <StatusChip
              label={t("platform_health.queues.paused")}
              tone="warning"
            />
          ) : (
            <StatusChip
              label={t("platform_health.queues.running")}
              tone="success"
            />
          ),
      }),
    ],
    [t],
  );

  return (
    <EntitySectionCard title={t("platform_health.queues.title")}>
      <div className="space-y-4">
        <p className="text-muted-foreground text-sm">
          {t("platform_health.queues.description")}
        </p>
        {queuesError ? (
          <p className="text-destructive flex items-center gap-2 text-sm">
            <AlertTriangle className="size-4" />
            {t("platform_health.queues.error")}
          </p>
        ) : null}
        <DataTable
          columns={columns}
          data={queues}
          getRowId={(row) => row.queue}
          emptyTitle={t("platform_health.queues.empty_title")}
          emptyDescription={t("platform_health.queues.empty_description")}
          initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
          features={{
            sorting: true,
            multiSort: false,
            globalFilter: false,
            columnFilters: false,
            columnVisibility: false,
            columnOrdering: false,
            columnPinning: false,
            columnResizing: false,
            rowSelection: false,
            pagination: false,
            density: false,
            viewMode: false,
            facetedFilters: false,
          }}
        />
        <div className="space-y-2">
          <h3 className="text-sm font-medium">
            {t("platform_health.jobs.title")}
          </h3>
          {jobs.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("platform_health.jobs.empty")}
            </p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {jobs.map((j) => (
                <StatusChip
                  key={j.status}
                  label={`${healthLabel(locale, "job_status", j.status)}: ${j.count.toLocaleString(locale)}`}
                  tone={
                    j.count > 0 && ATTENTION_STATUSES.has(j.status)
                      ? j.status === "failed"
                        ? "danger"
                        : "warning"
                      : "default"
                  }
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </EntitySectionCard>
  );
}
