"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { fleetKeys } from "@/features/fleet/hooks/query-keys";
import { fleetService } from "@/features/fleet/services/fleet.service";
import type {
  FleetResourceKind,
  ListGroupsParams,
  ListProductsParams,
  ListSegmentsParams,
  RentalGroupCreateRequest,
  RentalGroupPatchRequest,
  RentalProductCreateRequest,
  RentalProductPatchRequest,
  SegmentCreateRequest,
  SegmentPatchRequest,
} from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const META_STALE = 5 * 60_000;

/** Fleet resources reference each other (counts, facets): refresh them together. */
function useInvalidateFleet() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: fleetKeys.all(slug) });
}

// --- segments ---------------------------------------------------------------

export function useSegmentsList(params: ListSegmentsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.list(slug, "segments", params),
    queryFn: () => fleetService.listSegments(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useSegmentsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.meta(slug, "segments"),
    queryFn: () => fleetService.segmentsMeta(slug),
    staleTime: META_STALE,
  });
}

export function useSegmentFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.facets(slug, "segments"),
    queryFn: () => fleetService.segmentFacets(slug),
    staleTime: 60_000,
  });
}

/** Active segments for selects (first 100 by sort order). */
export function useSegmentOptions() {
  return useSegmentsList({ limit: 100, offset: 0, sort: "sort_order" });
}

export function useSegment(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.detail(slug, "segments", uuid ?? ""),
    queryFn: () => fleetService.getSegment(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useCreateSegment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: (body: SegmentCreateRequest) =>
      fleetService.createSegment(slug, body),
    onSuccess: () => {
      appToast.success(t("fleet.segments.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateSegment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: SegmentPatchRequest }) =>
      fleetService.updateSegment(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("fleet.segments.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteSegment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: (uuid: string) => fleetService.removeSegment(slug, uuid),
    onSuccess: () => {
      appToast.success(t("fleet.segments.toast.deleted"));
      void invalidate();
    },
  });
}

// --- rental groups ----------------------------------------------------------

export function useGroupsList(params: ListGroupsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.list(slug, "groups", params),
    queryFn: () => fleetService.listGroups(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useGroupsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.meta(slug, "groups"),
    queryFn: () => fleetService.groupsMeta(slug),
    staleTime: META_STALE,
  });
}

export function useGroupFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.facets(slug, "groups"),
    queryFn: () => fleetService.groupFacets(slug),
    staleTime: 60_000,
  });
}

export function useGroup(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.detail(slug, "groups", uuid ?? ""),
    queryFn: () => fleetService.getGroup(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useCreateGroup() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: (body: RentalGroupCreateRequest) =>
      fleetService.createGroup(slug, body),
    onSuccess: () => {
      appToast.success(t("fleet.groups.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateGroup() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: RentalGroupPatchRequest;
    }) => fleetService.updateGroup(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("fleet.groups.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteGroup() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: (uuid: string) => fleetService.removeGroup(slug, uuid),
    onSuccess: () => {
      appToast.success(t("fleet.groups.toast.deleted"));
      void invalidate();
    },
  });
}

// --- rental products --------------------------------------------------------

export function useProductsList(params: ListProductsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.list(slug, "products", params),
    queryFn: () => fleetService.listProducts(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useProductsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.meta(slug, "products"),
    queryFn: () => fleetService.productsMeta(slug),
    staleTime: META_STALE,
  });
}

export function useProductFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.facets(slug, "products"),
    queryFn: () => fleetService.productFacets(slug),
    staleTime: 60_000,
  });
}

export function useProduct(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.detail(slug, "products", uuid ?? ""),
    queryFn: () => fleetService.getProduct(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useProductVersions(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...fleetKeys.versions(slug, uuid), params],
    queryFn: () => fleetService.listVersions(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useProductVersion(uuid: string, versionUuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetKeys.version(slug, uuid, versionUuid ?? ""),
    queryFn: () => fleetService.getVersion(slug, uuid, versionUuid!),
    enabled: Boolean(versionUuid),
  });
}

export function useCreateProduct() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: (body: RentalProductCreateRequest) =>
      fleetService.createProduct(slug, body),
    onSuccess: () => {
      appToast.success(t("fleet.products.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateProduct() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: RentalProductPatchRequest;
      /** Current version before the edit (to announce a new version). */
      previousVersion?: number;
    }) => fleetService.updateProduct(slug, uuid, body),
    onSuccess: (product, variables) => {
      const versioned =
        variables.previousVersion !== undefined &&
        product.current_version > variables.previousVersion;
      appToast.success(
        versioned
          ? t("fleet.products.toast.version_created", {
              version: product.current_version,
            })
          : t("fleet.products.toast.updated"),
      );
      void invalidate();
    },
  });
}

export function useDeleteProduct() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  return useMutation({
    mutationFn: (uuid: string) => fleetService.removeProduct(slug, uuid),
    onSuccess: () => {
      appToast.success(t("fleet.products.toast.deleted"));
      void invalidate();
    },
  });
}

export function useProductImages(productUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateFleet();
  const upload = useMutation({
    mutationFn: (file: File) =>
      fleetService.uploadImage(slug, productUuid, file),
    onSuccess: () => void invalidate(),
  });
  const reorder = useMutation({
    mutationFn: (imageUuids: string[]) =>
      fleetService.reorderImages(slug, productUuid, imageUuids),
    onSuccess: () => {
      appToast.success(t("fleet.images.toast.reordered"));
      void invalidate();
    },
  });
  const remove = useMutation({
    mutationFn: (imageUuid: string) =>
      fleetService.removeImage(slug, productUuid, imageUuid),
    onSuccess: () => {
      appToast.success(t("fleet.images.toast.deleted"));
      void invalidate();
    },
  });
  return { upload, reorder, remove };
}

// --- shared -----------------------------------------------------------------

export function useFleetActivity(
  kind: FleetResourceKind,
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...fleetKeys.activity(slug, kind, uuid), params],
    queryFn: () => fleetService.activity(slug, kind, uuid, params),
    placeholderData: (previous) => previous,
  });
}
