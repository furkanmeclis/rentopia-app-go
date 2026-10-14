"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Plus, Trash2 } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Timeline } from "@/components/common/timeline";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { fleetService } from "@/features/fleet/services/fleet.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  KaskoValue,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { REASON_LABEL_KEYS } from "@/features/vehicles/constants";
import {
  useAddVehicleGroup,
  useRemoveVehicleGroup,
  useVehicleActivity,
  useVehicleEvents,
  useVehicleGroups,
  useVehicleKasko,
} from "@/features/vehicles/hooks/use-vehicles";
import type {
  VehicleEvent,
  VehicleGroupSuggestion,
  VehicleMembership,
  VehicleStatus,
  VehicleValuation,
} from "@/features/vehicles/types";
import { ApiError } from "@/lib/api";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

function useReasonText() {
  const { t } = useLocale();
  return useCallback(
    (reasons: string[]) =>
      reasons
        .map((reason) =>
          REASON_LABEL_KEYS[reason] ? t(REASON_LABEL_KEYS[reason]) : reason,
        )
        .join(", "),
    [t],
  );
}

// --- rental groups ----------------------------------------------------------

/**
 * Memberships (with current eligibility), suggested groups and a picker to
 * add any group: the API checks eligibility and answers 422 with the failed
 * criteria.
 */
