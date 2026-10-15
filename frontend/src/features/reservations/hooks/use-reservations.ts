"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { availabilityKeys } from "@/features/availability/hooks/use-availability";
import { fleetCalendarKeys } from "@/features/fleet-calendar/hooks/use-fleet-calendar";
import { reservationsService } from "@/features/reservations/services/reservations.service";
import type {
  ListQuotesParams,
  ListReservationsParams,
  ListTasksParams,
  QuoteRequest,
  QuoteSearchRequest,
  ReservationApplyChangeRequest,
  ReservationChangeQuoteRequest,
  ReservationCreateRequest,
  ReservationDriversRequest,
  ReservationPlateRequest,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const reservationKeys = {
  all: (slug: string) => ["tenant", slug, "reservations"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...reservationKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...reservationKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...reservationKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...reservationKeys.all(slug), "detail", uuid] as const,
  events: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...reservationKeys.all(slug), "events", uuid, params] as const,
  quotes: (slug: string, params: Record<string, unknown>) =>
    [...reservationKeys.all(slug), "quotes", params] as const,
  quotesMeta: (slug: string) =>
    [...reservationKeys.all(slug), "quotes-meta"] as const,
  quote: (slug: string, uuid: string) =>
    [...reservationKeys.all(slug), "quote", uuid] as const,
  search: (slug: string, body: QuoteSearchRequest | null) =>
    [...reservationKeys.all(slug), "search", body] as const,
  tasks: (slug: string, params: Record<string, unknown>) =>
    [...reservationKeys.all(slug), "tasks", params] as const,
  tasksMeta: (slug: string) =>
    [...reservationKeys.all(slug), "tasks-meta"] as const,
  candidates: (slug: string, uuid: string) =>
    [...reservationKeys.all(slug), "candidates", uuid] as const,
};

/**
 * Reservations move capacity: refresh reservations, quotes, tasks,
 * availability searches / holds and the fleet calendar.
 */
export function useInvalidateReservations() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: reservationKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: availabilityKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: fleetCalendarKeys.all(slug) }),
    ]);
}

export function useReservationsList(params: ListReservationsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.list(slug, params),
    queryFn: () => reservationsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useReservationsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.meta(slug),
    queryFn: () => reservationsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useReservationFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.facets(slug),
    queryFn: () => reservationsService.facets(slug),
    staleTime: 60_000,
  });
}

export function useReservation(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.detail(slug, uuid),
    queryFn: () => reservationsService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useReservationEvents(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.events(slug, uuid, params),
    queryFn: () => reservationsService.events(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

/** Vehicles the plan can move to (plate dialog). */
export function usePlateCandidates(uuid: string, enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.candidates(slug, uuid),
    queryFn: () => reservationsService.plateCandidates(slug, uuid),
    enabled,
    staleTime: 0,
  });
}

/** Product search with prices (consumes nothing). */
export function useQuoteSearch(body: QuoteSearchRequest | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.search(slug, body),
    queryFn: () => reservationsService.search(slug, body!),
    enabled: body !== null,
    placeholderData: (previous) => previous,
  });
}

export function useQuotesList(params: ListQuotesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.quotes(slug, params),
    queryFn: () => reservationsService.quotes(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useQuotesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.quotesMeta(slug),
    queryFn: () => reservationsService.quotesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useQuote(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.quote(slug, uuid ?? ""),
    queryFn: () => reservationsService.quote(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useTasksList(params: ListTasksParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.tasks(slug, params),
    queryFn: () => reservationsService.tasks(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useTasksMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reservationKeys.tasksMeta(slug),
    queryFn: () => reservationsService.tasksMeta(slug),
    staleTime: 5 * 60_000,
  });
}

// --------------------------------------------------------------- mutations

/** Errors are rendered by the wizard (price changed, drivers, capacity). */
export function useCreateQuote() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: QuoteRequest) =>
      reservationsService.createQuote(slug, body),
    meta: { silent: true },
  });
}

