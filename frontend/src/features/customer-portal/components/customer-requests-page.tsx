"use client";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CustomerRequestsTable } from "@/features/customer-portal/components/customer-requests-table";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/** Customer requests of every reservation in the member's branch scope. */
export function CustomerRequestsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("customer_portal.staff.title")}
      description={t("customer_portal.staff.description")}
      permission={permissions.tenant.reservations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customer_portal.staff.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("customer_portal.staff.title") },
      ]}
    >
      <CustomerRequestsTable persistKey="tenant-customer-requests-v1" />
    </EntityPage>
  );
}