export function VehicleGroupsTab({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.vehicles.write);
  const query = useVehicleGroups(uuid);
  const add = useAddVehicleGroup();
  const remove = useRemoveVehicleGroup();
  const reasonText = useReasonText();
  const [pick, setPick] = useState("");

  const addGroup = useCallback(
    async (groupUuid: string) => {
      try {
        await add.mutateAsync({ uuid, groupUuid });
        setPick("");
      } catch (error) {
        if (error instanceof ApiError && error.status === 422) {
          appToast.error(
            t("vehicles.groups.not_eligible", {
              reasons: reasonText(error.details.map((d) => d.code ?? "")),
            }),
          );
          return;
        }
        appToast.error(t("vehicles.groups.add_failed"));
      }
    },
    [add, reasonText, t, uuid],
  );

  const loadGroups = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await fleetService.listGroups(slug, {
        limit: 50,
        offset: 0,
        q: q || undefined,
        is_active: "true",
        sort: "code",
      });
      return page.items.map((g) => ({
        value: g.uuid,
        label: `${g.code} · ${g.name}`,
        description: g.segment.name,
      }));
    },
    [slug],
  );

  const memberColumns = useMemo(
    () =>
      [
        createColumn<VehicleMembership>({
          id: "code",
          accessorFn: (row) => row.group.code,
          labelKey: "vehicles.groups.columns.group",
          enableSorting: true,
          cell: ({ row }) => (
            <Link
              className="font-medium underline"
              href={routes.tenant.fleet.groups.detail(
                slug,
                row.original.group.uuid,
              )}
            >
              {row.original.group.code} · {row.original.group.name}
            </Link>
          ),
        }),
        createColumn<VehicleMembership>({
          id: "segment",
          accessorFn: (row) => row.group.segment.name,
          labelKey: "vehicles.groups.columns.segment",
          enableSorting: true,
          cell: ({ row }) => row.original.group.segment.name,
        }),
        createColumn<VehicleMembership>({
          id: "eligible",
          accessorFn: (row) => String(row.eligible),
          labelKey: "vehicles.groups.columns.eligible",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.eligible ? (
              <StatusChip
                label={t("vehicles.groups.eligible")}
                tone="success"
              />
            ) : (
              <div className="space-y-1">
                <StatusChip
                  label={t("vehicles.groups.not_eligible_badge")}
                  tone="danger"
                />
                <p className="text-muted-foreground text-xs">
                  {reasonText(row.original.reasons)}
                </p>
              </div>
            ),
        }),
        createColumn<VehicleMembership>({
          id: "added_at",
          accessorFn: (row) => row.added_at,
          labelKey: "vehicles.groups.columns.added_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.added_at),
        }),
        createColumn<VehicleMembership>({
          id: "actions",
          labelKey: "vehicles.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "remove",
                  label: t("vehicles.groups.remove"),
                  icon: Trash2,
                  permission: permissions.tenant.vehicles.write,
                  variant: "destructive",
                  onSelect: () =>
                    remove.mutate({ uuid, groupUuid: row.original.group.uuid }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<VehicleMembership, unknown>[],
    [reasonText, remove, slug, t, uuid],
  );

  const suggestionColumns = useMemo(
    () =>
      [
        createColumn<VehicleGroupSuggestion>({
          id: "code",
          accessorFn: (row) => row.group.code,
          labelKey: "vehicles.groups.columns.group",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.group.code} · {row.original.group.name}
            </span>
          ),
        }),
        createColumn<VehicleGroupSuggestion>({
          id: "segment",
          accessorFn: (row) => row.group.segment.name,
          labelKey: "vehicles.groups.columns.segment",
          enableSorting: true,
          cell: ({ row }) => row.original.group.segment.name,
        }),
        createColumn<VehicleGroupSuggestion>({
          id: "add",
          labelKey: "vehicles.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) =>
            canWrite ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={add.isPending}
                onClick={() => void addGroup(row.original.group.uuid)}
              >
                <Plus className="size-4" />
                {t("vehicles.groups.add")}
              </Button>
            ) : null,
        }),
      ] as ColumnDef<VehicleGroupSuggestion, unknown>[],
    [add.isPending, addGroup, canWrite, t],
  );

  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <div className="space-y-6">
      <EntitySectionCard
        title={t("vehicles.groups.memberships")}
        badge={query.data.items.length}
      >
        {canWrite ? (
          <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
            <AsyncCombobox
              className="sm:max-w-sm"
              value={pick}
              onValueChange={setPick}
              loadOptions={loadGroups}
              placeholder={t("vehicles.groups.pick_placeholder")}
            />
            <Button
              type="button"
              size="sm"
              disabled={!pick || add.isPending}
              onClick={() => void addGroup(pick)}
            >
              <Plus className="size-4" />
              {t("vehicles.groups.add")}
            </Button>
          </div>
        ) : null}
        <EntityTable
          columns={memberColumns}
          data={query.data.items}
          getRowId={(row) => row.group.uuid}
          emptyTitle={t("vehicles.groups.empty_title")}
          emptyDescription={t("vehicles.groups.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-vehicle-groups-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>

      <EntitySectionCard
        title={t("vehicles.groups.suggestions")}
        badge={query.data.suggestions.length}
      >
        <p className="text-muted-foreground mb-4 text-sm">
          {t("vehicles.groups.suggestions_help")}
        </p>
        <EntityTable
          columns={suggestionColumns}
          data={query.data.suggestions}
          getRowId={(row) => row.group.uuid}
          emptyTitle={t("vehicles.groups.suggestions_empty_title")}
          emptyDescription={t("vehicles.groups.suggestions_empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-vehicle-group-suggestions-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}

// --- status history ----------------------------------------------------------

function actorName(event: { actor: VehicleEvent["actor"] }) {
  if (!event.actor) return null;
  const full = `${event.actor.name} ${event.actor.surname}`.trim();
  return full || event.actor.email;
}

/** Status history (vehicle_events), server-side paginated DataTable. */
export function VehicleEventsTable({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const listState = useServerListState({ initialPageSize: 10 });
  const params = useMemo(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
    }),
    [listState.params.limit, listState.params.offset],
  );
  const query = useVehicleEvents(uuid, params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<VehicleEvent>({
          accessorKey: "created_at",
          labelKey: "vehicles.events.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<VehicleEvent>({
          id: "transition",
          labelKey: "vehicles.events.columns.transition",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-2">
              {row.original.from_status ? (
                <>
                  <VehicleStatusChip
                    status={row.original.from_status as VehicleStatus}
                  />
                  <span className="text-muted-foreground">→</span>
                </>
              ) : (
                <Badge variant="outline">{t("vehicles.events.created")}</Badge>
              )}
              <VehicleStatusChip
                status={row.original.to_status as VehicleStatus}
              />
            </div>
          ),
        }),
        createColumn<VehicleEvent>({
          accessorKey: "odometer_km",
          labelKey: "vehicles.events.columns.odometer_km",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.odometer_km != null
              ? `${row.original.odometer_km.toLocaleString(locale === "tr" ? "tr-TR" : "en-US")} km`
              : "—",
        }),
        createColumn<VehicleEvent>({
          accessorKey: "fuel_level",
          labelKey: "vehicles.events.columns.fuel_level",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.fuel_level != null
              ? `%${row.original.fuel_level}`
              : "—",
        }),
        createColumn<VehicleEvent>({
          accessorKey: "note",
          labelKey: "vehicles.events.columns.note",
          enableSorting: false,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<VehicleEvent>({
          id: "actor",
          labelKey: "vehicles.events.columns.actor",
          enableSorting: false,
          cell: ({ row }) =>
            actorName(row.original) ?? t("vehicles.events.system_actor"),
        }),
      ] as ColumnDef<VehicleEvent, unknown>[],
    [locale, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      emptyTitle={t("vehicles.events.empty_title")}
      emptyDescription={t("vehicles.events.empty_description")}
      pageCount={Math.max(1, Math.ceil(total / listState.pagination.pageSize))}
      state={listState.tableState}
      features={{
        persistKey: "tenant-vehicle-events-v1",
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
      }}
    />
  );
}

// --- kasko --------------------------------------------------------------------

/** TSB kasko value history of the vehicle's catalog type and model year. */
export function VehicleKaskoTab({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const query = useVehicleKasko(uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<VehicleValuation>({
          accessorKey: "period",
          labelKey: "vehicles.kasko.columns.period",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.period}</span>
          ),
        }),
        createColumn<VehicleValuation>({
          id: "value",
          accessorFn: (row) => Number(row.value),
          labelKey: "vehicles.kasko.columns.value",
          enableSorting: true,
          cell: ({ row }) => (
            <KaskoValue
              value={row.original.value}
              period={row.original.period}
            />
          ),
        }),
      ] as ColumnDef<VehicleValuation, unknown>[],
    [],
  );

  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const data = query.data;
  if (!data.catalog) {
    return (
      <EmptyState
        className="py-10"
        title={t("vehicles.kasko.manual_title")}
        description={t("vehicles.kasko.manual_description")}
      />
    );
  }
  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-sm">
        {t("vehicles.kasko.help", {
          type: `${data.catalog.brand_name} ${data.catalog.type_name}`,
          year: data.model_year,
        })}
      </p>
      <EntityTable
        columns={columns}
        data={data.items}
        getRowId={(row) => row.period}
        emptyTitle={t("vehicles.kasko.empty_title")}
        emptyDescription={t("vehicles.kasko.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-vehicle-kasko-v1",
          rowSelection: false,
        }}
      />
    </div>
  );
}

// --- activity -----------------------------------------------------------------

const ACTIVITY_PAGE = 10;

/** Audit trail of a vehicle. */
export function VehicleActivityCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(ACTIVITY_PAGE);
  const query = useVehicleActivity(uuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const describe = (action: string) => {
    const key = `activity.actions.${action}`;
    const label = t(key);
    return label === key ? action : label;
  };

  return (
    <EntitySectionCard
      title={t("vehicles.activity.title")}
      badge={query.data ? total : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <EmptyState className="py-8" title={t("vehicles.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry.action),
              description:
                actorName(entry) ?? t("vehicles.events.system_actor"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetching}
              onClick={() => setLimit((current) => current + ACTIVITY_PAGE)}
            >
              {t("vehicles.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}
