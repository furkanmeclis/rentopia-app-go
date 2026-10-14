"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import {
  DaysLeft,
  ExpiryDate,
  KmValue,
} from "@/features/vehicle-records/components/record-cells";
import {
  ALERT_KIND_LABEL_KEYS,
  ALERT_STATE_LABEL_KEYS,
  DOCUMENTS_RESOURCE,
  DOCUMENT_TYPE_LABEL_KEYS,
  MAINTENANCE_RESOURCE,
  MAINTENANCE_TYPE_LABEL_KEYS,
  type FleetAlertKind,
  type FleetAlertState,
} from "@/features/vehicle-records/constants";
import { useFleetAlerts } from "@/features/vehicle-records/hooks/use-vehicle-records";
import type {
  FleetAlert,
  MaintenanceType,
  VehicleDocumentType,
} from "@/features/vehicle-records/types";
import { PlateBadge } from "@/features/vehicles/components/vehicle-cells";
import { VEHICLES_RESOURCE } from "@/features/vehicles/constants";
import { useLocale } from "@/providers/locale-provider";

const KINDS: FleetAlertKind[] = ["document", "maintenance"];
const STATES: FleetAlertState[] = ["overdue", "expiring"];

function alertHref(slug: string, alert: FleetAlert) {
  const tab = alert.kind === "document" ? "documents" : "maintenance";
  return `${routes.tenant.fleet.vehicles.detail(slug, alert.vehicle.uuid)}?tab=${tab}`;
}

/**
 * "Operasyon özeti": upcoming and overdue documents (30 days) and
 * maintenance plans (15 days / 1000 km) of the vehicles in the member's
 * branch scope. Refreshes live when a vehicle changes.
 */
export function FleetAlertsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const query = useFleetAlerts();
  const live = tenantLiveList(organization, VEHICLES_RESOURCE);

  const typeLabel = useCallback(
    (alert: FleetAlert) =>
      alert.kind === "document"
        ? t(DOCUMENT_TYPE_LABEL_KEYS[alert.type as VehicleDocumentType] ?? "")
        : t(MAINTENANCE_TYPE_LABEL_KEYS[alert.type as MaintenanceType] ?? ""),
    [t],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<FleetAlert>({
          id: "state",
          accessorFn: (row) => row.state,
          labelKey: "vehicle_records.alerts.columns.state",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: STATES.map((value) => ({
            value,
            labelKey: ALERT_STATE_LABEL_KEYS[value],
            label: t(ALERT_STATE_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(ALERT_STATE_LABEL_KEYS[row.original.state])}
              tone={row.original.state === "overdue" ? "danger" : "warning"}
            />
          ),
        }),
        createColumn<FleetAlert>({
          id: "plate",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "vehicle_records.alerts.columns.vehicle",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={alertHref(slug, row.original)}
              className="flex min-w-0 flex-col gap-1"
              onClick={(event) => event.stopPropagation()}
            >
              <PlateBadge plate={row.original.vehicle.plate} />
              <span className="text-muted-foreground truncate text-xs">
                {row.original.vehicle.label}
              </span>
            </Link>
          ),
        }),
        createColumn<FleetAlert>({
          id: "kind",
          accessorFn: (row) => row.kind,
          labelKey: "vehicle_records.alerts.columns.kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: KINDS.map((value) => ({
            value,
            labelKey: ALERT_KIND_LABEL_KEYS[value],
            label: t(ALERT_KIND_LABEL_KEYS[value]),
          })),
          cell: ({ row }) =>
            t(ALERT_KIND_LABEL_KEYS[row.original.kind as FleetAlertKind]),
        }),
        createColumn<FleetAlert>({
          id: "type",
          accessorFn: (row) => typeLabel(row),
          labelKey: "vehicle_records.alerts.columns.type",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span className="font-medium">{typeLabel(row.original)}</span>
              {row.original.document_number ? (
                <span className="text-muted-foreground text-xs">
                  {row.original.document_number}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<FleetAlert>({
          id: "due",
          accessorFn: (row) => row.due_date ?? "9999-12-31",
          labelKey: "vehicle_records.alerts.columns.due",
          enableSorting: true,
          cell: ({ row }) => {
            const a = row.original;
            return (
              <div className="flex flex-col gap-0.5">
                {a.due_date ? (
                  <ExpiryDate
                    value={a.due_date}
                    horizonDays={a.kind === "document" ? 30 : 15}
                  />
                ) : null}
                {a.due_km != null ? <KmValue value={a.due_km} /> : null}
                <DaysLeft days={a.days_left} />
                {a.km_left != null ? (
                  <span
                    className={
                      a.km_left <= 0
                        ? "text-destructive text-xs"
                        : "text-muted-foreground text-xs"
                    }
                  >
                    {a.km_left <= 0
                      ? t("vehicle_records.km_overdue", { count: -a.km_left })
                      : t("vehicle_records.km_left", { count: a.km_left })}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<FleetAlert>({
          id: "branch",
          accessorFn: (row) => row.vehicle.current_branch,
          labelKey: "vehicle_records.alerts.columns.branch",
          enableSorting: true,
          cell: ({ row }) => row.original.vehicle.current_branch,
        }),
        createColumn<FleetAlert>({
          id: "odometer",
          accessorFn: (row) => row.vehicle.odometer_km,
          labelKey: "vehicle_records.alerts.columns.odometer",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <KmValue value={row.original.vehicle.odometer_km} />
          ),
        }),
        createColumn<FleetAlert>({
          id: "actions",
          labelKey: "vehicle_records.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("vehicle_records.alerts.open_vehicle"),
                  icon: Eye,
                  onSelect: () => router.push(alertHref(slug, row.original)),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<FleetAlert, unknown>[],
    [router, slug, t, typeLabel],
  );

  const data = query.data;
  const refetch = () => void query.refetch();

  return (
    <EntityPage
      title={t("vehicle_records.alerts.title")}
      description={
        data
          ? t("vehicle_records.alerts.description", {
              documents: data.document_horizon_days,
              days: data.maintenance_horizon_days,
              km: data.maintenance_horizon_km,
            })
          : t("vehicle_records.alerts.description_short")
      }
      permission={permissions.tenant.vehicles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("vehicle_records.alerts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("vehicle_records.alerts.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={data?.items ?? []}
        getRowId={(row) => `${row.kind}-${row.subject_uuid}`}
        onRowClick={(row) => router.push(alertHref(slug, row))}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={refetch}
        realtime={live ? { ...live, refetch } : undefined}
        emptyTitle={t("vehicle_records.alerts.empty_title")}
        emptyDescription={t("vehicle_records.alerts.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-fleet-alerts-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={DOCUMENTS_RESOURCE}
              capabilities={{ export: true }}
            />
            <ResourceIOToolbar
              resource={MAINTENANCE_RESOURCE}
              capabilities={{ export: true }}
            />
            <EntityToolbar
              onRefresh={refetch}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
