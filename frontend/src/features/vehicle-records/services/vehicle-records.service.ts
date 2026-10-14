import type {
  FleetAlerts,
  VehicleDocument,
  VehicleDocumentCreateRequest,
  VehicleDocumentList,
  VehicleDocumentPatchRequest,
  VehicleMaintenance,
  VehicleMaintenanceCompleteRequest,
  VehicleMaintenanceCreateRequest,
  VehicleMaintenanceList,
  VehicleMaintenancePatchRequest,
} from "@/features/vehicle-records/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const vehicle = (uuid: string) => `/v1/tenant/vehicles/${uuid}`;

/**
 * Vehicle documents, maintenance and fleet alerts of one organization
 * (`X-Organization: {slug}`). Document files are uploaded and streamed by
 * the API; the browser never sees a storage key or URL.
 */
export const vehicleRecordsService = {
  documents(slug: string, uuid: string) {
    return tenantRequest<VehicleDocumentList>(
      slug,
      "GET",
      `${vehicle(uuid)}/documents`,
    );
  },
  createDocument(
    slug: string,
    uuid: string,
    body: VehicleDocumentCreateRequest,
  ) {
    return tenantRequest<VehicleDocument>(
      slug,
      "POST",
      `${vehicle(uuid)}/documents`,
      { body },
    );
  },
  updateDocument(
    slug: string,
    uuid: string,
    documentUuid: string,
    body: VehicleDocumentPatchRequest,
  ) {
    return tenantRequest<VehicleDocument>(
      slug,
      "PATCH",
      `${vehicle(uuid)}/documents/${documentUuid}`,
      { body },
    );
  },
  deleteDocument(slug: string, uuid: string, documentUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${vehicle(uuid)}/documents/${documentUuid}`,
    );
  },
  uploadDocumentFile(
    slug: string,
    uuid: string,
    documentUuid: string,
    file: File,
  ) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<VehicleDocument>(
      slug,
      "PUT",
      `${vehicle(uuid)}/documents/${documentUuid}/file`,
      form,
    );
  },
  downloadDocumentFile(slug: string, uuid: string, documentUuid: string) {
    return tenantDownloadFile(
      slug,
      `${vehicle(uuid)}/documents/${documentUuid}/file`,
    );
  },
  maintenance(slug: string, uuid: string) {
    return tenantRequest<VehicleMaintenanceList>(
      slug,
      "GET",
      `${vehicle(uuid)}/maintenance`,
    );
  },
  createMaintenance(
    slug: string,
    uuid: string,
    body: VehicleMaintenanceCreateRequest,
  ) {
    return tenantRequest<VehicleMaintenance>(
      slug,
      "POST",
      `${vehicle(uuid)}/maintenance`,
      { body },
    );
  },
  updateMaintenance(
    slug: string,
    uuid: string,
    itemUuid: string,
    body: VehicleMaintenancePatchRequest,
  ) {
    return tenantRequest<VehicleMaintenance>(
      slug,
      "PATCH",
      `${vehicle(uuid)}/maintenance/${itemUuid}`,
      { body },
    );
  },
  completeMaintenance(
    slug: string,
    uuid: string,
    itemUuid: string,
    body: VehicleMaintenanceCompleteRequest,
  ) {
    return tenantRequest<VehicleMaintenance>(
      slug,
      "POST",
      `${vehicle(uuid)}/maintenance/${itemUuid}/complete`,
      { body },
    );
  },
  deleteMaintenance(slug: string, uuid: string, itemUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${vehicle(uuid)}/maintenance/${itemUuid}`,
    );
  },
  alerts(slug: string) {
    return tenantRequest<FleetAlerts>(slug, "GET", "/v1/tenant/fleet/alerts");
  },
};
