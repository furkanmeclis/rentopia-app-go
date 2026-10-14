"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { AppChart, buildChartConfig } from "@/components/charts";
import { Badge } from "@/components/ui/badge";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { MAINTENANCE_TYPE_LABEL_KEYS } from "@/features/vehicle-records/constants";
import { useVehicleMaintenance } from "@/features/vehicle-records/hooks/use-vehicle-records";
import type { MaintenanceType } from "@/features/vehicle-records/types";
import { useVehicleEvents } from "@/features/vehicles/hooks/use-vehicles";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Newest status events read for the history (API page maximum). */
const EVENT_LIMIT = 100;

type Reading = {
  id: string;
  at: string;
  /** true: a day without a time (maintenance done date). */
  dayOnly: boolean;
  source: "status" | "maintenance";
  km: number | null;
  fuel: number | null;
  detail: string;
};

/**
 * "Km / yakıt" tab: odometer and fuel readings derived from the status
 * history (vehicle_events, newest 100) and the km of done maintenance, as
 * a line chart of the km and a DataTable of every reading.
 */
export function VehicleOdometerTab({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const events = useVehicleEvents(uuid, { limit: EVENT_LIMIT, offset: 0 });
  const maintenance = useVehicleMaintenance(uuid);
  const nf = useMemo(
    () => new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US"),
    [locale],
  );

  const readings = useMemo<Reading[]>(() => {
    const out: Reading[] = [];
    for (const ev of events.data?.items ?? []) {
      if (ev.odometer_km == null && ev.fuel_level == null) continue;
      out.push({
        id: `e-${ev.uuid}`,
        at: ev.created_at,
        dayOnly: false,
        source: "status",
        km: ev.odometer_km,
        fuel: ev.fuel_level,
        detail: ev.note,
      });
    }
    for (const m of maintenance.data?.items ?? []) {
      if (m.kind !== "done" || m.done_km == null || !m.done_date) continue;
      out.push({
        id: `m-${m.uuid}`,
        at: m.done_date,
        dayOnly: true,
        source: "maintenance",
        km: m.done_km,
        fuel: null,
        detail: t(
          MAINTENANCE_TYPE_LABEL_KEYS[m.maintenance_type as MaintenanceType],
        ),
      });
    }
    return out.sort((a, b) => b.at.localeCompare(a.at));
  }, [events.data, maintenance.data, t]);

  const chartData = useMemo(
    () =>
      readings
        .filter((r) => r.km != null)
        .slice()
        .reverse()
        .map((r) => ({ label: date(r.at, "dd.MM.yy", locale), km: r.km })),
    [locale, readings],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Reading>({
          accessorKey: "at",
          labelKey: "vehicles.odometer.columns.at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.dayOnly
              ? date(row.original.at, undefined, locale)
              : datetime(row.original.at, undefined, locale),
        }),
        createColumn<Reading>({
          id: "source",
          accessorFn: (row) => row.source,
          labelKey: "vehicles.odometer.columns.source",
          enableSorting: true,
          cell: ({ row }) => (
            <Badge variant="outline">
              {row.original.source === "maintenance"
                ? t("vehicles.odometer.source_maintenance")
                : t("vehicles.odometer.source_status")}
            </Badge>
          ),
        }),
        createColumn<Reading>({
          id: "km",
          accessorFn: (row) => row.km ?? -1,
          labelKey: "vehicles.odometer.columns.km",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.km != null ? (
              <span className="tabular-nums">
                {nf.format(row.original.km)} km
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<Reading>({
          id: "fuel",
          accessorFn: (row) => row.fuel ?? -1,
          labelKey: "vehicles.odometer.columns.fuel",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.fuel != null ? `%${row.original.fuel}` : "—",
        }),
        createColumn<Reading>({
          accessorKey: "detail",
          labelKey: "vehicles.odometer.columns.detail",
          enableSorting: false,
          cell: ({ row }) => row.original.detail || "—",
        }),
      ] as ColumnDef<Reading, unknown>[],
    [locale, nf, t],
  );

  const loading = events.isLoading || maintenance.isLoading;
  return (
    <div className="space-y-6">
      {chartData.length > 1 ? (
        <AppChart
          type="line"
          title={t("vehicles.odometer.chart_title")}
          description={t("vehicles.odometer.chart_description")}
          data={chartData}
          config={buildChartConfig([
            { key: "km", label: t("vehicles.odometer.columns.km") },
          ])}
          valueFormatter={(v) => `${nf.format(v)} km`}
          height={220}
          showLegend={false}
        />
      ) : null}
      <EntitySectionCard
        title={t("vehicles.tabs.odometer")}
        badge={readings.length}
      >
        <EntityTable
          columns={columns}
          data={readings}
          getRowId={(row) => row.id}
          isLoading={loading}
          isError={events.isError || maintenance.isError}
          onRetry={() => {
            void events.refetch();
            void maintenance.refetch();
          }}
          emptyTitle={t("vehicles.odometer.empty_title")}
          emptyDescription={t("vehicles.odometer.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{ pagination: { pageIndex: 0, pageSize: 10 } }}
          features={{
            persistKey: "tenant-vehicle-odometer-v1",
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}
