"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import type { CalendarAction } from "@/features/fleet-calendar/components/bar-details";
import type { FleetCalendarInterval } from "@/features/fleet-calendar/types";
import { ReleaseBlockDialog } from "@/features/fleet-ops/components/block-dialogs";
import { TransferActionDialog } from "@/features/fleet-ops/components/transfer-dialogs";
import { fleetOpsKeys } from "@/features/fleet-ops/hooks/use-fleet-ops";
import { fleetOpsService } from "@/features/fleet-ops/services/fleet-ops.service";
import type {
  Transfer,
  TransferAction,
  VehicleBlock,
} from "@/features/fleet-ops/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * The block of a calendar bar in the shape `ReleaseBlockDialog` takes (it
 * reads the uuid and the kind; the rest mirrors what the bar knows).
 */
function blockFromBar(
  vehicleUuid: string,
  interval: FleetCalendarInterval,
): VehicleBlock | null {
  if (!interval.block) return null;
  return {
    uuid: interval.uuid,
    vehicle_uuid: vehicleUuid,
    kind: interval.block.kind,
    starts_at: interval.starts_at,
    ends_at: interval.ends_at,
    state: "active",
    reason: interval.block.reason,
    created_by: null,
    released_at: interval.block.released_at,
    released_by: null,
    created_at: interval.starts_at,
    updated_at: interval.starts_at,
  };
}

/**
 * Popover actions of the calendar bars, run through the existing fleet-ops
 * dialogs ("Engeli kapat", transfer start / complete / cancel). The dialogs
 * invalidate the fleet-ops cache, which holds the calendar windows too.
 */
export function useCalendarActions() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [release, setRelease] = useState<{
    vehicleUuid: string;
    block: VehicleBlock;
  } | null>(null);
  const [transferTarget, setTransferTarget] = useState<{
    transfer: Transfer;
    action: TransferAction;
  } | null>(null);

  const onAction = useCallback(
    async (action: CalendarAction) => {
      if (action.type === "release-block") {
        const block = blockFromBar(action.vehicle.uuid, action.interval);
        if (block) setRelease({ vehicleUuid: action.vehicle.uuid, block });
        return;
      }
      try {
        const transfer = await queryClient.fetchQuery({
          queryKey: [
            ...fleetOpsKeys.transfers(slug),
            "detail",
            action.transferUuid,
          ],
          queryFn: () => fleetOpsService.transfer(slug, action.transferUuid),
          staleTime: 0,
        });
        setTransferTarget({ transfer, action: action.action });
      } catch {
        appToast.error(t("fleet_calendar.transfer_load_failed"));
      }
    },
    [queryClient, slug, t],
  );

  const dialogs = (
    <>
      <ReleaseBlockDialog
        vehicleUuid={release?.vehicleUuid ?? ""}
        block={release?.block ?? null}
        onOpenChange={(open) => {
          if (!open) setRelease(null);
        }}
      />
      <TransferActionDialog
        target={transferTarget}
        onOpenChange={(open) => {
          if (!open) setTransferTarget(null);
        }}
      />
    </>
  );

  return {
    onAction: (action: CalendarAction) => void onAction(action),
    dialogs,
  };
}
