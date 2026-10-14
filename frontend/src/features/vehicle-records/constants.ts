import type {
  MaintenanceStatus,
  MaintenanceType,
  VehicleDocumentStatus,
  VehicleDocumentType,
} from "@/features/vehicle-records/types";

/** Export-only io resources (organization and branch scoped). */
export const DOCUMENTS_RESOURCE = "tenant.vehicle_documents" as const;
export const MAINTENANCE_RESOURCE = "tenant.vehicle_maintenance" as const;

/** Upload limit enforced by the API (bytes sniffed server-side). */
export const MAX_DOCUMENT_FILE_BYTES = 10 * 1024 * 1024;
export const DOCUMENT_FILE_ACCEPT =
  "application/pdf,image/jpeg,image/png,image/webp,.pdf,.jpg,.jpeg,.png,.webp";

export const DOCUMENT_TYPES: VehicleDocumentType[] = [
  "ruhsat",
  "trafik_sigortasi",
  "kasko",
  "muayene",
  "egzoz",
  "diger",
];

export const MAINTENANCE_TYPES: MaintenanceType[] = [
  "periodic",
  "tire",
  "brake",
  "oil",
  "repair",
  "other",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const DOCUMENT_TYPE_LABEL_KEYS: Record<VehicleDocumentType, string> = {
  ruhsat: "vehicle_records.doc_type.ruhsat",
  trafik_sigortasi: "vehicle_records.doc_type.trafik_sigortasi",
  kasko: "vehicle_records.doc_type.kasko",
  muayene: "vehicle_records.doc_type.muayene",
  egzoz: "vehicle_records.doc_type.egzoz",
  diger: "vehicle_records.doc_type.diger",
};

export const DOCUMENT_STATUSES: VehicleDocumentStatus[] = [
  "expired",
  "expiring",
  "valid",
  "none",
];

export const DOCUMENT_STATUS_LABEL_KEYS: Record<VehicleDocumentStatus, string> =
  {
    none: "vehicle_records.doc_status.none",
    valid: "vehicle_records.doc_status.valid",
    expiring: "vehicle_records.doc_status.expiring",
    expired: "vehicle_records.doc_status.expired",
  };

export const DOCUMENT_STATUS_TONES: Record<
  VehicleDocumentStatus,
  "default" | "success" | "warning" | "danger"
> = {
  none: "default",
  valid: "success",
  expiring: "warning",
  expired: "danger",
};

export const MAINTENANCE_TYPE_LABEL_KEYS: Record<MaintenanceType, string> = {
  periodic: "vehicle_records.maintenance_type.periodic",
  tire: "vehicle_records.maintenance_type.tire",
  brake: "vehicle_records.maintenance_type.brake",
  oil: "vehicle_records.maintenance_type.oil",
  repair: "vehicle_records.maintenance_type.repair",
  other: "vehicle_records.maintenance_type.other",
};

export const MAINTENANCE_STATUSES: MaintenanceStatus[] = [
  "overdue",
  "due",
  "scheduled",
  "done",
];

export const MAINTENANCE_STATUS_LABEL_KEYS: Record<MaintenanceStatus, string> =
  {
    done: "vehicle_records.maintenance_status.done",
    scheduled: "vehicle_records.maintenance_status.scheduled",
    due: "vehicle_records.maintenance_status.due",
    overdue: "vehicle_records.maintenance_status.overdue",
  };

export const MAINTENANCE_STATUS_TONES: Record<
  MaintenanceStatus,
  "default" | "success" | "warning" | "danger"
> = {
  done: "success",
  scheduled: "default",
  due: "warning",
  overdue: "danger",
};

/** Vehicle-level maintenance state (Vehicle.maintenance_status). */
export const VEHICLE_MAINTENANCE_LABEL_KEYS: Record<
  "none" | "scheduled" | "due" | "overdue",
  string
> = {
  none: "vehicle_records.vehicle_maintenance.none",
  scheduled: "vehicle_records.maintenance_status.scheduled",
  due: "vehicle_records.maintenance_status.due",
  overdue: "vehicle_records.maintenance_status.overdue",
};

export const ALERT_KIND_LABEL_KEYS: Record<FleetAlertKind, string> = {
  document: "vehicle_records.alerts.kind.document",
  maintenance: "vehicle_records.alerts.kind.maintenance",
};

export const ALERT_STATE_LABEL_KEYS: Record<FleetAlertState, string> = {
  expiring: "vehicle_records.alerts.state.expiring",
  overdue: "vehicle_records.alerts.state.overdue",
};

export type FleetAlertKind = "document" | "maintenance";
export type FleetAlertState = "expiring" | "overdue";
