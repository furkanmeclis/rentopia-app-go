"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useCalendarActions } from "@/features/fleet-calendar/components/calendar-actions";
import {
  CalendarLegend,
  CalendarNavigation,
} from "@/features/fleet-calendar/components/calendar-controls";
import { FleetCalendarList } from "@/features/fleet-calendar/components/fleet-calendar-list";
import { FleetTimeline } from "@/features/fleet-calendar/components/fleet-timeline";
import {
  useFleetCalendar,
  useFleetCalendarLive,
} from "@/features/fleet-calendar/hooks/use-fleet-calendar";
import {
  calendarWindow,
  shiftAnchor,
} from "@/features/fleet-calendar/lib/timeline";
import type { CalendarZoom } from "@/features/fleet-calendar/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useIsMobile } from "@/hooks/use-mobile";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Takvim" tab of the vehicle page: the fleet calendar timeline reduced to
 * this vehicle (one row, same bars, popovers and actions).
 */
export function VehicleTimelineCard({ vehicleUuid }: { vehicleUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const isMobile = useIsMobile();
  const [zoom, setZoom] = useState<CalendarZoom>("month");
  const [anchor, setAnchor] = useState(() => new Date());
  const window = useMemo(() => calendarWindow(zoom, anchor), [zoom, anchor]);
  const { onAction, dialogs } = useCalendarActions();
  const query = useFleetCalendar({
    from: window.from.toISOString(),
    to: window.to.toISOString(),
    limit: 1,
    offset: 0,
    vehicle: vehicleUuid,
  });
  useFleetCalendarLive(() => query.refetch());
  const rows = query.data?.items ?? [];
  const row = rows[0];
  const empty =
    row && row.intervals.length === 0 && row.maintenance.length === 0;

  return (
    <EntitySectionCard title={t("vehicles.tabs.calendar")}>
      <div className="space-y-4">
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
        ) : !row ? (
          <EmptyState className="py-8" title={t("vehicles.not_found")} />
        ) : isMobile ? (
          <FleetCalendarList
            rows={rows}
            onAction={onAction}
            emptyTitle={t("fleet_calendar.vehicle_free_title")}
          />
        ) : (
          <div className="space-y-2">
            <FleetTimeline
              rows={rows}
              zoom={zoom}
              window={window}
              compact
              onAction={onAction}
            />
            {empty ? (
              <p className="text-muted-foreground text-sm">
                {t("fleet_calendar.vehicle_free_title")}
              </p>
            ) : null}
          </div>
        )}
        <div>
          <Button asChild size="sm" variant="link" className="px-0">
            <Link href={routes.tenant.fleet.calendar(slug)}>
              {t("fleet_calendar.open_fleet_calendar")}
            </Link>
          </Button>
        </div>
      </div>
      {dialogs}
    </EntitySectionCard>
  );
}
