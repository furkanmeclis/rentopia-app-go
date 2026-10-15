"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";

import { portalRoutes } from "@/features/customer-portal/constants";
import { portalService } from "@/features/customer-portal/services/portal.service";
import { ApiError } from "@/lib/api";

export const portalKeys = {
  all: (orgSlug: string) => ["customer-portal", orgSlug] as const,
  info: (orgSlug: string) => [...portalKeys.all(orgSlug), "info"] as const,
  session: (orgSlug: string) =>
    [...portalKeys.all(orgSlug), "session"] as const,
  reservations: (orgSlug: string) =>
    [...portalKeys.all(orgSlug), "reservations"] as const,
  reservation: (orgSlug: string, uuid: string) =>
    [...portalKeys.all(orgSlug), "reservation", uuid] as const,
};

/** 401 = not signed in (or the session ended). */
export function isSignedOut(error: unknown) {
  return error instanceof ApiError && error.status === 401;
}

const noRetryOn401 = (count: number, error: unknown) =>
  !isSignedOut(error) && count < 2;

export function usePortalInfo(orgSlug: string) {
  return useQuery({
    queryKey: portalKeys.info(orgSlug),
    queryFn: () => portalService.info(orgSlug),
    retry: false,
    staleTime: 5 * 60_000,
  });
}

export function usePortalSession(orgSlug: string) {
  return useQuery({
    queryKey: portalKeys.session(orgSlug),
    queryFn: () => portalService.session(orgSlug),
    retry: noRetryOn401,
  });
}

export function usePortalReservations(orgSlug: string, enabled: boolean) {
  return useQuery({
    queryKey: portalKeys.reservations(orgSlug),
    queryFn: () => portalService.reservations(orgSlug),
    retry: noRetryOn401,
    enabled,
  });
}

export function usePortalReservation(orgSlug: string, uuid: string) {
  return useQuery({
    queryKey: portalKeys.reservation(orgSlug, uuid),
    queryFn: () => portalService.reservation(orgSlug, uuid),
    retry: noRetryOn401,
  });
}

export function usePortalSignOut(orgSlug: string) {
  const client = useQueryClient();
  const router = useRouter();
  return useMutation({
    mutationFn: () => portalService.logout(orgSlug),
    onSettled: () => {
      client.removeQueries({ queryKey: portalKeys.all(orgSlug) });
      router.replace(portalRoutes.home(orgSlug));
      void client.invalidateQueries({ queryKey: portalKeys.session(orgSlug) });
    },
  });
}
