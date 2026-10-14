"use client";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TransfersTable } from "@/features/fleet-ops/components/transfers-table";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Fleet transfers between branches (planned, in transit, history). */
export function TransfersPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("transfers.title")}
      description={t("transfers.description")}
      permission={permissions.tenant.transfers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("transfers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("transfers.title") },
      ]}
    >
      <TransfersTable />
    </EntityPage>
  );
}

/** "Transferler" tab of the vehicle page. */
export function VehicleTransfersTab({ vehicleUuid }: { vehicleUuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  if (!can(permissions.tenant.transfers.read)) {
    return (
      <EmptyState
        className="py-8"
        title={t("transfers.no_access_title")}
        description={t("transfers.no_access_description")}
      />
    );
  }
  return (
    <EntitySectionCard title={t("transfers.title")}>
      <TransfersTable vehicleUuid={vehicleUuid} />
    </EntitySectionCard>
  );
}
