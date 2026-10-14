"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useTenant } from "@/features/tenant/components/tenant-provider";
import { vehicleRecordsService } from "@/features/vehicle-records/services/vehicle-records.service";
import type {
  VehicleDocumentCreateRequest,
  VehicleDocumentPatchRequest,
  VehicleMaintenanceCompleteRequest,
  VehicleMaintenanceCreateRequest,
  VehicleMaintenancePatchRequest,
} from "@/features/vehicle-records/types";
import { vehicleKeys } from "@/features/vehicles/hooks/use-vehicles";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys live under the vehicle so a vehicle refresh covers them. */
export const vehicleRecordKeys = {
  documents: (slug: string, uuid: string) =>
    [...vehicleKeys.detail(slug, uuid), "documents"] as const,
  maintenance: (slug: string, uuid: string) =>
    [...vehicleKeys.detail(slug, uuid), "maintenance"] as const,
  alerts: (slug: string) => ["tenant", slug, "fleet-alerts"] as const,
};

/**
 * Documents and maintenance change the vehicle's expiry columns and the
 * alerts: refresh every vehicle query of the organization and the alerts.
 */
function useInvalidateRecords() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: vehicleKeys.all(slug) }),
      queryClient.invalidateQueries({
        queryKey: vehicleRecordKeys.alerts(slug),
      }),
    ]);
}

export function useVehicleDocuments(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleRecordKeys.documents(slug, uuid),
    queryFn: () => vehicleRecordsService.documents(slug, uuid),
  });
}

export function useVehicleMaintenance(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleRecordKeys.maintenance(slug, uuid),
    queryFn: () => vehicleRecordsService.maintenance(slug, uuid),
  });
}

export function useFleetAlerts() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: vehicleRecordKeys.alerts(slug),
    queryFn: () => vehicleRecordsService.alerts(slug),
  });
}

/**
 * Creates or updates a document, then uploads its file when one is given
 * (two calls: metadata first, so a failed upload keeps the record and can
 * be retried from the row actions).
 */
export function useSaveVehicleDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRecords();
  return useMutation({
    mutationFn: async ({
      documentUuid,
      body,
      file,
    }: {
      documentUuid?: string;
      body: VehicleDocumentCreateRequest | VehicleDocumentPatchRequest;
      file?: File | null;
    }) => {
      const saved = documentUuid
        ? await vehicleRecordsService.updateDocument(
            slug,
            uuid,
            documentUuid,
            body,
          )
        : await vehicleRecordsService.createDocument(
            slug,
            uuid,
            body as VehicleDocumentCreateRequest,
          );
      if (!file) return saved;
      try {
        return await vehicleRecordsService.uploadDocumentFile(
          slug,
          uuid,
          saved.uuid,
          file,
        );
      } catch (error) {
        void invalidate();
        throw error;
      }
    },
    onSuccess: (_, vars) => {
      appToast.success(
        t(
          vars.documentUuid
            ? "vehicle_records.toast.document_updated"
            : "vehicle_records.toast.document_created",
        ),
      );
      void invalidate();
    },
  });
}

export function useUploadVehicleDocumentFile(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRecords();
  return useMutation({
    mutationFn: ({
      documentUuid,
      file,
    }: {
      documentUuid: string;
      file: File;
    }) =>
      vehicleRecordsService.uploadDocumentFile(slug, uuid, documentUuid, file),
    onSuccess: () => {
      appToast.success(t("vehicle_records.toast.file_uploaded"));
      void invalidate();
    },
  });
}

export function useDeleteVehicleDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRecords();
  return useMutation({
    mutationFn: (documentUuid: string) =>
      vehicleRecordsService.deleteDocument(slug, uuid, documentUuid),
    onSuccess: () => {
      appToast.success(t("vehicle_records.toast.document_deleted"));
      void invalidate();
    },
  });
}

/** Streams a document file through the API and saves it. */
export function useDownloadVehicleDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  return useMutation({
    mutationFn: async ({
      documentUuid,
      fallbackName,
    }: {
      documentUuid: string;
      fallbackName: string;
    }) => {
      const { blob, filename } =
        await vehicleRecordsService.downloadDocumentFile(
          slug,
          uuid,
          documentUuid,
        );
      const href = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = href;
      link.download = filename ?? fallbackName;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(href);
    },
    onError: () => appToast.error(t("vehicle_records.toast.download_failed")),
  });
}

export function useSaveVehicleMaintenance(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRecords();
  return useMutation({
    mutationFn: ({
      itemUuid,
      body,
    }: {
      itemUuid?: string;
      body: VehicleMaintenanceCreateRequest | VehicleMaintenancePatchRequest;
    }) =>
      itemUuid
        ? vehicleRecordsService.updateMaintenance(slug, uuid, itemUuid, body)
        : vehicleRecordsService.createMaintenance(
            slug,
            uuid,
            body as VehicleMaintenanceCreateRequest,
          ),
    onSuccess: (_, vars) => {
      appToast.success(
        t(
          vars.itemUuid
            ? "vehicle_records.toast.maintenance_updated"
            : "vehicle_records.toast.maintenance_created",
        ),
      );
      void invalidate();
    },
  });
}

export function useCompleteVehicleMaintenance(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRecords();
  return useMutation({
    mutationFn: ({
      itemUuid,
      body,
    }: {
      itemUuid: string;
      body: VehicleMaintenanceCompleteRequest;
    }) => vehicleRecordsService.completeMaintenance(slug, uuid, itemUuid, body),
    onSuccess: () => {
      appToast.success(t("vehicle_records.toast.maintenance_completed"));
      void invalidate();
    },
  });
}

export function useDeleteVehicleMaintenance(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateRecords();
  return useMutation({
    mutationFn: (itemUuid: string) =>
      vehicleRecordsService.deleteMaintenance(slug, uuid, itemUuid),
    onSuccess: () => {
      appToast.success(t("vehicle_records.toast.maintenance_deleted"));
      void invalidate();
    },
  });
}
