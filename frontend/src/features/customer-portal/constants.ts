import type {
  CustomerRequestStatus,
  CustomerRequestType,
} from "@/features/customer-portal/types";

type Tone = "default" | "success" | "warning" | "danger";

/** Live resource of customer requests (`tenant.reservations.read`). */
export const CUSTOMER_REQUESTS_RESOURCE = "tenant.customer_requests" as const;

export const REQUEST_TYPES: CustomerRequestType[] = ["change", "cancel"];
export const REQUEST_STATUSES: CustomerRequestStatus[] = [
  "open",
  "resolved",
  "rejected",
];

export const REQUEST_TYPE_LABEL_KEYS: Record<CustomerRequestType, string> = {
  change: "customer_portal.request_types.change",
  cancel: "customer_portal.request_types.cancel",
};

export const REQUEST_STATUS_LABEL_KEYS: Record<CustomerRequestStatus, string> =
  {
    open: "customer_portal.request_statuses.open",
    resolved: "customer_portal.request_statuses.resolved",
    rejected: "customer_portal.request_statuses.rejected",
  };

export const REQUEST_STATUS_TONES: Record<CustomerRequestStatus, Tone> = {
  open: "warning",
  resolved: "success",
  rejected: "danger",
};

/** Portal paths (the M5 branded site mounts the same pages). */
export const portalRoutes = {
  home: (orgSlug: string) => `/r/${orgSlug}`,
  reservation: (orgSlug: string, uuid: string) =>
    `/r/${orgSlug}/reservations/${uuid}`,
};
