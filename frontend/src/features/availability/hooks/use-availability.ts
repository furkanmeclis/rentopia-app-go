"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { availabilityService } from "@/features/availability/services/availability.service";
import { fleetCalendarKeys } from "@/features/fleet-calendar/hooks/use-fleet-calendar";
import type {
  AvailabilityHoldRequest,
  AvailabilitySearchRequest,
  CapacityRuleRequest,
  ListCapacityRulesParams,
  ListHoldsParams,
} from "@/features/availability/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: two organizations never share cache. */
export const availabilityKeys = {
  all: (slug: string) => ["tenant", slug, "availability"] as const,
  search: (slug: string, body: AvailabilitySearchRequest | null) =>
    [...availabilityKeys.all(slug), "search", body] as const,
  holds: (slug: string) => [...availabilityKeys.all(slug), "holds"] as const,
  holdList: (slug: string, params: ListHoldsParams) =>
    [...availabilityKeys.holds(slug), params] as const,
  rules: (slug: string) => [...availabilityKeys.all(slug), "rules"] as const,
  ruleList: (slug: string, params: ListCapacityRulesParams) =>
    [...availabilityKeys.rules(slug), params] as const,
};

/** Capacity per product; runs when a request is set (a search consumes nothing). */
export function useAvailabilitySearch(body: AvailabilitySearchRequest | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: availabilityKeys.search(slug, body),
    queryFn: () => availabilityService.search(slug, body!),
    enabled: body !== null,
    placeholderData: (previous) => previous,
  });
}

export function useHoldsList(params: ListHoldsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: availabilityKeys.holdList(slug, params),
    queryFn: () => availabilityService.holds(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

/** Holds change capacity: refresh searches, hold lists and the calendar. */
function useInvalidateAvailability() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: availabilityKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: fleetCalendarKeys.all(slug) }),
    ]);
}

/** Errors are not toasted here: the hold dialog renders them. */
export function useCreateHold() {
  const { slug } = useTenant();
  const invalidate = useInvalidateAvailability();
  return useMutation({
    mutationFn: (body: AvailabilityHoldRequest) =>
      availabilityService.createHold(slug, body),
    onSuccess: () => void invalidate(),
  });
}

export function useReleaseHold() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateAvailability();
  return useMutation({
    mutationFn: (uuid: string) => availabilityService.releaseHold(slug, uuid),
    onSuccess: (result) => {
      appToast.success(t("availability.toast.released"));
      if (result.affected.length > 0) {
        appToast.warning(
          t("availability.toast.release_affected", {
            count: result.affected.length,
          }),
        );
      }
      void invalidate();
    },
  });
}

export function useExtendHold() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateAvailability();
  return useMutation({
    mutationFn: ({ uuid, minutes }: { uuid: string; minutes: number }) =>
      availabilityService.extendHold(slug, uuid, minutes),
    onSuccess: () => {
      appToast.success(t("availability.toast.extended"));
      void invalidate();
    },
  });
}

// ------------------------------------------------------------ capacity rules

export function useCapacityRulesList(params: ListCapacityRulesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: availabilityKeys.ruleList(slug, params),
    queryFn: () => availabilityService.rules(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useSaveCapacityRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateAvailability();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string | null;
      body: CapacityRuleRequest;
    }) =>
      uuid
        ? availabilityService.updateRule(slug, uuid, body)
        : availabilityService.createRule(slug, body),
    onSuccess: (_, variables) => {
      appToast.success(
        variables.uuid
          ? t("availability.rules.toast.updated")
          : t("availability.rules.toast.created"),
      );
      void invalidate();
    },
  });
}

export function useDeleteCapacityRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateAvailability();
  return useMutation({
    mutationFn: (uuid: string) => availabilityService.removeRule(slug, uuid),
    onSuccess: () => {
      appToast.success(t("availability.rules.toast.deleted"));
      void invalidate();
    },
  });
}
