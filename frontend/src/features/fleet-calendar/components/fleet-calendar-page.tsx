"use client";

import { ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { useMemo, useState } from "react";

import { Card, CardContent } from "@/components/common/card";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCalendarActions } from "@/features/fleet-calendar/components/calendar-actions";
import {
  CalendarLegend,
  CalendarNavigation,
  FacetFilter,
  type FacetOption,
  PlateSearch,
} from "@/features/fleet-calendar/components/calendar-controls";
import { FleetCalendarList } from "@/features/fleet-calendar/components/fleet-calendar-list";
import { FleetTimeline } from "@/features/fleet-calendar/components/fleet-timeline";
import { CALENDAR_PAGE_SIZE } from "@/features/fleet-calendar/constants";
import {
  useFleetCalendar,
  useFleetCalendarLive,
} from "@/features/fleet-calendar/hooks/use-fleet-calendar";
import {
  calendarWindow,
  shiftAnchor,
} from "@/features/fleet-calendar/lib/timeline";
import type {
  CalendarZoom,
  FleetCalendarFilters,
  FleetCalendarParams,
} from "@/features/fleet-calendar/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { STATUS_LABEL_KEYS } from "@/features/vehicles/constants";
import { useVehicleFacets } from "@/features/vehicles/hooks/use-vehicles";
import type { VehicleStatus } from "@/features/vehicles/types";
import { useDebounce } from "@/hooks/use-debounce";
import { useIsMobile } from "@/hooks/use-mobile";
import { useLocale } from "@/providers/locale-provider";

const EMPTY_FILTERS: FleetCalendarFilters = {
  q: "",
  status: [],
  segment: [],
  rental_group: [],
  home_branch: [],
  current_branch: [],
};

const join = (values: string[]) =>
  values.length ? values.join(",") : undefined;

/**
 * Fleet calendar (`/t/{slug}/fleet/calendar`): vehicles × time with
 * blocks, transfers and maintenance days; day / week / month zoom,
 * navigation, filters, live refresh and a stacked list on phones.
 */
