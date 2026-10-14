"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Wrench } from "lucide-react";
import { useMemo } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  type CalendarAction,
  IntervalDetails,
  MaintenanceDetails,
} from "@/features/fleet-calendar/components/bar-details";
import {
  BLOCK_BAR_CLASSES,
  MAINTENANCE_MARKER_CLASSES,
  TRANSFER_BAR_CLASSES,
} from "@/features/fleet-calendar/constants";
import type {
  FleetCalendarInterval,
  FleetCalendarRow,
} from "@/features/fleet-calendar/types";
import { BLOCK_KIND_LABEL_KEYS } from "@/features/fleet-ops/constants";
import {
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { MAINTENANCE_TYPE_LABEL_KEYS } from "@/features/vehicle-records/constants";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function chipClasses(interval: FleetCalendarInterval) {
  if (interval.block) return BLOCK_BAR_CLASSES[interval.block.kind];
  if (interval.transfer) return TRANSFER_BAR_CLASSES[interval.transfer.status];
  return "";
}

/**
 * Phone fallback of the timeline: the same rows as a DataTable (cards on
 * small screens), each vehicle with its bars of the window as a stacked
 * list; every item opens the same popover as on the timeline.
 */
export function FleetCalendarList({
  rows,
  isLoading,
  onAction,
  emptyTitle,
  emptyDescription,
}: {
  rows: FleetCalendarRow[];
  isLoading?: boolean;
  onAction?: (action: CalendarAction) => void;
  emptyTitle: string;
  emptyDescription?: string;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<FleetCalendarRow>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "fleet_calendar.vehicle_column",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const v = row.original.vehicle;
            return (
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <PlateBadge plate={v.plate} />
                  <VehicleStatusChip status={v.status} />
                </div>
                <p className="text-muted-foreground text-xs">
                  {v.label || "—"} · {v.current_branch.name}
                </p>
              </div>
            );
          },
        }),
        createColumn<FleetCalendarRow>({
          id: "items",
          labelKey: "fleet_calendar.items_column",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const { vehicle, intervals, maintenance } = row.original;
            if (intervals.length === 0 && maintenance.length === 0) {
              return (
                <span className="text-muted-foreground text-sm">
                  {t("fleet_calendar.free")}
                </span>
              );
            }
            return (
              <ul className="space-y-1.5">
                {intervals.map((interval) => (
                  <li key={`${interval.kind}-${interval.uuid}`}>
                    <Popover>
                      <PopoverTrigger asChild>
                        <button
                          type="button"
                          className={cn(
                            "focus-visible:ring-ring flex w-full items-center justify-between gap-2 rounded border px-2 py-1 text-start text-xs outline-none focus-visible:ring-2",
                            chipClasses(interval),
                          )}
                        >
                          <span className="truncate font-medium">
                            {interval.block
                              ? t(BLOCK_KIND_LABEL_KEYS[interval.block.kind])
                              : interval.transfer
                                ? `${interval.transfer.from_branch.code} → ${interval.transfer.to_branch.code}`
                                : ""}
                          </span>
                          <span className="shrink-0 tabular-nums">
                            {datetime(
                              interval.starts_at,
                              "dd.MM HH:mm",
                              locale,
                            )}{" "}
                            –{" "}
                            {interval.ends_at
                              ? datetime(
                                  interval.ends_at,
                                  "dd.MM HH:mm",
                                  locale,
                                )
                              : t("vehicle_blocks.open_ended")}
                          </span>
                        </button>
                      </PopoverTrigger>
                      <PopoverContent className="w-80" align="start">
                        <IntervalDetails
                          vehicle={vehicle}
                          interval={interval}
                          onAction={onAction}
                        />
                      </PopoverContent>
                    </Popover>
                  </li>
                ))}
                {maintenance.map((item) => (
                  <li key={`m-${item.uuid}`}>
                    <Popover>
                      <PopoverTrigger asChild>
                        <button
                          type="button"
                          className={cn(
                            "focus-visible:ring-ring flex w-full items-center justify-between gap-2 rounded border px-2 py-1 text-start text-xs outline-none focus-visible:ring-2",
                            MAINTENANCE_MARKER_CLASSES[item.state],
                          )}
                        >
                          <span className="flex items-center gap-1 truncate font-medium">
                            <Wrench className="size-3" aria-hidden />
                            {t(
                              MAINTENANCE_TYPE_LABEL_KEYS[
                                item.maintenance_type
                              ],
                            )}
                          </span>
                          <span className="shrink-0 tabular-nums">
                            {date(item.date, undefined, locale)}
                          </span>
                        </button>
                      </PopoverTrigger>
                      <PopoverContent className="w-80" align="start">
                        <MaintenanceDetails vehicle={vehicle} item={item} />
                      </PopoverContent>
                    </Popover>
                  </li>
                ))}
              </ul>
            );
          },
        }),
      ] as ColumnDef<FleetCalendarRow, unknown>[],
    [locale, onAction, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={rows}
      getRowId={(row) => row.vehicle.uuid}
      isLoading={isLoading}
      emptyTitle={emptyTitle}
      emptyDescription={emptyDescription}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-fleet-calendar-list-v1",
        rowSelection: false,
        globalFilter: false,
        columnFilters: false,
        facetedFilters: false,
        pagination: false,
        viewMode: false,
        density: false,
        columnVisibility: false,
      }}
    />
  );
}
