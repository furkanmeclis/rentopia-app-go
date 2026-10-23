"use client";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { RentalsTable } from "@/features/rentals/components/rentals-table";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Rentals ("Kiralamalar"): every rental whose pick-up, return or actual
 * return branch is in the member's branch scope. Facets, live updates,
 * export; rentals are created from a confirmed reservation.
 */
export function RentalsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();

  return (
    <EntityPage
      title={t("rentals.title")}
      description={t("rentals.description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("rentals.title") },
      ]}
    >
      <RentalsTable persistKey="tenant-rentals-v1" />
    </EntityPage>
  );
}