export function FleetCalendarPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const isMobile = useIsMobile();
  const [zoom, setZoom] = useState<CalendarZoom>("week");
  const [anchor, setAnchor] = useState(() => new Date());
  const [filters, setFilters] = useState<FleetCalendarFilters>(EMPTY_FILTERS);
  const [offset, setOffset] = useState(0);
  const q = useDebounce(filters.q.trim(), 300);
  const window = useMemo(() => calendarWindow(zoom, anchor), [zoom, anchor]);
  const facets = useVehicleFacets();
  const { onAction, dialogs } = useCalendarActions();

  const params = useMemo<FleetCalendarParams>(
    () => ({
      from: window.from.toISOString(),
      to: window.to.toISOString(),
      limit: CALENDAR_PAGE_SIZE,
      offset,
      q: q || undefined,
      status: join(filters.status),
      segment: join(filters.segment),
      rental_group: join(filters.rental_group),
      home_branch: join(filters.home_branch),
      current_branch: join(filters.current_branch),
    }),
    [filters, offset, q, window],
  );
  const query = useFleetCalendar(params);
  useFleetCalendarLive(() => query.refetch());

  const setFilter = <K extends keyof FleetCalendarFilters>(
    key: K,
    value: FleetCalendarFilters[K],
  ) => {
    setFilters((prev) => ({ ...prev, [key]: value }));
    setOffset(0);
  };
  const hasFilters =
    Boolean(q) ||
    filters.status.length +
      filters.segment.length +
      filters.rental_group.length +
      filters.home_branch.length +
      filters.current_branch.length >
      0;

  const facetOptions = (
    items: { value: string; label?: string; count: number }[] | undefined,
  ): FacetOption[] =>
    (items ?? []).map((f) => ({
      value: f.value,
      label: f.label || f.value,
      count: f.count,
    }));
  const statusOptions: FacetOption[] = (facets.data?.status ?? []).map((f) => ({
    value: f.value,
    label: STATUS_LABEL_KEYS[f.value as VehicleStatus]
      ? t(STATUS_LABEL_KEYS[f.value as VehicleStatus])
      : f.value,
    count: f.count,
  }));

  const data = query.data;
  const rows = data?.items ?? [];
  const total = data?.total ?? 0;
  const first = total === 0 ? 0 : offset + 1;
  const last = Math.min(offset + CALENDAR_PAGE_SIZE, total);

  return (
    <EntityPage
      title={t("fleet_calendar.title")}
      description={t("fleet_calendar.description")}
      permission={permissions.tenant.vehicles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("fleet_calendar.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("fleet_calendar.title") },
      ]}
      actions={
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw
            className={query.isFetching ? "size-4 animate-spin" : "size-4"}
          />
          {t("fleet_calendar.refresh")}
        </Button>
      }
    >
      <Card className="shadow-none">
        <CardContent className="space-y-4 p-4">
          <CalendarNavigation
            zoom={zoom}
            window={window}
            anchor={anchor}
            onAnchorChange={setAnchor}
            onShift={(direction) =>
              setAnchor((current) => shiftAnchor(zoom, current, direction))
            }
            onZoomChange={setZoom}
          />
          <div className="flex flex-wrap items-center gap-2">
            <PlateSearch
              value={filters.q}
              onChange={(value) => setFilter("q", value)}
            />
            <FacetFilter
              title={t("fleet_calendar.filters.status")}
              options={statusOptions}
              value={filters.status}
              onChange={(value) => setFilter("status", value)}
            />
            <FacetFilter
              title={t("fleet_calendar.filters.segment")}
              options={facetOptions(facets.data?.segment)}
              value={filters.segment}
              onChange={(value) => setFilter("segment", value)}
            />
            <FacetFilter
              title={t("fleet_calendar.filters.rental_group")}
              options={facetOptions(facets.data?.rental_group)}
              value={filters.rental_group}
              onChange={(value) => setFilter("rental_group", value)}
            />
            <FacetFilter
              title={t("fleet_calendar.filters.home_branch")}
              options={facetOptions(facets.data?.home_branch)}
              value={filters.home_branch}
              onChange={(value) => setFilter("home_branch", value)}
            />
            <FacetFilter
              title={t("fleet_calendar.filters.current_branch")}
              options={facetOptions(facets.data?.current_branch)}
              value={filters.current_branch}
              onChange={(value) => setFilter("current_branch", value)}
            />
            {hasFilters ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-9"
                onClick={() => {
                  setFilters(EMPTY_FILTERS);
                  setOffset(0);
                }}
              >
                {t("fleet_calendar.filters.reset")}
              </Button>
            ) : null}
          </div>
          <CalendarLegend />

          {query.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              description={t("fleet_calendar.load_failed")}
              onRetry={() => void query.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : query.isLoading ? (
            <Loading label={t("common.loading")} />
          ) : isMobile ? (
            <FleetCalendarList
              rows={rows}
              onAction={onAction}
              emptyTitle={
                hasFilters
                  ? t("fleet_calendar.empty_filtered_title")
                  : t("fleet_calendar.empty_title")
              }
              emptyDescription={t("fleet_calendar.empty_description")}
            />
          ) : rows.length === 0 ? (
            <EmptyState
              className="py-10"
              title={
                hasFilters
                  ? t("fleet_calendar.empty_filtered_title")
                  : t("fleet_calendar.empty_title")
              }
              description={t("fleet_calendar.empty_description")}
            />
          ) : (
            <FleetTimeline
              rows={rows}
              zoom={zoom}
              window={window}
              onAction={onAction}
            />
          )}

          {total > 0 ? (
            <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
              <span className="text-muted-foreground tabular-nums">
                {t("fleet_calendar.page_info", { first, last, total })}
              </span>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={offset === 0}
                  onClick={() =>
                    setOffset((o) => Math.max(0, o - CALENDAR_PAGE_SIZE))
                  }
                >
                  <ChevronLeft className="size-4" />
                  {t("fleet_calendar.page_prev")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={offset + CALENDAR_PAGE_SIZE >= total}
                  onClick={() => setOffset((o) => o + CALENDAR_PAGE_SIZE)}
                >
                  {t("fleet_calendar.page_next")}
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>
      {dialogs}
    </EntityPage>
  );
}
