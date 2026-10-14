import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type VehicleDocument = Schemas["VehicleDocument"];
export type VehicleDocumentList = Schemas["VehicleDocumentList"];
export type VehicleDocumentCreateRequest =
  Schemas["VehicleDocumentCreateRequest"];
export type VehicleDocumentPatchRequest =
  Schemas["VehicleDocumentPatchRequest"];
export type VehicleDocumentType = VehicleDocument["doc_type"];
export type VehicleDocumentStatus = VehicleDocument["status"];

export type VehicleMaintenance = Schemas["VehicleMaintenance"];
export type VehicleMaintenanceList = Schemas["VehicleMaintenanceList"];
export type VehicleMaintenanceCreateRequest =
  Schemas["VehicleMaintenanceCreateRequest"];
export type VehicleMaintenancePatchRequest =
  Schemas["VehicleMaintenancePatchRequest"];
export type VehicleMaintenanceCompleteRequest =
  Schemas["VehicleMaintenanceCompleteRequest"];
export type MaintenanceType = VehicleMaintenance["maintenance_type"];
export type MaintenanceStatus = VehicleMaintenance["status"];

export type FleetAlert = Schemas["FleetAlert"];
export type FleetAlerts = Schemas["FleetAlerts"];
