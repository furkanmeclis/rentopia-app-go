"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { fleetKeys } from "@/features/fleet/hooks/query-keys";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { vehiclesService } from "@/features/vehicles/services/vehicles.service";
import type {
  ListVehiclesParams,
  VehicleCreateRequest,
  VehiclePatchRequest,
  VehicleStatusRequest,
} from "@/features/vehicles/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: two organizations never share cache. */
export const vehicleKeys = {
  all: (slug: string) => ["tenant", slug, "vehicles"] as const,
  lists: (slug: string) => [...vehicleKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...vehicleKeys.lists(slug), params] as const,
  meta: (slug: string) => [...vehicleKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...vehicleKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...vehicleKeys.all(slug), "detail", uuid] as const,
  events: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...vehicleKeys.detail(slug, uuid), "events", params] as const,
  groups: (slug: string, uuid: string) =>
    [...vehicleKeys.detail(slug, uuid), "groups"] as const,
  kasko: (slug: string, uuid: string) =>
    [...vehicleKeys.detail(slug, uuid), "kasko"] as const,
  activity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...vehicleKeys.detail(slug, uuid), "activity", params] as const,
  product: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...vehicleKeys.all(slug), "product", uuid, params] as const,
};

/**
 * Vehicles change list rows, facets and group / product counts: refresh the
 * vehicles cache and the fleet cache (groups and products show vehicles).
 */
function useInvalidateVehicles() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: vehicleKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: fleetKeys.all(slug) }),
    ]);
}

export function useVehiclesList(params: ListVehiclesParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.list(slug, params),
    queryFn: () => vehiclesService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useVehiclesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.meta(slug),
    queryFn: () => vehiclesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useVehicleFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.facets(slug),
    queryFn: () => vehiclesService.facets(slug),
    staleTime: 60_000,
  });
}

export function useVehicle(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.detail(slug, uuid ?? ""),
    queryFn: () => vehiclesService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useVehicleEvents(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.events(slug, uuid, params),
    queryFn: () => vehiclesService.events(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useVehicleGroups(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.groups(slug, uuid),
    queryFn: () => vehiclesService.groups(slug, uuid),
  });
}

export function useVehicleKasko(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.kasko(slug, uuid),
    queryFn: () => vehiclesService.kasko(slug, uuid),
  });
}

export function useVehicleActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.activity(slug, uuid, params),
    queryFn: () => vehiclesService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useProductVehicles(
  productUuid: string,
  params: { limit: number; offset: number },
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleKeys.product(slug, productUuid, params),
    queryFn: () => vehiclesService.productVehicles(slug, productUuid, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCreateVehicle() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateVehicles();
  return useMutation({
    mutationFn: (body: VehicleCreateRequest) =>
      vehiclesService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("vehicles.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateVehicle() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateVehicles();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: VehiclePatchRequest }) =>
      vehiclesService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("vehicles.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteVehicle() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateVehicles();
  return useMutation({
    mutationFn: (uuid: string) => vehiclesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("vehicles.toast.deleted"));
      void invalidate();
    },
  });
}

export function useChangeVehicleStatus() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateVehicles();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: VehicleStatusRequest;
    }) => vehiclesService.changeStatus(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("vehicles.toast.status_changed"));
      void invalidate();
    },
  });
}

export function useAddVehicleGroup() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateVehicles();
  return useMutation({
    mutationFn: ({ uuid, groupUuid }: { uuid: string; groupUuid: string }) =>
      vehiclesService.addGroup(slug, uuid, groupUuid),
    onSuccess: () => {
      appToast.success(t("vehicles.toast.group_added"));
      void invalidate();
    },
  });
}

export function useRemoveVehicleGroup() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateVehicles();
  return useMutation({
    mutationFn: ({ uuid, groupUuid }: { uuid: string; groupUuid: string }) =>
      vehiclesService.removeGroup(slug, uuid, groupUuid),
    onSuccess: () => {
      appToast.success(t("vehicles.toast.group_removed"));
      void invalidate();
    },
  });
}
