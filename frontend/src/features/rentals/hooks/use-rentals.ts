"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { availabilityKeys } from "@/features/availability/hooks/use-availability";
import { fleetCalendarKeys } from "@/features/fleet-calendar/hooks/use-fleet-calendar";
import { rentalsService } from "@/features/rentals/services/rentals.service";
import type {
  ListOperationTasksParams,
  ListRentalsParams,
  RentalChargeRequest,
  RentalCompleteReturnRequest,
  RentalCreateRequest,
  RentalExtendRequest,
  RentalHandoverRequest,
  RentalReturnRequest,
  RentalSwapRequest,
} from "@/features/rentals/types";
import { reservationKeys } from "@/features/reservations/hooks/use-reservations";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const rentalKeys = {
  all: (slug: string) => ["tenant", slug, "rentals"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...rentalKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...rentalKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...rentalKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...rentalKeys.all(slug), "detail", uuid] as const,
  events: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...rentalKeys.all(slug), "events", uuid, params] as const,
  tasks: (slug: string, params: Record<string, unknown>) =>
    [...rentalKeys.all(slug), "tasks", params] as const,
  taskFacets: (slug: string) =>
    [...rentalKeys.all(slug), "task-facets"] as const,
};

/**
 * Rental actions move capacity and vehicles: refresh rentals, tasks,
 * reservations, availability and the fleet calendar (documents too).
 */
export function useInvalidateRentals() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: rentalKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: reservationKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: availabilityKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: fleetCalendarKeys.all(slug) }),
      queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "documents"],
      }),
    ]);
}

export function useRentalsList(params: ListRentalsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.list(slug, params),
    queryFn: () => rentalsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useRentalsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.meta(slug),
    queryFn: () => rentalsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useRentalFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.facets(slug),
    queryFn: () => rentalsService.facets(slug),
    staleTime: 60_000,
  });
}

export function useRental(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.detail(slug, uuid),
    queryFn: () => rentalsService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useRentalEvents(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.events(slug, uuid, params),
    queryFn: () => rentalsService.events(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useCreateRental() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRentals();
  return useMutation({
    mutationFn: (body: RentalCreateRequest) =>
      rentalsService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("rentals.toast.created"));
      void invalidate();
    },
  });
}

/** One mutation per rental action; dialogs render the errors (silent). */
function useRentalAction<T>(
  uuid: string,
  fn: (slug: string, uuid: string, body: T) => Promise<unknown>,
  toastKey: string,
  silent = true,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRentals();
  return useMutation({
    mutationFn: (body: T) => fn(slug, uuid, body),
    meta: silent ? { silent: true } : undefined,
    onSuccess: () => {
      appToast.success(t(toastKey));
      void invalidate();
    },
  });
}

export function useHandover(uuid: string) {
  return useRentalAction<RentalHandoverRequest>(
    uuid,
    rentalsService.handover,
    "rentals.toast.handed_over",
  );
}

export function useReturnRental(uuid: string) {
  return useRentalAction<RentalReturnRequest>(
    uuid,
    rentalsService.returnVehicle,
    "rentals.toast.returned",
  );
}

export function useCompleteReturn(uuid: string) {
  return useRentalAction<RentalCompleteReturnRequest>(
    uuid,
    rentalsService.completeReturn,
    "rentals.toast.checked",
    false,
  );
}

export function useExtendRental(uuid: string) {
  return useRentalAction<RentalExtendRequest>(
    uuid,
    rentalsService.extend,
    "rentals.toast.extended",
  );
}

export function useSwapVehicle(uuid: string) {
  return useRentalAction<RentalSwapRequest>(
    uuid,
    rentalsService.swap,
    "rentals.toast.swapped",
  );
}

export function useCancelRental(uuid: string) {
  return useRentalAction<string>(
    uuid,
    rentalsService.cancel,
    "rentals.toast.cancelled",
    false,
  );
}

export function useAddCharge(uuid: string) {
  return useRentalAction<RentalChargeRequest>(
    uuid,
    rentalsService.addCharge,
    "rentals.toast.charge_proposed",
    false,
  );
}

export function useDecideCharge(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRentals();
  return useMutation({
    mutationFn: ({
      charge,
      decision,
      note,
    }: {
      charge: string;
      decision: "approve" | "waive";
      note: string;
    }) => rentalsService.decideCharge(slug, uuid, charge, decision, note),
    onSuccess: (_, v) => {
      appToast.success(
        t(
          v.decision === "approve"
            ? "rentals.toast.charge_approved"
            : "rentals.toast.charge_waived",
        ),
      );
      void invalidate();
    },
  });
}

// ------------------------------------------------------------ tasks

const TASK_TOAST_KEYS = {
  start: "rentals.tasks.toast.start",
  complete: "rentals.tasks.toast.complete",
  cancel: "rentals.tasks.toast.cancel",
} as const;

export function useOperationTasks(params: ListOperationTasksParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.tasks(slug, params),
    queryFn: () => rentalsService.tasks(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useOperationTaskFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: rentalKeys.taskFacets(slug),
    queryFn: () => rentalsService.taskFacets(slug),
    staleTime: 60_000,
  });
}

export function useTaskAction() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRentals();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
      note,
    }: {
      uuid: string;
      action: "start" | "complete" | "cancel";
      note?: string;
    }) => rentalsService.taskAction(slug, uuid, action, note),
    onSuccess: (_, v) => {
      appToast.success(t(TASK_TOAST_KEYS[v.action]));
      void invalidate();
    },
  });
}

export function useAssignOperationTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRentals();
  return useMutation({
    mutationFn: ({
      uuid,
      userUuid,
    }: {
      uuid: string;
      userUuid: string | null;
    }) => rentalsService.assignTask(slug, uuid, userUuid),
    onSuccess: () => {
      appToast.success(t("rentals.tasks.toast.assigned"));
      void invalidate();
    },
  });
}
