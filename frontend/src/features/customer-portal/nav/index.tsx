"use client";

import { MessageSquareText } from "lucide-react";
import { useCallback } from "react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CUSTOMER_REQUESTS_RESOURCE } from "@/features/customer-portal/constants";
import { useCustomerRequestStats } from "@/features/customer-portal/hooks/use-customer-requests";
import {
  createNavAdornment,
  defineNavItem,
  type NavAdornment,
} from "@/features/nav-engine";
import { usePermission } from "@/providers/permission-provider";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import { useChannel } from "@/hooks/use-realtime";
import { useLocale } from "@/providers/locale-provider";

/** Open customer requests: count badge, refreshed live from the resource channel. */
function useCustomerRequestsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const { organization } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.reservations.read);
  const stats = useCustomerRequestStats(allowed);
  const refetch = stats.refetch;
  const onPublication = useCallback(() => {
    void refetch();
  }, [refetch]);
  useChannel(
    allowed && organization?.uuid
      ? tenantResourceChannel(organization.uuid, CUSTOMER_REQUESTS_RESOURCE)
      : null,
    onPublication,
  );
  const open = stats.data?.open ?? 0;
  return {
    badges: [{ kind: "count", value: open, variant: "warning" }],
    info:
      open > 0
        ? {
            title: t("customer_portal.staff.title"),
            rows: [
              {
                label: t("customer_portal.request_statuses.open"),
                value: open,
                tone: "warning",
              },
            ],
          }
        : null,
  };
}

export const CustomerRequestsNavAdornment = createNavAdornment(
  useCustomerRequestsNavAdornment,
);

export function customerRequestsNavItem(slug: string) {
  return defineNavItem({
    id: "reservations-customer-requests",
    titleKey: "layout.nav_customer_requests",
    href: routes.tenant.reservations.customerRequests(slug),
    icon: MessageSquareText,
    permission: permissions.tenant.reservations.read,
    Adornment: CustomerRequestsNavAdornment,
  });
}