export function useCreateReservation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (body: ReservationCreateRequest) =>
      reservationsService.create(slug, body),
    meta: { silent: true },
    onSuccess: (reservation) => {
      appToast.success(
        t("reservations.toast.created", { reference: reservation.reference }),
      );
      void invalidate();
    },
  });
}

export type TransitionAction =
  "submit" | "approve" | "confirm" | "cancel" | "no-show";

const TRANSITION_TOAST_KEYS: Record<TransitionAction, string> = {
  submit: "reservations.toast.submitted",
  approve: "reservations.toast.approved",
  confirm: "reservations.toast.confirmed",
  cancel: "reservations.toast.cancelled",
  "no-show": "reservations.toast.no_show",
};

export function useReservationTransition() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
      reason,
    }: {
      uuid: string;
      action: TransitionAction;
      reason?: string;
    }) => reservationsService.transition(slug, uuid, action, reason),
    onSuccess: (_, variables) => {
      appToast.success(t(TRANSITION_TOAST_KEYS[variables.action]));
      void invalidate();
    },
  });
}

export function useChangeQuote(uuid: string) {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: ReservationChangeQuoteRequest) =>
      reservationsService.changeQuote(slug, uuid, body),
    meta: { silent: true },
  });
}

export function useApplyChange(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (body: ReservationApplyChangeRequest) =>
      reservationsService.applyChange(slug, uuid, body),
    meta: { silent: true },
    onSuccess: (result) => {
      appToast.success(t("reservations.toast.changed"));
      if (result.reassigned > 0) {
        appToast.info(
          t("reservations.toast.reassigned", { count: result.reassigned }),
        );
      }
      void invalidate();
    },
  });
}

export function useExtendReservation(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (body: { ends_at: string; reason?: string }) =>
      reservationsService.extend(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("reservations.toast.extended"));
      void invalidate();
    },
  });
}

export function useChangePlate() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: ReservationPlateRequest;
    }) => reservationsService.plate(slug, uuid, body),
    onSuccess: (result) => {
      appToast.success(
        t("reservations.toast.plate_changed", {
          reference: result.reservation.reference,
        }),
      );
      if (result.reassigned > 0) {
        appToast.info(
          t("reservations.toast.reassigned", { count: result.reassigned }),
        );
      }
      void invalidate();
    },
    onError: () => void invalidate(),
  });
}

export function useUpdateDrivers(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (body: ReservationDriversRequest) =>
      reservationsService.drivers(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("reservations.toast.drivers_updated"));
      void invalidate();
    },
  });
}

export function useAssignReservation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      userUuid,
    }: {
      uuid: string;
      userUuid: string | null;
    }) => reservationsService.assign(slug, uuid, userUuid),
    onSuccess: () => {
      appToast.success(t("reservations.toast.assigned"));
      void invalidate();
    },
  });
}

export function useResolveTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({ uuid, note }: { uuid: string; note: string }) =>
      reservationsService.resolveTask(slug, uuid, note),
    onSuccess: () => {
      appToast.success(t("reservations.tasks.toast.resolved"));
      void invalidate();
    },
  });
}

export function useAssignTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: ({
      uuid,
      userUuid,
    }: {
      uuid: string;
      userUuid: string | null;
    }) => reservationsService.assignTask(slug, uuid, userUuid),
    onSuccess: () => {
      appToast.success(t("reservations.tasks.toast.assigned"));
      void invalidate();
    },
  });
}

export function useReplanTask() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateReservations();
  return useMutation({
    mutationFn: (uuid: string) => reservationsService.replanTask(slug, uuid),
    onSuccess: (result) => {
      if (result.replanned) {
        appToast.success(t("reservations.tasks.toast.replanned"));
      } else {
        appToast.warning(t("reservations.tasks.toast.replan_failed"));
      }
      void invalidate();
    },
  });
}
